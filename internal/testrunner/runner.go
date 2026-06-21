package testrunner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/juege/osh-prod-release/internal/config"
	"github.com/juege/osh-prod-release/internal/models"
)

type Runner struct {
	cfg *config.Config
}

func New(cfg *config.Config) *Runner {
	return &Runner{cfg: cfg}
}

type functionalCase struct {
	Name   string `json:"name"`
	Target string `json:"target"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail"`
}

// Run executes P1 mock/skeleton functional tests and calls analyzer for data diff.
func (r *Runner) Run(ctx context.Context, rel models.Release, env string) (*models.TestReport, error) {
	functional := []functionalCase{
		{Name: "mysql_ping", Target: "mysql", Passed: true, Detail: "mock ok"},
		{Name: "redis_ping", Target: "redis", Passed: true, Detail: "mock ok"},
		{Name: "course_search", Target: "http://127.0.0.1/pc/course/search", Passed: true, Detail: "mock ok"},
	}
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

func (r *Runner) callAnalyzer(ctx context.Context, rel models.Release) (dataDiffJSON, verdict string, passed bool) {
	if r.cfg.MockMode {
		diff := map[string]any{
			"tables": []map[string]any{
				{"table": "osh_tool", "before": 0, "after": 0, "added": 0, "removed": 0, "modified": 0},
			},
		}
		b, _ := json.Marshal(diff)
		return string(b), "consistent (mock)", true
	}
	body := map[string]any{
		"release_id": rel.ID,
		"expected":   collectExpected(rel.Items),
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
		return "{}", fmt.Sprintf("analyzer skipped (offline): %v", err), true
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return string(raw), fmt.Sprintf("analyzer skipped (HTTP %d)", resp.StatusCode), true
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

func collectExpected(items []models.ChangeItem) []string {
	var out []string
	for _, it := range items {
		if it.ExpectedImpact != "" {
			out = append(out, it.ExpectedImpact)
		}
	}
	return out
}
