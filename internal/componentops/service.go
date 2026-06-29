package componentops

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/juege/osh-prod-release/internal/config"
	"github.com/juege/osh-prod-release/internal/models"
	"github.com/juege/osh-prod-release/internal/ssh"
	"github.com/juege/osh-prod-release/internal/store"
	"github.com/juege/osh-prod-release/internal/testrunner"
)

type ApplyRequest struct {
	Kind        string `json:"kind"`
	Slot        string `json:"slot"`
	Action      string `json:"action"`
	Ref         string `json:"ref"`
	Payload     string `json:"payload"`
	Node        string `json:"node"`
	Label       string `json:"label"`
	DeployOrder int    `json:"deploy_order,omitempty"`
	Actor       string `json:"actor"`
}

type BatchApplyRequest struct {
	Items []ApplyRequest `json:"items"`
	Actor string         `json:"actor"`
}

type BatchApplyResult struct {
	Results      []OpResult                  `json:"results"`
	Failed       bool                        `json:"failed"`
	SuccessCount int                         `json:"success_count"`
	FailCount    int                         `json:"fail_count"`
	Message      string                      `json:"message,omitempty"`
	BatchID      string                      `json:"batch_id,omitempty"`
	AutoTest     *models.BatchAutoTestReport `json:"auto_test,omitempty"`
}

type OpResult struct {
	ID           string `json:"id"`
	Kind         string `json:"kind"`
	Slot         string `json:"slot"`
	Action       string `json:"action"`
	Status       string `json:"status"`
	Output       string `json:"output"`
	RefPath      string `json:"ref_path,omitempty"`
	DataDiffJSON string `json:"data_diff_json,omitempty"`
	RolledBack   bool   `json:"rolled_back,omitempty"`
}

type Service struct {
	cfg   *config.Config
	store *store.Store
	ssh   *ssh.Client
	test  *testrunner.Runner
}

func New(cfg *config.Config, st *store.Store, sshClient *ssh.Client) *Service {
	return &Service{cfg: cfg, store: st, ssh: sshClient, test: testrunner.New(cfg)}
}

func (s *Service) Apply(ctx context.Context, req ApplyRequest) (*OpResult, error) {
	kind := normalizeKind(req.Kind)
	slot := strings.ToLower(strings.TrimSpace(req.Slot))
	if slot == "" {
		slot = "green"
	}
	if slot != "green" {
		return nil, fmt.Errorf("direct component ops only allowed on green slot for now")
	}
	action := strings.TrimSpace(req.Action)
	if action == "" {
		action = defaultAction(kind)
	}
	ref := strings.TrimSpace(req.Ref)
	payload := strings.TrimSpace(req.Payload)
	if ref == "" && payload == "" {
		return nil, fmt.Errorf("ref or payload required")
	}

	opID := uuid.New().String()[:12]
	releaseKey := "direct-" + opID
	itemKey := "op"

	if s.cfg.MockMode {
		out := fmt.Sprintf("[MOCK] %s apply-green slot=%s action=%s node=%s", kind, slot, action, req.Node)
		_ = s.store.SaveComponentDirectOp(ctx, store.ComponentDirectOp{
			ID: opID, Kind: kind, Slot: slot, Action: action, RefPath: ref,
			Node: req.Node, WorkRelease: releaseKey, WorkItem: itemKey,
			Actor: req.Actor, Status: "success", Output: out,
		})
		return &OpResult{ID: opID, Kind: kind, Slot: slot, Action: action, Status: "success", Output: out}, nil
	}

	refPath, needsUpload, payloadContent, err := s.resolveRef(kind, releaseKey, itemKey, action, ref, payload)
	if err != nil {
		return nil, err
	}

	full, err := s.runApplyPipeline(ctx, kind, slot, releaseKey, itemKey, action, refPath, req.Node, needsUpload, payloadContent)
	if err != nil {
		_ = s.store.SaveComponentDirectOp(ctx, store.ComponentDirectOp{
			ID: opID, Kind: kind, Slot: slot, Action: action, RefPath: refPath,
			Node: req.Node, WorkRelease: releaseKey, WorkItem: itemKey,
			Actor: req.Actor, Status: "failed", Output: full,
		})
		return &OpResult{ID: opID, Kind: kind, Slot: slot, Action: action, Status: "failed", Output: full, RefPath: refPath}, err
	}

	full = strings.TrimSpace(full)
	dataDiff := parseDataDiffFromOutput(full)
	_ = s.store.SaveComponentDirectOp(ctx, store.ComponentDirectOp{
		ID: opID, Kind: kind, Slot: slot, Action: action, RefPath: refPath,
		Node: req.Node, WorkRelease: releaseKey, WorkItem: itemKey,
		Actor: req.Actor, Status: "success", Output: full,
	})
	_ = s.store.AddAudit(ctx, req.Actor, "component_direct_apply_"+kind, opID, full)
	return &OpResult{ID: opID, Kind: kind, Slot: slot, Action: action, Status: "success", Output: full, RefPath: refPath, DataDiffJSON: dataDiff}, nil
}

func (s *Service) Rollback(ctx context.Context, kind, slot, actor string) (*OpResult, error) {
	kind = normalizeKind(kind)
	if slot == "" {
		slot = "green"
	}
	op, err := s.store.GetLatestComponentDirectOp(ctx, kind, slot, "success")
	if err != nil {
		return nil, err
	}
	if s.cfg.MockMode {
		out := fmt.Sprintf("[MOCK] rollback %s slot=%s op=%s", kind, slot, op.ID)
		_ = s.store.AddAudit(ctx, actor, "component_direct_rollback_"+kind, op.ID, out)
		return &OpResult{ID: op.ID, Kind: kind, Slot: slot, Action: op.Action, Status: "success", Output: out, RolledBack: true}, nil
	}
	out, err := s.runPhase(ctx, "rollback-green", kind, slot, op.WorkRelease, op.WorkItem, op.Action, op.RefPath, op.Node)
	res := &OpResult{ID: op.ID, Kind: kind, Slot: slot, Action: op.Action, Output: out, RefPath: op.RefPath, RolledBack: true}
	if err != nil {
		res.Status = "failed"
		return res, err
	}
	res.Status = "success"
	_ = s.store.UpdateComponentDirectOpStatus(ctx, op.ID, "rolled_back", out)
	_ = s.store.AddAudit(ctx, actor, "component_direct_rollback_"+kind, op.ID, out)
	return res, nil
}

func (s *Service) ListHistory(ctx context.Context, kind, slot string, limit int) ([]store.ComponentDirectOp, error) {
	k := normalizeKind(kind)
	if k == "all" {
		k = ""
	}
	return s.store.ListComponentDirectOps(ctx, k, slot, limit)
}

func (s *Service) ApplyBatch(ctx context.Context, req BatchApplyRequest) (*BatchApplyResult, error) {
	if len(req.Items) == 0 {
		return nil, fmt.Errorf("batch items required")
	}
	items := append([]ApplyRequest(nil), req.Items...)
	sortBatchItems(items)

	batchID := uuid.New().String()[:12]
	var results []OpResult
	var failedKinds []string
	for i, item := range items {
		if i > 0 {
			// Space out SSH sessions so 149 sshd/fail2ban does not block the last items (often Kafka).
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(3 * time.Second):
			}
		}
		if item.Actor == "" {
			item.Actor = req.Actor
		}
		res, err := s.Apply(ctx, item)
		if res != nil {
			results = append(results, *res)
			if err != nil || res.Status != "success" {
				failedKinds = append(failedKinds, item.Kind)
			}
			continue
		}
		failedKinds = append(failedKinds, item.Kind)
		results = append(results, OpResult{
			Kind:   normalizeKind(item.Kind),
			Slot:   item.Slot,
			Action: item.Action,
			Status: "failed",
			Output: err.Error(),
		})
	}

	successCount := len(results) - len(failedKinds)
	if successCount < 0 {
		successCount = 0
	}
	failCount := len(failedKinds)
	msg := fmt.Sprintf("逐个执行完成：%d 成功，%d 失败（失败不回滚已成功项；SSH 已复用连接）", successCount, failCount)
	if failCount > 0 {
		msg += "；失败项：" + strings.Join(failedKinds, ", ")
	}
	return &BatchApplyResult{
		Results:      results,
		Failed:       failCount > 0,
		SuccessCount: successCount,
		FailCount:    failCount,
		Message:      msg,
		BatchID:      batchID,
		AutoTest:     s.runBatchAutoTest(ctx, batchID, items, results, req.Actor),
	}, nil
}

func (s *Service) rollbackBatchSuccess(ctx context.Context, results []OpResult, actor string) string {
	var parts []string
	for i := len(results) - 1; i >= 0; i-- {
		r := results[i]
		if r.Status != "success" {
			continue
		}
		_, err := s.Rollback(ctx, r.Kind, r.Slot, actor)
		if err != nil {
			parts = append(parts, fmt.Sprintf("%s rollback err: %v", r.Kind, err))
		} else {
			parts = append(parts, r.Kind+" rollback ok")
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return "; " + strings.Join(parts, "; ")
}

func sortBatchItems(items []ApplyRequest) {
	for i := range items {
		if items[i].DeployOrder <= 0 {
			items[i].DeployOrder = defaultBatchOrder(items[i].Kind)
		}
	}
	for i := 0; i < len(items); i++ {
		for j := i + 1; j < len(items); j++ {
			if items[j].DeployOrder < items[i].DeployOrder {
				items[i], items[j] = items[j], items[i]
			}
		}
	}
}

func defaultBatchOrder(kind string) int {
	switch normalizeKind(kind) {
	case "mysql":
		return 10
	case "nacos":
		return 20
	case "redis":
		return 30
	case "es":
		return 40
	case "kafka":
		return 50
	default:
		return 100
	}
}

func (s *Service) componentScript() string {
	if s.cfg.ComponentChangeScript != "" {
		return s.cfg.ComponentChangeScript
	}
	return "/opt/osh-green/005-scripts/osh-component-change.sh"
}

func (s *Service) runPhase(ctx context.Context, phase, kind, slot, releaseKey, itemKey, action, refPath, node string) (string, error) {
	cmd := fmt.Sprintf(
		`bash %q --phase %q --slot %q --kind %q --release %q --item %q --action %q --ref %q --node %q`,
		s.componentScript(), phase, slot, kind, releaseKey, itemKey, action, refPath, node,
	)
	return s.ssh.Run(ctx, cmd, phaseTimeout(phase))
}

func (s *Service) resolveRef(kind, releaseKey, itemKey, action, ref, payload string) (refPath string, needsUpload bool, content string, err error) {
	kind = normalizeKind(kind)
	action = strings.TrimSpace(action)
	if kind == "kafka" && (action == "create-topic" || action == "create_topic") {
		topic := strings.TrimSpace(ref)
		if topic == "" {
			topic = strings.TrimSpace(payload)
		}
		if topic == "" {
			return "", false, "", fmt.Errorf("kafka topic required")
		}
		return topic, false, "", nil
	}
	if ref != "" && strings.HasPrefix(ref, "/") && !strings.Contains(ref, "\n") {
		return ref, false, "", nil
	}
	content = payload
	if content == "" {
		content = ref
	}
	if content == "" {
		return "", false, "", fmt.Errorf("empty payload")
	}
	ext := refExtension(kind)
	refPath = fmt.Sprintf("/tmp/osh-component-%s-%s-%s.%s", kind, releaseKey, itemKey, ext)
	return refPath, true, content, nil
}

func (s *Service) runApplyPipeline(ctx context.Context, kind, slot, releaseKey, itemKey, action, refPath, node string, needsUpload bool, payloadContent string) (string, error) {
	script := s.componentScript()
	var b strings.Builder
	b.WriteString("set -euo pipefail\n")
	if needsUpload {
		b64 := base64.StdEncoding.EncodeToString([]byte(payloadContent))
		b.WriteString(fmt.Sprintf("tmp=%q\n", refPath))
		b.WriteString(fmt.Sprintf("printf '%%s' %q | base64 -d > \"$tmp\"\n", b64))
		if ext := refExtension(kind); ext == "sh" {
			b.WriteString("chmod +x \"$tmp\"\n")
		}
		b.WriteString("echo \"__OSH_REF__ $tmp\"\n")
	}
	b.WriteString(fmt.Sprintf("script=%q\n", script))
	b.WriteString("run_phase(){ bash \"$script\" --phase \"$1\" --slot ")
	b.WriteString(fmt.Sprintf("%q --kind %q --release %q --item %q --action %q --ref %q --node %q; }\n",
		slot, kind, releaseKey, itemKey, action, refPath, node))
	b.WriteString(`failed=0
for phase in snapshot apply-green test diff-report; do
  echo "=== phase=$phase ==="
  if ! run_phase "$phase"; then failed=1; break; fi
done
if [ "$failed" -eq 1 ]; then
  echo "=== phase=rollback-green ==="
  run_phase rollback-green || true
  exit 1
fi
`)
	return s.ssh.Run(ctx, b.String(), 45*time.Minute)
}

func normalizeKind(kind string) string {
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind == "elasticsearch" {
		return "es"
	}
	return kind
}

func defaultAction(kind string) string {
	switch kind {
	case "kafka":
		return "create-topic"
	default:
		return "apply"
	}
}

func refExtension(kind string) string {
	switch kind {
	case "mysql":
		return "sql"
	case "redis":
		return "redis"
	case "nacos", "es":
		return "sh"
	default:
		return "txt"
	}
}

func phaseTimeout(phase string) time.Duration {
	switch phase {
	case "apply-green", "rollback-green":
		return 30 * time.Minute
	case "snapshot":
		return 15 * time.Minute
	default:
		return 5 * time.Minute
	}
}
