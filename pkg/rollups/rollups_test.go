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
	RealMiles    float64
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
	err := db.Raw(`SELECT id, platform, username, events_miles, real_miles, session_count, first_seen, last_seen, extra_miles
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

func TestReconcile_ReadOnlySkipsEverything(t *testing.T) {
	db := testdb.New(t)
	parkWatermark(t, db)
	wm := watermark(t, db)
	addSession(t, db, "readonly_user", 0, 90*time.Minute)

	orig := c.Conf.ReadOnly
	c.Conf.ReadOnly = true
	t.Cleanup(func() { c.Conf.ReadOnly = orig })

	Reconcile(context.Background())

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

	Reconcile(context.Background())

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

	Reconcile(context.Background())

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

func TestReconcile_DropsSessionsOverTwentyFourHours(t *testing.T) {
	db := testdb.New(t)
	parkWatermark(t, db)

	// A missed logout: the pair spans 26h and earns nothing. The following
	// 60min session still pairs (2nd login <-> 2nd logout) and counts.
	addSession(t, db, "forgot_to_leave", 0, 26*time.Hour)
	addSession(t, db, "forgot_to_leave", 27*time.Hour, 60*time.Minute)

	Reconcile(context.Background())

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

	Reconcile(context.Background())

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

	Reconcile(context.Background())

	got := getRollup(t, db, "twitch", "bonus_haver")
	closeTo(t, "extra_miles", got.ExtraMiles, 3.0)
	// The bonus stays out of the pairing base: 90min is still 3.0mi.
	closeTo(t, "events_miles", got.EventsMiles, 3.0)
}

func TestReconcile_IsIdempotentAndRecomputesInPlace(t *testing.T) {
	db := testdb.New(t)
	parkWatermark(t, db)
	addSession(t, db, "repeat_visitor", 0, 90*time.Minute)

	Reconcile(context.Background())
	first := getRollup(t, db, "twitch", "repeat_visitor")

	// A second tick with no new events short-circuits; the row must survive.
	Reconcile(context.Background())
	if again := getRollup(t, db, "twitch", "repeat_visitor"); again != first {
		t.Errorf("second tick changed the row: %+v -> %+v", first, again)
	}

	// Rewinding the watermark forces a full recompute. The ON CONFLICT path
	// must update the same row to the same values, not duplicate it.
	if err := db.Exec(`UPDATE rollup_watermarks SET last_event_id = 0 WHERE name = ?`, watermarkName).Error; err != nil {
		t.Fatalf("rewind watermark: %v", err)
	}
	Reconcile(context.Background())

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

	Reconcile(context.Background())

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

func TestReconcile_SnapshotsPreviousMonthScoreboard(t *testing.T) {
	db := testdb.New(t)
	parkWatermark(t, db)

	board := scoreboards.PreviousMilesScoreboard()
	boardID := addBoard(t, db, board)
	addScore(t, db, boardID, "silver", false, 30)
	addScore(t, db, boardID, "gold", false, 60)
	addScore(t, db, boardID, "tripbot4000", true, 999)

	Reconcile(context.Background())

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
		t.Fatalf("snapshot rows: got %+v, want %+v (bots are excluded)", rows, want)
	}
	for i := range want {
		if rows[i] != want[i] {
			t.Errorf("snapshot row %d: got %+v, want %+v", i, rows[i], want[i])
		}
	}

	// The NOT EXISTS guard makes the freeze once-only: a later tick must not
	// re-snapshot, even after the board gains a score.
	addScore(t, db, boardID, "latecomer", false, 100)
	Reconcile(context.Background())

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

	Reconcile(context.Background())

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

// addVideo writes one videos row carrying a miles_driven value (NULL when
// miles is 0) and returns its id.
func addVideo(t *testing.T, db *gorm.DB, slug string, miles float64) int64 {
	t.Helper()
	var m any
	if miles != 0 {
		m = miles
	}
	var id int64
	err := db.Raw(`INSERT INTO videos (slug, lat, lng, date_filmed, miles_driven)
	               VALUES (?, 40.0, -111.0, ?, ?) RETURNING id`, slug, base, m).Scan(&id).Error
	if err != nil {
		t.Fatalf("insert video %s: %v", slug, err)
	}
	return id
}

// addPlay writes one video_plays row at `at`.
func addPlay(t *testing.T, db *gorm.DB, platform string, videoID int64, at time.Time) {
	t.Helper()
	err := db.Exec(`INSERT INTO video_plays (platform, video_id, started_at)
	                VALUES (?, ?, ?)`, platform, videoID, at).Error
	if err != nil {
		t.Fatalf("insert play of video %d: %v", videoID, err)
	}
}

func TestReconcile_ComputesRealMilesFromVideoPlays(t *testing.T) {
	db := testdb.New(t)
	parkWatermark(t, db)

	shortClip := addVideo(t, db, "rm_short", 1.5)
	longClip := addVideo(t, db, "rm_long", 2.25)
	unknownClip := addVideo(t, db, "rm_unknown", 0) // miles_driven NULL

	// 60-minute session; plays inside the window sum to 1.5 + 2.25.
	addSession(t, db, "roadtripper", 0, 60*time.Minute)
	addPlay(t, db, "twitch", shortClip, base.Add(5*time.Minute))
	addPlay(t, db, "twitch", longClip, base.Add(10*time.Minute))
	// None of these count: unknown distance, before login, after logout,
	// and another platform's stream.
	addPlay(t, db, "twitch", unknownClip, base.Add(15*time.Minute))
	addPlay(t, db, "twitch", shortClip, base.Add(-5*time.Minute))
	addPlay(t, db, "twitch", shortClip, base.Add(65*time.Minute))
	addPlay(t, db, "youtube", longClip, base.Add(20*time.Minute))

	Reconcile(context.Background())

	got := getRollup(t, db, "twitch", "roadtripper")
	closeTo(t, "real_miles", got.RealMiles, 3.75)
	// The plain time-based rate is untouched by the plays.
	closeTo(t, "events_miles", got.EventsMiles, 2.0)
}

func TestReconcile_RealMilesZeroWithoutPlays(t *testing.T) {
	db := testdb.New(t)
	parkWatermark(t, db)

	addSession(t, db, "quiet_watcher", 0, 30*time.Minute)

	Reconcile(context.Background())

	got := getRollup(t, db, "twitch", "quiet_watcher")
	closeTo(t, "real_miles", got.RealMiles, 0)
}
