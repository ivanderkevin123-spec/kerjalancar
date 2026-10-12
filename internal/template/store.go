package template

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("template: not found")

// ValidationError carries field messages keyed by form field.
type ValidationError struct {
	Fields map[string]string
}

func (e *ValidationError) Error() string { return fmt.Sprintf("template: invalid input %v", e.Fields) }

// Template is one of a workspace's three message slots.
type Template struct {
	ID          uuid.UUID
	WorkspaceID uuid.UUID
	Key         Key
	Name        string
	Body        string
	Position    int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Store reads and edits templates of one workspace at a time.
type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

const columns = `id, workspace_id, key, name, body, position, created_at, updated_at`

func scan(row pgx.Row) (Template, error) {
	var t Template
	err := row.Scan(&t.ID, &t.WorkspaceID, &t.Key, &t.Name, &t.Body, &t.Position, &t.CreatedAt, &t.UpdatedAt)
	return t, err
}

// SeedTx inserts the default templates for a new workspace inside the
// caller's transaction (workspace.CreateWithOwner).
func SeedTx(ctx context.Context, tx pgx.Tx, workspaceID uuid.UUID) error {
	for _, s := range Seeds {
		id, err := uuid.NewV7()
		if err != nil {
			return fmt.Errorf("template: id: %w", err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO message_templates (id, workspace_id, key, name, body, position) VALUES ($1, $2, $3, $4, $5, $6)`,
			id, workspaceID, s.Key, s.Name, s.Body, s.Position); err != nil {
			return fmt.Errorf("template: seed %s: %w", s.Key, err)
		}
	}
	return nil
}

// ForWorkspace lists the workspace's templates in display order.
func (s *Store) ForWorkspace(ctx context.Context, workspaceID uuid.UUID) ([]Template, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+columns+` FROM message_templates WHERE workspace_id = $1 ORDER BY position`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("template: list: %w", err)
	}
	defer rows.Close()
	var out []Template
	for rows.Next() {
		t, err := scan(rows)
		if err != nil {
			return nil, fmt.Errorf("template: scan: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// Get returns one slot of the workspace; unknown keys and other workspaces are ErrNotFound.
func (s *Store) Get(ctx context.Context, workspaceID uuid.UUID, key Key) (Template, error) {
	if !key.Valid() {
		return Template{}, ErrNotFound
	}
	t, err := scan(s.pool.QueryRow(ctx, `SELECT `+columns+` FROM message_templates WHERE workspace_id = $1 AND key = $2`, workspaceID, key))
	if errors.Is(err, pgx.ErrNoRows) {
		return Template{}, ErrNotFound
	}
	if err != nil {
		return Template{}, fmt.Errorf("template: get: %w", err)
	}
	return t, nil
}

// Update edits a slot's name and body after validation.
func (s *Store) Update(ctx context.Context, workspaceID uuid.UUID, key Key, name, body string) (Template, error) {
	if !key.Valid() {
		return Template{}, ErrNotFound
	}
	name, body = strings.TrimSpace(name), strings.TrimSpace(body)
	if errs := Validate(name, body); len(errs) > 0 {
		return Template{}, &ValidationError{Fields: errs}
	}
	t, err := scan(s.pool.QueryRow(ctx, `
		UPDATE message_templates SET name = $3, body = $4, updated_at = now()
		WHERE workspace_id = $1 AND key = $2
		RETURNING `+columns, workspaceID, key, name, body))
	if errors.Is(err, pgx.ErrNoRows) {
		return Template{}, ErrNotFound
	}
	if err != nil {
		return Template{}, fmt.Errorf("template: update: %w", err)
	}
	return t, nil
}
