package web

import (
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"kerjalancar/internal/workspace"
)

const msgWorkspaceSaved = "Nama usaha tersimpan."

func (s *server) appView(r *http.Request, title, nav string) *View {
	user := currentUser(r.Context())
	ws := currentWorkspace(r.Context())
	v := newView(title)
	v.Nav = nav
	v.User = &user
	v.Workspace = &ws
	return v
}

func (s *server) getBeranda(w http.ResponseWriter, r *http.Request) {
	v := s.appView(r, "Beranda", "beranda")
	v.Flash = s.Sessions.PopString(r.Context(), sessionFlashKey)
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
		if err := s.Templates.Fragment(w, http.StatusOK, "pengaturan", "pengaturan_form", v); err != nil {
			s.Logger.Error("render fragment", "err", err)
		}
		return
	}
	s.render(w, r, http.StatusOK, "pengaturan", v)
}
