package customer

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PerPage is the fixed list page size; lists are never unbounded.
const PerPage = 25

// MaxQueryLen caps free-text search input.
const MaxQueryLen = 100

// Querier is satisfied by *pgxpool.Pool and pgx.Tx, so a customer can be
// created inside another package's transaction (the lead form does this).
type Querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Store reads and writes customers of one workspace at a time.
type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

const columns = `id, workspace_id, name, coalesce(phone, ''), coalesce(email::text, ''), coalesce(company, ''), coalesce(notes, ''), created_at, updated_at`

func scan(row pgx.Row) (Customer, error) {
	var c Customer
	err := row.Scan(&c.ID, &c.WorkspaceID, &c.Name, &c.Phone, &c.Email, &c.Company, &c.Notes, &c.CreatedAt, &c.UpdatedAt)
	return c, err
}

// Create validates, then inserts a customer into workspaceID using the pool.
func (s *Store) Create(ctx context.Context, workspaceID uuid.UUID, in Input) (Customer, error) {
	return Create(ctx, s.pool, workspaceID, in)
}

// Create inserts a customer using q, which may be a transaction owned by the caller.
func Create(ctx context.Context, q Querier, workspaceID uuid.UUID, in Input) (Customer, error) {
	in, errs := Validate(in)
	if len(errs) > 0 {
		return Customer{}, &ValidationError{Fields: errs}
	}
	if err := phoneFree(ctx, q, workspaceID, in.Phone, uuid.Nil); err != nil {
		return Customer{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Customer{}, fmt.Errorf("customer: id: %w", err)
	}
	c, err := scan(q.QueryRow(ctx, `
		INSERT INTO customers (id, workspace_id, name, phone, email, company, notes)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING `+columns,
		id, workspaceID, in.Name, nullable(in.Phone), nullable(in.Email), nullable(in.Company), nullable(in.Notes)))
	if err != nil {
		return Customer{}, duplicateOr(fmt.Errorf("customer: insert: %w", err), err)
	}
	return c, nil
}

// Get returns the customer only when it belongs to workspaceID; otherwise ErrNotFound.
func (s *Store) Get(ctx context.Context, workspaceID, id uuid.UUID) (Customer, error) {
	c, err := scan(s.pool.QueryRow(ctx, `SELECT `+columns+` FROM customers WHERE workspace_id = $1 AND id = $2`, workspaceID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Customer{}, ErrNotFound
	}
	if err != nil {
		return Customer{}, fmt.Errorf("customer: get: %w", err)
	}
	return c, nil
}

// Update replaces every editable field. A customer outside workspaceID is
// reported as ErrNotFound and left untouched.
func (s *Store) Update(ctx context.Context, workspaceID, id uuid.UUID, in Input) (Customer, error) {
	in, errs := Validate(in)
	if len(errs) > 0 {
		return Customer{}, &ValidationError{Fields: errs}
	}
	if err := phoneFree(ctx, s.pool, workspaceID, in.Phone, id); err != nil {
		return Customer{}, err
	}
	c, err := scan(s.pool.QueryRow(ctx, `
		UPDATE customers SET name = $3, phone = $4, email = $5, company = $6, notes = $7, updated_at = now()
		WHERE workspace_id = $1 AND id = $2
		RETURNING `+columns,
		workspaceID, id, in.Name, nullable(in.Phone), nullable(in.Email), nullable(in.Company), nullable(in.Notes)))
	if errors.Is(err, pgx.ErrNoRows) {
		return Customer{}, ErrNotFound
	}
	if err != nil {
		return Customer{}, duplicateOr(fmt.Errorf("customer: update: %w", err), err)
	}
	return c, nil
}

// Query selects a page of a workspace's customers. Q matches name and company
// (case-insensitive) and phone digits in any format the user types.
type Query struct {
	Q    string
	Page int
}

// Page is one page of results plus what the template needs for navigation.
type Page struct {
	Items   []Customer
	Total   int
	Page    int
	Pages   int
	PerPage int
}

// List returns newest customers first. Pages outside the range clamp to the
// nearest valid page, so a stale link never errors.
func (s *Store) List(ctx context.Context, workspaceID uuid.UUID, q Query) (Page, error) {
	text, phone := searchPatterns(q.Q)
	where := ` FROM customers
		WHERE workspace_id = $1
		  AND ($2 = '' OR name ILIKE $2 ESCAPE '\' OR company ILIKE $2 ESCAPE '\' OR ($3 <> '' AND phone LIKE $3))`

	var total int
	if err := s.pool.QueryRow(ctx, `SELECT count(*)`+where, workspaceID, text, phone).Scan(&total); err != nil {
		return Page{}, fmt.Errorf("customer: count: %w", err)
	}
	page, pages := clampPage(q.Page, total, PerPage)

	rows, err := s.pool.Query(ctx, `SELECT `+columns+where+` ORDER BY created_at DESC, id DESC LIMIT $4 OFFSET $5`,
		workspaceID, text, phone, PerPage, (page-1)*PerPage)
	if err != nil {
		return Page{}, fmt.Errorf("customer: list: %w", err)
	}
	defer rows.Close()
	out := Page{Total: total, Page: page, Pages: pages, PerPage: PerPage}
	for rows.Next() {
		c, err := scan(rows)
		if err != nil {
			return Page{}, fmt.Errorf("customer: scan: %w", err)
		}
		out.Items = append(out.Items, c)
	}
	return out, rows.Err()
}

// clampPage returns the page to show (1-based) and the page count for total rows.
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

// searchPatterns turns user input into a LIKE pattern for text columns and
// one for phone digits. Wildcards in the input are escaped so "%" is literal.
func searchPatterns(q string) (text, phone string) {
	q = strings.TrimSpace(q)
	if q == "" {
		return "", ""
	}
	if r := []rune(q); len(r) > MaxQueryLen {
		q = string(r[:MaxQueryLen])
	}
	esc := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(q)
	text = "%" + esc + "%"

	var digits strings.Builder
	for _, c := range q {
		if c >= '0' && c <= '9' {
			digits.WriteRune(c)
		}
	}
	d := digits.String()
	if strings.HasPrefix(d, "0") {
		d = "62" + d[1:]
	}
	if len(d) >= 3 {
		phone = "%" + d + "%"
	}
	return text, phone
}

// phoneFree returns DuplicatePhoneError when another customer of the workspace
// (not exceptID) already has the number. Checked before writing so the error
// can name the owner even inside a caller's transaction.
func phoneFree(ctx context.Context, q Querier, workspaceID uuid.UUID, phone string, exceptID uuid.UUID) error {
	if phone == "" {
		return nil
	}
	var dup DuplicatePhoneError
	err := q.QueryRow(ctx, `SELECT id, name FROM customers WHERE workspace_id = $1 AND phone = $2 AND id <> $3`,
		workspaceID, phone, exceptID).Scan(&dup.ID, &dup.Name)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("customer: phone check: %w", err)
	}
	return &dup
}

// duplicateOr maps a unique violation that slipped past phoneFree (a
// concurrent insert) to a DuplicatePhoneError without a name; anything else
// is returned wrapped.
func duplicateOr(wrapped, raw error) error {
	var pgErr *pgconn.PgError
	if errors.As(raw, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "customers_workspace_phone_idx" {
		return &DuplicatePhoneError{}
	}
	return wrapped
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// MaxAll caps the customer list used by select boxes.
const MaxAll = 500

// All returns the workspace's customers ordered by name (case-insensitive),
// at most MaxAll rows, for choosing a customer on the lead form.
func (s *Store) All(ctx context.Context, workspaceID uuid.UUID) ([]Customer, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+columns+` FROM customers WHERE workspace_id = $1 ORDER BY lower(name), created_at LIMIT $2`, workspaceID, MaxAll)
	if err != nil {
		return nil, fmt.Errorf("customer: all: %w", err)
	}
	defer rows.Close()
	var out []Customer
	for rows.Next() {
		c, err := scan(rows)
		if err != nil {
			return nil, fmt.Errorf("customer: scan: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
