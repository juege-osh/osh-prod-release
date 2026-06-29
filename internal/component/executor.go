package component

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/juege/osh-prod-release/internal/config"
	"github.com/juege/osh-prod-release/internal/models"
	"github.com/juege/osh-prod-release/internal/ssh"
)

type Phase string

const (
	PhasePlan          Phase = "plan"
	PhaseSnapshot      Phase = "snapshot"
	PhaseApplyGreen    Phase = "apply-green"
	PhaseTest          Phase = "test"
	PhaseRollbackGreen Phase = "rollback-green"
	PhaseApplyBlue     Phase = "apply-blue"
	PhaseRollbackBlue  Phase = "rollback-blue"
)

type Result struct {
	Phase       Phase             `json:"phase"`
	Slot        string            `json:"slot"`
	Component   string            `json:"component"`
	Action      string            `json:"action"`
	Node        string            `json:"node,omitempty"`
	Status      string            `json:"status"`
	Message     string            `json:"message"`
	Output      string            `json:"output,omitempty"`
	DataDiff    map[string]any    `json:"data_diff,omitempty"`
	Functional  map[string]any    `json:"functional,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

type Executor interface {
	Kind() string
	Plan(context.Context, models.Release, models.ChangeItem) (Result, error)
	Snapshot(context.Context, models.Release, models.ChangeItem, string) (Result, error)
	ApplyGreen(context.Context, models.Release, models.ChangeItem) (Result, error)
	Test(context.Context, models.Release, models.ChangeItem, string) (Result, error)
	RollbackGreen(context.Context, models.Release, models.ChangeItem) (Result, error)
	ApplyBlue(context.Context, models.Release, models.ChangeItem) (Result, error)
	RollbackBlue(context.Context, models.Release, models.ChangeItem) (Result, error)
}

type Service struct {
	executors map[string]Executor
}

func NewService(cfg *config.Config, sshClient *ssh.Client) *Service {
	svc := &Service{executors: map[string]Executor{}}
	for _, kind := range []string{"mysql", "nacos", "redis", "es", "elasticsearch", "kafka", "hbase", "mongodb"} {
		svc.executors[kind] = &scriptExecutor{cfg: cfg, ssh: sshClient, kind: kind}
	}
	return svc
}

func (s *Service) RegisterExecutor(kind string, ex Executor) {
	if s == nil || ex == nil {
		return
	}
	if s.executors == nil {
		s.executors = map[string]Executor{}
	}
	s.executors[normalizeKind(kind)] = ex
}

func (s *Service) ExecutorFor(item models.ChangeItem) Executor {
	kind := normalizeKind(item.ComponentType)
	if kind == "" || kind == "application" || item.Type == models.ItemTypeCode {
		return nil
	}
	if ex, ok := s.executors[kind]; ok {
		return ex
	}
	return &placeholderExecutor{kind: kind}
}

func SortItems(items []models.ChangeItem) []models.ChangeItem {
	out := append([]models.ChangeItem(nil), items...)
	sort.SliceStable(out, func(i, j int) bool {
		oi, oj := out[i].DeployOrder, out[j].DeployOrder
		if oi == 0 {
			oi = 100
		}
		if oj == 0 {
			oj = 100
		}
		if oi == oj {
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		}
		return oi < oj
	})
	return out
}

func MarshalResult(r Result) string {
	b, _ := json.Marshal(r)
	return string(b)
}

type scriptExecutor struct {
	cfg  *config.Config
	ssh  *ssh.Client
	kind string
}

func (e *scriptExecutor) Kind() string { return normalizeKind(e.kind) }

func (e *scriptExecutor) Plan(ctx context.Context, rel models.Release, item models.ChangeItem) (Result, error) {
	return e.localResult(PhasePlan, "green", item, "planned", "组件变更计划已生成"), nil
}

func (e *scriptExecutor) Snapshot(ctx context.Context, rel models.Release, item models.ChangeItem, slot string) (Result, error) {
	return e.run(ctx, rel, item, slot, PhaseSnapshot)
}

func (e *scriptExecutor) ApplyGreen(ctx context.Context, rel models.Release, item models.ChangeItem) (Result, error) {
	return e.run(ctx, rel, item, "green", PhaseApplyGreen)
}

func (e *scriptExecutor) Test(ctx context.Context, rel models.Release, item models.ChangeItem, slot string) (Result, error) {
	return e.run(ctx, rel, item, slot, PhaseTest)
}

func (e *scriptExecutor) RollbackGreen(ctx context.Context, rel models.Release, item models.ChangeItem) (Result, error) {
	return e.run(ctx, rel, item, "green", PhaseRollbackGreen)
}

func (e *scriptExecutor) ApplyBlue(ctx context.Context, rel models.Release, item models.ChangeItem) (Result, error) {
	return e.run(ctx, rel, item, "blue", PhaseApplyBlue)
}

func (e *scriptExecutor) RollbackBlue(ctx context.Context, rel models.Release, item models.ChangeItem) (Result, error) {
	return e.run(ctx, rel, item, "blue", PhaseRollbackBlue)
}

func (e *scriptExecutor) run(ctx context.Context, rel models.Release, item models.ChangeItem, slot string, phase Phase) (Result, error) {
	if e.cfg == nil || e.cfg.MockMode {
		return e.localResult(phase, slot, item, "success", "mock component executor"), nil
	}
	script := strings.TrimSpace(e.cfg.ComponentChangeScript)
	if script == "" {
		return e.localResult(phase, slot, item, "skipped", "COMPONENT_CHANGE_SCRIPT not configured"), nil
	}
	cmd := fmt.Sprintf("if [ -x %[1]q ] || [ -f %[1]q ]; then bash %[1]q --phase %[2]q --slot %[3]q --kind %[4]q --release %[5]q --item %[6]q --action %[7]q --ref %[8]q --node %[9]q; else echo 'component change script not found: %[1]s' >&2; exit 127; fi",
		script, string(phase), slot, e.Kind(), rel.ID, item.ID, defaultValue(item.Action, "apply"), item.Ref, item.TargetNode)
	out, err := e.ssh.Run(ctx, cmd, phaseTimeout(phase))
	status := "success"
	if err != nil {
		status = "failed"
	}
	res := e.localResult(phase, slot, item, status, string(phase)+" finished")
	res.Output = out
	return res, err
}

func (e *scriptExecutor) localResult(phase Phase, slot string, item models.ChangeItem, status, msg string) Result {
	return Result{
		Phase:     phase,
		Slot:      slot,
		Component: defaultValue(item.Component, e.Kind()),
		Action:    defaultValue(item.Action, "apply"),
		Node:      item.TargetNode,
		Status:    status,
		Message:   msg,
		Functional: map[string]any{
			"kind":   e.Kind(),
			"status": status,
		},
		DataDiff: map[string]any{
			"expected": item.DataImpact,
			"summary":  "executor-level diff is delegated to component script/analyzer",
		},
		Annotations: map[string]string{
			"impact_scope": item.ImpactScope,
			"test_plan":    item.TestPlan,
		},
	}
}

type placeholderExecutor struct {
	kind string
}

func (e *placeholderExecutor) Kind() string { return normalizeKind(e.kind) }
func (e *placeholderExecutor) Plan(ctx context.Context, rel models.Release, item models.ChangeItem) (Result, error) {
	return placeholderResult(PhasePlan, "green", e.Kind(), item), nil
}
func (e *placeholderExecutor) Snapshot(ctx context.Context, rel models.Release, item models.ChangeItem, slot string) (Result, error) {
	return placeholderResult(PhaseSnapshot, slot, e.Kind(), item), nil
}
func (e *placeholderExecutor) ApplyGreen(ctx context.Context, rel models.Release, item models.ChangeItem) (Result, error) {
	return placeholderResult(PhaseApplyGreen, "green", e.Kind(), item), nil
}
func (e *placeholderExecutor) Test(ctx context.Context, rel models.Release, item models.ChangeItem, slot string) (Result, error) {
	return placeholderResult(PhaseTest, slot, e.Kind(), item), nil
}
func (e *placeholderExecutor) RollbackGreen(ctx context.Context, rel models.Release, item models.ChangeItem) (Result, error) {
	return placeholderResult(PhaseRollbackGreen, "green", e.Kind(), item), nil
}
func (e *placeholderExecutor) ApplyBlue(ctx context.Context, rel models.Release, item models.ChangeItem) (Result, error) {
	return placeholderResult(PhaseApplyBlue, "blue", e.Kind(), item), nil
}
func (e *placeholderExecutor) RollbackBlue(ctx context.Context, rel models.Release, item models.ChangeItem) (Result, error) {
	return placeholderResult(PhaseRollbackBlue, "blue", e.Kind(), item), nil
}

func placeholderResult(phase Phase, slot, kind string, item models.ChangeItem) Result {
	return Result{
		Phase:     phase,
		Slot:      slot,
		Component: defaultValue(item.Component, kind),
		Action:    defaultValue(item.Action, "apply"),
		Node:      item.TargetNode,
		Status:    "skipped",
		Message:   kind + " executor is registered as an extension placeholder",
		Functional: map[string]any{
			"kind":      kind,
			"available": false,
		},
		DataDiff: map[string]any{
			"expected": item.DataImpact,
			"summary":  "placeholder executor",
		},
	}
}

func normalizeKind(kind string) string {
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind == "elastic" || kind == "elasticsearch" {
		return "es"
	}
	return kind
}

func defaultValue(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return strings.TrimSpace(v)
}

func phaseTimeout(phase Phase) time.Duration {
	switch phase {
	case PhaseApplyGreen, PhaseApplyBlue, PhaseRollbackGreen, PhaseRollbackBlue:
		return 30 * time.Minute
	case PhaseSnapshot:
		return 15 * time.Minute
	default:
		return 5 * time.Minute
	}
}
