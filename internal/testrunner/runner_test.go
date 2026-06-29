package testrunner

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/juege/osh-prod-release/internal/config"
	"github.com/juege/osh-prod-release/internal/models"
)

func TestRunMockReportIncludesFunctionalAndDataImpactSummary(t *testing.T) {
	runner := New(&config.Config{MockMode: true})
	rel := models.Release{
		ID: "rel-auto-test",
		Items: []models.ChangeItem{
			{
				Title:          "新增 Kafka topic",
				Component:      "kafka",
				ComponentType:  "kafka",
				ExpectedImpact: "新增支付事件 topic",
				DataImpact:     "topic +1",
				TestPlan:       "列出 topic 并发送 smoke 消息",
			},
		},
	}

	report, err := runner.Run(context.Background(), rel, "green")
	if err != nil {
		t.Fatal(err)
	}
	if !report.Passed {
		t.Fatalf("mock report Passed = false, verdict=%s", report.AIVerdict)
	}

	var cases []functionalCase
	if err := json.Unmarshal([]byte(report.Functional), &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) < 6 {
		t.Fatalf("functional cases len = %d, want at least 6", len(cases))
	}
	for _, name := range []string{"mysql_ping_schema", "redis_ping_key", "nacos_config_checksum", "java_api_key_paths"} {
		if !hasFunctionalCase(cases, name) {
			t.Fatalf("functional report missing case %q: %#v", name, cases)
		}
	}
	if !strings.Contains(report.DataDiff, "新增 Kafka topic") || !strings.Contains(report.DataDiff, "topic +1") {
		t.Fatalf("data diff summary missing release item impact:\n%s", report.DataDiff)
	}
}

func TestAnalyzerOfflineFallsBackToStructuredImpactSummary(t *testing.T) {
	runner := New(&config.Config{MockMode: false, AnalyzerURL: "http://127.0.0.1:1"})
	rel := models.Release{
		ID: "rel-analyzer-offline",
		Items: []models.ChangeItem{
			{Title: "Redis key 增量", ComponentType: "redis", DataImpact: "新增 session key 前缀", TestPlan: "PING + key count"},
		},
	}

	dataDiff, verdict, passed := runner.callAnalyzer(context.Background(), rel)
	if !passed {
		t.Fatalf("offline analyzer should not block green validation: verdict=%s", verdict)
	}
	if !strings.Contains(verdict, "offline") {
		t.Fatalf("verdict = %q, want offline marker", verdict)
	}
	if !strings.Contains(dataDiff, "Redis key 增量") || !strings.Contains(dataDiff, "新增 session key 前缀") {
		t.Fatalf("fallback data diff missing item impact:\n%s", dataDiff)
	}
}

func hasFunctionalCase(cases []functionalCase, name string) bool {
	for _, c := range cases {
		if c.Name == name {
			return true
		}
	}
	return false
}
