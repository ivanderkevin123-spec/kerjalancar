package web

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"net/mail"
	"strings"
	"unicode/utf8"

	"kerjalancar/internal/auth"
)

const (
	msgEmailRequired    = "Email wajib diisi."
	msgEmailInvalid     = "Email tidak valid."
	msgEmailUnusable    = "Email tidak dapat digunakan. Jika sudah punya akun, silakan masuk."
	msgPasswordRequired = "Kata sandi wajib diisi."
	msgPasswordShort    = "Kata sandi minimal 8 karakter."
	msgPasswordLong     = "Kata sandi terlalu panjang."
	msgNameRequired     = "Nama wajib diisi."
	msgNameLong         = "Nama maksimal 100 karakter."
	msgWorkspaceReq     = "Nama usaha wajib diisi."
	msgWorkspaceLong    = "Nama usaha maksimal 100 karakter."
	msgCodeInvalid      = "Kode undangan tidak valid."
	msgLoginFailed      = "Email atau kata sandi salah."
	msgRegistered       = "Akun dan workspace berhasil dibuat."
)

func (s *server) root(w http.ResponseWriter, r *http.Request) {
	if s.loggedIn(r) {
		http.Redirect(w, r, "/app", http.StatusSeeOther)
		return
	}
	redirectToLogin(w, r)
}

func (s *server) loginView() *View {
	v := newView("Masuk")
	v.RegistrationOpen = s.RegistrationOpen
	return v
}

func (s *server) getMasuk(w http.ResponseWriter, r *http.Request) {
	if s.loggedIn(r) {
		http.Redirect(w, r, "/app", http.StatusSeeOther)
		return
	}
	v := s.loginView()
	v.Flash = s.Sessions.PopString(r.Context(), sessionFlashKey)
	s.render(w, r, http.StatusOK, "masuk", v)
}

func (s *server) postMasuk(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.badRequest(w, r)
		return
	}
	email := r.PostFormValue("email")
	password := r.PostFormValue("password")

	v := s.loginView()
	v.Form["email"] = email
	if strings.TrimSpace(email) == "" {
		v.Errors["email"] = msgEmailRequired
	}
	if password == "" {
		v.Errors["password"] = msgPasswordRequired
	}
	if len(v.Errors) > 0 {
		s.render(w, r, http.StatusOK, "masuk", v)
		return
	}

	user, err := s.Auth.Authenticate(r.Context(), email, password)
	if errors.Is(err, auth.ErrInvalidCredentials) {
		v.Errors["form"] = msgLoginFailed
		s.render(w, r, http.StatusOK, "masuk", v)
		return
	}
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	workspaces, err := s.Workspaces.ForUser(r.Context(), user.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if len(workspaces) == 0 {
		s.serverError(w, r, errors.New("user has no workspace"))
		return
	}
	if err := s.startSession(r, user.ID, workspaces[0].ID); err != nil {
		s.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/app", http.StatusSeeOther)
}

func (s *server) registerView() *View {
	v := newView("Daftar")
	v.RegistrationOpen = true
	v.RequireCode = s.RegistrationCode != ""
	return v
}

func (s *server) getDaftar(w http.ResponseWriter, r *http.Request) {
	if !s.RegistrationOpen {
		s.notFound(w, r)
		return
	}
	if s.loggedIn(r) {
		http.Redirect(w, r, "/app", http.StatusSeeOther)
		return
	}
	s.render(w, r, http.StatusOK, "daftar", s.registerView())
}

func (s *server) postDaftar(w http.ResponseWriter, r *http.Request) {
	if !s.RegistrationOpen {
		s.notFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.badRequest(w, r)
		return
	}
	name := strings.TrimSpace(r.PostFormValue("name"))
	wsName := strings.TrimSpace(r.PostFormValue("workspace_name"))
	email := strings.TrimSpace(r.PostFormValue("email"))
	password := r.PostFormValue("password")
	code := strings.TrimSpace(r.PostFormValue("code"))

	v := s.registerView()
	v.Form["name"], v.Form["workspace_name"], v.Form["email"], v.Form["code"] = name, wsName, email, code // never echo the password

	switch {
	case name == "":
		v.Errors["name"] = msgNameRequired
	case utf8.RuneCountInString(name) > 100:
		v.Errors["name"] = msgNameLong
	}
	switch {
	case wsName == "":
		v.Errors["workspace_name"] = msgWorkspaceReq
	case utf8.RuneCountInString(wsName) > 100:
		v.Errors["workspace_name"] = msgWorkspaceLong
	}
	switch {
	case email == "":
		v.Errors["email"] = msgEmailRequired
	case !validEmail(email):
		v.Errors["email"] = msgEmailInvalid
	}
	switch {
	case len(password) < 8:
		v.Errors["password"] = msgPasswordShort
	case len(password) > 1024:
		v.Errors["password"] = msgPasswordLong
	}
	if s.RegistrationCode != "" && subtle.ConstantTimeCompare([]byte(code), []byte(s.RegistrationCode)) != 1 {
		v.Errors["code"] = msgCodeInvalid
	}
	if len(v.Errors) > 0 {
		s.render(w, r, http.StatusOK, "daftar", v)
		return
	}

	user, wsID, err := s.Auth.Register(r.Context(), auth.RegisterInput{Email: email, Password: password, Name: name, WorkspaceName: wsName})
	if errors.Is(err, auth.ErrEmailTaken) {
		v.Errors["email"] = msgEmailUnusable
		s.render(w, r, http.StatusOK, "daftar", v)
		return
	}
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if err := s.startSession(r, user.ID, wsID); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.Sessions.Put(r.Context(), sessionFlashKey, msgRegistered)
	http.Redirect(w, r, "/app", http.StatusSeeOther)
}

func (s *server) postKeluar(w http.ResponseWriter, r *http.Request) {
	if err := s.Sessions.Destroy(r.Context()); err != nil {
		s.serverError(w, r, err)
		return
	}
	redirectToLogin(w, r)
}

// validEmail accepts a single plain address (no display name) of sane length.
func validEmail(e string) bool {
	if len(e) > 254 || strings.ContainsAny(e, " \t\r\n") {
		return false
	}
	addr, err := mail.ParseAddress(e)
	return err == nil && addr.Address == e && strings.Contains(e, "@")
}
