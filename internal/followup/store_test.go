package followup_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"kerjalancar/internal/auth"
	"kerjalancar/internal/customer"
	"kerjalancar/internal/db/testdb"
	"kerjalancar/internal/followup"
	"kerjalancar/internal/lead"
)

var lightArgon = auth.Params{MemoryKiB: 8192, Time: 1, Threads: 1}

type fixture struct {
	ctx        context.Context
	fu         *followup.Store
	leads      *lead.Store
	wsA, wsB   uuid.UUID
	userA      uuid.UUID
	userB      uuid.UUID
	leadA      lead.Lead
	leadB      lead.Lead
	today      time.Time
	activities func(t *testing.T, ws, leadID uuid.UUID) []lead.Activity
	cachedDate func(t *testing.T, leadID uuid.UUID) *time.Time
}

func setup(t *testing.T) fixture {
	t.Helper()
	pool := testdb.Pool(t)
	ctx := context.Background()
	svc := auth.NewService(pool, lightArgon)
	customers := customer.NewStore(pool)
	leads := lead.NewStore(pool)
	leads.ScheduleHook = followup.ScheduleForLead
	leads.CloseHook = followup.CloseForLead
	reg := func(label string) (uuid.UUID, uuid.UUID, lead.Lead) {
		u, ws, err := svc.Register(ctx, auth.RegisterInput{Email: fmt.Sprintf("fu-%s-%s@contoh.id", label, uuid.NewString()[:8]), Password: "rahasia-123", Name: "Pemilik " + label, WorkspaceName: "Studio " + label})
		require.NoError(t, err)
		c, err := customers.Create(ctx, ws, customer.Input{Name: "Pelanggan " + label, Phone: "+62812" + fmt.Sprintf("%08d", time.Now().UnixNano()%100_000_000)})
		require.NoError(t, err)
		l, err := leads.Create(ctx, ws, u.ID, lead.CreateInput{CustomerID: c.ID, Input: lead.Input{Title: "Proyek " + label}})
		require.NoError(t, err)
		return u.ID, ws, l
	}
	f := fixture{ctx: ctx, fu: followup.NewStore(pool), leads: leads, today: date(2026, 10, 20)}
	f.userA, f.wsA, f.leadA = reg("a")
	f.userB, f.wsB, f.leadB = reg("b")
	f.activities = func(t *testing.T, ws, leadID uuid.UUID) []lead.Activity {
		acts, err := leads.Activities(ctx, ws, leadID, 50)
		require.NoError(t, err)
		return acts
	}
	f.cachedDate = func(t *testing.T, leadID uuid.UUID) *time.Time {
		var d *time.Time
		require.NoError(t, pool.QueryRow(ctx, `SELECT next_follow_up_on FROM leads WHERE id = $1`, leadID).Scan(&d))
		return d
	}
	return f
}

func kinds(acts []lead.Activity) []lead.Kind {
	out := make([]lead.Kind, 0, len(acts))
	for _, a := range acts {
		out = append(out, a.Kind)
	}
	return out
}

func TestScheduleCreatesThenMovesTheSingleOpenFollowUp(t *testing.T) {
	f := setup(t)
	first, err := f.fu.Schedule(f.ctx, f.wsA, f.userA, f.leadA.ID, followup.Normalized{DueOn: date(2026, 10, 23), Note: "telepon"})
	require.NoError(t, err)
	assert.True(t, first.Open())
	assert.Equal(t, "Proyek a", first.LeadTitle)
	assert.Equal(t, "Pelanggan a", first.CustomerName)
	assert.Equal(t, date(2026, 10, 23), first.DueOn)
	require.NotNil(t, f.cachedDate(t, f.leadA.ID))
	assert.Equal(t, date(2026, 10, 23), *f.cachedDate(t, f.leadA.ID), "lead cache follows the open follow-up")

	moved, err := f.fu.Schedule(f.ctx, f.wsA, f.userA, f.leadA.ID, followup.Normalized{DueOn: date(2026, 10, 25), Note: "WA dulu"})
	require.NoError(t, err)
	assert.Equal(t, first.ID, moved.ID, "rescheduling moves the same row")
	assert.Equal(t, "WA dulu", moved.Note)
	assert.Equal(t, date(2026, 10, 25), *f.cachedDate(t, f.leadA.ID))

	open, ok, err := f.fu.OpenForLead(f.ctx, f.wsA, f.leadA.ID)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, first.ID, open.ID)

	acts := f.activities(t, f.wsA, f.leadA.ID)
	assert.Equal(t, []lead.Kind{lead.KindFollowUpRescheduled, lead.KindFollowUpScheduled, lead.KindCreated}, kinds(acts))
	assert.Equal(t, "Follow-up dijadwalkan ulang: 23 Okt 2026 → 25 Okt 2026", acts[0].Describe())
	assert.Equal(t, "Follow-up dijadwalkan: 23 Okt 2026", acts[1].Describe())
}

func TestDoneAndUndoAreIdempotentAndRecorded(t *testing.T) {
	f := setup(t)
	fu, err := f.fu.Schedule(f.ctx, f.wsA, f.userA, f.leadA.ID, followup.Normalized{DueOn: date(2026, 10, 23)})
	require.NoError(t, err)

	done, err := f.fu.Done(f.ctx, f.wsA, f.userA, fu.ID)
	require.NoError(t, err)
	require.NotNil(t, done.DoneAt)
	require.NotNil(t, done.DoneBy)
	assert.Equal(t, f.userA, *done.DoneBy)
	assert.Nil(t, f.cachedDate(t, f.leadA.ID), "cache cleared when done")

	_, err = f.fu.Done(f.ctx, f.wsA, f.userA, fu.ID)
	assert.ErrorIs(t, err, followup.ErrAlreadyDone, "second device taps Selesai again")
	assert.Equal(t, []lead.Kind{lead.KindFollowUpDone, lead.KindFollowUpScheduled, lead.KindCreated}, kinds(f.activities(t, f.wsA, f.leadA.ID)), "one done activity only")

	_, ok, err := f.fu.OpenForLead(f.ctx, f.wsA, f.leadA.ID)
	require.NoError(t, err)
	assert.False(t, ok)

	undone, err := f.fu.Undo(f.ctx, f.wsA, f.userA, fu.ID)
	require.NoError(t, err)
	assert.True(t, undone.Open())
	assert.Equal(t, date(2026, 10, 23), *f.cachedDate(t, f.leadA.ID), "cache restored on undo")
	_, err = f.fu.Undo(f.ctx, f.wsA, f.userA, fu.ID)
	assert.ErrorIs(t, err, followup.ErrNotDone)

	// Done again, then a new follow-up: the old one cannot be reopened while another is open.
	_, err = f.fu.Done(f.ctx, f.wsA, f.userA, fu.ID)
	require.NoError(t, err)
	second, err := f.fu.Schedule(f.ctx, f.wsA, f.userA, f.leadA.ID, followup.Normalized{DueOn: date(2026, 11, 1)})
	require.NoError(t, err)
	assert.NotEqual(t, fu.ID, second.ID)
	_, err = f.fu.Undo(f.ctx, f.wsA, f.userA, fu.ID)
	assert.ErrorIs(t, err, followup.ErrAnotherOpen)

	page, err := f.fu.DonePage(f.ctx, f.wsA, 1)
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	assert.Equal(t, fu.ID, page.Items[0].ID)
}

func TestListsBucketByTheWorkspaceToday(t *testing.T) {
	f := setup(t)
	pool := testdb.Pool(t)
	customers := customer.NewStore(pool)
	mk := func(due time.Time) {
		c, err := customers.Create(f.ctx, f.wsA, customer.Input{Name: "C " + due.Format("0102")})
		require.NoError(t, err)
		l, err := f.leads.Create(f.ctx, f.wsA, f.userA, lead.CreateInput{CustomerID: c.ID, Input: lead.Input{Title: "L " + due.Format("0102")}})
		require.NoError(t, err)
		_, err = f.fu.Schedule(f.ctx, f.wsA, f.userA, l.ID, followup.Normalized{DueOn: due})
		require.NoError(t, err)
	}
	today := date(2026, 10, 20)
	for _, d := range []time.Time{date(2026, 10, 1), date(2026, 10, 19), date(2026, 10, 20), date(2026, 10, 21), date(2026, 10, 27), date(2026, 10, 28)} {
		mk(d)
	}
	// B has its own follow-up that must never appear in A's lists.
	_, err := f.fu.Schedule(f.ctx, f.wsB, f.userB, f.leadB.ID, followup.Normalized{DueOn: today})
	require.NoError(t, err)

	lists, err := f.fu.Lists(f.ctx, f.wsA, today)
	require.NoError(t, err)
	assert.Equal(t, followup.Counts{Overdue: 2, Today: 1, Upcoming: 2}, lists.Counts)
	assert.Len(t, lists.Overdue, 2)
	assert.Len(t, lists.Today, 1)
	assert.Len(t, lists.Upcoming, 2)
	assert.Len(t, lists.Later, 1)
	assert.Equal(t, date(2026, 10, 1), lists.Overdue[0].DueOn, "oldest first")
	assert.Equal(t, followup.BucketToday, lists.Today[0].Bucket)
	for _, l := range [][]followup.FollowUp{lists.Overdue, lists.Today, lists.Upcoming, lists.Later} {
		for _, x := range l {
			assert.Equal(t, f.wsA, x.WorkspaceID)
		}
	}

	// The same instant is a different day across zones (the MVP acceptance case).
	instant := time.Date(2026, 10, 19, 16, 59, 0, 0, time.UTC) // 23:59 WIB / 01:59 WIT
	wib, err := f.fu.Lists(f.ctx, f.wsA, followup.Today(instant, time.FixedZone("WIB", 7*3600)))
	require.NoError(t, err)
	wit, err := f.fu.Lists(f.ctx, f.wsA, followup.Today(instant, time.FixedZone("WIT", 9*3600)))
	require.NoError(t, err)
	assert.Equal(t, 1, wib.Counts.Today, "19 Oct is today in WIB")
	assert.Equal(t, 1, wit.Counts.Today, "20 Oct is today in WIT")
	assert.Equal(t, date(2026, 10, 19), wib.Today[0].DueOn)
	assert.Equal(t, date(2026, 10, 20), wit.Today[0].DueOn)
}

func TestCrossWorkspaceAccessIsRefusedAndWritesNothing(t *testing.T) {
	f := setup(t)
	fu, err := f.fu.Schedule(f.ctx, f.wsA, f.userA, f.leadA.ID, followup.Normalized{DueOn: date(2026, 10, 23)})
	require.NoError(t, err)

	_, err = f.fu.Schedule(f.ctx, f.wsB, f.userB, f.leadA.ID, followup.Normalized{DueOn: date(2026, 12, 1)})
	assert.ErrorIs(t, err, followup.ErrLeadNotFound)
	_, err = f.fu.Get(f.ctx, f.wsB, fu.ID)
	assert.ErrorIs(t, err, followup.ErrNotFound)
	_, err = f.fu.Done(f.ctx, f.wsB, f.userB, fu.ID)
	assert.ErrorIs(t, err, followup.ErrNotFound)
	_, err = f.fu.Undo(f.ctx, f.wsB, f.userB, fu.ID)
	assert.ErrorIs(t, err, followup.ErrNotFound)

	again, err := f.fu.Get(f.ctx, f.wsA, fu.ID)
	require.NoError(t, err)
	assert.True(t, again.Open())
	assert.Equal(t, date(2026, 10, 23), again.DueOn)
	assert.Len(t, f.activities(t, f.wsA, f.leadA.ID), 2)
	lists, err := f.fu.Lists(f.ctx, f.wsB, f.today)
	require.NoError(t, err)
	assert.Equal(t, followup.Counts{}, lists.Counts)
}

func TestLeadStatusChangesDriveFollowUps(t *testing.T) {
	f := setup(t)

	// Entering penawaran with no follow-up schedules one for today + 3 (D-035/D-045).
	l, err := f.leads.ChangeStatus(f.ctx, f.wsA, f.userA, f.leadA.ID, lead.StatusPenawaran, "", f.today)
	require.NoError(t, err)
	open, ok, err := f.fu.OpenForLead(f.ctx, f.wsA, f.leadA.ID)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, followup.DefaultDue(f.today), open.DueOn)
	require.NotNil(t, l.NextFollowUpOn)
	assert.Equal(t, open.DueOn, *l.NextFollowUpOn, "lead cache set by the hook")

	// Closing the lead finishes the follow-up automatically and says so in the history.
	_, err = f.leads.ChangeStatus(f.ctx, f.wsA, f.userA, f.leadA.ID, lead.StatusKalah, "harga", f.today)
	require.NoError(t, err)
	_, ok, err = f.fu.OpenForLead(f.ctx, f.wsA, f.leadA.ID)
	require.NoError(t, err)
	assert.False(t, ok)
	closed, err := f.fu.Get(f.ctx, f.wsA, open.ID)
	require.NoError(t, err)
	assert.False(t, closed.Open())
	assert.Nil(t, f.cachedDate(t, f.leadA.ID))
	acts := f.activities(t, f.wsA, f.leadA.ID)
	assert.Equal(t, []lead.Kind{lead.KindFollowUpDone, lead.KindStatusChanged, lead.KindFollowUpScheduled, lead.KindStatusChanged, lead.KindCreated}, kinds(acts))
	assert.Equal(t, "Follow-up selesai (prospek ditutup)", acts[0].Describe())

	// A closed lead takes no new follow-up and its auto-closed one cannot be reopened (D-045).
	_, err = f.fu.Schedule(f.ctx, f.wsA, f.userA, f.leadA.ID, followup.Normalized{DueOn: date(2026, 11, 2)})
	assert.ErrorIs(t, err, followup.ErrLeadClosed)
	_, err = f.fu.Undo(f.ctx, f.wsA, f.userA, open.ID)
	assert.ErrorIs(t, err, followup.ErrLeadClosed)
	assert.Nil(t, f.cachedDate(t, f.leadA.ID))

	// Reopening leaves the old follow-up done; a new one can be scheduled.
	_, err = f.leads.ChangeStatus(f.ctx, f.wsA, f.userA, f.leadA.ID, lead.StatusSurvei, "", f.today)
	require.NoError(t, err)
	_, ok, _ = f.fu.OpenForLead(f.ctx, f.wsA, f.leadA.ID)
	assert.False(t, ok, "reopening does not resurrect the finished follow-up")
	fresh, err := f.fu.Schedule(f.ctx, f.wsA, f.userA, f.leadA.ID, followup.Normalized{DueOn: date(2026, 11, 2)})
	require.NoError(t, err)
	assert.NotEqual(t, open.ID, fresh.ID)

	// A lead created with a date gets its follow-up through the same hook.
	pool := testdb.Pool(t)
	c, err := customer.NewStore(pool).Create(f.ctx, f.wsA, customer.Input{Name: "Dengan tanggal"})
	require.NoError(t, err)
	withDate, err := f.leads.Create(f.ctx, f.wsA, f.userA, lead.CreateInput{CustomerID: c.ID, Input: lead.Input{Title: "Teras", NextFollowUpOn: "2026-10-30"}})
	require.NoError(t, err)
	_, ok, err = f.fu.OpenForLead(f.ctx, f.wsA, withDate.ID)
	require.NoError(t, err)
	assert.True(t, ok)
	require.NotNil(t, withDate.NextFollowUpOn)
	assert.Equal(t, date(2026, 10, 30), *withDate.NextFollowUpOn)
}

// D-044: counts and lists come from the same place; a flood of overdue rows
// must not hide today's or upcoming follow-ups.
func TestListsNeverStarveLaterBuckets(t *testing.T) {
	f := setup(t)
	pool := testdb.Pool(t)
	today := date(2026, 10, 20)
	// Insert 105 overdue follow-ups directly (one lead each) plus one for today and one
	// upcoming. The raw inserts keep the cache invariant (D-043) like the store would.
	dueFor := func(i int) time.Time {
		switch i {
		case 105:
			return today
		case 106:
			return today.AddDate(0, 0, 2)
		}
		return today.AddDate(0, 0, -1-i%30)
	}
	var sql strings.Builder
	sql.WriteString("INSERT INTO leads (id, workspace_id, customer_id, title, status, next_follow_up_on) VALUES ")
	ids := make([]uuid.UUID, 0, 107)
	for i := 0; i < 107; i++ {
		id := uuid.New()
		ids = append(ids, id)
		if i > 0 {
			sql.WriteString(", ")
		}
		fmt.Fprintf(&sql, "('%s', '%s', '%s', 'Massal %03d', 'baru', '%s')", id, f.wsA, f.leadA.CustomerID, i, dueFor(i).Format("2006-01-02"))
	}
	_, err := pool.Exec(f.ctx, sql.String())
	require.NoError(t, err)
	var fus strings.Builder
	fus.WriteString("INSERT INTO follow_ups (id, workspace_id, lead_id, due_on) VALUES ")
	for i, id := range ids {
		if i > 0 {
			fus.WriteString(", ")
		}
		fmt.Fprintf(&fus, "('%s', '%s', '%s', '%s')", uuid.New(), f.wsA, id, dueFor(i).Format("2006-01-02"))
	}
	_, err = pool.Exec(f.ctx, fus.String())
	require.NoError(t, err)

	lists, err := f.fu.Lists(f.ctx, f.wsA, today)
	require.NoError(t, err)
	assert.Equal(t, followup.Counts{Overdue: 105, Today: 1, Upcoming: 1}, lists.Counts)
	assert.Len(t, lists.Overdue, followup.MaxPerBucket, "capped, oldest first")
	assert.Len(t, lists.Today, 1, "today's row is listed although overdue rows exceed the cap")
	assert.Len(t, lists.Upcoming, 1)
	assert.Equal(t, today.AddDate(0, 0, -30), lists.Overdue[0].DueOn)
}
