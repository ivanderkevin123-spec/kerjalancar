package web

import (
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"kerjalancar/internal/followup"
	"kerjalancar/internal/lead"
	"kerjalancar/internal/workspace"
)

var locations sync.Map // zone name → *time.Location (LoadLocation reads tzdata each call)

// location resolves the workspace time zone, falling back to Asia/Jakarta.
func location(name string) *time.Location {
	if v, ok := locations.Load(name); ok {
		return v.(*time.Location)
	}
	loc, err := time.LoadLocation(name)
	if err != nil || name == "" {
		if loc, err = time.LoadLocation("Asia/Jakarta"); err != nil {
			loc = time.FixedZone("WIB", 7*3600)
		}
	}
	locations.Store(name, loc)
	return loc
}

// now is the injectable clock (Deps.Clock), time.Now in production.
func (s *server) now() time.Time {
	if s.Clock != nil {
		return s.Clock()
	}
	return time.Now()
}

// today is the workspace's calendar day as a date (UTC midnight).
func (s *server) today(ws workspace.Workspace) time.Time {
	return followup.Today(s.now(), location(ws.Timezone))
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
	v.Today = s.today(ws)
	return v
}

type berandaData struct {
	Summary   lead.Summary
	Total     int
	FollowUps followup.Lists
	Agenda    []fuRow // overdue first, then today's, at most five
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
	today, loc := s.today(ws), location(ws.Timezone)
	lists, err := s.FollowUps.Lists(r.Context(), ws.ID, today)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	v := s.appView(r, "Beranda", "beranda")
	v.Flash = s.Sessions.PopString(r.Context(), sessionFlashKey)
	v.Data = berandaData{Summary: sum, Total: total, FollowUps: lists, Agenda: s.dashboardFollowUps(lists, today, loc, 5)}
	s.render(w, r, http.StatusOK, "beranda", v)
}

func (s *server) getPengaturan(w http.ResponseWriter, r *http.Request) {
	v := s.appView(r, "Pengaturan", "pengaturan")
	v.Form["name"] = v.Workspace.Name
	v.Flash = s.Sessions.PopString(r.Context(), sessionFlashKey)
	v.Data = s.pengaturanData(*v.Workspace, "")
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
	v.Data = s.pengaturanData(*v.Workspace, "")
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
