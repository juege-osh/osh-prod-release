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
	"github.com/juege/osh-prod-release/internal/models"
)

type BatchAutoTestInput struct {
	BatchID        string
	Env            string
	Actor          string
	ExpectedItems  []map[string]any
	ComponentDiffs []json.RawMessage
}

func (r *Runner) RunBatchAutoTest(ctx context.Context, in BatchAutoTestInput) (*models.BatchAutoTestReport, error) {
	env := in.Env
	if env == "" {
		env = "green"
	}
	functional := r.runFunctionalProbes(ctx, env)
	fb, _ := json.Marshal(functional)

	passed := true
	for _, c := range functional {
		if !c.Passed {
			passed = false
			break
		}
	}

	dataDiff, aiVerdict, aiPassed := r.callBatchAnalyzer(ctx, in.BatchID, functional, in.ExpectedItems, in.ComponentDiffs)
	if !aiPassed {
		passed = false
	}

	return &models.BatchAutoTestReport{
		ID:         uuid.New().String()[:12],
		BatchID:    in.BatchID,
		Slot:       env,
		Functional: string(fb),
		DataDiff:   dataDiff,
		AIVerdict:  aiVerdict,
		AIPassed:   aiPassed,
		Passed:     passed,
		Actor:      in.Actor,
		CreatedAt:  time.Now().UTC(),
	}, nil
}

func (r *Runner) callBatchAnalyzer(ctx context.Context, batchID string, functional []functionalCase, expected []map[string]any, diffs []json.RawMessage) (dataDiffJSON, verdict string, passed bool) {
	if r.cfg.MockMode {
		diff := map[string]any{
			"batch_id":        batchID,
			"source":          "mock",
			"component_diffs": diffs,
			"summary":         "mock batch auto test",
		}
		b, _ := json.Marshal(diff)
		return string(b), "consistent (mock)", true
	}

	body := map[string]any{
		"release_id":      batchID,
		"functional":      functional,
		"expected_items":  expected,
		"component_diffs": diffs,
	}
	payload, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.cfg.AnalyzerURL+"/api/diff-and-judge", bytes.NewReader(payload))
	if err != nil {
		diff := fallbackBatchDiff(batchID, diffs, expected, "analyzer_unavailable")
		b, _ := json.Marshal(diff)
		return string(b), "analyzer unavailable", false
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		diff := fallbackBatchDiff(batchID, diffs, expected, "analyzer_offline")
		b, _ := json.Marshal(diff)
		return string(b), fmt.Sprintf("analyzer offline, rule fallback: %v", err), fallbackBatchPassed(functional, diffs, expected)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		diff := fallbackBatchDiff(batchID, diffs, expected, fmt.Sprintf("http_%d", resp.StatusCode))
		b, _ := json.Marshal(diff)
		return string(b), fmt.Sprintf("analyzer HTTP %d, rule fallback", resp.StatusCode), fallbackBatchPassed(functional, diffs, expected)
	}
	var out struct {
		DataDiff string `json:"data_diff"`
		Verdict  string `json:"verdict"`
		Passed   bool   `json:"passed"`
	}
	if json.Unmarshal(raw, &out) != nil {
		return string(raw), "parse error", false
	}
	if out.DataDiff == "" {
		diff := fallbackBatchDiff(batchID, diffs, expected, "empty_response")
		b, _ := json.Marshal(diff)
		return string(b), out.Verdict, out.Passed
	}
	return out.DataDiff, out.Verdict, out.Passed
}

func fallbackBatchDiff(batchID string, diffs []json.RawMessage, expected []map[string]any, source string) map[string]any {
	parsed := make([]any, 0, len(diffs))
	for _, d := range diffs {
		var obj any
		if json.Unmarshal(d, &obj) == nil {
			parsed = append(parsed, obj)
		}
	}
	return map[string]any{
		"batch_id":        batchID,
		"source":          source,
		"component_diffs": parsed,
		"expected_items":  expected,
		"summary":         "上线前后组件级 diff 汇总；analyzer 离线时使用规则引擎判定。",
	}
}

func fallbackBatchPassed(functional []functionalCase, diffs []json.RawMessage, expected []map[string]any) bool {
	for _, c := range functional {
		if !c.Passed {
			return false
		}
	}
	return ruleJudgeBatch(diffs, expected)
}

func ruleJudgeBatch(diffs []json.RawMessage, expected []map[string]any) bool {
	if len(diffs) == 0 {
		return len(expected) == 0
	}
	for _, raw := range diffs {
		var diff map[string]any
		if json.Unmarshal(raw, &diff) != nil {
			continue
		}
		kind, _ := diff["component"].(string)
		added, _ := diff["added"].([]any)
		if len(added) == 0 {
			continue
		}
		hay := stringsForExpected(expected, kind)
		for _, a := range added {
			name := fmt.Sprint(a)
			if name == "" {
				continue
			}
			if !strings.Contains(strings.ToLower(hay), strings.ToLower(name)) &&
				!strings.Contains(strings.ToLower(hay), strings.ToLower(kind)) {
				return false
			}
		}
	}
	return true
}

func stringsForExpected(expected []map[string]any, kind string) string {
	var parts []string
	for _, it := range expected {
		if k, _ := it["kind"].(string); k != "" && k != kind {
			continue
		}
		for _, key := range []string{"label", "ref", "payload", "action"} {
			if v, ok := it[key].(string); ok && v != "" {
				parts = append(parts, v)
			}
		}
	}
	return strings.Join(parts, " ")
}
