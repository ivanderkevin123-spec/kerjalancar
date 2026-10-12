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

// requireLogin resolves the session's user and their current workspace from
// the database on every request. The workspace chosen at login is pinned in
// the session and re-checked against memberships; a user whose pinned
// membership is gone falls back to their first remaining membership (and is
// re-pinned), and a user with no membership at all is logged out.
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
		ws, err := s.currentWorkspaceFor(ctx, user.ID)
		if errors.Is(err, workspace.ErrNotFound) {
			_ = s.Sessions.Destroy(ctx)
			redirectToLogin(w, r)
			return
		}
		if err != nil {
			s.serverError(w, r, err)
			return
		}
		ctx = context.WithValue(ctx, ctxUser, user)
		ctx = context.WithValue(ctx, ctxWorkspace, ws)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// currentWorkspaceFor returns the pinned workspace when the user is still a
// member, otherwise their first membership (and pins it). workspace.ErrNotFound
// means the user has no membership at all.
func (s *server) currentWorkspaceFor(ctx context.Context, userID uuid.UUID) (workspace.Workspace, error) {
	if pinned, err := uuid.Parse(s.Sessions.GetString(ctx, sessionWorkspaceKey)); err == nil {
		ws, err := s.Workspaces.Get(ctx, pinned, userID)
		if err == nil {
			return ws, nil
		}
		if !errors.Is(err, workspace.ErrNotFound) {
			return workspace.Workspace{}, err
		}
	}
	list, err := s.Workspaces.ForUser(ctx, userID)
	if err != nil {
		return workspace.Workspace{}, err
	}
	if len(list) == 0 {
		return workspace.Workspace{}, workspace.ErrNotFound
	}
	s.Sessions.Put(ctx, sessionWorkspaceKey, list[0].ID.String())
	return list[0], nil
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

// startSession rotates the session token (defeats fixation) and binds the
// user and their current workspace.
func (s *server) startSession(r *http.Request, userID, workspaceID uuid.UUID) error {
	if err := s.Sessions.RenewToken(r.Context()); err != nil {
		return err
	}
	s.Sessions.Put(r.Context(), sessionUserKey, userID.String())
	s.Sessions.Put(r.Context(), sessionWorkspaceKey, workspaceID.String())
	return nil
}
