package componentops

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/juege/osh-prod-release/internal/models"
	"github.com/juege/osh-prod-release/internal/testrunner"
)

const dataDiffMarker = "__OSH_DATA_DIFF__"

func parseDataDiffFromOutput(output string) string {
	idx := strings.LastIndex(output, dataDiffMarker)
	if idx < 0 {
		return ""
	}
	raw := strings.TrimSpace(output[idx+len(dataDiffMarker):])
	if nl := strings.IndexByte(raw, '\n'); nl >= 0 {
		raw = raw[:nl]
	}
	if !json.Valid([]byte(raw)) {
		return ""
	}
	return raw
}

type ManualAutoTestRequest struct {
	Slot             string           `json:"slot"`
	Actor            string           `json:"actor"`
	IncludeRecentOps bool             `json:"include_recent_ops"`
	RecentLimit      int              `json:"recent_limit"`
	ExpectedItems    []map[string]any `json:"expected_items,omitempty"`
	Notes            string           `json:"notes,omitempty"`
}

func (s *Service) RunManualAutoTest(ctx context.Context, req ManualAutoTestRequest) (*models.BatchAutoTestReport, error) {
	if s.test == nil {
		return nil, fmt.Errorf("auto test runner not configured")
	}
	slot := strings.ToLower(strings.TrimSpace(req.Slot))
	if slot == "" {
		slot = "green"
	}
	if slot != "green" {
		return nil, fmt.Errorf("manual auto test only supports green slot for now")
	}
	limit := req.RecentLimit
	if limit <= 0 {
		limit = 10
	}

	batchID := "manual-" + uuid.New().String()[:12]
	diffs := s.collectLiveComponentDiffs(ctx, batchID, slot)
	expected := append([]map[string]any(nil), req.ExpectedItems...)
	if req.IncludeRecentOps {
		ops, err := s.store.ListComponentDirectOps(ctx, "", slot, limit)
		if err != nil {
			return nil, err
		}
		for _, op := range ops {
			if op.Status != "success" {
				continue
			}
			if diff := parseDataDiffFromOutput(op.Output); diff != "" {
				diffs = append(diffs, json.RawMessage(diff))
			}
			expected = append(expected, map[string]any{
				"kind":    op.Kind,
				"label":   fmt.Sprintf("recent op %s", op.ID),
				"ref":     op.RefPath,
				"action":  op.Action,
				"payload": truncate(op.Output, 200),
			})
		}
	}
	if req.Notes != "" {
		expected = append(expected, map[string]any{"label": req.Notes, "expected_impact": req.Notes})
	}

	report, err := s.test.RunBatchAutoTest(ctx, testrunner.BatchAutoTestInput{
		BatchID:        batchID,
		Env:            slot,
		Actor:          req.Actor,
		ExpectedItems:  expected,
		ComponentDiffs: diffs,
	})
	if err != nil {
		return nil, err
	}
	if report != nil {
		report.Trigger = "manual"
		_ = s.store.SaveBatchAutoTestReport(ctx, *report)
	}
	return report, nil
}

func (s *Service) runBatchAutoTest(ctx context.Context, batchID string, items []ApplyRequest, results []OpResult, actor string) *models.BatchAutoTestReport {
	if s.test == nil || batchID == "" {
		return nil
	}
	var diffs []json.RawMessage
	for _, r := range results {
		if r.Status != "success" || r.DataDiffJSON == "" {
			continue
		}
		diffs = append(diffs, json.RawMessage(r.DataDiffJSON))
	}
	expected := make([]map[string]any, 0, len(items))
	for _, it := range items {
		expected = append(expected, map[string]any{
			"kind":    normalizeKind(it.Kind),
			"label":   it.Label,
			"ref":     it.Ref,
			"payload": truncate(it.Payload, 500),
			"action":  it.Action,
		})
	}
	report, err := s.test.RunBatchAutoTest(ctx, testrunner.BatchAutoTestInput{
		BatchID:        batchID,
		Env:            "green",
		Actor:          actor,
		ExpectedItems:  expected,
		ComponentDiffs: diffs,
	})
	if err != nil || report == nil {
		return report
	}
	report.Trigger = "batch"
	_ = s.store.SaveBatchAutoTestReport(ctx, *report)
	return report
}

func (s *Service) collectLiveComponentDiffs(ctx context.Context, batchID, slot string) []json.RawMessage {
	if s.cfg == nil || s.cfg.MockMode || s.ssh == nil {
		return nil
	}
	kinds := []string{"mysql", "redis", "nacos", "es", "kafka"}
	out := make([]json.RawMessage, 0, len(kinds))
	for _, kind := range kinds {
		var combined strings.Builder
		for _, phase := range []string{"snapshot", "test", "diff-report"} {
			part, err := s.runPhase(ctx, phase, kind, slot, batchID, "live-inventory", "apply", "", "")
			combined.WriteString(part)
			if err != nil && phase != "diff-report" {
				break
			}
		}
		if diff := parseDataDiffFromOutput(combined.String()); diff != "" {
			out = append(out, json.RawMessage(diff))
		}
	}
	return out
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
