package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"kerjalancar/internal/workspace"
)

var (
	ErrEmailTaken         = errors.New("auth: e-mail already registered")
	ErrInvalidCredentials = errors.New("auth: invalid credentials")
	ErrUserNotFound       = errors.New("auth: user not found")
)

// User is the authenticated identity. Never carries the password hash.
type User struct {
	ID    uuid.UUID
	Email string
	Name  string
}

// RegisterInput is already validated for shape by the handler; the service
// normalises and enforces database constraints.
type RegisterInput struct {
	Email         string
	Password      string
	Name          string
	WorkspaceName string
}

// Service registers users and checks credentials.
type Service struct {
	pool      *pgxpool.Pool
	params    Params
	dummyHash string // verified when the e-mail is unknown, so timing does not reveal accounts
}

// NewService builds a Service hashing with params (DefaultParams in production).
func NewService(pool *pgxpool.Pool, params Params) *Service {
	dummy, err := HashPasswordWith(params, "kerjalancar-dummy-"+uuid.NewString())
	if err != nil {
		panic("auth: invalid argon2 params: " + err.Error())
	}
	return &Service{pool: pool, params: params, dummyHash: dummy}
}

// NormalizeEmail trims and lower-cases an address the way the database compares it.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// Register creates the user, their first workspace, and the owner membership
// in one transaction. Nothing is left behind when any step fails.
func (s *Service) Register(ctx context.Context, in RegisterInput) (User, uuid.UUID, error) {
	email := NormalizeEmail(in.Email)
	name := strings.TrimSpace(in.Name)

	hash, err := HashPasswordWith(s.params, in.Password)
	if err != nil {
		return User{}, uuid.Nil, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return User{}, uuid.Nil, fmt.Errorf("auth: id: %w", err)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return User{}, uuid.Nil, fmt.Errorf("auth: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	_, err = tx.Exec(ctx, `INSERT INTO users (id, email, password_hash, name) VALUES ($1, $2, $3, $4)`, id, email, hash, name)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
			return User{}, uuid.Nil, ErrEmailTaken
		}
		return User{}, uuid.Nil, fmt.Errorf("auth: insert user: %w", err)
	}

	ws, err := workspace.CreateWithOwner(ctx, tx, in.WorkspaceName, id)
	if err != nil {
		return User{}, uuid.Nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return User{}, uuid.Nil, fmt.Errorf("auth: commit: %w", err)
	}
	return User{ID: id, Email: email, Name: name}, ws.ID, nil
}

// Authenticate returns the user when e-mail and password match. Unknown
// e-mail and wrong password produce the same error and similar timing.
func (s *Service) Authenticate(ctx context.Context, email, password string) (User, error) {
	email = NormalizeEmail(email)
	var u User
	var hash string
	err := s.pool.QueryRow(ctx,
		`SELECT id, email::text, name, password_hash FROM users WHERE email = $1`, email,
	).Scan(&u.ID, &u.Email, &u.Name, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		_, _ = VerifyPassword(password, s.dummyHash)
		return User{}, ErrInvalidCredentials
	}
	if err != nil {
		return User{}, fmt.Errorf("auth: lookup: %w", err)
	}
	ok, err := VerifyPassword(password, hash)
	if err != nil {
		return User{}, fmt.Errorf("auth: verify: %w", err)
	}
	if !ok {
		return User{}, ErrInvalidCredentials
	}
	return u, nil
}

// UserByID loads a user for an authenticated session.
func (s *Service) UserByID(ctx context.Context, id uuid.UUID) (User, error) {
	var u User
	err := s.pool.QueryRow(ctx, `SELECT id, email::text, name FROM users WHERE id = $1`, id).Scan(&u.ID, &u.Email, &u.Name)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrUserNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("auth: load user: %w", err)
	}
	return u, nil
}
