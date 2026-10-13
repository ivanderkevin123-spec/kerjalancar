package invoice

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"kerjalancar/internal/lead"
	"kerjalancar/internal/money"
	"kerjalancar/internal/quotation"
)

// PerPage is the list page size.
const PerPage = 25

// Store reads and writes invoices, payments and reversals for one workspace
// at a time. Lock order: lead row → invoice row (payments and reversals take
// only the invoice row; they never touch the lead).
type Store struct {
	pool  *pgxpool.Pool
	leads *lead.Store
}

func NewStore(pool *pgxpool.Pool, leads *lead.Store) *Store { return &Store{pool: pool, leads: leads} }

// paid is the sum of payments that were not reversed; balance derives from it.
const columns = `
	i.id, i.workspace_id, i.lead_id, i.customer_id, i.quotation_id, l.title, c.name, coalesce(c.phone, ''), w.name,
	i.seq, i.status, i.due_on, coalesce(i.notes, ''), i.discount_pct, i.tax_pct,
	i.subtotal, i.discount_amount, i.tax_amount, i.total, coalesce(pd.paid, 0), i.total - coalesce(pd.paid, 0),
	coalesce(i.share_token, ''), i.share_revoked_at, i.first_opened_at, i.open_count,
	i.issued_at, i.voided_at, i.version, i.created_at, i.updated_at`

const from = ` FROM invoices i
	JOIN leads l ON l.workspace_id = i.workspace_id AND l.id = i.lead_id
	JOIN customers c ON c.workspace_id = i.workspace_id AND c.id = i.customer_id
	JOIN workspaces w ON w.id = i.workspace_id
	LEFT JOIN LATERAL (
		SELECT sum(p.amount) AS paid FROM payments p
		WHERE p.workspace_id = i.workspace_id AND p.invoice_id = i.id
		  AND NOT EXISTS (SELECT 1 FROM payment_reversals r WHERE r.workspace_id = p.workspace_id AND r.payment_id = p.id)
	) pd ON true`

type rowScanner interface{ Scan(dest ...any) error }

func scan(row rowScanner) (Invoice, error) {
	var i Invoice
	err := row.Scan(&i.ID, &i.WorkspaceID, &i.LeadID, &i.CustomerID, &i.QuotationID, &i.LeadTitle, &i.CustomerName, &i.CustomerPhone, &i.WorkspaceName,
		&i.Seq, &i.Status, &i.DueOn, &i.Notes, &i.DiscountPct, &i.TaxPct,
		&i.Subtotal, &i.DiscountAmount, &i.TaxAmount, &i.Total, &i.Paid, &i.Balance,
		&i.ShareToken, &i.ShareRevokedAt, &i.FirstOpenedAt, &i.OpenCount,
		&i.IssuedAt, &i.VoidedAt, &i.Version, &i.CreatedAt, &i.UpdatedAt)
	return i, err
}

type querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// get loads one invoice with its items and payments.
func get(ctx context.Context, q querier, workspaceID, id uuid.UUID) (Invoice, error) {
	out, err := scan(q.QueryRow(ctx, `SELECT`+columns+from+` WHERE i.workspace_id = $1 AND i.id = $2`, workspaceID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Invoice{}, ErrNotFound
	}
	if err != nil {
		return Invoice{}, fmt.Errorf("invoice: get: %w", err)
	}
	return withDetails(ctx, q, out)
}

func withDetails(ctx context.Context, q querier, inv Invoice) (Invoice, error) {
	var err error
	if inv.Items, err = items(ctx, q, inv.WorkspaceID, inv.ID); err != nil {
		return Invoice{}, err
	}
	if inv.Payments, err = payments(ctx, q, inv.WorkspaceID, inv.ID); err != nil {
		return Invoice{}, err
	}
	return inv, nil
}

func items(ctx context.Context, q querier, workspaceID, id uuid.UUID) ([]quotation.Item, error) {
	rows, err := q.Query(ctx, `SELECT id, position, description, qty_milli, coalesce(unit, ''), unit_price, line_total
		FROM invoice_items WHERE workspace_id = $1 AND invoice_id = $2 ORDER BY position`, workspaceID, id)
	if err != nil {
		return nil, fmt.Errorf("invoice: items: %w", err)
	}
	defer rows.Close()
	var out []quotation.Item
	for rows.Next() {
		var it quotation.Item
		if err := rows.Scan(&it.ID, &it.Position, &it.Description, &it.QtyMilli, &it.Unit, &it.UnitPrice, &it.LineTotal); err != nil {
			return nil, fmt.Errorf("invoice: scan item: %w", err)
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

const paymentColumns = `
	p.id, p.invoice_id, p.kind, p.method, p.amount, p.received_on, coalesce(p.note, ''), p.recorded_by, coalesce(u.name, ''), p.created_at,
	r.id, coalesce(r.reason, ''), r.reversed_by, coalesce(ru.name, ''), r.created_at`

const paymentFrom = ` FROM payments p
	LEFT JOIN users u ON u.id = p.recorded_by
	LEFT JOIN payment_reversals r ON r.workspace_id = p.workspace_id AND r.payment_id = p.id
	LEFT JOIN users ru ON ru.id = r.reversed_by`

func scanPayment(row rowScanner) (Payment, error) {
	var p Payment
	var revID *uuid.UUID
	var revReason string
	var revBy *uuid.UUID
	var revByName string
	var revAt *time.Time
	err := row.Scan(&p.ID, &p.InvoiceID, &p.Kind, &p.Method, &p.Amount, &p.ReceivedOn, &p.Note, &p.RecordedBy, &p.RecordedByName, &p.CreatedAt,
		&revID, &revReason, &revBy, &revByName, &revAt)
	if err != nil {
		return Payment{}, err
	}
	if revID != nil && revAt != nil {
		p.Reversal = &Reversal{ID: *revID, Reason: revReason, ReversedBy: revBy, ReversedByName: revByName, CreatedAt: *revAt}
	}
	return p, nil
}

func payments(ctx context.Context, q querier, workspaceID, invoiceID uuid.UUID) ([]Payment, error) {
	rows, err := q.Query(ctx, `SELECT`+paymentColumns+paymentFrom+` WHERE p.workspace_id = $1 AND p.invoice_id = $2 ORDER BY p.received_on, p.created_at, p.id`, workspaceID, invoiceID)
	if err != nil {
		return nil, fmt.Errorf("invoice: payments: %w", err)
	}
	defer rows.Close()
	var out []Payment
	for rows.Next() {
		p, err := scanPayment(rows)
		if err != nil {
			return nil, fmt.Errorf("invoice: scan payment: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func paymentByID(ctx context.Context, q querier, workspaceID, id uuid.UUID) (Payment, error) {
	p, err := scanPayment(q.QueryRow(ctx, `SELECT`+paymentColumns+paymentFrom+` WHERE p.workspace_id = $1 AND p.id = $2`, workspaceID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Payment{}, ErrNotFound
	}
	if err != nil {
		return Payment{}, fmt.Errorf("invoice: payment: %w", err)
	}
	return p, nil
}

// Get returns the invoice with items and payments when it belongs to workspaceID.
func (s *Store) Get(ctx context.Context, workspaceID, id uuid.UUID) (Invoice, error) {
	return get(ctx, s.pool, workspaceID, id)
}

// lockLead locks the lead row (root lock) and returns its status and customer.
// FOR NO KEY UPDATE: it serializes with the other lead-first writers but does
// not block the KEY SHARE lock that inserting a history line takes through
// the lead_activities foreign key. Payment transactions hold the invoice row
// while they append history, so a stronger lead lock here would let
// Void-while-paying deadlock (M6 review #1, pinned by TestVoidWhilePayingDoesNotDeadlock).
func lockLead(ctx context.Context, tx pgx.Tx, workspaceID, leadID uuid.UUID) (lead.Status, uuid.UUID, error) {
	var st lead.Status
	var customerID uuid.UUID
	err := tx.QueryRow(ctx, `SELECT status, customer_id FROM leads WHERE workspace_id = $1 AND id = $2 FOR NO KEY UPDATE`, workspaceID, leadID).Scan(&st, &customerID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", uuid.Nil, ErrLeadNotFound
	}
	if err != nil {
		return "", uuid.Nil, fmt.Errorf("invoice: lock lead: %w", err)
	}
	return st, customerID, nil
}

// lockInvoice takes the invoice row lock (after the lead lock when the caller
// holds one) and returns the fresh row.
func lockInvoice(ctx context.Context, tx pgx.Tx, workspaceID, id uuid.UUID) (Invoice, error) {
	var leadID uuid.UUID
	err := tx.QueryRow(ctx, `SELECT lead_id FROM invoices WHERE workspace_id = $1 AND id = $2 FOR UPDATE`, workspaceID, id).Scan(&leadID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Invoice{}, ErrNotFound
	}
	if err != nil {
		return Invoice{}, fmt.Errorf("invoice: lock: %w", err)
	}
	return get(ctx, tx, workspaceID, id)
}

// lockLeadThenInvoice follows the lock order for operations that change the
// invoice itself (update, issue, void).
func lockLeadThenInvoice(ctx context.Context, tx pgx.Tx, workspaceID, id uuid.UUID) (Invoice, error) {
	var leadID uuid.UUID
	err := tx.QueryRow(ctx, `SELECT lead_id FROM invoices WHERE workspace_id = $1 AND id = $2`, workspaceID, id).Scan(&leadID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Invoice{}, ErrNotFound
	}
	if err != nil {
		return Invoice{}, fmt.Errorf("invoice: lookup: %w", err)
	}
	if _, _, err := lockLead(ctx, tx, workspaceID, leadID); err != nil {
		return Invoice{}, err
	}
	return lockInvoice(ctx, tx, workspaceID, id)
}

// nextSeq takes the workspace's next number under a row lock (D-051).
func nextSeq(ctx context.Context, tx pgx.Tx, workspaceID uuid.UUID) (int, error) {
	var seq int
	err := tx.QueryRow(ctx, `
		INSERT INTO invoice_sequences (workspace_id, last_seq) VALUES ($1, 1)
		ON CONFLICT (workspace_id) DO UPDATE SET last_seq = invoice_sequences.last_seq + 1
		RETURNING last_seq`, workspaceID).Scan(&seq)
	if err != nil {
		return 0, fmt.Errorf("invoice: sequence: %w", err)
	}
	return seq, nil
}

func writeItems(ctx context.Context, tx pgx.Tx, workspaceID, invoiceID uuid.UUID, items []quotation.NormalizedItem) error {
	if _, err := tx.Exec(ctx, `DELETE FROM invoice_items WHERE workspace_id = $1 AND invoice_id = $2`, workspaceID, invoiceID); err != nil {
		return fmt.Errorf("invoice: clear items: %w", err)
	}
	for i, it := range items {
		id, err := uuid.NewV7()
		if err != nil {
			return fmt.Errorf("invoice: item id: %w", err)
		}
		_, err = tx.Exec(ctx, `INSERT INTO invoice_items (id, workspace_id, invoice_id, position, description, qty_milli, unit, unit_price, line_total)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`, id, workspaceID, invoiceID, i, it.Description, it.QtyMilli, nullable(it.Unit), it.UnitPrice, it.LineTotal)
		if err != nil {
			return fmt.Errorf("invoice: insert item: %w", err)
		}
	}
	return nil
}

// checkQuotation verifies that the source quotation belongs to this lead of
// this workspace. Its rows were already copied into the form by the editor;
// the invoice saves what the owner typed and keeps the quotation as provenance.
func checkQuotation(ctx context.Context, tx pgx.Tx, workspaceID, leadID, quotationID uuid.UUID) error {
	var qLead uuid.UUID
	err := tx.QueryRow(ctx, `SELECT lead_id FROM quotations WHERE workspace_id = $1 AND id = $2`, workspaceID, quotationID).Scan(&qLead)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && qLead != leadID) {
		return ErrQuotationNotFound
	}
	if err != nil {
		return fmt.Errorf("invoice: source quotation: %w", err)
	}
	return nil
}

func activityData(inv Invoice) lead.ActivityData {
	return lead.ActivityData{Number: inv.Number(), Total: money.FormatIDR(inv.Total)}
}

// Create inserts a draft for a lead of the workspace (the customer is the
// lead's) from the typed form n. fromQuotation, when given, must be a
// quotation of that lead and is recorded as the invoice's source; the editor
// pre-fills the form from it and the owner may change any row before saving
// (D-056). Lost leads get no invoices.
func (s *Store) Create(ctx context.Context, workspaceID, actorID, leadID uuid.UUID, fromQuotation *uuid.UUID, n Normalized) (Invoice, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Invoice{}, fmt.Errorf("invoice: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	st, customerID, err := lockLead(ctx, tx, workspaceID, leadID)
	if err != nil {
		return Invoice{}, err
	}
	if st == lead.StatusKalah {
		return Invoice{}, ErrLeadClosed
	}
	if fromQuotation != nil {
		if err := checkQuotation(ctx, tx, workspaceID, leadID, *fromQuotation); err != nil {
			return Invoice{}, err
		}
	}
	seq, err := nextSeq(ctx, tx, workspaceID)
	if err != nil {
		return Invoice{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Invoice{}, fmt.Errorf("invoice: id: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO invoices (id, workspace_id, lead_id, customer_id, quotation_id, seq, status, due_on, notes, discount_pct, tax_pct,
		                      subtotal, discount_amount, tax_amount, total, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, 'draft', $7, $8, $9, $10, $11, $12, $13, $14, $15)`,
		id, workspaceID, leadID, customerID, fromQuotation, seq, n.DueOn, nullable(n.Notes), n.DiscountPct, n.TaxPct,
		n.Subtotal, n.DiscountAmount, n.TaxAmount, n.Total, nullableID(actorID))
	if err != nil {
		return Invoice{}, fmt.Errorf("invoice: insert: %w", err)
	}
	if err := writeItems(ctx, tx, workspaceID, id, n.Items); err != nil {
		return Invoice{}, err
	}
	inv, err := get(ctx, tx, workspaceID, id)
	if err != nil {
		return Invoice{}, err
	}
	if err := lead.AppendActivity(ctx, tx, workspaceID, leadID, actorID, lead.KindInvoiceCreated, "", activityData(inv)); err != nil {
		return Invoice{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Invoice{}, fmt.Errorf("invoice: commit: %w", err)
	}
	return inv, nil
}

// Update replaces items and terms while the invoice is a draft. version must
// match the version the form was rendered from (optimistic lock).
func (s *Store) Update(ctx context.Context, workspaceID, actorID, id uuid.UUID, n Normalized, version int) (Invoice, error) {
	_ = actorID // edits of a draft are not history; the issue line records the final figures
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Invoice{}, fmt.Errorf("invoice: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	inv, err := lockLeadThenInvoice(ctx, tx, workspaceID, id)
	if err != nil {
		return Invoice{}, err
	}
	if !inv.Editable() {
		return Invoice{}, ErrLocked
	}
	if inv.Version != version {
		return Invoice{}, ErrStale
	}
	_, err = tx.Exec(ctx, `
		UPDATE invoices SET due_on = $3, notes = $4, discount_pct = $5, tax_pct = $6,
		       subtotal = $7, discount_amount = $8, tax_amount = $9, total = $10, version = version + 1, updated_at = now()
		WHERE workspace_id = $1 AND id = $2`,
		workspaceID, id, n.DueOn, nullable(n.Notes), n.DiscountPct, n.TaxPct, n.Subtotal, n.DiscountAmount, n.TaxAmount, n.Total)
	if err != nil {
		return Invoice{}, fmt.Errorf("invoice: update: %w", err)
	}
	if err := writeItems(ctx, tx, workspaceID, id, n.Items); err != nil {
		return Invoice{}, err
	}
	out, err := get(ctx, tx, workspaceID, id)
	if err != nil {
		return Invoice{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Invoice{}, fmt.Errorf("invoice: commit: %w", err)
	}
	return out, nil
}

// Issue fixes a draft: it needs a due date and a total above zero, gets its
// public link, and is recorded in the lead's history.
func (s *Store) Issue(ctx context.Context, workspaceID, actorID, id uuid.UUID) (Invoice, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Invoice{}, fmt.Errorf("invoice: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	inv, err := lockLeadThenInvoice(ctx, tx, workspaceID, id)
	if err != nil {
		return Invoice{}, err
	}
	switch {
	case inv.Status != StatusDraft:
		return Invoice{}, ErrLocked
	case inv.DueOn == nil:
		return Invoice{}, ErrNoDueDate
	case inv.Total <= 0:
		return Invoice{}, ErrEmpty
	}
	token, err := newToken()
	if err != nil {
		return Invoice{}, err
	}
	_, err = tx.Exec(ctx, `UPDATE invoices SET status = 'issued', issued_at = now(), share_token = $3, share_revoked_at = NULL, version = version + 1, updated_at = now()
		WHERE workspace_id = $1 AND id = $2`, workspaceID, id, token)
	if err != nil {
		return Invoice{}, fmt.Errorf("invoice: issue: %w", err)
	}
	out, err := get(ctx, tx, workspaceID, id)
	if err != nil {
		return Invoice{}, err
	}
	if err := lead.AppendActivity(ctx, tx, workspaceID, out.LeadID, actorID, lead.KindInvoiceIssued, "", activityData(out)); err != nil {
		return Invoice{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Invoice{}, fmt.Errorf("invoice: commit: %w", err)
	}
	return out, nil
}

// Void cancels a draft or an issued invoice that has no counting payment;
// the public link stops with it.
func (s *Store) Void(ctx context.Context, workspaceID, actorID, id uuid.UUID) (Invoice, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Invoice{}, fmt.Errorf("invoice: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	inv, err := lockLeadThenInvoice(ctx, tx, workspaceID, id)
	if err != nil {
		return Invoice{}, err
	}
	if inv.Status == StatusVoid {
		return Invoice{}, ErrLocked
	}
	if inv.Paid > 0 {
		return Invoice{}, ErrHasPayments
	}
	_, err = tx.Exec(ctx, `UPDATE invoices SET status = 'void', voided_at = now(), share_revoked_at = CASE WHEN share_token IS NULL THEN share_revoked_at ELSE coalesce(share_revoked_at, now()) END,
		version = version + 1, updated_at = now() WHERE workspace_id = $1 AND id = $2`, workspaceID, id)
	if err != nil {
		return Invoice{}, fmt.Errorf("invoice: void: %w", err)
	}
	out, err := get(ctx, tx, workspaceID, id)
	if err != nil {
		return Invoice{}, err
	}
	if err := lead.AppendActivity(ctx, tx, workspaceID, out.LeadID, actorID, lead.KindInvoiceVoided, "", lead.ActivityData{Number: out.Number()}); err != nil {
		return Invoice{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Invoice{}, fmt.Errorf("invoice: commit: %w", err)
	}
	return out, nil
}

// RecordPayment appends a payment under the invoice row lock. The same
// idempotency key on the same invoice returns the first payment again
// (created=false); on another invoice it is ErrKeyMismatch. Amounts above the
// balance are refused with the balance in the error.
func (s *Store) RecordPayment(ctx context.Context, workspaceID, actorID, id uuid.UUID, p NormalizedPayment) (Payment, bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Payment{}, false, fmt.Errorf("invoice: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	inv, err := lockInvoice(ctx, tx, workspaceID, id)
	if err != nil {
		return Payment{}, false, err
	}
	var existing uuid.UUID
	err = tx.QueryRow(ctx, `SELECT id FROM payments WHERE workspace_id = $1 AND idempotency_key = $2`, workspaceID, p.IdempotencyKey).Scan(&existing)
	switch {
	case err == nil:
		pay, err := paymentByID(ctx, tx, workspaceID, existing)
		if err != nil {
			return Payment{}, false, err
		}
		if pay.InvoiceID != id {
			return Payment{}, false, ErrKeyMismatch
		}
		return pay, false, tx.Commit(ctx)
	case !errors.Is(err, pgx.ErrNoRows):
		return Payment{}, false, fmt.Errorf("invoice: idempotency lookup: %w", err)
	}
	if inv.Status != StatusIssued {
		return Payment{}, false, ErrNotIssued
	}
	if p.Amount > inv.Balance {
		return Payment{}, false, &OverpaymentError{Balance: inv.Balance}
	}
	payID, err := uuid.NewV7()
	if err != nil {
		return Payment{}, false, fmt.Errorf("invoice: payment id: %w", err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO payments (id, workspace_id, invoice_id, kind, method, amount, received_on, note, idempotency_key, recorded_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		payID, workspaceID, id, p.Kind, p.Method, p.Amount, p.ReceivedOn, nullable(p.Note), p.IdempotencyKey, nullableID(actorID))
	if err != nil {
		return Payment{}, false, fmt.Errorf("invoice: insert payment: %w", err)
	}
	data := lead.ActivityData{Number: inv.Number(), Amount: money.FormatIDR(p.Amount), Label: p.Kind.Label()}
	if err := lead.AppendActivity(ctx, tx, workspaceID, inv.LeadID, actorID, lead.KindPaymentRecorded, "", data); err != nil {
		return Payment{}, false, err
	}
	pay, err := paymentByID(ctx, tx, workspaceID, payID)
	if err != nil {
		return Payment{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Payment{}, false, fmt.Errorf("invoice: commit: %w", err)
	}
	return pay, true, nil
}

// ReversePayment records that a payment did not happen after all. The
// payment row stays; the reversal carries the reason and the actor.
func (s *Store) ReversePayment(ctx context.Context, workspaceID, actorID, paymentID uuid.UUID, reason string) (Payment, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" || len([]rune(reason)) > MaxNoteLen {
		return Payment{}, ErrInvalidReason
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Payment{}, fmt.Errorf("invoice: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var invoiceID uuid.UUID
	var amount int64
	err = tx.QueryRow(ctx, `SELECT invoice_id, amount FROM payments WHERE workspace_id = $1 AND id = $2`, workspaceID, paymentID).Scan(&invoiceID, &amount)
	if errors.Is(err, pgx.ErrNoRows) {
		return Payment{}, ErrNotFound
	}
	if err != nil {
		return Payment{}, fmt.Errorf("invoice: payment lookup: %w", err)
	}
	inv, err := lockInvoice(ctx, tx, workspaceID, invoiceID)
	if err != nil {
		return Payment{}, err
	}
	revID, err := uuid.NewV7()
	if err != nil {
		return Payment{}, fmt.Errorf("invoice: reversal id: %w", err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO payment_reversals (id, workspace_id, payment_id, reason, reversed_by) VALUES ($1, $2, $3, $4, $5)`,
		revID, workspaceID, paymentID, reason, nullableID(actorID))
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return Payment{}, ErrAlreadyReversed
	}
	if err != nil {
		return Payment{}, fmt.Errorf("invoice: insert reversal: %w", err)
	}
	data := lead.ActivityData{Number: inv.Number(), Amount: money.FormatIDR(amount)}
	if err := lead.AppendActivity(ctx, tx, workspaceID, inv.LeadID, actorID, lead.KindPaymentReversed, "", data); err != nil {
		return Payment{}, err
	}
	pay, err := paymentByID(ctx, tx, workspaceID, paymentID)
	if err != nil {
		return Payment{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Payment{}, fmt.Errorf("invoice: commit: %w", err)
	}
	return pay, nil
}

func newToken() (string, error) {
	k, err := NewIdempotencyKey()
	if err != nil {
		return "", fmt.Errorf("invoice: token: %w", err)
	}
	return k, nil
}

// RevokeShare disables the current link; RenewShare issues a fresh one. Both
// apply to issued invoices only.
func (s *Store) RevokeShare(ctx context.Context, workspaceID, id uuid.UUID) (Invoice, error) {
	return s.setShare(ctx, workspaceID, id, `UPDATE invoices SET share_revoked_at = now(), updated_at = now() WHERE workspace_id = $1 AND id = $2 AND status = 'issued'`, nil)
}

func (s *Store) RenewShare(ctx context.Context, workspaceID, id uuid.UUID) (Invoice, error) {
	token, err := newToken()
	if err != nil {
		return Invoice{}, err
	}
	return s.setShare(ctx, workspaceID, id, `UPDATE invoices SET share_token = $3, share_revoked_at = NULL, updated_at = now() WHERE workspace_id = $1 AND id = $2 AND status = 'issued'`, []any{token})
}

func (s *Store) setShare(ctx context.Context, workspaceID, id uuid.UUID, sql string, extra []any) (Invoice, error) {
	args := append([]any{workspaceID, id}, extra...)
	tag, err := s.pool.Exec(ctx, sql, args...)
	if err != nil {
		return Invoice{}, fmt.Errorf("invoice: share: %w", err)
	}
	if tag.RowsAffected() == 0 {
		inv, err := get(ctx, s.pool, workspaceID, id)
		if err != nil {
			return Invoice{}, err
		}
		if inv.Status != StatusIssued {
			return Invoice{}, ErrLocked
		}
		return Invoice{}, ErrNotFound
	}
	return get(ctx, s.pool, workspaceID, id)
}

// GetByToken is the public lookup: only issued invoices with a live token
// resolve; drafts, void and revoked ones are ErrNotFound.
func (s *Store) GetByToken(ctx context.Context, token string) (Invoice, error) {
	if len(token) != 22 {
		return Invoice{}, ErrNotFound
	}
	inv, err := scan(s.pool.QueryRow(ctx, `SELECT`+columns+from+` WHERE i.share_token = $1 AND i.share_revoked_at IS NULL AND i.status = 'issued'`, token))
	if errors.Is(err, pgx.ErrNoRows) {
		return Invoice{}, ErrNotFound
	}
	if err != nil {
		return Invoice{}, fmt.Errorf("invoice: by token: %w", err)
	}
	return withDetails(ctx, s.pool, inv)
}

// RecordOpen counts a visit to the public page.
func (s *Store) RecordOpen(ctx context.Context, workspaceID, id uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, `UPDATE invoices SET open_count = open_count + 1, first_opened_at = coalesce(first_opened_at, now()) WHERE workspace_id = $1 AND id = $2`, workspaceID, id)
	if err != nil {
		return fmt.Errorf("invoice: record open: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ListQuery filters the list. Status: "" (belum lunas), terlambat, lunas,
// draf, dibatalkan, semua. Q matches the customer name or the number.
type ListQuery struct {
	Status string
	Q      string
	Page   int
}

type Page struct {
	Items   []Invoice
	Total   int
	Page    int
	Pages   int
	PerPage int
}

// List pages invoices. The default view is what is still owed, overdue first
// and then by due date; the other views are newest first.
func (s *Store) List(ctx context.Context, workspaceID uuid.UUID, lq ListQuery, today time.Time) (Page, error) {
	// Every parameter is referenced with an explicit type in each branch so
	// Postgres can infer $2..$4 whatever the filter.
	var cond, order string
	switch lq.Status {
	case "terlambat":
		cond = ` AND i.status = 'issued' AND i.total - coalesce(pd.paid, 0) > 0 AND i.due_on < $3::date AND $2::text = 'terlambat'`
		order = ` ORDER BY i.due_on ASC, i.created_at ASC, i.id ASC`
	case "lunas":
		cond = ` AND i.status = 'issued' AND i.total - coalesce(pd.paid, 0) <= 0 AND $2::text = 'lunas' AND $3::date IS NOT NULL`
		order = ` ORDER BY i.updated_at DESC, i.id DESC`
	case "draf":
		cond = ` AND i.status = 'draft' AND $2::text = 'draf' AND $3::date IS NOT NULL`
		order = ` ORDER BY i.created_at DESC, i.id DESC`
	case "dibatalkan":
		cond = ` AND i.status = 'void' AND $2::text = 'dibatalkan' AND $3::date IS NOT NULL`
		order = ` ORDER BY i.updated_at DESC, i.id DESC`
	case "semua":
		cond = ` AND $2::text = 'semua' AND $3::date IS NOT NULL`
		order = ` ORDER BY i.created_at DESC, i.id DESC`
	default:
		lq.Status = ""
		cond = ` AND i.status = 'issued' AND i.total - coalesce(pd.paid, 0) > 0 AND $2::text = '' AND $3::date IS NOT NULL`
		order = ` ORDER BY (i.due_on < $3::date) DESC, i.due_on ASC NULLS LAST, i.created_at ASC, i.id ASC`
	}
	pattern := likePattern(lq.Q)
	where := from + ` WHERE i.workspace_id = $1` + cond + ` AND ($4::text = '' OR c.name ILIKE $4::text ESCAPE '\' OR lpad(i.seq::text, 4, '0') LIKE $4::text)`
	args := []any{workspaceID, lq.Status, today, pattern}
	var total int
	if err := s.pool.QueryRow(ctx, `SELECT count(*)`+where, args...).Scan(&total); err != nil {
		return Page{}, fmt.Errorf("invoice: count: %w", err)
	}
	page, pages := clampPage(lq.Page, total, PerPage)
	rows, err := s.pool.Query(ctx, `SELECT`+columns+where+order+` LIMIT $5 OFFSET $6`, append(args, PerPage, (page-1)*PerPage)...)
	if err != nil {
		return Page{}, fmt.Errorf("invoice: list: %w", err)
	}
	defer rows.Close()
	out := Page{Total: total, Page: page, Pages: pages, PerPage: PerPage}
	for rows.Next() {
		inv, err := scan(rows)
		if err != nil {
			return Page{}, fmt.Errorf("invoice: scan: %w", err)
		}
		out.Items = append(out.Items, inv)
	}
	return out, rows.Err()
}

// ForLead lists the lead's invoices, newest first, without items or payments.
func (s *Store) ForLead(ctx context.Context, workspaceID, leadID uuid.UUID) ([]Invoice, error) {
	rows, err := s.pool.Query(ctx, `SELECT`+columns+from+` WHERE i.workspace_id = $1 AND i.lead_id = $2 ORDER BY i.created_at DESC, i.id DESC LIMIT 100`, workspaceID, leadID)
	if err != nil {
		return nil, fmt.Errorf("invoice: for lead: %w", err)
	}
	defer rows.Close()
	return collect(rows)
}

// Overdue lists issued, unpaid invoices whose due date has passed, oldest due first.
func (s *Store) Overdue(ctx context.Context, workspaceID uuid.UUID, today time.Time, limit int) ([]Invoice, error) {
	rows, err := s.pool.Query(ctx, `SELECT`+columns+from+` WHERE i.workspace_id = $1 AND i.status = 'issued' AND i.total - coalesce(pd.paid, 0) > 0 AND i.due_on < $2
		ORDER BY i.due_on ASC, i.created_at ASC, i.id ASC LIMIT $3`, workspaceID, today, limit)
	if err != nil {
		return nil, fmt.Errorf("invoice: overdue: %w", err)
	}
	defer rows.Close()
	return collect(rows)
}

func collect(rows pgx.Rows) ([]Invoice, error) {
	var out []Invoice
	for rows.Next() {
		inv, err := scan(rows)
		if err != nil {
			return nil, fmt.Errorf("invoice: scan: %w", err)
		}
		out = append(out, inv)
	}
	return out, rows.Err()
}

// Summary feeds the dashboard: what is owed, what is overdue, what came in
// this month. The numbers reconcile with List (tested).
type Summary struct {
	Outstanding      int64
	OutstandingCount int
	OverdueAmount    int64
	OverdueCount     int
	PaidThisMonth    int64
}

func (s *Store) Summary(ctx context.Context, workspaceID uuid.UUID, today time.Time) (Summary, error) {
	var out Summary
	err := s.pool.QueryRow(ctx, `
		SELECT coalesce(sum(x.balance), 0), count(*),
		       coalesce(sum(x.balance) FILTER (WHERE x.due_on < $2::date), 0), count(*) FILTER (WHERE x.due_on < $2::date)
		FROM (
			SELECT i.due_on, i.total - coalesce(pd.paid, 0) AS balance
			FROM invoices i
			LEFT JOIN LATERAL (
				SELECT sum(p.amount) AS paid FROM payments p
				WHERE p.workspace_id = i.workspace_id AND p.invoice_id = i.id
				  AND NOT EXISTS (SELECT 1 FROM payment_reversals r WHERE r.workspace_id = p.workspace_id AND r.payment_id = p.id)
			) pd ON true
			WHERE i.workspace_id = $1 AND i.status = 'issued'
		) x WHERE x.balance > 0`, workspaceID, today).Scan(&out.Outstanding, &out.OutstandingCount, &out.OverdueAmount, &out.OverdueCount)
	if err != nil {
		return Summary{}, fmt.Errorf("invoice: summary: %w", err)
	}
	err = s.pool.QueryRow(ctx, `
		SELECT coalesce(sum(p.amount), 0) FROM payments p
		WHERE p.workspace_id = $1
		  AND p.received_on >= date_trunc('month', $2::date)::date AND p.received_on < (date_trunc('month', $2::date) + interval '1 month')::date
		  AND NOT EXISTS (SELECT 1 FROM payment_reversals r WHERE r.workspace_id = p.workspace_id AND r.payment_id = p.id)`, workspaceID, today).Scan(&out.PaidThisMonth)
	if err != nil {
		return Summary{}, fmt.Errorf("invoice: paid this month: %w", err)
	}
	return out, nil
}

// LatestShared is the lead's most recently issued invoice with a live link,
// for the WhatsApp {tautan} placeholder.
func (s *Store) LatestShared(ctx context.Context, workspaceID, leadID uuid.UUID) (Invoice, bool, error) {
	inv, err := scan(s.pool.QueryRow(ctx, `SELECT`+columns+from+` WHERE i.workspace_id = $1 AND i.lead_id = $2 AND i.status = 'issued' AND i.share_token IS NOT NULL AND i.share_revoked_at IS NULL
		ORDER BY i.issued_at DESC, i.id DESC LIMIT 1`, workspaceID, leadID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Invoice{}, false, nil
	}
	if err != nil {
		return Invoice{}, false, fmt.Errorf("invoice: latest shared: %w", err)
	}
	return inv, true, nil
}

func clampPage(page, total, perPage int) (int, int) {
	pages := (total + perPage - 1) / perPage
	if pages == 0 {
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

func likePattern(q string) string {
	q = strings.TrimSpace(q)
	if q == "" {
		return ""
	}
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + r.Replace(q) + "%"
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
