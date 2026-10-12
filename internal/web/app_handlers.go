package web

import (
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"kerjalancar/internal/lead"
	"kerjalancar/internal/workspace"
)

// location resolves the workspace time zone, falling back to Asia/Jakarta.
func location(name string) *time.Location {
	if loc, err := time.LoadLocation(name); err == nil && name != "" {
		return loc
	}
	if loc, err := time.LoadLocation("Asia/Jakarta"); err == nil {
		return loc
	}
	return time.FixedZone("WIB", 7*3600)
}

const msgWorkspaceSaved = "Nama usaha tersimpan."

func (s *server) appView(r *http.Request, title, nav string) *View {
	user := currentUser(r.Context())
	ws := currentWorkspace(r.Context())
	v := newView(title)
	v.Nav = nav
	v.User = &user
	v.Workspace = &ws
	v.Loc = location(ws.Timezone)
	return v
}

type berandaData struct {
	Summary lead.Summary
	Total   int
}

func (s *server) getBeranda(w http.ResponseWriter, r *http.Request) {
	ws := currentWorkspace(r.Context())
	sum, err := s.Leads.Summary(r.Context(), ws.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	total := 0
	for _, n := range sum.ByStatus {
		total += n
	}
	v := s.appView(r, "Beranda", "beranda")
	v.Flash = s.Sessions.PopString(r.Context(), sessionFlashKey)
	v.Data = berandaData{Summary: sum, Total: total}
	s.render(w, r, http.StatusOK, "beranda", v)
}

func (s *server) getPengaturan(w http.ResponseWriter, r *http.Request) {
	v := s.appView(r, "Pengaturan", "pengaturan")
	v.Form["name"] = v.Workspace.Name
	v.Flash = s.Sessions.PopString(r.Context(), sessionFlashKey)
	s.render(w, r, http.StatusOK, "pengaturan", v)
}

// postPengaturan renames the caller's current workspace. The workspace comes
// from the session-resolved membership only; ids in the form are ignored.
func (s *server) postPengaturan(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.badRequest(w, r)
		return
	}
	name := strings.TrimSpace(r.PostFormValue("name"))
	v := s.appView(r, "Pengaturan", "pengaturan")
	v.Form["name"] = name
	hx := r.Header.Get("HX-Request") == "true"

	switch {
	case name == "":
		v.Errors["name"] = msgWorkspaceReq
	case utf8.RuneCountInString(name) > 100:
		v.Errors["name"] = msgWorkspaceLong
	}
	if len(v.Errors) > 0 {
		s.renderSettings(w, r, hx, v)
		return
	}

	user := currentUser(r.Context())
	ws := currentWorkspace(r.Context())
	renamed, err := s.Workspaces.Rename(r.Context(), ws.ID, user.ID, name)
	switch {
	case errors.Is(err, workspace.ErrInvalidName):
		v.Errors["name"] = msgWorkspaceReq
		s.renderSettings(w, r, hx, v)
		return
	case errors.Is(err, workspace.ErrNotFound):
		s.notFound(w, r)
		return
	case err != nil:
		s.serverError(w, r, err)
		return
	}

	if hx {
		v.Workspace = &renamed
		v.Form["name"] = renamed.Name
		v.Saved = true
		s.renderSettings(w, r, true, v)
		return
	}
	s.Sessions.Put(r.Context(), sessionFlashKey, msgWorkspaceSaved)
	http.Redirect(w, r, "/app/pengaturan", http.StatusSeeOther)
}

func (s *server) renderSettings(w http.ResponseWriter, r *http.Request, hx bool, v *View) {
	if hx {
		s.fragment(w, r, "pengaturan", "pengaturan_form", v)
		return
	}
	s.render(w, r, http.StatusOK, "pengaturan", v)
}
