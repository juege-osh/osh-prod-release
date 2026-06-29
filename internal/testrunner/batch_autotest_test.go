package testrunner

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/juege/osh-prod-release/internal/config"
)

func TestRunBatchAutoTestMock(t *testing.T) {
	runner := New(&config.Config{MockMode: true})
	report, err := runner.RunBatchAutoTest(context.Background(), BatchAutoTestInput{
		BatchID: "batch-1",
		Env:     "green",
		Actor:   "ops",
		ExpectedItems: []map[string]any{
			{"kind": "mysql", "label": "osh_platform_test", "payload": "CREATE TABLE osh_platform_test"},
		},
		ComponentDiffs: []json.RawMessage{
			json.RawMessage(`{"component":"mysql","added":["osh_platform_test"],"removed":[],"modified":[],"summary":{"added_count":1}}`),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !report.Passed {
		t.Fatalf("expected passed, verdict=%s", report.AIVerdict)
	}
}
