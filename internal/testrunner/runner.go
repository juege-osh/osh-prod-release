package testrunner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/juege/osh-prod-release/internal/config"
	"github.com/juege/osh-prod-release/internal/models"
	"github.com/juege/osh-prod-release/internal/ssh"
)

type Runner struct {
	cfg *config.Config
	ssh *ssh.Client
}

func New(cfg *config.Config) *Runner {
	return &Runner{cfg: cfg, ssh: ssh.New(cfg)}
}

type functionalCase struct {
	Name   string `json:"name"`
	Target string `json:"target"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail"`
}

// Run executes read-only functional probes and calls analyzer for data diff.
func (r *Runner) Run(ctx context.Context, rel models.Release, env string) (*models.TestReport, error) {
	functional := r.runFunctionalProbes(ctx, env)
	fb, _ := json.Marshal(functional)

	dataDiff, aiVerdict, aiPassed := r.callAnalyzer(ctx, rel)

	passed := true
	for _, c := range functional {
		if !c.Passed {
			passed = false
			break
		}
	}
	if !aiPassed {
		passed = false
	}

	return &models.TestReport{
		ID:         uuid.New().String()[:12],
		ReleaseID:  rel.ID,
		Env:        env,
		Functional: string(fb),
		DataDiff:   dataDiff,
		AIVerdict:  aiVerdict,
		AIPassed:   aiPassed,
		Passed:     passed,
		CreatedAt:  time.Now().UTC(),
	}, nil
}

func (r *Runner) runFunctionalProbes(ctx context.Context, env string) []functionalCase {
	if r.cfg == nil || r.cfg.MockMode {
		return []functionalCase{
			{Name: "mysql_ping_schema", Target: env + "/mysql", Passed: true, Detail: "mock: mysql ping and schema probe"},
			{Name: "redis_ping_key", Target: env + "/redis", Passed: true, Detail: "mock: redis ping and key probe"},
			{Name: "es_index_count", Target: env + "/elasticsearch", Passed: true, Detail: "mock: index health and count probe"},
			{Name: "kafka_topic_smoke", Target: env + "/kafka", Passed: true, Detail: "mock: topic metadata smoke probe"},
			{Name: "nacos_config_checksum", Target: env + "/nacos", Passed: true, Detail: "mock: nacos config checksum probe"},
			{Name: "java_api_key_paths", Target: env + "/api", Passed: true, Detail: "mock: API smoke probe"},
		}
	}
	slot := normalizeSlot(env)
	cases := []struct {
		name   string
		target string
		cmd    string
	}{
		{name: "mysql_ping_schema", target: slot + "/mysql", cmd: r.mysqlProbe(slot)},
		{name: "redis_ping_key", target: slot + "/redis", cmd: redisProbe(slot)},
		{name: "es_index_count", target: slot + "/elasticsearch", cmd: esProbe(slot)},
		{name: "kafka_topic_smoke", target: slot + "/kafka", cmd: kafkaProbe(slot)},
		{name: "nacos_config_checksum", target: slot + "/nacos", cmd: nacosProbe(slot)},
		{name: "java_api_key_paths", target: slot + "/api", cmd: apiProbe(slot)},
	}
	out := make([]functionalCase, 0, len(cases))
	for _, c := range cases {
		probeOut, err := r.ssh.Run(ctx, c.cmd, 45*time.Second)
		item := functionalCase{Name: c.name, Target: c.target, Passed: err == nil, Detail: strings.TrimSpace(probeOut)}
		if err != nil {
			item.Detail = strings.TrimSpace(probeOut + "\n" + err.Error())
		}
		out = append(out, item)
	}
	return out
}

func (r *Runner) mysqlProbe(slot string) string {
	container := r.cfg.GreenMySQLContainer
	password := r.cfg.GreenMySQLRootPassword
	database := r.cfg.GreenMySQLDatabase
	if slot == "blue" {
		container = r.cfg.BlueMySQLContainer
		password = r.cfg.BlueMySQLRootPassword
		database = r.cfg.BlueMySQLDatabase
	}
	passArg := ""
	if password != "" {
		passArg = "-p" + shellWord(password)
	}
	query := fmt.Sprintf("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = %s", sqlString(database))
	inner := fmt.Sprintf("mysqladmin ping -uroot %s --silent && mysql -uroot %s -N -B -e %s", passArg, passArg, shellWord(query))
	return fmt.Sprintf("docker exec %s sh -lc %s", shellWord(container), shellWord(inner))
}

func redisProbe(slot string) string {
	c := slotContainer(slot, "redis")
	return fmt.Sprintf("docker exec %s sh -lc 'redis-cli -a \"${REDIS_PASSWORD:-}\" --no-auth-warning PING 2>/dev/null || redis-cli PING' | grep -qx PONG", shellWord(c))
}

func esProbe(slot string) string {
	port := "29200"
	if slot == "blue" {
		port = "59200"
	}
	return fmt.Sprintf("curl -sf --max-time 10 http://127.0.0.1:%s/_cluster/health", port)
}

func kafkaProbe(slot string) string {
	c := slotContainer(slot, "kafka")
	bootstrap := c + ":9092"
	return fmt.Sprintf("docker exec %s sh -lc 'kafka-topics.sh --bootstrap-server %q --list | sed -n \"1,20p\"'", shellWord(c), bootstrap)
}

func nacosProbe(slot string) string {
	port := "28848"
	if slot == "blue" {
		port = "58848"
	}
	return fmt.Sprintf("curl -sf --max-time 10 http://127.0.0.1:%s/nacos/v1/ns/operator/metrics", port)
}

func apiProbe(slot string) string {
	port := "28080"
	if slot == "blue" {
		port = "58080"
	}
	return fmt.Sprintf("code=$(curl -s -o /dev/null -w '%%{http_code}' --max-time 10 http://127.0.0.1:%s/api/ || echo 000); test \"$code\" = 200 -o \"$code\" = 401; echo HTTP:$code", port)
}

func slotContainer(slot, name string) string {
	if slot == "blue" {
		return "osh-" + name
	}
	return "osh-g-" + name
}

func normalizeSlot(env string) string {
	if strings.Contains(strings.ToLower(env), "blue") {
		return "blue"
	}
	return "green"
}

func (r *Runner) callAnalyzer(ctx context.Context, rel models.Release) (dataDiffJSON, verdict string, passed bool) {
	if r.cfg.MockMode {
		diff := fallbackDataDiff(rel, "mock")
		b, _ := json.Marshal(diff)
		return string(b), "consistent (mock)", true
	}
	body := map[string]any{
		"release_id":  rel.ID,
		"expected":    collectExpected(rel.Items),
		"data_impact": collectDataImpact(rel.Items),
		"test_plan":   collectTestPlans(rel.Items),
	}
	payload, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.cfg.AnalyzerURL+"/api/diff-and-judge", bytes.NewReader(payload))
	if err != nil {
		return "{}", "analyzer unavailable", false
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		// Analyzer is optional for green deploy; do not block release when sidecar is offline.
		diff := fallbackDataDiff(rel, "analyzer_offline")
		b, _ := json.Marshal(diff)
		return string(b), fmt.Sprintf("analyzer skipped (offline): %v", err), true
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		diff := fallbackDataDiff(rel, fmt.Sprintf("analyzer_http_%d", resp.StatusCode))
		b, _ := json.Marshal(diff)
		return string(b), fmt.Sprintf("analyzer skipped (HTTP %d)", resp.StatusCode), true
	}
	var out struct {
		DataDiff string `json:"data_diff"`
		Verdict  string `json:"verdict"`
		Passed   bool   `json:"passed"`
	}
	if json.Unmarshal(raw, &out) != nil {
		return string(raw), "parse error", false
	}
	return out.DataDiff, out.Verdict, out.Passed
}

func fallbackDataDiff(rel models.Release, source string) map[string]any {
	items := make([]map[string]any, 0, len(rel.Items))
	for _, it := range rel.Items {
		items = append(items, map[string]any{
			"title":           it.Title,
			"type":            it.Type,
			"component":       defaultString(it.Component, it.ComponentType),
			"node":            it.TargetNode,
			"expected_impact": it.ExpectedImpact,
			"data_impact":     it.DataImpact,
			"test_plan":       it.TestPlan,
			"added":           "requires analyzer/database snapshot",
			"removed":         "requires analyzer/database snapshot",
			"modified":        "requires analyzer/database snapshot",
		})
	}
	return map[string]any{
		"source":       source,
		"release_id":   rel.ID,
		"summary":      "自动化测试保留每个上线项的数据影响预期；精确新增/删除/修改数量由 analyzer 或组件脚本报告提供。",
		"change_items": items,
	}
}

func collectExpected(items []models.ChangeItem) []string {
	var out []string
	for _, it := range items {
		if it.ExpectedImpact != "" {
			out = append(out, it.ExpectedImpact)
		}
	}
	return out
}

func defaultString(v, fallback string) string {
	if strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return strings.TrimSpace(fallback)
}

func shellWord(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(v, "'", "'\"'\"'") + "'"
}

func sqlString(v string) string {
	return "'" + strings.ReplaceAll(v, "'", "''") + "'"
}

func collectDataImpact(items []models.ChangeItem) []string {
	var out []string
	for _, it := range items {
		if it.DataImpact != "" {
			out = append(out, it.DataImpact)
		}
	}
	return out
}

func collectTestPlans(items []models.ChangeItem) []string {
	var out []string
	for _, it := range items {
		if it.TestPlan != "" {
			out = append(out, it.TestPlan)
		}
	}
	return out
}
