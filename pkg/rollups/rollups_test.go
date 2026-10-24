package rollups

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/database/testdb"
	"github.com/adanalife/tripbot/pkg/scoreboards"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// base anchors every event timestamp. Explicit times, never NOW(): inside a
// test transaction NOW() is frozen, so rows inserted in sequence would all
// share one instant and the pairing's ORDER BY date_created would be a tie.
var base = time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)

// rollup mirrors a user_rollups row.
type rollup struct {
	ID           int64
	Platform     string
	Username     string
	EventsMiles  float64
	SessionCount int
	FirstSeen    time.Time
	LastSeen     time.Time
	ExtraMiles   float64
}

// parkWatermark moves the watermark up to the current max events.id, so a test
// only sees the events it inserts afterwards. Migration 025 seeds the
// 'user_rollups' row, so this updates rather than inserts.
func parkWatermark(t *testing.T, db *gorm.DB) {
	t.Helper()
	err := db.Exec(`UPDATE rollup_watermarks
	                SET last_event_id = (SELECT COALESCE(MAX(id), 0) FROM events)
	                WHERE name = ?`, watermarkName).Error
	if err != nil {
		t.Fatalf("park watermark: %v", err)
	}
}

func watermark(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var wm int64
	if err := db.Raw(`SELECT last_event_id FROM rollup_watermarks WHERE name = ?`, watermarkName).Scan(&wm).Error; err != nil {
		t.Fatalf("read watermark: %v", err)
	}
	return wm
}

func maxEventID(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var id int64
	if err := db.Raw(`SELECT COALESCE(MAX(id), 0) FROM events`).Scan(&id).Error; err != nil {
		t.Fatalf("read max event id: %v", err)
	}
	return id
}

// addEvent writes one events row. An extraMiles of 0 stores NULL, matching
// production: only bonus-bearing logouts and corrections carry a value.
func addEvent(t *testing.T, db *gorm.DB, platform, username, event string, at time.Time, extraMiles float64) {
	t.Helper()
	var extra any
	if extraMiles != 0 {
		extra = extraMiles
	}
	err := db.Exec(`INSERT INTO events (platform, username, event, date_created, extra_miles_earned)
	                VALUES (?, ?, ?, ?, ?)`, platform, username, event, at, extra).Error
	if err != nil {
		t.Fatalf("insert %s event for %s: %v", event, username, err)
	}
}

// addSession writes a twitch login/logout pair `minutes` long, starting
// `startOffset` after base.
func addSession(t *testing.T, db *gorm.DB, username string, startOffset, length time.Duration) {
	t.Helper()
	start := base.Add(startOffset)
	addEvent(t, db, "twitch", username, "login", start, 0)
	addEvent(t, db, "twitch", username, "logout", start.Add(length), 0)
}

func getRollup(t *testing.T, db *gorm.DB, platform, username string) rollup {
	t.Helper()
	var r rollup
	err := db.Raw(`SELECT id, platform, username, events_miles, session_count, first_seen, last_seen, extra_miles
	               FROM user_rollups WHERE platform = ? AND username = ?`, platform, username).Scan(&r).Error
	if err != nil {
		t.Fatalf("read rollup for %s/%s: %v", platform, username, err)
	}
	if r.Username == "" {
		t.Fatalf("no user_rollups row for %s/%s", platform, username)
	}
	return r
}

func countRows(t *testing.T, db *gorm.DB, query string, args ...any) int64 {
	t.Helper()
	var n int64
	if err := db.Raw(query, args...).Scan(&n).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

// closeTo compares against the REAL (float32) columns, which don't round-trip
// the aggregation arithmetic exactly.
func closeTo(t *testing.T, label string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 0.001 {
		t.Errorf("%s: got %v, want %v", label, got, want)
	}
}

// testConf is a writable config for the reconcile runs under test.
var testConf = &c.TripbotConfig{Environment: "testing", Platform: "twitch"}

func TestReconcile_ReadOnlySkipsEverything(t *testing.T) {
	db := testdb.New(t)
	parkWatermark(t, db)
	wm := watermark(t, db)
	addSession(t, db, "readonly_user", 0, 90*time.Minute)

	Reconcile(context.Background(), &c.TripbotConfig{ReadOnly: true})

	if got := watermark(t, db); got != wm {
		t.Errorf("ReadOnly advanced the watermark: %d -> %d", wm, got)
	}
	if n := countRows(t, db, `SELECT COUNT(*) FROM user_rollups WHERE username = 'readonly_user'`); n != 0 {
		t.Errorf("ReadOnly wrote %d rollup rows", n)
	}
}

func TestReconcile_NoNewEventsIsANoOp(t *testing.T) {
	db := testdb.New(t)
	addSession(t, db, "quiet_user", 0, 90*time.Minute)
	// Watermark parked *after* the events land, so nothing is new.
	parkWatermark(t, db)
	wm := watermark(t, db)
	before := countRows(t, db, `SELECT COUNT(*) FROM user_rollups`)

	Reconcile(context.Background(), testConf)

	if got := watermark(t, db); got != wm {
		t.Errorf("watermark moved with no new events: %d -> %d", wm, got)
	}
	if after := countRows(t, db, `SELECT COUNT(*) FROM user_rollups`); after != before {
		t.Errorf("rollup rows changed with no new events: %d -> %d", before, after)
	}
}

func TestReconcile_ComputesMilesFromPairedSessions(t *testing.T) {
	db := testdb.New(t)
	parkWatermark(t, db)

	// The N-th login pairs with the N-th logout: 90min -> 3.0mi, 30min -> 1.0mi
	// at the 0.1-mile-per-3-minutes rate.
	addSession(t, db, "hiker", 0, 90*time.Minute)
	addSession(t, db, "hiker", 4*time.Hour, 30*time.Minute)
	maxID := maxEventID(t, db)

	Reconcile(context.Background(), testConf)

	got := getRollup(t, db, "twitch", "hiker")
	closeTo(t, "events_miles", got.EventsMiles, 4.0)
	if got.SessionCount != 2 {
		t.Errorf("session_count: got %d, want 2", got.SessionCount)
	}
	if !got.FirstSeen.Equal(base) {
		t.Errorf("first_seen: got %v, want %v", got.FirstSeen.UTC(), base)
	}
	wantLast := base.Add(4*time.Hour + 30*time.Minute)
	if !got.LastSeen.Equal(wantLast) {
		t.Errorf("last_seen: got %v, want %v", got.LastSeen.UTC(), wantLast)
	}
	closeTo(t, "extra_miles", got.ExtraMiles, 0)

	if wm := watermark(t, db); wm != maxID {
		t.Errorf("watermark: got %d, want %d", wm, maxID)
	}
}

// addKeyedEvent writes a twitch login or logout carrying a session_id, the
// shape every session event has had since session_id was added.
func addKeyedEvent(t *testing.T, db *gorm.DB, username, event string, sessionID uuid.UUID, at time.Time) {
	t.Helper()
	err := db.Exec(`INSERT INTO events (platform, username, event, session_id, date_created)
	                VALUES ('twitch', ?, ?, ?, ?)`, username, event, sessionID, at).Error
	if err != nil {
		t.Fatalf("insert %s event for %s: %v", event, username, err)
	}
}

// A login whose logout was never written (a crash) pairs with nothing: the
// sessions after it still pair by session_id, so their lengths come out right
// instead of every later login pairing with the next session's logout.
func TestReconcile_OrphanedLoginDoesNotShiftLaterSessions(t *testing.T) {
	db := testdb.New(t)
	parkWatermark(t, db)

	addKeyedEvent(t, db, "crashed", "login", uuid.New(), base)
	for _, start := range []time.Duration{time.Hour, 2 * time.Hour} {
		id := uuid.New()
		addKeyedEvent(t, db, "crashed", "login", id, base.Add(start))
		addKeyedEvent(t, db, "crashed", "logout", id, base.Add(start+30*time.Minute))
	}

	Reconcile(context.Background(), testConf)

	// two 30-minute sessions at 0.1mi/3min
	got := getRollup(t, db, "twitch", "crashed")
	closeTo(t, "events_miles", got.EventsMiles, 2.0)
	if got.SessionCount != 3 {
		t.Errorf("session_count: got %d, want 3", got.SessionCount)
	}
}

func TestReconcile_DropsSessionsOverTwentyFourHours(t *testing.T) {
	db := testdb.New(t)
	parkWatermark(t, db)

	// A missed logout: the pair spans 26h and earns nothing. The following
	// 60min session still pairs (2nd login <-> 2nd logout) and counts.
	addSession(t, db, "forgot_to_leave", 0, 26*time.Hour)
	addSession(t, db, "forgot_to_leave", 27*time.Hour, 60*time.Minute)

	Reconcile(context.Background(), testConf)

	got := getRollup(t, db, "twitch", "forgot_to_leave")
	closeTo(t, "events_miles", got.EventsMiles, 2.0)
	// session_count counts logins, so the dropped session still shows up there.
	if got.SessionCount != 2 {
		t.Errorf("session_count: got %d, want 2", got.SessionCount)
	}
}

func TestReconcile_ExcludesPre2000SentinelFromSeenTimes(t *testing.T) {
	db := testdb.New(t)
	parkWatermark(t, db)

	// Historical events rows carry a zero-value date_created from an old
	// insert bug; first_seen/last_seen must ignore them.
	sentinel := time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC)
	addEvent(t, db, "twitch", "old_timer", "chat", sentinel, 0)
	addSession(t, db, "old_timer", 0, 30*time.Minute)

	Reconcile(context.Background(), testConf)

	got := getRollup(t, db, "twitch", "old_timer")
	if !got.FirstSeen.Equal(base) {
		t.Errorf("first_seen should skip the sentinel: got %v, want %v", got.FirstSeen.UTC(), base)
	}
	if !got.LastSeen.Equal(base.Add(30 * time.Minute)) {
		t.Errorf("last_seen: got %v", got.LastSeen.UTC())
	}
}

func TestReconcile_SumsExtraMilesFromLogoutAndCorrection(t *testing.T) {
	db := testdb.New(t)
	parkWatermark(t, db)

	addEvent(t, db, "twitch", "bonus_haver", "login", base, 99) // ignored: not a logout/correction
	addEvent(t, db, "twitch", "bonus_haver", "logout", base.Add(90*time.Minute), 1.5)
	addEvent(t, db, "twitch", "bonus_haver", "correction", base.Add(2*time.Hour), 2.0)
	addEvent(t, db, "twitch", "bonus_haver", "correction", base.Add(3*time.Hour), -0.5)

	Reconcile(context.Background(), testConf)

	got := getRollup(t, db, "twitch", "bonus_haver")
	closeTo(t, "extra_miles", got.ExtraMiles, 3.0)
	// The bonus stays out of the pairing base: 90min is still 3.0mi.
	closeTo(t, "events_miles", got.EventsMiles, 3.0)
}

func TestReconcile_IsIdempotentAndRecomputesInPlace(t *testing.T) {
	db := testdb.New(t)
	parkWatermark(t, db)
	addSession(t, db, "repeat_visitor", 0, 90*time.Minute)

	Reconcile(context.Background(), testConf)
	first := getRollup(t, db, "twitch", "repeat_visitor")

	// A second tick with no new events short-circuits; the row must survive.
	Reconcile(context.Background(), testConf)
	if again := getRollup(t, db, "twitch", "repeat_visitor"); again != first {
		t.Errorf("second tick changed the row: %+v -> %+v", first, again)
	}

	// Rewinding the watermark forces a full recompute. The ON CONFLICT path
	// must update the same row to the same values, not duplicate it.
	if err := db.Exec(`UPDATE rollup_watermarks SET last_event_id = 0 WHERE name = ?`, watermarkName).Error; err != nil {
		t.Fatalf("rewind watermark: %v", err)
	}
	Reconcile(context.Background(), testConf)

	if rebuilt := getRollup(t, db, "twitch", "repeat_visitor"); rebuilt != first {
		t.Errorf("full recompute is not self-healing: %+v -> %+v", first, rebuilt)
	}
	if n := countRows(t, db, `SELECT COUNT(*) FROM user_rollups WHERE username = 'repeat_visitor'`); n != 1 {
		t.Errorf("expected 1 row after recompute, got %d", n)
	}
}

func TestReconcile_KeepsPlatformsSeparate(t *testing.T) {
	db := testdb.New(t)
	parkWatermark(t, db)

	addEvent(t, db, "twitch", "twin", "login", base, 0)
	addEvent(t, db, "twitch", "twin", "logout", base.Add(90*time.Minute), 0)
	addEvent(t, db, "youtube", "twin", "login", base, 0)
	addEvent(t, db, "youtube", "twin", "logout", base.Add(30*time.Minute), 0)

	Reconcile(context.Background(), testConf)

	closeTo(t, "twitch events_miles", getRollup(t, db, "twitch", "twin").EventsMiles, 3.0)
	closeTo(t, "youtube events_miles", getRollup(t, db, "youtube", "twin").EventsMiles, 1.0)
}

func addBoard(t *testing.T, db *gorm.DB, name string) int64 {
	t.Helper()
	var id int64
	if err := db.Raw(`INSERT INTO scoreboards (name, platform) VALUES (?, 'twitch') RETURNING id`, name).Scan(&id).Error; err != nil {
		t.Fatalf("insert scoreboard %s: %v", name, err)
	}
	return id
}

// addScore wires up the users + scores rows a snapshot reads.
func addScore(t *testing.T, db *gorm.DB, boardID int64, username string, isBot bool, value float64) {
	t.Helper()
	var userID int64
	err := db.Raw(`INSERT INTO users (username, platform, is_bot) VALUES (?, 'twitch', ?) RETURNING id`,
		username, isBot).Scan(&userID).Error
	if err != nil {
		t.Fatalf("insert user %s: %v", username, err)
	}
	err = db.Exec(`INSERT INTO scores (user_id, scoreboard_id, value) VALUES (?, ?, ?)`,
		userID, boardID, value).Error
	if err != nil {
		t.Fatalf("insert score for %s: %v", username, err)
	}
}

// addOptedOutScore wires up the same rows for a human who has asked to stay
// off the leaderboards.
func addOptedOutScore(t *testing.T, db *gorm.DB, boardID int64, username string, value float64) {
	t.Helper()
	var userID int64
	err := db.Raw(`INSERT INTO users (username, platform, exclude_from_leaderboard)
	               VALUES (?, 'twitch', TRUE) RETURNING id`, username).Scan(&userID).Error
	if err != nil {
		t.Fatalf("insert opted-out user %s: %v", username, err)
	}
	err = db.Exec(`INSERT INTO scores (user_id, scoreboard_id, value) VALUES (?, ?, ?)`,
		userID, boardID, value).Error
	if err != nil {
		t.Fatalf("insert score for %s: %v", username, err)
	}
}

func TestReconcile_SnapshotsPreviousMonthScoreboard(t *testing.T) {
	db := testdb.New(t)
	parkWatermark(t, db)

	board := scoreboards.PreviousMilesScoreboard()
	boardID := addBoard(t, db, board)
	addScore(t, db, boardID, "silver", false, 30)
	addScore(t, db, boardID, "gold", false, 60)
	addScore(t, db, boardID, "tripbot4000", true, 999)
	addOptedOutScore(t, db, boardID, "adanalife_", 998)

	Reconcile(context.Background(), testConf)

	type snapshotRow struct {
		Rank     int
		Username string
		Value    float64
		Platform string
	}
	var rows []snapshotRow
	err := db.Raw(`SELECT rank, username, value, platform FROM scoreboard_snapshots
	               WHERE scoreboard_name = ? ORDER BY rank`, board).Scan(&rows).Error
	if err != nil {
		t.Fatalf("read snapshots: %v", err)
	}

	want := []snapshotRow{
		{Rank: 1, Username: "gold", Value: 60, Platform: "twitch"},
		{Rank: 2, Username: "silver", Value: 30, Platform: "twitch"},
	}
	if len(rows) != len(want) {
		t.Fatalf("snapshot rows: got %+v, want %+v (bots and opted-out accounts are excluded)", rows, want)
	}
	for i := range want {
		if rows[i] != want[i] {
			t.Errorf("snapshot row %d: got %+v, want %+v", i, rows[i], want[i])
		}
	}

	// The NOT EXISTS guard makes the freeze once-only: a later tick must not
	// re-snapshot, even after the board gains a score.
	addScore(t, db, boardID, "latecomer", false, 100)
	Reconcile(context.Background(), testConf)

	if n := countRows(t, db, `SELECT COUNT(*) FROM scoreboard_snapshots WHERE scoreboard_name = ?`, board); n != 2 {
		t.Errorf("snapshot re-ran: got %d rows, want 2", n)
	}
}

func TestReconcile_SnapshotCapsAtTopFifty(t *testing.T) {
	db := testdb.New(t)
	parkWatermark(t, db)

	board := scoreboards.PreviousGuessScoreboard()
	boardID := addBoard(t, db, board)
	// 55 contenders scored by their index, so guesser_55 is rank 1 and the
	// bottom five fall outside the cap.
	for i := 1; i <= 55; i++ {
		addScore(t, db, boardID, fmt.Sprintf("guesser_%d", i), false, float64(i))
	}

	Reconcile(context.Background(), testConf)

	if n := countRows(t, db, `SELECT COUNT(*) FROM scoreboard_snapshots WHERE scoreboard_name = ?`, board); n != 50 {
		t.Errorf("snapshot should cap at 50 rows, got %d", n)
	}
	var top string
	if err := db.Raw(`SELECT username FROM scoreboard_snapshots WHERE scoreboard_name = ? AND rank = 1`, board).Scan(&top).Error; err != nil {
		t.Fatalf("read rank 1: %v", err)
	}
	if top != "guesser_55" {
		t.Errorf("rank 1: got %q, want %q", top, "guesser_55")
	}
	if n := countRows(t, db, `SELECT COUNT(*) FROM scoreboard_snapshots
	                          WHERE scoreboard_name = ? AND value <= 5`, board); n != 0 {
		t.Errorf("bottom-5 scores should be cut by the top-50 cap, got %d", n)
	}
}

// dailyRow mirrors a scoreboard_daily_snapshots placement.
type dailyRow struct {
	Rank     int
	Username string
	Value    float64
}

func dailyRows(t *testing.T, db *gorm.DB, day, board string) []dailyRow {
	t.Helper()
	var rows []dailyRow
	err := db.Raw(`SELECT rank, username, value FROM scoreboard_daily_snapshots
	               WHERE day = ? AND scoreboard_name = ? ORDER BY rank`, day, board).Scan(&rows).Error
	if err != nil {
		t.Fatalf("read daily snapshots: %v", err)
	}
	return rows
}

func TestReconcile_DailySnapshotIsOncePerDay(t *testing.T) {
	db := testdb.New(t)
	parkWatermark(t, db)
	today := time.Now().Format("2006-01-02")

	board := scoreboards.CurrentMilesScoreboard()
	boardID := addBoard(t, db, board)
	addScore(t, db, boardID, "daily_silver", false, 30)
	addScore(t, db, boardID, "daily_gold", false, 60)
	addScore(t, db, boardID, "daily_bot", true, 999)
	addOptedOutScore(t, db, boardID, "daily_optout", 998)

	Reconcile(context.Background(), testConf)

	want := []dailyRow{{1, "daily_gold", 60}, {2, "daily_silver", 30}}
	got := dailyRows(t, db, today, board)
	if len(got) != len(want) {
		t.Fatalf("daily snapshot: got %+v, want %+v (bots and opted-out accounts are excluded)", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("daily row %d: got %+v, want %+v", i, got[i], want[i])
		}
	}

	// The day's first snapshot is the baseline: a later tick the same day
	// leaves it alone, even after the board moves.
	addScore(t, db, boardID, "daily_latecomer", false, 100)
	Reconcile(context.Background(), testConf)
	if n := countRows(t, db, `SELECT COUNT(*) FROM scoreboard_daily_snapshots WHERE day = ? AND scoreboard_name = ?`,
		today, board); n != 2 {
		t.Errorf("daily snapshot re-ran the same day: got %d rows, want 2", n)
	}

	// The month list reads scoreboard_snapshots only; a daily snapshot must
	// not freeze the live month into it.
	if n := countRows(t, db, `SELECT COUNT(*) FROM scoreboard_snapshots WHERE scoreboard_name = ?`, board); n != 0 {
		t.Errorf("daily snapshot leaked %d rows into scoreboard_snapshots", n)
	}
}

func TestReconcile_PrunesOldDailySnapshots(t *testing.T) {
	db := testdb.New(t)
	parkWatermark(t, db)

	old := time.Now().AddDate(0, 0, -41).Format("2006-01-02")
	recent := time.Now().AddDate(0, 0, -39).Format("2006-01-02")
	for _, day := range []string{old, recent} {
		err := db.Exec(`INSERT INTO scoreboard_daily_snapshots (day, scoreboard_name, platform, rank, username, value)
		                VALUES (?, 'miles_2026_01', 'twitch', 1, 'pruned_viewer', 5)`, day).Error
		if err != nil {
			t.Fatalf("seed daily snapshot %s: %v", day, err)
		}
	}

	Reconcile(context.Background(), testConf)

	if n := countRows(t, db, `SELECT COUNT(*) FROM scoreboard_daily_snapshots WHERE day = ?`, old); n != 0 {
		t.Errorf("41-day-old snapshot survived the prune: %d rows", n)
	}
	if n := countRows(t, db, `SELECT COUNT(*) FROM scoreboard_daily_snapshots WHERE day = ?`, recent); n != 1 {
		t.Errorf("39-day-old snapshot was pruned: %d rows, want 1", n)
	}
}
