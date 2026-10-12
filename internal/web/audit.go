package web

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"
)

// audit writes the technical audit line for a business mutation: who did
// what to which row, by id only. Content (names, phones, notes) never goes
// to the log; the user-visible history lives in lead_activities (D-036).
func (s *server) audit(r *http.Request, action string, attrs ...slog.Attr) {
	ctx := r.Context()
	base := []slog.Attr{
		slog.String("action", action),
		slog.String("request_id", middleware.GetReqID(ctx)),
		slog.String("user_id", currentUser(ctx).ID.String()),
		slog.String("workspace_id", currentWorkspace(ctx).ID.String()),
	}
	s.Logger.LogAttrs(ctx, slog.LevelInfo, "audit", append(base, attrs...)...)
}
