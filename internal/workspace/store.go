// Package workspace owns workspaces and memberships. Every read and write is
// scoped by the requesting user's membership; there is no unscoped access.
package workspace

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"kerjalancar/internal/template"
)

var (
	ErrNotFound        = errors.New("workspace: not found")
	ErrInvalidName     = errors.New("workspace: name must be 1-100 characters")
	ErrInvalidTimezone = errors.New("workspace: unsupported time zone")
)

// Timezones a workspace may use (mirrored by a CHECK constraint, migration 00004).
var Timezones = []string{"Asia/Jakarta", "Asia/Makassar", "Asia/Jayapura"}

var timezoneLabels = map[string]string{
	"Asia/Jakarta":  "WIB (Jakarta, Sumatra, Jawa, Kalimantan Barat)",
	"Asia/Makassar": "WITA (Makassar, Bali, Kalimantan Selatan)",
	"Asia/Jayapura": "WIT (Papua, Maluku)",
}

// TimezoneLabel is the Indonesian label of a supported zone ("" otherwise).
func TimezoneLabel(tz string) string { return timezoneLabels[tz] }

// ValidTimezone reports whether tz is one of Timezones.
func ValidTimezone(tz string) bool { _, ok := timezoneLabels[tz]; return ok }

// Workspace is one business tenant.
type Workspace struct {
	ID       uuid.UUID
	Name     string
	Timezone string
}

// CreateWithOwner inserts a workspace and its owner membership inside the
// caller's transaction, so user creation and workspace creation commit together.
func CreateWithOwner(ctx context.Context, tx pgx.Tx, name string, ownerID uuid.UUID) (Workspace, error) {
	name = strings.TrimSpace(name)
	id, err := uuid.NewV7()
	if err != nil {
		return Workspace{}, fmt.Errorf("workspace: id: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO workspaces (id, name) VALUES ($1, $2)`, id, name); err != nil {
		return Workspace{}, fmt.Errorf("workspace: insert: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO memberships (workspace_id, user_id, role) VALUES ($1, $2, 'owner')`, id, ownerID); err != nil {
		return Workspace{}, fmt.Errorf("workspace: owner membership: %w", err)
	}
	if err := template.SeedTx(ctx, tx, id); err != nil {
		return Workspace{}, err
	}
	return Workspace{ID: id, Name: name, Timezone: "Asia/Jakarta"}, nil
}

// Store reads and updates workspaces for a given member.
type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// ForUser lists the workspaces the user belongs to, oldest first.
func (s *Store) ForUser(ctx context.Context, userID uuid.UUID) ([]Workspace, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT w.id, w.name, w.timezone
		FROM workspaces w
		JOIN memberships m ON m.workspace_id = w.id
		WHERE m.user_id = $1
		ORDER BY w.created_at`, userID)
	if err != nil {
		return nil, fmt.Errorf("workspace: list: %w", err)
	}
	defer rows.Close()
	var out []Workspace
	for rows.Next() {
		var w Workspace
		if err := rows.Scan(&w.ID, &w.Name, &w.Timezone); err != nil {
			return nil, fmt.Errorf("workspace: scan: %w", err)
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// Get returns the workspace only when userID is a member; otherwise ErrNotFound.
func (s *Store) Get(ctx context.Context, workspaceID, userID uuid.UUID) (Workspace, error) {
	var w Workspace
	err := s.pool.QueryRow(ctx, `
		SELECT w.id, w.name, w.timezone
		FROM workspaces w
		JOIN memberships m ON m.workspace_id = w.id
		WHERE w.id = $1 AND m.user_id = $2`, workspaceID, userID).Scan(&w.ID, &w.Name, &w.Timezone)
	if errors.Is(err, pgx.ErrNoRows) {
		return Workspace{}, ErrNotFound
	}
	if err != nil {
		return Workspace{}, fmt.Errorf("workspace: get: %w", err)
	}
	return w, nil
}

// Rename changes the name; only the owner may do it. Non-members and staff
// get ErrNotFound, indistinguishable from a workspace that does not exist.
func (s *Store) Rename(ctx context.Context, workspaceID, userID uuid.UUID, name string) (Workspace, error) {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 100 {
		return Workspace{}, ErrInvalidName
	}
	var w Workspace
	err := s.pool.QueryRow(ctx, `
		UPDATE workspaces w SET name = $3, updated_at = now()
		FROM memberships m
		WHERE w.id = $1 AND m.workspace_id = w.id AND m.user_id = $2 AND m.role = 'owner'
		RETURNING w.id, w.name, w.timezone`, workspaceID, userID, name).Scan(&w.ID, &w.Name, &w.Timezone)
	if errors.Is(err, pgx.ErrNoRows) {
		return Workspace{}, ErrNotFound
	}
	if err != nil {
		return Workspace{}, fmt.Errorf("workspace: rename: %w", err)
	}
	return w, nil
}

// UpdateTimezone changes the workspace zone; only the owner may do it and only
// to a supported zone. Non-members and staff get ErrNotFound.
func (s *Store) UpdateTimezone(ctx context.Context, workspaceID, userID uuid.UUID, tz string) (Workspace, error) {
	if !ValidTimezone(tz) {
		return Workspace{}, ErrInvalidTimezone
	}
	var w Workspace
	err := s.pool.QueryRow(ctx, `
		UPDATE workspaces w SET timezone = $3, updated_at = now()
		FROM memberships m
		WHERE w.id = $1 AND m.workspace_id = w.id AND m.user_id = $2 AND m.role = 'owner'
		RETURNING w.id, w.name, w.timezone`, workspaceID, userID, tz).Scan(&w.ID, &w.Name, &w.Timezone)
	if errors.Is(err, pgx.ErrNoRows) {
		return Workspace{}, ErrNotFound
	}
	if err != nil {
		return Workspace{}, fmt.Errorf("workspace: update timezone: %w", err)
	}
	return w, nil
}
