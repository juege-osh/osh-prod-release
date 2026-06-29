package store

import (
	"context"
	"strings"
	"time"

	"github.com/juege/osh-prod-release/internal/models"
)

func (s *Store) SaveBatchAutoTestReport(ctx context.Context, r models.BatchAutoTestReport) error {
	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO batch_auto_test_reports (id, batch_id, slot, trigger, functional_json, data_diff_json,
			ai_verdict, ai_passed, passed, actor, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.BatchID, r.Slot, defaultStr(r.Trigger, inferAutoTestTrigger(r.BatchID)), r.Functional, r.DataDiff, r.AIVerdict,
		boolToInt(r.AIPassed), boolToInt(r.Passed), r.Actor, r.CreatedAt.Format(time.RFC3339))
	return err
}

func inferAutoTestTrigger(batchID string) string {
	if strings.HasPrefix(batchID, "manual-") {
		return "manual"
	}
	return "batch"
}

func (s *Store) GetLatestBatchAutoTestReport(ctx context.Context) (*models.BatchAutoTestReport, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, batch_id, slot, COALESCE(trigger, ''), functional_json, data_diff_json, ai_verdict,
			ai_passed, passed, actor, created_at
		FROM batch_auto_test_reports ORDER BY created_at DESC LIMIT 1`)
	return scanBatchAutoTestReport(row)
}

func (s *Store) GetBatchAutoTestReport(ctx context.Context, batchID string) (*models.BatchAutoTestReport, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, batch_id, slot, COALESCE(trigger, ''), functional_json, data_diff_json, ai_verdict,
			ai_passed, passed, actor, created_at
		FROM batch_auto_test_reports WHERE batch_id = ? ORDER BY created_at DESC LIMIT 1`, batchID)
	return scanBatchAutoTestReport(row)
}

func (s *Store) GetLatestTestReport(ctx context.Context, releaseID string) (*models.TestReport, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, release_id, env, functional_json, data_diff_json, ai_verdict,
			ai_passed, passed, created_at
		FROM test_reports WHERE release_id = ? ORDER BY created_at DESC LIMIT 1`, releaseID)
	var r models.TestReport
	var aiPassed, passed int
	var created string
	if err := row.Scan(&r.ID, &r.ReleaseID, &r.Env, &r.Functional, &r.DataDiff, &r.AIVerdict,
		&aiPassed, &passed, &created); err != nil {
		return nil, err
	}
	r.AIPassed = aiPassed == 1
	r.Passed = passed == 1
	r.CreatedAt, _ = time.Parse(time.RFC3339, created)
	return &r, nil
}

func scanBatchAutoTestReport(row interface{ Scan(...any) error }) (*models.BatchAutoTestReport, error) {
	var r models.BatchAutoTestReport
	var aiPassed, passed int
	var created string
	if err := row.Scan(&r.ID, &r.BatchID, &r.Slot, &r.Trigger, &r.Functional, &r.DataDiff, &r.AIVerdict,
		&aiPassed, &passed, &r.Actor, &created); err != nil {
		return nil, err
	}
	if r.Trigger == "" {
		r.Trigger = inferAutoTestTrigger(r.BatchID)
	}
	r.AIPassed = aiPassed == 1
	r.Passed = passed == 1
	r.CreatedAt, _ = time.Parse(time.RFC3339, created)
	return &r, nil
}
