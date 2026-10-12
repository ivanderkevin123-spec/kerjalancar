package lead

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"kerjalancar/internal/customer"
	"kerjalancar/internal/money"
)

// PerPage is the fixed list page size; lists are never unbounded.
const PerPage = 25

// MaxActivities caps the timeline on a lead detail page.
const MaxActivities = 100

// MaxQueryLen caps free-text search input.
const MaxQueryLen = 100

// Store reads and writes leads and their activities for one workspace at a time.
type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// CreateInput is the lead form plus either an existing customer (CustomerID)
// or a new one created in the same transaction (NewCustomer).
type CreateInput struct {
	Input
	CustomerID  uuid.UUID
	NewCustomer *customer.Input
}

const leadColumns = `
	l.id, l.workspace_id, l.customer_id, c.name, coalesce(c.phone, ''), l.title,
	coalesce(l.project_type, ''), coalesce(l.description, ''), l.estimated_value, l.status,
	coalesce(l.source, ''), l.next_follow_up_on, coalesce(l.lost_reason, ''), coalesce(l.notes, ''),
	l.status_changed_at, l.created_at, l.updated_at`

const leadFrom = ` FROM leads l JOIN customers c ON c.workspace_id = l.workspace_id AND c.id = l.customer_id`

func scanLead(row pgx.Row) (Lead, error) {
	var l Lead
	var pt, src string
	err := row.Scan(&l.ID, &l.WorkspaceID, &l.CustomerID, &l.CustomerName, &l.CustomerPhone, &l.Title,
		&pt, &l.Description, &l.EstimatedValue, &l.Status,
		&src, &l.NextFollowUpOn, &l.LostReason, &l.Notes,
		&l.StatusChangedAt, &l.CreatedAt, &l.UpdatedAt)
	l.ProjectType, l.Source = ProjectType(pt), Source(src)
	return l, err
}

func getLead(ctx context.Context, q interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}, workspaceID, id uuid.UUID, forUpdate bool) (Lead, error) {
	sql := `SELECT ` + leadColumns + leadFrom + ` WHERE l.workspace_id = $1 AND l.id = $2`
	if forUpdate {
		sql += ` FOR UPDATE OF l`
	}
	l, err := scanLead(q.QueryRow(ctx, sql, workspaceID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Lead{}, ErrNotFound
	}
	if err != nil {
		return Lead{}, fmt.Errorf("lead: get: %w", err)
	}
	return l, nil
}

// Get returns the lead only when it belongs to workspaceID; otherwise ErrNotFound.
func (s *Store) Get(ctx context.Context, workspaceID, id uuid.UUID) (Lead, error) {
	return getLead(ctx, s.pool, workspaceID, id, false)
}

// Create validates the form, resolves or creates the customer, inserts the
// lead with status baru and the "created" activity, all in one transaction.
func (s *Store) Create(ctx context.Context, workspaceID, actorID uuid.UUID, in CreateInput) (Lead, error) {
	n, errs := Validate(in.Input)
	if len(errs) > 0 {
		return Lead{}, &ValidationError{Fields: errs}
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Lead{}, fmt.Errorf("lead: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	customerID := in.CustomerID
	if in.NewCustomer != nil {
		c, err := customer.Create(ctx, tx, workspaceID, *in.NewCustomer)
		if err != nil {
			return Lead{}, err
		}
		customerID = c.ID
	} else {
		var ok bool
		if err := tx.QueryRow(ctx, `SELECT true FROM customers WHERE workspace_id = $1 AND id = $2`, workspaceID, customerID).Scan(&ok); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return Lead{}, ErrCustomerNotFound
			}
			return Lead{}, fmt.Errorf("lead: customer check: %w", err)
		}
	}

	id, err := uuid.NewV7()
	if err != nil {
		return Lead{}, fmt.Errorf("lead: id: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO leads (id, workspace_id, customer_id, title, project_type, description, estimated_value, status, source, next_follow_up_on, notes)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		id, workspaceID, customerID, n.Title, nullable(string(n.ProjectType)), nullable(n.Description), n.EstimatedValue,
		StatusBaru, nullable(string(n.Source)), n.NextFollowUpOn, nullable(n.Notes))
	if err != nil {
		return Lead{}, fmt.Errorf("lead: insert: %w", err)
	}
	if err := appendActivity(ctx, tx, workspaceID, id, actorID, KindCreated, "", ActivityData{}); err != nil {
		return Lead{}, err
	}
	l, err := getLead(ctx, tx, workspaceID, id, false)
	if err != nil {
		return Lead{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Lead{}, fmt.Errorf("lead: commit: %w", err)
	}
	return l, nil
}

// Update replaces the editable fields and records which ones changed. A lead
// outside workspaceID is ErrNotFound; an unchanged form writes no activity.
func (s *Store) Update(ctx context.Context, workspaceID, actorID, id uuid.UUID, in Input) (Lead, error) {
	n, errs := Validate(in)
	if len(errs) > 0 {
		return Lead{}, &ValidationError{Fields: errs}
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Lead{}, fmt.Errorf("lead: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	cur, err := getLead(ctx, tx, workspaceID, id, true)
	if err != nil {
		return Lead{}, err
	}
	changes := diff(cur, n)
	if len(changes) == 0 {
		return cur, nil
	}
	_, err = tx.Exec(ctx, `
		UPDATE leads SET title = $3, project_type = $4, description = $5, estimated_value = $6, source = $7,
		       next_follow_up_on = $8, notes = $9, updated_at = now()
		WHERE workspace_id = $1 AND id = $2`,
		workspaceID, id, n.Title, nullable(string(n.ProjectType)), nullable(n.Description), n.EstimatedValue,
		nullable(string(n.Source)), n.NextFollowUpOn, nullable(n.Notes))
	if err != nil {
		return Lead{}, fmt.Errorf("lead: update: %w", err)
	}
	if err := appendActivity(ctx, tx, workspaceID, id, actorID, KindUpdated, "", ActivityData{Changes: changes}); err != nil {
		return Lead{}, err
	}
	l, err := getLead(ctx, tx, workspaceID, id, false)
	if err != nil {
		return Lead{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Lead{}, fmt.Errorf("lead: commit: %w", err)
	}
	return l, nil
}

// diff lists the fields that differ between the stored lead and the form.
// Short fields carry display values; long text fields only their label.
func diff(cur Lead, n Normalized) []Change {
	var out []Change
	if cur.Title != n.Title {
		out = append(out, Change{Field: "title", From: cur.Title, To: n.Title})
	}
	if cur.ProjectType != n.ProjectType {
		out = append(out, Change{Field: "project_type", From: cur.ProjectType.Label(), To: n.ProjectType.Label()})
	}
	if cur.Description != n.Description {
		out = append(out, Change{Field: "description"})
	}
	if !equalInt64(cur.EstimatedValue, n.EstimatedValue) {
		out = append(out, Change{Field: "estimated_value", From: formatValue(cur.EstimatedValue), To: formatValue(n.EstimatedValue)})
	}
	if cur.Source != n.Source {
		out = append(out, Change{Field: "source", From: cur.Source.Label(), To: n.Source.Label()})
	}
	if !equalDate(cur.NextFollowUpOn, n.NextFollowUpOn) {
		out = append(out, Change{Field: "next_follow_up_on", From: formatDate(cur.NextFollowUpOn), To: formatDate(n.NextFollowUpOn)})
	}
	if cur.Notes != n.Notes {
		out = append(out, Change{Field: "notes"})
	}
	return out
}

// ChangeStatus applies the transition rules (D-035), updates the status and
// the derived fields, and appends a status_changed activity. loc is the
// workspace time zone used to compute "today" for the follow-up default.
func (s *Store) ChangeStatus(ctx context.Context, workspaceID, actorID, id uuid.UUID, to Status, lostReason string, loc *time.Location) (Lead, error) {
	lostReason = strings.TrimSpace(lostReason)
	if msg := ValidateLostReason(lostReason); msg != "" {
		return Lead{}, &ValidationError{Fields: map[string]string{"lost_reason": msg}}
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Lead{}, fmt.Errorf("lead: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	cur, err := getLead(ctx, tx, workspaceID, id, true)
	if err != nil {
		return Lead{}, err
	}
	if err := CanTransition(cur.Status, to); err != nil {
		return Lead{}, err
	}

	followUp := cur.NextFollowUpOn
	switch {
	case !to.Open():
		followUp = nil
	case to == StatusPenawaran && followUp == nil:
		if loc == nil {
			loc = time.UTC
		}
		now := time.Now().In(loc)
		d := time.Date(now.Year(), now.Month(), now.Day()+3, 0, 0, 0, 0, time.UTC)
		followUp = &d
	}
	reason := ""
	if to == StatusKalah {
		reason = lostReason
	}
	_, err = tx.Exec(ctx, `
		UPDATE leads SET status = $3, lost_reason = $4, next_follow_up_on = $5, status_changed_at = now(), updated_at = now()
		WHERE workspace_id = $1 AND id = $2`,
		workspaceID, id, to, nullable(reason), followUp)
	if err != nil {
		return Lead{}, fmt.Errorf("lead: change status: %w", err)
	}
	if err := appendActivity(ctx, tx, workspaceID, id, actorID, KindStatusChanged, "", ActivityData{From: cur.Status, To: to, Reason: reason}); err != nil {
		return Lead{}, err
	}
	l, err := getLead(ctx, tx, workspaceID, id, false)
	if err != nil {
		return Lead{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Lead{}, fmt.Errorf("lead: commit: %w", err)
	}
	return l, nil
}

// AddNote appends a free-text note to the lead's history.
func (s *Store) AddNote(ctx context.Context, workspaceID, actorID, id uuid.UUID, body string) (Activity, error) {
	body = strings.TrimSpace(body)
	switch {
	case body == "":
		return Activity{}, &ValidationError{Fields: map[string]string{"body": "Catatan wajib diisi."}}
	case len([]rune(body)) > MaxNotesLen:
		return Activity{}, &ValidationError{Fields: map[string]string{"body": msgNotesLong}}
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Activity{}, fmt.Errorf("lead: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := getLead(ctx, tx, workspaceID, id, true); err != nil {
		return Activity{}, err
	}
	if err := appendActivity(ctx, tx, workspaceID, id, actorID, KindNote, body, ActivityData{}); err != nil {
		return Activity{}, err
	}
	acts, err := listActivities(ctx, tx, workspaceID, id, 1)
	if err != nil {
		return Activity{}, err
	}
	if len(acts) == 0 {
		return Activity{}, errors.New("lead: note was not recorded")
	}
	if err := tx.Commit(ctx); err != nil {
		return Activity{}, fmt.Errorf("lead: commit: %w", err)
	}
	return acts[0], nil
}

// Activities returns the newest activities of a lead (at most limit, capped
// at MaxActivities). A lead outside workspaceID is ErrNotFound.
func (s *Store) Activities(ctx context.Context, workspaceID, id uuid.UUID, limit int) ([]Activity, error) {
	if _, err := getLead(ctx, s.pool, workspaceID, id, false); err != nil {
		return nil, err
	}
	return listActivities(ctx, s.pool, workspaceID, id, limit)
}

func listActivities(ctx context.Context, q interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}, workspaceID, id uuid.UUID, limit int) ([]Activity, error) {
	if limit < 1 || limit > MaxActivities {
		limit = MaxActivities
	}
	rows, err := q.Query(ctx, `
		SELECT a.id, a.workspace_id, a.lead_id, a.kind, a.actor_user_id, coalesce(u.name, ''), coalesce(a.body, ''), a.data, a.created_at
		FROM lead_activities a LEFT JOIN users u ON u.id = a.actor_user_id
		WHERE a.workspace_id = $1 AND a.lead_id = $2
		ORDER BY a.created_at DESC, a.id DESC
		LIMIT $3`, workspaceID, id, limit)
	if err != nil {
		return nil, fmt.Errorf("lead: activities: %w", err)
	}
	defer rows.Close()
	var out []Activity
	for rows.Next() {
		var a Activity
		if err := rows.Scan(&a.ID, &a.WorkspaceID, &a.LeadID, &a.Kind, &a.ActorID, &a.ActorName, &a.Body, &a.Data, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("lead: scan activity: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func appendActivity(ctx context.Context, tx pgx.Tx, workspaceID, leadID, actorID uuid.UUID, kind Kind, body string, data ActivityData) error {
	id, err := uuid.NewV7()
	if err != nil {
		return fmt.Errorf("lead: activity id: %w", err)
	}
	var actor *uuid.UUID
	if actorID != uuid.Nil {
		actor = &actorID
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO lead_activities (id, workspace_id, lead_id, kind, actor_user_id, body, data)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`, id, workspaceID, leadID, kind, actor, nullable(body), data)
	if err != nil {
		return fmt.Errorf("lead: append activity: %w", err)
	}
	return nil
}

// ListQuery selects a page of the pipeline. Status is "aktif" (default: open
// leads), "semua", or one status value. Sort is "terbaru" (default), "nilai"
// or "followup". Q matches the lead title and the customer name.
type ListQuery struct {
	Status string
	Q      string
	Sort   string
	Page   int
}

// Page is one page of leads plus navigation data.
type Page struct {
	Items   []Lead
	Total   int
	Page    int
	Pages   int
	PerPage int
}

// List returns a page of leads for the workspace.
func (s *Store) List(ctx context.Context, workspaceID uuid.UUID, q ListQuery) (Page, error) {
	statuses := statusFilter(q.Status)
	pattern := likePattern(q.Q)
	where := leadFrom + `
		WHERE l.workspace_id = $1
		  AND l.status = ANY($2)
		  AND ($3 = '' OR l.title ILIKE $3 ESCAPE '\' OR c.name ILIKE $3 ESCAPE '\')`
	var total int
	if err := s.pool.QueryRow(ctx, `SELECT count(*)`+where, workspaceID, statuses, pattern).Scan(&total); err != nil {
		return Page{}, fmt.Errorf("lead: count: %w", err)
	}
	page, pages := clampPage(q.Page, total, PerPage)

	order := ` ORDER BY l.created_at DESC, l.id DESC`
	switch q.Sort {
	case "nilai":
		order = ` ORDER BY l.estimated_value DESC NULLS LAST, l.created_at DESC, l.id DESC`
	case "followup":
		order = ` ORDER BY l.next_follow_up_on ASC NULLS LAST, l.created_at DESC, l.id DESC`
	}
	rows, err := s.pool.Query(ctx, `SELECT `+leadColumns+where+order+` LIMIT $4 OFFSET $5`,
		workspaceID, statuses, pattern, PerPage, (page-1)*PerPage)
	if err != nil {
		return Page{}, fmt.Errorf("lead: list: %w", err)
	}
	defer rows.Close()
	out := Page{Total: total, Page: page, Pages: pages, PerPage: PerPage}
	for rows.Next() {
		l, err := scanLead(rows)
		if err != nil {
			return Page{}, fmt.Errorf("lead: scan: %w", err)
		}
		out.Items = append(out.Items, l)
	}
	return out, rows.Err()
}

// ForCustomer lists a customer's leads, newest first (at most MaxActivities rows).
func (s *Store) ForCustomer(ctx context.Context, workspaceID, customerID uuid.UUID) ([]Lead, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+leadColumns+leadFrom+`
		WHERE l.workspace_id = $1 AND l.customer_id = $2
		ORDER BY l.created_at DESC, l.id DESC LIMIT $3`, workspaceID, customerID, MaxActivities)
	if err != nil {
		return nil, fmt.Errorf("lead: for customer: %w", err)
	}
	defer rows.Close()
	var out []Lead
	for rows.Next() {
		l, err := scanLead(rows)
		if err != nil {
			return nil, fmt.Errorf("lead: scan: %w", err)
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// Summary counts leads per status and sums the estimated value of open leads.
type Summary struct {
	ByStatus  map[Status]int
	OpenCount int
	OpenValue int64 // rupiah; estimates only, never revenue
}

func (s *Store) Summary(ctx context.Context, workspaceID uuid.UUID) (Summary, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT status, count(*), coalesce(sum(estimated_value), 0)
		FROM leads WHERE workspace_id = $1 GROUP BY status`, workspaceID)
	if err != nil {
		return Summary{}, fmt.Errorf("lead: summary: %w", err)
	}
	defer rows.Close()
	sum := Summary{ByStatus: map[Status]int{}}
	for rows.Next() {
		var st Status
		var n int
		var value int64
		if err := rows.Scan(&st, &n, &value); err != nil {
			return Summary{}, fmt.Errorf("lead: scan summary: %w", err)
		}
		sum.ByStatus[st] = n
		if st.Open() {
			sum.OpenCount += n
			sum.OpenValue += value
		}
	}
	return sum, rows.Err()
}

func statusFilter(v string) []Status {
	switch v {
	case "semua":
		return Statuses
	default:
		if st, ok := ParseStatus(v); ok {
			return []Status{st}
		}
		return []Status{StatusBaru, StatusDihubungi, StatusSurvei, StatusPenawaran}
	}
}

func likePattern(q string) string {
	q = strings.TrimSpace(q)
	if q == "" {
		return ""
	}
	if r := []rune(q); len(r) > MaxQueryLen {
		q = string(r[:MaxQueryLen])
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

func equalInt64(a, b *int64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func equalDate(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Year() == b.Year() && a.YearDay() == b.YearDay()
}

func formatValue(v *int64) string {
	if v == nil {
		return ""
	}
	return money.FormatIDR(*v)
}

func formatDate(d *time.Time) string {
	if d == nil {
		return ""
	}
	return d.Format("02-01-2006")
}
