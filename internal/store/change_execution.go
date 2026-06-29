package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
	"github.com/juege/osh-prod-release/internal/models"
)

func (s *Store) SeedDefaultComponentSpecs(ctx context.Context) error {
	now := time.Now().UTC().Format(time.RFC3339)
	specs := []models.ComponentSpec{
		{Name: "mysql", Kind: "mysql", DataDir: "002-data/osh/osh-mysql", ConfigDir: "001-docker-compose/osh", LogDir: "004-log/osh/mysql", PortPolicy: "blue 53306, green 23306", BackupStrategy: "mysqldump before apply", RollbackStrategy: "restore snapshot or rollback SQL", HealthCheck: "mysql ping and schema check", SyncStrategy: "schema delta + upsert"},
		{Name: "nacos", Kind: "nacos", DataDir: "002-data/osh/osh-nacos", ConfigDir: "nacos tenants", LogDir: "004-log/osh/nacos", PortPolicy: "blue 58848, green 28848", BackupStrategy: "export config before apply", RollbackStrategy: "restore config snapshot", HealthCheck: "config checksum", SyncStrategy: "config diff with slot rewrite"},
		{Name: "redis", Kind: "redis", DataDir: "002-data/osh/osh-redis", ConfigDir: "001-docker-compose/osh", LogDir: "004-log/osh/redis", PortPolicy: "blue 56379, green 26379", BackupStrategy: "RDB snapshot", RollbackStrategy: "restore key snapshot", HealthCheck: "PING and key probe", SyncStrategy: "key restore replace"},
		{Name: "elasticsearch", Kind: "es", DataDir: "002-data/osh/osh-es", ConfigDir: "001-docker-compose/osh", LogDir: "004-log/osh/es", PortPolicy: "blue 59200, green 29200", BackupStrategy: "index dump before apply", RollbackStrategy: "restore index dump", HealthCheck: "cluster health and index count", SyncStrategy: "mapping + data upsert"},
		{Name: "kafka", Kind: "kafka", DataDir: "002-data/osh/osh-kafka", ConfigDir: "001-docker-compose/osh", LogDir: "004-log/osh/kafka", PortPolicy: "blue 59092, green 29092", BackupStrategy: "topic metadata snapshot", RollbackStrategy: "delete created topic or restore config", HealthCheck: "topic metadata and smoke topic", SyncStrategy: "topic metadata sync"},
		{Name: "mongodb", Kind: "mongodb", DataDir: "002-data/osh/osh-mongodb", ConfigDir: "001-docker-compose/osh", LogDir: "004-log/osh/mongodb", PortPolicy: "must reserve blue/green ports", BackupStrategy: "mongodump before apply", RollbackStrategy: "mongorestore snapshot", HealthCheck: "mongo ping", SyncStrategy: "collection diff/upsert"},
		{Name: "hbase", Kind: "hbase", DataDir: "002-data/osh/osh-hbase", ConfigDir: "001-docker-compose/osh", LogDir: "004-log/osh/hbase", PortPolicy: "must reserve blue/green ports", BackupStrategy: "snapshot table before apply", RollbackStrategy: "restore HBase snapshot", HealthCheck: "table exists and row count", SyncStrategy: "snapshot/export incremental"},
	}
	for _, sp := range specs {
		id := sp.Kind
		if _, err := s.db.ExecContext(ctx, `
			INSERT INTO component_specs (id, name, kind, compose_path, data_dir, config_dir, log_dir,
				port_policy, backup_strategy, rollback_strategy, health_check, sync_strategy,
				rewrite_rules, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(name) DO UPDATE SET
				kind=excluded.kind, data_dir=excluded.data_dir, config_dir=excluded.config_dir,
				log_dir=excluded.log_dir, port_policy=excluded.port_policy,
				backup_strategy=excluded.backup_strategy, rollback_strategy=excluded.rollback_strategy,
				health_check=excluded.health_check, sync_strategy=excluded.sync_strategy,
				rewrite_rules=excluded.rewrite_rules, updated_at=excluded.updated_at`,
			id, sp.Name, sp.Kind, sp.ComposePath, sp.DataDir, sp.ConfigDir, sp.LogDir,
			sp.PortPolicy, sp.BackupStrategy, sp.RollbackStrategy, sp.HealthCheck, sp.SyncStrategy,
			sp.RewriteRules, now, now); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) UpsertChangeExecution(ctx context.Context, e models.ChangeExecution) (models.ChangeExecution, error) {
	now := time.Now().UTC()
	if e.ID == "" {
		e.ID = uuid.New().String()[:12]
	}
	if e.StartedAt.IsZero() {
		e.StartedAt = now
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO change_executions (id, release_id, item_id, slot, component, action, node,
			status, plan_json, output, error, started_at, finished_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			status=excluded.status, plan_json=excluded.plan_json, output=excluded.output,
			error=excluded.error, finished_at=excluded.finished_at`,
		e.ID, e.ReleaseID, e.ItemID, e.Slot, e.Component, e.Action, e.Node, e.Status,
		e.PlanJSON, e.Output, e.Error, e.StartedAt.Format(time.RFC3339), timePtrString(e.FinishedAt))
	return e, err
}

func (s *Store) ListChangeExecutions(ctx context.Context, releaseID string) ([]models.ChangeExecution, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, release_id, item_id, slot, component, action, node, status, plan_json,
			output, error, started_at, finished_at
		FROM change_executions WHERE release_id = ? ORDER BY started_at`, releaseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.ChangeExecution
	for rows.Next() {
		var e models.ChangeExecution
		var started string
		var finished sql.NullString
		if err := rows.Scan(&e.ID, &e.ReleaseID, &e.ItemID, &e.Slot, &e.Component, &e.Action,
			&e.Node, &e.Status, &e.PlanJSON, &e.Output, &e.Error, &started, &finished); err != nil {
			return nil, err
		}
		e.StartedAt, _ = time.Parse(time.RFC3339, started)
		if finished.Valid {
			t, _ := time.Parse(time.RFC3339, finished.String)
			e.FinishedAt = &t
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) ListComponentSpecs(ctx context.Context) ([]models.ComponentSpec, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, kind, compose_path, data_dir, config_dir, log_dir, port_policy,
			backup_strategy, rollback_strategy, health_check, sync_strategy, rewrite_rules,
			created_at, updated_at
		FROM component_specs ORDER BY kind`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.ComponentSpec
	for rows.Next() {
		var sp models.ComponentSpec
		var created, updated string
		if err := rows.Scan(&sp.ID, &sp.Name, &sp.Kind, &sp.ComposePath, &sp.DataDir,
			&sp.ConfigDir, &sp.LogDir, &sp.PortPolicy, &sp.BackupStrategy, &sp.RollbackStrategy,
			&sp.HealthCheck, &sp.SyncStrategy, &sp.RewriteRules, &created, &updated); err != nil {
			return nil, err
		}
		sp.CreatedAt, _ = time.Parse(time.RFC3339, created)
		sp.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
		out = append(out, sp)
	}
	return out, rows.Err()
}

func (s *Store) ListComponentTestReports(ctx context.Context, releaseID string) ([]models.ComponentTestReport, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, release_id, item_id, slot, component, functional_json, data_diff_json,
			ai_verdict, passed, created_at
		FROM component_test_reports WHERE release_id = ? ORDER BY created_at`, releaseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.ComponentTestReport
	for rows.Next() {
		var r models.ComponentTestReport
		var passed int
		var created string
		if err := rows.Scan(&r.ID, &r.ReleaseID, &r.ItemID, &r.Slot, &r.Component,
			&r.FunctionalJSON, &r.DataDiffJSON, &r.AIVerdict, &passed, &created); err != nil {
			return nil, err
		}
		r.Passed = passed == 1
		r.CreatedAt, _ = time.Parse(time.RFC3339, created)
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) SaveRollbackSnapshot(ctx context.Context, snap models.RollbackSnapshot) error {
	if snap.ID == "" {
		snap.ID = uuid.New().String()[:12]
	}
	if snap.CreatedAt.IsZero() {
		snap.CreatedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO rollback_snapshots (id, release_id, item_id, slot, component, snapshot_type,
			snapshot_path, metadata_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		snap.ID, snap.ReleaseID, snap.ItemID, snap.Slot, snap.Component, snap.SnapshotType,
		snap.SnapshotPath, snap.MetadataJSON, snap.CreatedAt.Format(time.RFC3339))
	return err
}

func (s *Store) SaveComponentTestReport(ctx context.Context, r models.ComponentTestReport) error {
	if r.ID == "" {
		r.ID = uuid.New().String()[:12]
	}
	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO component_test_reports (id, release_id, item_id, slot, component,
			functional_json, data_diff_json, ai_verdict, passed, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.ReleaseID, r.ItemID, r.Slot, r.Component, r.FunctionalJSON, r.DataDiffJSON,
		r.AIVerdict, boolToInt(r.Passed), r.CreatedAt.Format(time.RFC3339))
	return err
}

func (s *Store) AddConflictNotification(ctx context.Context, n models.ConflictNotification) error {
	now := time.Now().UTC()
	if n.ID == "" {
		n.ID = uuid.New().String()[:12]
	}
	if n.Status == "" {
		n.Status = "pending"
	}
	if n.CreatedAt.IsZero() {
		n.CreatedAt = now
	}
	n.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO conflict_notifications (id, release_id, item_id, file_path, owner, email,
			status, message, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		n.ID, n.ReleaseID, n.ItemID, n.FilePath, n.Owner, n.Email, n.Status, n.Message,
		n.CreatedAt.Format(time.RFC3339), n.UpdatedAt.Format(time.RFC3339))
	return err
}

func (s *Store) ListConflictNotifications(ctx context.Context, releaseID string) ([]models.ConflictNotification, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, release_id, item_id, file_path, owner, email, status, message, created_at, updated_at
		FROM conflict_notifications WHERE release_id = ? ORDER BY created_at DESC`, releaseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.ConflictNotification
	for rows.Next() {
		var n models.ConflictNotification
		var created, updated string
		if err := rows.Scan(&n.ID, &n.ReleaseID, &n.ItemID, &n.FilePath, &n.Owner,
			&n.Email, &n.Status, &n.Message, &created, &updated); err != nil {
			return nil, err
		}
		n.CreatedAt, _ = time.Parse(time.RFC3339, created)
		n.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
		out = append(out, n)
	}
	return out, rows.Err()
}

func timePtrString(t *time.Time) any {
	if t == nil || t.IsZero() {
		return nil
	}
	return t.UTC().Format(time.RFC3339)
}
