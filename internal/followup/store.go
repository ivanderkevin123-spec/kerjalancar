package followup

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"kerjalancar/internal/lead"
)

var (
	ErrNotFound     = errors.New("followup: not found")
	ErrLeadNotFound = errors.New("followup: lead not found in workspace")
	ErrLeadClosed   = errors.New("followup: lead is closed")
	ErrAlreadyDone  = errors.New("followup: already done")
	ErrNotDone      = errors.New("followup: not done")
	ErrAnotherOpen  = errors.New("followup: the lead already has an open follow-up")
)

// Lock order: the lead row is the root lock for everything that touches a
// lead's follow-ups (lead → follow-up), the same order the lead store uses
// when it changes status, so two devices cannot deadlock each other.

// lockLead locks the lead row and returns its status; ErrLeadNotFound when it
// is not in the workspace.
func lockLead(ctx context.Context, tx pgx.Tx, workspaceID, leadID uuid.UUID) (lead.Status, error) {
	var st lead.Status
	err := tx.QueryRow(ctx, `SELECT status FROM leads WHERE workspace_id = $1 AND id = $2 FOR UPDATE`, workspaceID, leadID).Scan(&st)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrLeadNotFound
	}
	if err != nil {
		return "", fmt.Errorf("followup: lock lead: %w", err)
	}
	return st, nil
}

// isUniqueViolation reports a race on the one-open-per-lead index.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// PerPage is the page size of the "Selesai" list.
const PerPage = 25

// MaxPerBucket caps each open list.
const MaxPerBucket = 100

// FollowUp is one reminder on a lead, with the lead and customer joined for lists.
type FollowUp struct {
	ID            uuid.UUID
	WorkspaceID   uuid.UUID
	LeadID        uuid.UUID
	LeadTitle     string
	LeadStatus    lead.Status
	CustomerID    uuid.UUID
	CustomerName  string
	CustomerPhone string // current number, read at request time (never stored here)
	DueOn         time.Time
	Note          string
	DoneAt        *time.Time
	DoneBy        *uuid.UUID
	CreatedBy     *uuid.UUID
	CreatedAt     time.Time
	UpdatedAt     time.Time
	Bucket        Bucket // filled by Lists
}

// Open reports whether the follow-up is still pending.
func (f FollowUp) Open() bool { return f.DoneAt == nil }

// Store reads and writes follow-ups for one workspace at a time.
type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

const columns = `
	f.id, f.workspace_id, f.lead_id, l.title, l.status, l.customer_id, c.name, coalesce(c.phone, ''),
	f.due_on, coalesce(f.note, ''), f.done_at, f.done_by, f.created_by, f.created_at, f.updated_at`

const from = ` FROM follow_ups f
	JOIN leads l ON l.workspace_id = f.workspace_id AND l.id = f.lead_id
	JOIN customers c ON c.workspace_id = l.workspace_id AND c.id = l.customer_id`

type rowScanner interface{ Scan(dest ...any) error }

func scan(row rowScanner) (FollowUp, error) {
	var f FollowUp
	err := row.Scan(&f.ID, &f.WorkspaceID, &f.LeadID, &f.LeadTitle, &f.LeadStatus, &f.CustomerID, &f.CustomerName, &f.CustomerPhone,
		&f.DueOn, &f.Note, &f.DoneAt, &f.DoneBy, &f.CreatedBy, &f.CreatedAt, &f.UpdatedAt)
	return f, err
}

type querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func get(ctx context.Context, q querier, workspaceID, id uuid.UUID, forUpdate bool) (FollowUp, error) {
	sql := `SELECT` + columns + from + ` WHERE f.workspace_id = $1 AND f.id = $2`
	if forUpdate {
		sql += ` FOR UPDATE OF f`
	}
	f, err := scan(q.QueryRow(ctx, sql, workspaceID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return FollowUp{}, ErrNotFound
	}
	if err != nil {
		return FollowUp{}, fmt.Errorf("followup: get: %w", err)
	}
	return f, nil
}

// Get returns the follow-up only when it belongs to workspaceID.
func (s *Store) Get(ctx context.Context, workspaceID, id uuid.UUID) (FollowUp, error) {
	return get(ctx, s.pool, workspaceID, id, false)
}

// OpenForLead returns the lead's open follow-up, if any.
func (s *Store) OpenForLead(ctx context.Context, workspaceID, leadID uuid.UUID) (FollowUp, bool, error) {
	f, err := scan(s.pool.QueryRow(ctx, `SELECT`+columns+from+` WHERE f.workspace_id = $1 AND f.lead_id = $2 AND f.done_at IS NULL`, workspaceID, leadID))
	if errors.Is(err, pgx.ErrNoRows) {
		return FollowUp{}, false, nil
	}
	if err != nil {
		return FollowUp{}, false, fmt.Errorf("followup: open for lead: %w", err)
	}
	return f, true, nil
}

// Schedule creates the lead's open follow-up or moves it to a new date and
// note. The lead must belong to the workspace.
func (s *Store) Schedule(ctx context.Context, workspaceID, actorID, leadID uuid.UUID, n Normalized) (FollowUp, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return FollowUp{}, fmt.Errorf("followup: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	st, err := lockLead(ctx, tx, workspaceID, leadID)
	if err != nil {
		return FollowUp{}, err
	}
	if !st.Open() {
		return FollowUp{}, ErrLeadClosed
	}
	f, err := ScheduleTx(ctx, tx, workspaceID, actorID, leadID, n)
	if err != nil {
		return FollowUp{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return FollowUp{}, fmt.Errorf("followup: commit: %w", err)
	}
	return f, nil
}

// ScheduleTx is Schedule inside the caller's transaction (the lead store uses
// it when a lead reaches penawaran). It refreshes leads.next_follow_up_on and
// appends follow_up_scheduled or follow_up_rescheduled.
func ScheduleTx(ctx context.Context, tx pgx.Tx, workspaceID, actorID, leadID uuid.UUID, n Normalized) (FollowUp, error) {
	var id uuid.UUID
	var prev time.Time
	err := tx.QueryRow(ctx, `SELECT id, due_on FROM follow_ups WHERE workspace_id = $1 AND lead_id = $2 AND done_at IS NULL FOR UPDATE`,
		workspaceID, leadID).Scan(&id, &prev)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		id, err = uuid.NewV7()
		if err != nil {
			return FollowUp{}, fmt.Errorf("followup: id: %w", err)
		}
		_, err = tx.Exec(ctx, `INSERT INTO follow_ups (id, workspace_id, lead_id, due_on, note, created_by) VALUES ($1, $2, $3, $4, $5, $6)`,
			id, workspaceID, leadID, n.DueOn, nullable(n.Note), nullableID(actorID))
		if isUniqueViolation(err) {
			return FollowUp{}, ErrAnotherOpen
		}
		if err != nil {
			return FollowUp{}, fmt.Errorf("followup: insert: %w", err)
		}
		if err := lead.AppendActivity(ctx, tx, workspaceID, leadID, actorID, lead.KindFollowUpScheduled, "", lead.ActivityData{DueOn: n.DueOn.Format(DateLayout)}); err != nil {
			return FollowUp{}, err
		}
	case err != nil:
		return FollowUp{}, fmt.Errorf("followup: lookup: %w", err)
	default:
		_, err = tx.Exec(ctx, `UPDATE follow_ups SET due_on = $3, note = $4, updated_at = now() WHERE workspace_id = $1 AND id = $2`,
			workspaceID, id, n.DueOn, nullable(n.Note))
		if err != nil {
			return FollowUp{}, fmt.Errorf("followup: reschedule: %w", err)
		}
		if err := lead.AppendActivity(ctx, tx, workspaceID, leadID, actorID, lead.KindFollowUpRescheduled, "",
			lead.ActivityData{PrevDueOn: prev.Format(DateLayout), DueOn: n.DueOn.Format(DateLayout)}); err != nil {
			return FollowUp{}, err
		}
	}
	if err := setCache(ctx, tx, workspaceID, leadID, &n.DueOn); err != nil {
		return FollowUp{}, err
	}
	return get(ctx, tx, workspaceID, id, false)
}

// ScheduleForLead adapts ScheduleTx to the lead store's hook signature.
func ScheduleForLead(ctx context.Context, tx pgx.Tx, workspaceID, actorID, leadID uuid.UUID, dueOn time.Time) error {
	_, err := ScheduleTx(ctx, tx, workspaceID, actorID, leadID, Normalized{DueOn: dueOn})
	return err
}

// CloseForLead finishes the lead's open follow-up when the lead is closed
// (hook of the lead store). No-op when there is none.
func CloseForLead(ctx context.Context, tx pgx.Tx, workspaceID, actorID, leadID uuid.UUID) error {
	tag, err := tx.Exec(ctx, `UPDATE follow_ups SET done_at = now(), done_by = $3, updated_at = now()
		WHERE workspace_id = $1 AND lead_id = $2 AND done_at IS NULL`, workspaceID, leadID, nullableID(actorID))
	if err != nil {
		return fmt.Errorf("followup: close for lead: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil
	}
	if err := setCache(ctx, tx, workspaceID, leadID, nil); err != nil {
		return err
	}
	return lead.AppendActivity(ctx, tx, workspaceID, leadID, actorID, lead.KindFollowUpDone, "", lead.ActivityData{Auto: true})
}

// lockFollowUp locks the follow-up's lead row first, then the follow-up row
// (lock order above), and returns the follow-up with its lead joined.
func lockFollowUp(ctx context.Context, tx pgx.Tx, workspaceID, id uuid.UUID) (FollowUp, error) {
	var leadID uuid.UUID
	err := tx.QueryRow(ctx, `SELECT lead_id FROM follow_ups WHERE workspace_id = $1 AND id = $2`, workspaceID, id).Scan(&leadID)
	if errors.Is(err, pgx.ErrNoRows) {
		return FollowUp{}, ErrNotFound
	}
	if err != nil {
		return FollowUp{}, fmt.Errorf("followup: lookup: %w", err)
	}
	if _, err := lockLead(ctx, tx, workspaceID, leadID); err != nil {
		return FollowUp{}, err
	}
	return get(ctx, tx, workspaceID, id, true)
}

// Done marks a follow-up finished. Already done is ErrAlreadyDone (nothing written).
func (s *Store) Done(ctx context.Context, workspaceID, actorID, id uuid.UUID) (FollowUp, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return FollowUp{}, fmt.Errorf("followup: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	cur, err := lockFollowUp(ctx, tx, workspaceID, id)
	if err != nil {
		return FollowUp{}, err
	}
	if !cur.Open() {
		return FollowUp{}, ErrAlreadyDone
	}
	if _, err := tx.Exec(ctx, `UPDATE follow_ups SET done_at = now(), done_by = $3, updated_at = now() WHERE workspace_id = $1 AND id = $2`,
		workspaceID, id, nullableID(actorID)); err != nil {
		return FollowUp{}, fmt.Errorf("followup: done: %w", err)
	}
	if err := setCache(ctx, tx, workspaceID, cur.LeadID, nil); err != nil {
		return FollowUp{}, err
	}
	if err := lead.AppendActivity(ctx, tx, workspaceID, cur.LeadID, actorID, lead.KindFollowUpDone, "", lead.ActivityData{DueOn: cur.DueOn.Format(DateLayout)}); err != nil {
		return FollowUp{}, err
	}
	f, err := get(ctx, tx, workspaceID, id, false)
	if err != nil {
		return FollowUp{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return FollowUp{}, fmt.Errorf("followup: commit: %w", err)
	}
	return f, nil
}

// Undo reopens a finished follow-up. Open is ErrNotDone; a lead that already
// has another open follow-up is ErrAnotherOpen (nothing written either way).
func (s *Store) Undo(ctx context.Context, workspaceID, actorID, id uuid.UUID) (FollowUp, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return FollowUp{}, fmt.Errorf("followup: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	cur, err := lockFollowUp(ctx, tx, workspaceID, id)
	if err != nil {
		return FollowUp{}, err
	}
	if cur.Open() {
		return FollowUp{}, ErrNotDone
	}
	if !cur.LeadStatus.Open() {
		return FollowUp{}, ErrLeadClosed
	}
	var other bool
	err = tx.QueryRow(ctx, `SELECT true FROM follow_ups WHERE workspace_id = $1 AND lead_id = $2 AND done_at IS NULL`, workspaceID, cur.LeadID).Scan(&other)
	if err == nil {
		return FollowUp{}, ErrAnotherOpen
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return FollowUp{}, fmt.Errorf("followup: open check: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE follow_ups SET done_at = NULL, done_by = NULL, updated_at = now() WHERE workspace_id = $1 AND id = $2`, workspaceID, id); err != nil {
		if isUniqueViolation(err) {
			return FollowUp{}, ErrAnotherOpen
		}
		return FollowUp{}, fmt.Errorf("followup: undo: %w", err)
	}
	if err := setCache(ctx, tx, workspaceID, cur.LeadID, &cur.DueOn); err != nil {
		return FollowUp{}, err
	}
	if err := lead.AppendActivity(ctx, tx, workspaceID, cur.LeadID, actorID, lead.KindFollowUpUndone, "", lead.ActivityData{DueOn: cur.DueOn.Format(DateLayout)}); err != nil {
		return FollowUp{}, err
	}
	f, err := get(ctx, tx, workspaceID, id, false)
	if err != nil {
		return FollowUp{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return FollowUp{}, fmt.Errorf("followup: commit: %w", err)
	}
	return f, nil
}

// Counts of open follow-ups per bucket for the workspace's today.
type Counts struct {
	Overdue  int
	Today    int
	Upcoming int // next seven days
}

// Lists are the open follow-ups split by bucket (each capped at MaxPerBucket)
// plus exact counts; the dashboard and the list page share this one query.
type Lists struct {
	Overdue  []FollowUp
	Today    []FollowUp
	Upcoming []FollowUp
	Later    []FollowUp
	Counts   Counts
}

// Lists bucket every open follow-up against today (a date from Today()).
func (s *Store) Lists(ctx context.Context, workspaceID uuid.UUID, today time.Time) (Lists, error) {
	var out Lists
	err := s.pool.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE due_on < $2),
		       count(*) FILTER (WHERE due_on = $2),
		       count(*) FILTER (WHERE due_on > $2 AND due_on <= $3)
		FROM follow_ups WHERE workspace_id = $1 AND done_at IS NULL`,
		workspaceID, today, today.AddDate(0, 0, 7)).Scan(&out.Counts.Overdue, &out.Counts.Today, &out.Counts.Upcoming)
	if err != nil {
		return Lists{}, fmt.Errorf("followup: counts: %w", err)
	}
	// One query per bucket, each capped on its own, so a flood of overdue
	// rows can never hide today's (D-044).
	week := today.AddDate(0, 0, 7)
	buckets := []struct {
		dst  *[]FollowUp
		cond string
		args []any
	}{
		{&out.Overdue, `f.due_on < $2`, []any{workspaceID, today}},
		{&out.Today, `f.due_on = $2`, []any{workspaceID, today}},
		{&out.Upcoming, `f.due_on > $2 AND f.due_on <= $3`, []any{workspaceID, today, week}},
		{&out.Later, `f.due_on > $2`, []any{workspaceID, week}},
	}
	for _, b := range buckets {
		list, err := s.openList(ctx, b.cond, b.args, today)
		if err != nil {
			return Lists{}, err
		}
		*b.dst = list
	}
	return out, nil
}

// openList reads open follow-ups matching cond (which may use $2, $3), oldest due first.
func (s *Store) openList(ctx context.Context, cond string, args []any, today time.Time) ([]FollowUp, error) {
	rows, err := s.pool.Query(ctx, `SELECT`+columns+from+`
		WHERE f.workspace_id = $1 AND f.done_at IS NULL AND `+cond+`
		ORDER BY f.due_on, f.created_at LIMIT `+fmt.Sprint(MaxPerBucket), args...)
	if err != nil {
		return nil, fmt.Errorf("followup: list: %w", err)
	}
	defer rows.Close()
	var out []FollowUp
	for rows.Next() {
		f, err := scan(rows)
		if err != nil {
			return nil, fmt.Errorf("followup: scan: %w", err)
		}
		f.Bucket = BucketOf(f.DueOn, today)
		out = append(out, f)
	}
	return out, rows.Err()
}

// Page is one page of finished follow-ups.
type Page struct {
	Items   []FollowUp
	Total   int
	Page    int
	Pages   int
	PerPage int
}

// DonePage lists finished follow-ups, newest first.
func (s *Store) DonePage(ctx context.Context, workspaceID uuid.UUID, page int) (Page, error) {
	var total int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM follow_ups WHERE workspace_id = $1 AND done_at IS NOT NULL`, workspaceID).Scan(&total); err != nil {
		return Page{}, fmt.Errorf("followup: count done: %w", err)
	}
	pages := (total + PerPage - 1) / PerPage
	if pages < 1 {
		pages = 1
	}
	if page < 1 {
		page = 1
	}
	if page > pages {
		page = pages
	}
	rows, err := s.pool.Query(ctx, `SELECT`+columns+from+`
		WHERE f.workspace_id = $1 AND f.done_at IS NOT NULL
		ORDER BY f.done_at DESC, f.id DESC LIMIT $2 OFFSET $3`, workspaceID, PerPage, (page-1)*PerPage)
	if err != nil {
		return Page{}, fmt.Errorf("followup: done page: %w", err)
	}
	defer rows.Close()
	out := Page{Total: total, Page: page, Pages: pages, PerPage: PerPage}
	for rows.Next() {
		f, err := scan(rows)
		if err != nil {
			return Page{}, fmt.Errorf("followup: scan: %w", err)
		}
		out.Items = append(out.Items, f)
	}
	return out, rows.Err()
}

// setCache keeps leads.next_follow_up_on equal to the open follow-up's date (D-043).
func setCache(ctx context.Context, tx pgx.Tx, workspaceID, leadID uuid.UUID, due *time.Time) error {
	if _, err := tx.Exec(ctx, `UPDATE leads SET next_follow_up_on = $3, updated_at = now() WHERE workspace_id = $1 AND id = $2`, workspaceID, leadID, due); err != nil {
		return fmt.Errorf("followup: cache: %w", err)
	}
	return nil
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
