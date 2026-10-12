package web

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"

	"kerjalancar/internal/auth"
	"kerjalancar/internal/workspace"
)

type ctxKey int

const (
	ctxUser ctxKey = iota
	ctxWorkspace
)

// requireLogin resolves the session's user and their workspace from the
// database on every request. Anything else redirects to the login page.
func (s *server) requireLogin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		id, err := uuid.Parse(s.Sessions.GetString(ctx, sessionUserKey))
		if err != nil {
			redirectToLogin(w, r)
			return
		}
		user, err := s.Auth.UserByID(ctx, id)
		if errors.Is(err, auth.ErrUserNotFound) {
			_ = s.Sessions.Destroy(ctx)
			redirectToLogin(w, r)
			return
		}
		if err != nil {
			s.serverError(w, r, err)
			return
		}
		workspaces, err := s.Workspaces.ForUser(ctx, user.ID)
		if err != nil {
			s.serverError(w, r, err)
			return
		}
		if len(workspaces) == 0 {
			s.serverError(w, r, errors.New("user has no workspace"))
			return
		}
		ctx = context.WithValue(ctx, ctxUser, user)
		ctx = context.WithValue(ctx, ctxWorkspace, workspaces[0])
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func currentUser(ctx context.Context) auth.User {
	u, _ := ctx.Value(ctxUser).(auth.User)
	return u
}

func currentWorkspace(ctx context.Context) workspace.Workspace {
	w, _ := ctx.Value(ctxWorkspace).(workspace.Workspace)
	return w
}

func redirectToLogin(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/masuk", http.StatusSeeOther)
}

func (s *server) loggedIn(r *http.Request) bool {
	return s.Sessions.GetString(r.Context(), sessionUserKey) != ""
}

// startSession rotates the session token (defeats fixation) and binds the user.
func (s *server) startSession(r *http.Request, userID uuid.UUID) error {
	if err := s.Sessions.RenewToken(r.Context()); err != nil {
		return err
	}
	s.Sessions.Put(r.Context(), sessionUserKey, userID.String())
	return nil
}
