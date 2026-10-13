package quotation

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"kerjalancar/internal/lead"
	"kerjalancar/internal/money"
)

var (
	ErrNotFound     = errors.New("quotation: not found")
	ErrLeadNotFound = errors.New("quotation: lead not found in workspace")
	ErrLeadClosed   = errors.New("quotation: lead is closed; reopen it first")
	ErrLocked       = errors.New("quotation: accepted or rejected quotations cannot change")
	ErrStale        = errors.New("quotation: edited elsewhere; reload")
	ErrSameStatus   = errors.New("quotation: status unchanged")
	ErrExpired      = errors.New("quotation: no longer valid")
)

// PerPage is the list page size; MaxPerLead caps the lead's quotation list.
const (
	PerPage    = 25
	MaxPerLead = 50
)

// Store reads and writes quotations for one workspace at a time. Accepting a
// quotation wins its lead through the lead store, inside the same transaction.
// Lock order: lead row → quotation row (→ follow-up rows through the lead hooks).
type Store struct {
	pool  *pgxpool.Pool
	leads *lead.Store
}

func NewStore(pool *pgxpool.Pool, leads *lead.Store) *Store { return &Store{pool: pool, leads: leads} }

const columns = `
	q.id, q.workspace_id, q.lead_id, q.customer_id, l.title, c.name, coalesce(c.phone, ''), w.name,
	q.seq, q.status, q.valid_until, coalesce(q.notes, ''), q.discount_pct, q.tax_pct,
	q.subtotal, q.discount_amount, q.tax_amount, q.total,
	coalesce(q.share_token, ''), q.share_revoked_at, q.first_opened_at, q.open_count,
	q.sent_at, q.accepted_at, q.rejected_at, q.version, q.created_at, q.updated_at`

const from = ` FROM quotations q
	JOIN leads l ON l.workspace_id = q.workspace_id AND l.id = q.lead_id
	JOIN customers c ON c.workspace_id = q.workspace_id AND c.id = q.customer_id
	JOIN workspaces w ON w.id = q.workspace_id`

type rowScanner interface{ Scan(dest ...any) error }

func scan(row rowScanner) (Quotation, error) {
	var q Quotation
	err := row.Scan(&q.ID, &q.WorkspaceID, &q.LeadID, &q.CustomerID, &q.LeadTitle, &q.CustomerName, &q.CustomerPhone, &q.WorkspaceName,
		&q.Seq, &q.Status, &q.ValidUntil, &q.Notes, &q.DiscountPct, &q.TaxPct,
		&q.Subtotal, &q.DiscountAmount, &q.TaxAmount, &q.Total,
		&q.ShareToken, &q.ShareRevokedAt, &q.FirstOpenedAt, &q.OpenCount,
		&q.SentAt, &q.AcceptedAt, &q.RejectedAt, &q.Version, &q.CreatedAt, &q.UpdatedAt)
	return q, err
}

type querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

func get(ctx context.Context, q querier, workspaceID, id uuid.UUID, forUpdate bool) (Quotation, error) {
	sql := `SELECT` + columns + from + ` WHERE q.workspace_id = $1 AND q.id = $2`
	if forUpdate {
		sql += ` FOR UPDATE OF q`
	}
	out, err := scan(q.QueryRow(ctx, sql, workspaceID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Quotation{}, ErrNotFound
	}
	if err != nil {
		return Quotation{}, fmt.Errorf("quotation: get: %w", err)
	}
	out.Items, err = items(ctx, q, workspaceID, id)
	return out, err
}

func items(ctx context.Context, q querier, workspaceID, id uuid.UUID) ([]Item, error) {
	rows, err := q.Query(ctx, `SELECT id, position, description, qty_milli, coalesce(unit, ''), unit_price, line_total
		FROM quotation_items WHERE workspace_id = $1 AND quotation_id = $2 ORDER BY position`, workspaceID, id)
	if err != nil {
		return nil, fmt.Errorf("quotation: items: %w", err)
	}
	defer rows.Close()
	var out []Item
	for rows.Next() {
		var it Item
		if err := rows.Scan(&it.ID, &it.Position, &it.Description, &it.QtyMilli, &it.Unit, &it.UnitPrice, &it.LineTotal); err != nil {
			return nil, fmt.Errorf("quotation: scan item: %w", err)
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// Get returns the quotation with its items when it belongs to workspaceID.
func (s *Store) Get(ctx context.Context, workspaceID, id uuid.UUID) (Quotation, error) {
	return get(ctx, s.pool, workspaceID, id, false)
}

// lockLead locks the lead row (root lock) and returns its status and customer.
func lockLead(ctx context.Context, tx pgx.Tx, workspaceID, leadID uuid.UUID) (lead.Status, uuid.UUID, error) {
	var st lead.Status
	var customerID uuid.UUID
	err := tx.QueryRow(ctx, `SELECT status, customer_id FROM leads WHERE workspace_id = $1 AND id = $2 FOR UPDATE`, workspaceID, leadID).Scan(&st, &customerID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", uuid.Nil, ErrLeadNotFound
	}
	if err != nil {
		return "", uuid.Nil, fmt.Errorf("quotation: lock lead: %w", err)
	}
	return st, customerID, nil
}

// nextSeq takes the workspace's next number under a row lock (D-051).
func nextSeq(ctx context.Context, tx pgx.Tx, workspaceID uuid.UUID) (int, error) {
	var seq int
	err := tx.QueryRow(ctx, `
		INSERT INTO quotation_sequences (workspace_id, last_seq) VALUES ($1, 1)
		ON CONFLICT (workspace_id) DO UPDATE SET last_seq = quotation_sequences.last_seq + 1
		RETURNING last_seq`, workspaceID).Scan(&seq)
	if err != nil {
		return 0, fmt.Errorf("quotation: sequence: %w", err)
	}
	return seq, nil
}

func writeItems(ctx context.Context, tx pgx.Tx, workspaceID, quotationID uuid.UUID, items []NormalizedItem) error {
	if _, err := tx.Exec(ctx, `DELETE FROM quotation_items WHERE workspace_id = $1 AND quotation_id = $2`, workspaceID, quotationID); err != nil {
		return fmt.Errorf("quotation: clear items: %w", err)
	}
	for i, it := range items {
		id, err := uuid.NewV7()
		if err != nil {
			return fmt.Errorf("quotation: item id: %w", err)
		}
		_, err = tx.Exec(ctx, `INSERT INTO quotation_items (id, workspace_id, quotation_id, position, description, qty_milli, unit, unit_price, line_total)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`, id, workspaceID, quotationID, i, it.Description, it.QtyMilli, nullable(it.Unit), it.UnitPrice, it.LineTotal)
		if err != nil {
			return fmt.Errorf("quotation: insert item: %w", err)
		}
	}
	return nil
}

func activityData(q Quotation) lead.ActivityData {
	return lead.ActivityData{Number: q.Number(), Total: money.FormatIDR(q.Total)}
}

// Create inserts a draft quotation with its items for a lead of the workspace
// (the customer is the lead's), takes the next number, and records history,
// all in one transaction.
func (s *Store) Create(ctx context.Context, workspaceID, actorID, leadID uuid.UUID, n Normalized) (Quotation, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Quotation{}, fmt.Errorf("quotation: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	_, customerID, err := lockLead(ctx, tx, workspaceID, leadID)
	if err != nil {
		return Quotation{}, err
	}
	seq, err := nextSeq(ctx, tx, workspaceID)
	if err != nil {
		return Quotation{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Quotation{}, fmt.Errorf("quotation: id: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO quotations (id, workspace_id, lead_id, customer_id, seq, status, valid_until, notes, discount_pct, tax_pct,
		                        subtotal, discount_amount, tax_amount, total, created_by)
		VALUES ($1, $2, $3, $4, $5, 'draft', $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
		id, workspaceID, leadID, customerID, seq, n.ValidUntil, nullable(n.Notes), n.DiscountPct, n.TaxPct,
		n.Subtotal, n.DiscountAmount, n.TaxAmount, n.Total, nullableID(actorID))
	if err != nil {
		return Quotation{}, fmt.Errorf("quotation: insert: %w", err)
	}
	if err := writeItems(ctx, tx, workspaceID, id, n.Items); err != nil {
		return Quotation{}, err
	}
	q, err := get(ctx, tx, workspaceID, id, false)
	if err != nil {
		return Quotation{}, err
	}
	if err := lead.AppendActivity(ctx, tx, workspaceID, leadID, actorID, lead.KindQuotationCreated, "", activityData(q)); err != nil {
		return Quotation{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Quotation{}, fmt.Errorf("quotation: commit: %w", err)
	}
	return q, nil
}

// lockQuotation locks the lead row first, then the quotation (lock order).
func lockQuotation(ctx context.Context, tx pgx.Tx, workspaceID, id uuid.UUID) (Quotation, lead.Status, error) {
	var leadID uuid.UUID
	err := tx.QueryRow(ctx, `SELECT lead_id FROM quotations WHERE workspace_id = $1 AND id = $2`, workspaceID, id).Scan(&leadID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Quotation{}, "", ErrNotFound
	}
	if err != nil {
		return Quotation{}, "", fmt.Errorf("quotation: lookup: %w", err)
	}
	st, _, err := lockLead(ctx, tx, workspaceID, leadID)
	if err != nil {
		return Quotation{}, "", err
	}
	q, err := get(ctx, tx, workspaceID, id, true)
	return q, st, err
}

// Update replaces items and terms while the quotation is editable. version
// must match the version the form was rendered from (optimistic lock).
func (s *Store) Update(ctx context.Context, workspaceID, actorID, id uuid.UUID, n Normalized, version int) (Quotation, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Quotation{}, fmt.Errorf("quotation: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	cur, _, err := lockQuotation(ctx, tx, workspaceID, id)
	if err != nil {
		return Quotation{}, err
	}
	if !cur.Editable() {
		return Quotation{}, ErrLocked
	}
	if cur.Version != version {
		return Quotation{}, ErrStale
	}
	_, err = tx.Exec(ctx, `
		UPDATE quotations SET valid_until = $3, notes = $4, discount_pct = $5, tax_pct = $6,
		       subtotal = $7, discount_amount = $8, tax_amount = $9, total = $10, version = version + 1, updated_at = now()
		WHERE workspace_id = $1 AND id = $2`,
		workspaceID, id, n.ValidUntil, nullable(n.Notes), n.DiscountPct, n.TaxPct, n.Subtotal, n.DiscountAmount, n.TaxAmount, n.Total)
	if err != nil {
		return Quotation{}, fmt.Errorf("quotation: update: %w", err)
	}
	if err := writeItems(ctx, tx, workspaceID, id, n.Items); err != nil {
		return Quotation{}, err
	}
	q, err := get(ctx, tx, workspaceID, id, false)
	if err != nil {
		return Quotation{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Quotation{}, fmt.Errorf("quotation: commit: %w", err)
	}
	_ = actorID
	return q, nil
}

// newToken is 16 random bytes as 22 base64url characters.
func newToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("quotation: token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// Send marks a draft as sent and issues the share link (keeps an existing active one).
func (s *Store) Send(ctx context.Context, workspaceID, actorID, id uuid.UUID) (Quotation, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Quotation{}, fmt.Errorf("quotation: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	cur, _, err := lockQuotation(ctx, tx, workspaceID, id)
	if err != nil {
		return Quotation{}, err
	}
	switch cur.Status {
	case StatusSent:
		return Quotation{}, ErrSameStatus
	case StatusAccepted, StatusRejected:
		return Quotation{}, ErrLocked
	}
	token := cur.ShareToken
	if !cur.ShareActive() {
		if token, err = newToken(); err != nil {
			return Quotation{}, err
		}
	}
	_, err = tx.Exec(ctx, `UPDATE quotations SET status = 'sent', sent_at = now(), share_token = $3, share_revoked_at = NULL, version = version + 1, updated_at = now()
		WHERE workspace_id = $1 AND id = $2`, workspaceID, id, token)
	if err != nil {
		return Quotation{}, fmt.Errorf("quotation: send: %w", err)
	}
	q, err := get(ctx, tx, workspaceID, id, false)
	if err != nil {
		return Quotation{}, err
	}
	if err := lead.AppendActivity(ctx, tx, workspaceID, q.LeadID, actorID, lead.KindQuotationSent, "", activityData(q)); err != nil {
		return Quotation{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Quotation{}, fmt.Errorf("quotation: commit: %w", err)
	}
	return q, nil
}

// Accept marks the quotation accepted and wins the lead (status menang) in the
// same transaction; a lead that is already won is left as it is, a lost lead
// must be reopened first.
func (s *Store) Accept(ctx context.Context, workspaceID, actorID, id uuid.UUID, today time.Time) (Quotation, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Quotation{}, fmt.Errorf("quotation: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	cur, leadStatus, err := lockQuotation(ctx, tx, workspaceID, id)
	if err != nil {
		return Quotation{}, err
	}
	switch cur.Status {
	case StatusAccepted:
		return Quotation{}, ErrSameStatus
	case StatusRejected:
		return Quotation{}, ErrLocked
	}
	if leadStatus == lead.StatusKalah {
		return Quotation{}, ErrLeadClosed
	}
	_, err = tx.Exec(ctx, `UPDATE quotations SET status = 'accepted', accepted_at = now(), version = version + 1, updated_at = now() WHERE workspace_id = $1 AND id = $2`, workspaceID, id)
	if err != nil {
		return Quotation{}, fmt.Errorf("quotation: accept: %w", err)
	}
	q, err := get(ctx, tx, workspaceID, id, false)
	if err != nil {
		return Quotation{}, err
	}
	if err := lead.AppendActivity(ctx, tx, workspaceID, q.LeadID, actorID, lead.KindQuotationAccepted, "", activityData(q)); err != nil {
		return Quotation{}, err
	}
	if leadStatus != lead.StatusMenang {
		if _, err := s.leads.ChangeStatusTx(ctx, tx, workspaceID, actorID, q.LeadID, lead.StatusMenang, "", today); err != nil {
			return Quotation{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Quotation{}, fmt.Errorf("quotation: commit: %w", err)
	}
	return q, nil
}

// Reject marks a draft or sent quotation rejected and records history.
func (s *Store) Reject(ctx context.Context, workspaceID, actorID, id uuid.UUID) (Quotation, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Quotation{}, fmt.Errorf("quotation: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	cur, _, err := lockQuotation(ctx, tx, workspaceID, id)
	if err != nil {
		return Quotation{}, err
	}
	switch cur.Status {
	case StatusRejected:
		return Quotation{}, ErrSameStatus
	case StatusAccepted:
		return Quotation{}, ErrLocked
	}
	_, err = tx.Exec(ctx, `UPDATE quotations SET status = 'rejected', rejected_at = now(), version = version + 1, updated_at = now() WHERE workspace_id = $1 AND id = $2`, workspaceID, id)
	if err != nil {
		return Quotation{}, fmt.Errorf("quotation: reject: %w", err)
	}
	q, err := get(ctx, tx, workspaceID, id, false)
	if err != nil {
		return Quotation{}, err
	}
	if err := lead.AppendActivity(ctx, tx, workspaceID, q.LeadID, actorID, lead.KindQuotationRejected, "", activityData(q)); err != nil {
		return Quotation{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Quotation{}, fmt.Errorf("quotation: commit: %w", err)
	}
	return q, nil
}

// RevokeShare disables the current link; RenewShare issues a fresh one.
func (s *Store) RevokeShare(ctx context.Context, workspaceID, id uuid.UUID) (Quotation, error) {
	return s.setShare(ctx, workspaceID, id, `share_revoked_at = now()`, nil)
}

func (s *Store) RenewShare(ctx context.Context, workspaceID, id uuid.UUID) (Quotation, error) {
	token, err := newToken()
	if err != nil {
		return Quotation{}, err
	}
	return s.setShare(ctx, workspaceID, id, `share_token = $3, share_revoked_at = NULL`, []any{token})
}

func (s *Store) setShare(ctx context.Context, workspaceID, id uuid.UUID, set string, extra []any) (Quotation, error) {
	args := append([]any{workspaceID, id}, extra...)
	tag, err := s.pool.Exec(ctx, `UPDATE quotations SET `+set+`, updated_at = now() WHERE workspace_id = $1 AND id = $2`, args...)
	if err != nil {
		return Quotation{}, fmt.Errorf("quotation: share: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return Quotation{}, ErrNotFound
	}
	return get(ctx, s.pool, workspaceID, id, false)
}

// GetByToken is the public lookup: unknown or revoked tokens are ErrNotFound,
// a sent quotation past its validity is ErrExpired. Drafts never have a token.
func (s *Store) GetByToken(ctx context.Context, token string, today time.Time) (Quotation, error) {
	if len(token) != 22 {
		return Quotation{}, ErrNotFound
	}
	q, err := scan(s.pool.QueryRow(ctx, `SELECT`+columns+from+` WHERE q.share_token = $1 AND q.share_revoked_at IS NULL AND q.status <> 'draft'`, token))
	if errors.Is(err, pgx.ErrNoRows) {
		return Quotation{}, ErrNotFound
	}
	if err != nil {
		return Quotation{}, fmt.Errorf("quotation: by token: %w", err)
	}
	if q.EffectiveStatus(today) == StatusExpired {
		return Quotation{}, ErrExpired
	}
	q.Items, err = items(ctx, s.pool, q.WorkspaceID, q.ID)
	return q, err
}

// RecordOpen counts a view of the share page.
func (s *Store) RecordOpen(ctx context.Context, workspaceID, id uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, `UPDATE quotations SET open_count = open_count + 1, first_opened_at = coalesce(first_opened_at, now()) WHERE workspace_id = $1 AND id = $2`, workspaceID, id)
	if err != nil {
		return fmt.Errorf("quotation: record open: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ListQuery selects a page: Status is "semua" (default), a stored status, or
// "expired" (sent and past validity); Q matches the number or customer name.
type ListQuery struct {
	Status string
	Q      string
	Page   int
}

// Page is one page of quotations (without items).
type Page struct {
	Items   []Quotation
	Total   int
	Page    int
	Pages   int
	PerPage int
}

// List returns newest quotations first for the workspace.
func (s *Store) List(ctx context.Context, workspaceID uuid.UUID, lq ListQuery, today time.Time) (Page, error) {
	// Every parameter is referenced with an explicit type in each branch so
	// Postgres can infer $2..$4 whatever the filter.
	var cond string
	switch Status(lq.Status) {
	case StatusDraft, StatusAccepted, StatusRejected:
		cond = ` AND q.status = $2::text AND $3::date IS NOT NULL`
	case StatusSent:
		cond = ` AND q.status = $2::text AND (q.valid_until IS NULL OR q.valid_until >= $3::date)`
	case StatusExpired:
		cond = ` AND q.status = 'sent' AND $2::text = 'expired' AND q.valid_until < $3::date`
	default:
		lq.Status = "semua"
		cond = ` AND $2::text = 'semua' AND $3::date IS NOT NULL`
	}
	pattern := likePattern(lq.Q)
	where := from + ` WHERE q.workspace_id = $1` + cond + ` AND ($4::text = '' OR c.name ILIKE $4::text ESCAPE '\' OR lpad(q.seq::text, 4, '0') LIKE $4::text)`
	args := []any{workspaceID, lq.Status, today, pattern}
	var total int
	if err := s.pool.QueryRow(ctx, `SELECT count(*)`+where, args...).Scan(&total); err != nil {
		return Page{}, fmt.Errorf("quotation: count: %w", err)
	}
	page, pages := clampPage(lq.Page, total, PerPage)
	rows, err := s.pool.Query(ctx, `SELECT`+columns+where+` ORDER BY q.created_at DESC, q.id DESC LIMIT $5 OFFSET $6`, append(args, PerPage, (page-1)*PerPage)...)
	if err != nil {
		return Page{}, fmt.Errorf("quotation: list: %w", err)
	}
	defer rows.Close()
	out := Page{Total: total, Page: page, Pages: pages, PerPage: PerPage}
	for rows.Next() {
		q, err := scan(rows)
		if err != nil {
			return Page{}, fmt.Errorf("quotation: scan: %w", err)
		}
		out.Items = append(out.Items, q)
	}
	return out, rows.Err()
}

// ForLead lists a lead's quotations, newest first (without items).
func (s *Store) ForLead(ctx context.Context, workspaceID, leadID uuid.UUID) ([]Quotation, error) {
	rows, err := s.pool.Query(ctx, `SELECT`+columns+from+` WHERE q.workspace_id = $1 AND q.lead_id = $2 ORDER BY q.created_at DESC, q.id DESC LIMIT $3`, workspaceID, leadID, MaxPerLead)
	if err != nil {
		return nil, fmt.Errorf("quotation: for lead: %w", err)
	}
	defer rows.Close()
	var out []Quotation
	for rows.Next() {
		q, err := scan(rows)
		if err != nil {
			return nil, fmt.Errorf("quotation: scan: %w", err)
		}
		out = append(out, q)
	}
	return out, rows.Err()
}

// LatestShared is the most recently sent quotation of the lead whose link is
// active (used for the WhatsApp {tautan} placeholder).
func (s *Store) LatestShared(ctx context.Context, workspaceID, leadID uuid.UUID) (Quotation, bool, error) {
	q, err := scan(s.pool.QueryRow(ctx, `SELECT`+columns+from+`
		WHERE q.workspace_id = $1 AND q.lead_id = $2 AND q.share_token IS NOT NULL AND q.share_revoked_at IS NULL AND q.status <> 'draft'
		ORDER BY q.sent_at DESC NULLS LAST, q.created_at DESC LIMIT 1`, workspaceID, leadID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Quotation{}, false, nil
	}
	if err != nil {
		return Quotation{}, false, fmt.Errorf("quotation: latest shared: %w", err)
	}
	return q, true, nil
}

func likePattern(q string) string {
	q = strings.TrimSpace(q)
	if q == "" {
		return ""
	}
	if r := []rune(q); len(r) > 100 {
		q = string(r[:100])
	}
	return "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(q) + "%"
}

func clampPage(page, total, perPage int) (int, int) {
	pages := (total + perPage - 1) / perPage
	if pages < 1 {
		pages = 1
	}
	if page < 1 {
		page = 1
	}
	if page > pages {
		page = pages
	}
	return page, pages
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func nullableID(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}
	return &id
}
