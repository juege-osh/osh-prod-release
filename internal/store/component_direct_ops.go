package store

import (
	"context"
	"fmt"
	"time"

	"github.com/juege/osh-prod-release/internal/models"
)

type ComponentDirectOp struct {
	ID          string    `json:"id"`
	Kind        string    `json:"kind"`
	Slot        string    `json:"slot"`
	Action      string    `json:"action"`
	RefPath     string    `json:"ref_path,omitempty"`
	Node        string    `json:"node,omitempty"`
	WorkRelease string    `json:"work_release"`
	WorkItem    string    `json:"work_item"`
	Actor       string    `json:"actor"`
	Status      string    `json:"status"`
	Output      string    `json:"output,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

func (s *Store) SaveComponentDirectOp(ctx context.Context, op ComponentDirectOp) error {
	if op.CreatedAt.IsZero() {
		op.CreatedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO component_direct_ops (id, kind, slot, action, ref_path, node,
			work_release, work_item, actor, status, output, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		op.ID, op.Kind, op.Slot, op.Action, op.RefPath, op.Node,
		op.WorkRelease, op.WorkItem, op.Actor, op.Status, op.Output,
		op.CreatedAt.Format(time.RFC3339))
	return err
}

func (s *Store) UpdateComponentDirectOpStatus(ctx context.Context, id, status, output string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE component_direct_ops SET status = ?, output = ? WHERE id = ?`,
		status, output, id)
	return err
}

func (s *Store) GetLatestComponentDirectOp(ctx context.Context, kind, slot, status string) (*ComponentDirectOp, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, kind, slot, action, ref_path, node, work_release, work_item,
			actor, status, output, created_at
		FROM component_direct_ops
		WHERE kind = ? AND slot = ? AND status = ?
		ORDER BY created_at DESC LIMIT 1`, kind, slot, status)
	return scanComponentDirectOp(row)
}

func (s *Store) ListComponentDirectOps(ctx context.Context, kind, slot string, limit int) ([]ComponentDirectOp, error) {
	if limit <= 0 {
		limit = 20
	}
	query := `
		SELECT id, kind, slot, action, ref_path, node, work_release, work_item,
			actor, status, output, created_at
		FROM component_direct_ops WHERE 1=1`
	args := []any{}
	if kind != "" {
		query += ` AND kind = ?`
		args = append(args, kind)
	}
	if slot != "" {
		query += ` AND slot = ?`
		args = append(args, slot)
	}
	query += ` ORDER BY created_at DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ComponentDirectOp
	for rows.Next() {
		op, err := scanComponentDirectOp(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *op)
	}
	return out, rows.Err()
}

func scanComponentDirectOp(row interface{ Scan(...any) error }) (*ComponentDirectOp, error) {
	var op ComponentDirectOp
	var created string
	if err := row.Scan(&op.ID, &op.Kind, &op.Slot, &op.Action, &op.RefPath, &op.Node,
		&op.WorkRelease, &op.WorkItem, &op.Actor, &op.Status, &op.Output, &created); err != nil {
		return nil, err
	}
	op.CreatedAt, _ = time.Parse(time.RFC3339, created)
	return &op, nil
}

func (s *Store) ListUsers(ctx context.Context) ([]models.UserPublicRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT username, role, display_name, created_at FROM users ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.UserPublicRecord
	for rows.Next() {
		var u models.UserPublicRecord
		var role, created string
		if err := rows.Scan(&u.Username, &role, &u.DisplayName, &created); err != nil {
			return nil, err
		}
		u.Role = models.UserRole(role)
		u.CreatedAt, _ = time.Parse(time.RFC3339, created)
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Store) UpdateUser(ctx context.Context, username string, role models.UserRole, displayName string) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE users SET role = ?, display_name = ? WHERE username = ?`,
		string(role), displayName, username)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("user not found")
	}
	return nil
}

func (s *Store) UpdateUserPassword(ctx context.Context, username, passwordHash string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE users SET password_hash = ? WHERE username = ?`, passwordHash, username)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("user not found")
	}
	return nil
}

func (s *Store) DeleteUser(ctx context.Context, username string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM users WHERE username = ?`, username)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("user not found")
	}
	_, _ = s.db.ExecContext(ctx, `DELETE FROM sessions WHERE username = ?`, username)
	return nil
}
