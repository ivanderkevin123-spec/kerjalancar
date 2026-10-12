package web

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"kerjalancar/internal/followup"
)

const (
	msgFollowUpScheduled   = "Follow-up dijadwalkan."
	msgFollowUpRescheduled = "Follow-up dijadwalkan ulang."
	msgFollowUpDone        = "Selesai ditandai."
	msgFollowUpAlreadyDone = "Sudah ditandai selesai."
	msgFollowUpUndone      = "Follow-up dibuka kembali."
	msgFollowUpStillOpen   = "Follow-up ini masih terbuka."
	msgFollowUpAnotherOpen = "Prospek ini sudah punya follow-up lain yang terbuka."
	msgLeadClosed          = "Prospek sudah ditutup; buka kembali dulu (pilih status aktif) untuk menjadwalkan follow-up."
)

var followUpTabs = []struct{ Key, Label string }{
	{"terlambat", "Terlambat"}, {"hari-ini", "Hari ini"}, {"mendatang", "Mendatang"}, {"selesai", "Selesai"},
}

// fuRow is one follow-up prepared for a template (dates rendered in the workspace zone).
type fuRow struct {
	F      followup.FollowUp
	Due    string
	Bucket followup.Bucket
	DoneAt string
	Msg    string
	Err    string
	Tab    string
}

type followupListData struct {
	Tab   string
	Lists followup.Lists
	Done  followup.Page
	Rows  []fuRow // rows of the active tab
	Later []fuRow // beyond seven days, shown under "Mendatang"
	Tabs  []chip
	Pager Pager
	Row   *fuRow // set when rendering one row fragment
}

func (s *server) rowView(f followup.FollowUp, today time.Time, loc *time.Location) fuRow {
	r := fuRow{F: f, Due: formatDateID(f.DueOn), Bucket: followup.BucketOf(f.DueOn, today)}
	if f.DoneAt != nil {
		r.DoneAt = formatDateID(f.DoneAt.In(loc)) + f.DoneAt.In(loc).Format(", 15.04")
	}
	return r
}

func (s *server) rows(list []followup.FollowUp, today time.Time, loc *time.Location, tab string) []fuRow {
	out := make([]fuRow, 0, len(list))
	for _, f := range list {
		r := s.rowView(f, today, loc)
		r.Tab = tab
		out = append(out, r)
	}
	return out
}

func normalizeTab(tab string) string {
	for _, t := range followUpTabs {
		if t.Key == tab {
			return tab
		}
	}
	return "hari-ini"
}

// tabURL maps a normalised tab to a fixed redirect target (never user input).
func tabURL(tab string) string {
	switch tab {
	case "terlambat":
		return "/app/follow-up?tab=terlambat"
	case "mendatang":
		return "/app/follow-up?tab=mendatang"
	case "selesai":
		return "/app/follow-up?tab=selesai"
	}
	return "/app/follow-up?tab=hari-ini"
}

// followUpListData loads everything the list page (or its counts fragment) needs.
func (s *server) followUpListData(r *http.Request, tab string) (followupListData, error) {
	ws := currentWorkspace(r.Context())
	today, loc := s.today(ws), location(ws.Timezone)
	lists, err := s.FollowUps.Lists(r.Context(), ws.ID, today)
	if err != nil {
		return followupListData{}, err
	}
	done, err := s.FollowUps.DonePage(r.Context(), ws.ID, parsePage(r))
	if err != nil {
		return followupListData{}, err
	}
	d := followupListData{Tab: tab, Lists: lists, Done: done}
	counts := map[string]int{"terlambat": lists.Counts.Overdue, "hari-ini": lists.Counts.Today, "mendatang": lists.Counts.Upcoming, "selesai": done.Total}
	for _, t := range followUpTabs {
		d.Tabs = append(d.Tabs, chip{Key: t.Key, Label: t.Label, Count: counts[t.Key], URL: "/app/follow-up?tab=" + t.Key, Active: t.Key == tab})
	}
	switch tab {
	case "terlambat":
		d.Rows = s.rows(lists.Overdue, today, loc, tab)
	case "mendatang":
		d.Rows = s.rows(lists.Upcoming, today, loc, tab)
		d.Later = s.rows(lists.Later, today, loc, tab)
	case "selesai":
		d.Rows = s.rows(done.Items, today, loc, tab)
		d.Pager = newPager("/app/follow-up", url.Values{"tab": {"selesai"}}, done.Page, done.Pages)
	default:
		d.Rows = s.rows(lists.Today, today, loc, tab)
	}
	return d, nil
}

func (s *server) getFollowUpList(w http.ResponseWriter, r *http.Request) {
	tab := normalizeTab(r.URL.Query().Get("tab"))
	d, err := s.followUpListData(r, tab)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	v := s.appView(r, "Follow-up", "follow-up")
	v.Flash = s.Sessions.PopString(r.Context(), sessionFlashKey)
	v.Data = d
	s.render(w, r, http.StatusOK, "followup_daftar", v)
}

// postProspekFollowUp schedules or moves the lead's open follow-up.
func (s *server) postProspekFollowUp(w http.ResponseWriter, r *http.Request) {
	l, ok := s.loadLead(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		s.badRequest(w, r)
		return
	}
	ws, user := currentWorkspace(r.Context()), currentUser(r.Context())
	hx := r.Header.Get("HX-Request") == "true"
	form := formValues(r, "due_on", "note")
	data := leadDetailData{}

	n, errs := followup.Validate(followup.Input{DueOn: form["due_on"], Note: form["note"]}, s.today(ws))
	if len(errs) == 0 {
		_, existed, err := s.FollowUps.OpenForLead(r.Context(), ws.ID, l.ID)
		if err != nil {
			s.serverError(w, r, err)
			return
		}
		_, err = s.FollowUps.Schedule(r.Context(), ws.ID, user.ID, l.ID, n)
		switch {
		case err == nil:
			s.audit(r, "followup.schedule", slog.String("lead_id", l.ID.String()), slog.String("due_on", n.DueOn.Format(followup.DateLayout)))
			msg := msgFollowUpScheduled
			if existed {
				msg = msgFollowUpRescheduled
			}
			if !hx {
				s.Sessions.Put(r.Context(), sessionFlashKey, msg)
				http.Redirect(w, r, "/app/prospek/"+l.ID.String(), http.StatusSeeOther)
				return
			}
			data.FollowUpMsg = msg
		case errors.Is(err, followup.ErrLeadClosed):
			data.FollowUpErr = msgLeadClosed
		case errors.Is(err, followup.ErrAnotherOpen):
			data.FollowUpErr = msgFollowUpAnotherOpen
		case errors.Is(err, followup.ErrLeadNotFound):
			s.notFound(w, r)
			return
		default:
			s.serverError(w, r, err)
			return
		}
	}
	v, err := s.detailView(r, l, data)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if len(errs) > 0 {
		v.Errors = errs
		v.Form["due_on"], v.Form["note"] = form["due_on"], form["note"]
	}
	if hx {
		d := v.Data.(leadDetailData)
		d.Oob = true
		v.Data = d
		s.fragment(w, r, "prospek_detail", "prospek_followup_oob", v)
		return
	}
	s.render(w, r, http.StatusOK, "prospek_detail", v)
}

func (s *server) postFollowUpDone(w http.ResponseWriter, r *http.Request) {
	s.followUpToggle(w, r, true)
}

func (s *server) postFollowUpUndo(w http.ResponseWriter, r *http.Request) {
	s.followUpToggle(w, r, false)
}

// followUpToggle marks done or undoes; both are idempotent with a message.
// "tampilan" says where the request came from: "prospek" (the lead panel)
// or "daftar" (the list), which decides the fragment and the redirect.
func (s *server) followUpToggle(w http.ResponseWriter, r *http.Request, done bool) {
	ws, user := currentWorkspace(r.Context()), currentUser(r.Context())
	id, ok := parseID(r)
	if !ok {
		s.notFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.badRequest(w, r)
		return
	}
	hx := r.Header.Get("HX-Request") == "true"
	view := r.PostFormValue("tampilan")
	tab := normalizeTab(r.PostFormValue("tab"))

	var f followup.FollowUp
	var err error
	var msg, errMsg string
	if done {
		f, err = s.FollowUps.Done(r.Context(), ws.ID, user.ID, id)
	} else {
		f, err = s.FollowUps.Undo(r.Context(), ws.ID, user.ID, id)
	}
	switch {
	case err == nil:
		msg = msgFollowUpDone
		action := "followup.done"
		if !done {
			msg, action = msgFollowUpUndone, "followup.undo"
		}
		s.audit(r, action, slog.String("follow_up_id", f.ID.String()), slog.String("lead_id", f.LeadID.String()))
	case errors.Is(err, followup.ErrAlreadyDone):
		errMsg = msgFollowUpAlreadyDone
	case errors.Is(err, followup.ErrNotDone):
		errMsg = msgFollowUpStillOpen
	case errors.Is(err, followup.ErrAnotherOpen):
		errMsg = msgFollowUpAnotherOpen
	case errors.Is(err, followup.ErrLeadClosed):
		errMsg = msgLeadClosed
	case errors.Is(err, followup.ErrNotFound), errors.Is(err, followup.ErrLeadNotFound):
		s.notFound(w, r)
		return
	default:
		s.serverError(w, r, err)
		return
	}
	if errMsg != "" {
		// Nothing changed; show the current state with the message.
		f, err = s.FollowUps.Get(r.Context(), ws.ID, id)
		if err != nil {
			s.serverError(w, r, err)
			return
		}
	}

	if view == "prospek" {
		l, err := s.Leads.Get(r.Context(), ws.ID, f.LeadID)
		if err != nil {
			s.serverError(w, r, err)
			return
		}
		if !hx {
			s.Sessions.Put(r.Context(), sessionFlashKey, firstNonEmpty(msg, errMsg))
			http.Redirect(w, r, "/app/prospek/"+l.ID.String(), http.StatusSeeOther)
			return
		}
		v, err := s.detailView(r, l, leadDetailData{FollowUpMsg: msg, FollowUpErr: errMsg})
		if err != nil {
			s.serverError(w, r, err)
			return
		}
		d := v.Data.(leadDetailData)
		d.Oob = true
		v.Data = d
		s.fragment(w, r, "prospek_detail", "prospek_followup_oob", v)
		return
	}

	if !hx {
		s.Sessions.Put(r.Context(), sessionFlashKey, firstNonEmpty(msg, errMsg))
		http.Redirect(w, r, tabURL(tab), http.StatusSeeOther)
		return
	}
	d, err := s.followUpListData(r, tab)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	row := s.rowView(f, s.today(ws), location(ws.Timezone))
	row.Msg, row.Err, row.Tab = msg, errMsg, tab
	d.Row = &row
	v := s.appView(r, "Follow-up", "follow-up")
	v.Data = d
	s.fragment(w, r, "followup_daftar", "followup_row_oob", v)
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// dashboardFollowUps returns the open follow-ups to show on the dashboard:
// overdue first, then today's, at most limit rows.
func (s *server) dashboardFollowUps(lists followup.Lists, today time.Time, loc *time.Location, limit int) []fuRow {
	var out []fuRow
	for _, l := range [][]followup.FollowUp{lists.Overdue, lists.Today} {
		for _, f := range l {
			if len(out) == limit {
				return out
			}
			out = append(out, s.rowView(f, today, loc))
		}
	}
	return out
}
