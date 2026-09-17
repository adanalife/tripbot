package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/adanalife/tripbot/pkg/database/testdb"
	"gorm.io/gorm"
)

// The empty-data contract for the stats page: arrays are [], the nullable
// scalars (since, hours, peak_chatters) are null, counters are zero.
func TestLifetimeStatsHandler_EmptyDataShape(t *testing.T) {
	testdb.New(t)
	rec := insightsGET(t, "/api/stats/lifetime", lifetimeStatsHandler)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{`"since":null`, `"events_total":0`, `"event_kinds":[]`,
		`"known":0`, `"bots":0`, `"sessions":0`, `"events_miles":0`, `"extra_miles":0`,
		`"clips":0`, `"states":0`, `"hours":null`, `"peak_chatters":null`} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %s: %s", want, body)
		}
	}
}

func TestPlaybackStatsHandler_EmptyDataShape(t *testing.T) {
	testdb.New(t)
	rec := insightsGET(t, "/api/stats/playback", playbackStatsHandler)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{`"days":7`, `"plays":0`, `"distinct_clips":0`,
		`"crossings":0`, `"sequential_crossings":0`, `"timewarps":0`, `"guesses":0`, `"commands":0`} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %s: %s", want, body)
		}
	}
}

func TestCommunityStatsHandler_EmptyDataShape(t *testing.T) {
	testdb.New(t)
	rec := insightsGET(t, "/api/stats/community", communityStatsHandler)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{`"days":30`, `"subscribes":0`, `"unsubscribes":0`,
		`"corrections":0`, `"top_miles":[]`, `"boards":[]`} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %s: %s", want, body)
		}
	}
}

// The stats endpoints clamp ?days through the same parser as the insights
// ones; these pin each endpoint's wiring (default + clamp), not the parser
// itself (TestInsightsDays covers the table).
func TestStatsHandlers_ClampDays(t *testing.T) {
	testdb.New(t)
	rec := insightsGET(t, "/api/stats/playback?days=500", playbackStatsHandler)
	if !strings.Contains(rec.Body.String(), `"days":90`) {
		t.Errorf("playback days=500 should clamp to 90: %s", rec.Body.String())
	}
	rec = insightsGET(t, "/api/stats/community?days=0", communityStatsHandler)
	if !strings.Contains(rec.Body.String(), `"days":1`) {
		t.Errorf("community days=0 should clamp to 1: %s", rec.Body.String())
	}
}

// seedPlainEvent writes an events row with no meta payload, the way the
// presence/subscription writers do.
func seedPlainEvent(t *testing.T, db *gorm.DB, username, event string, at time.Time) {
	t.Helper()
	err := db.Exec(`INSERT INTO events (platform, username, event, date_created)
	                VALUES ('twitch', ?, ?, ?)`, username, event, at).Error
	if err != nil {
		t.Fatalf("insert %s event for %s: %v", event, username, err)
	}
}

// seedRollup writes a user_rollups row the reconciler would have produced.
func seedRollup(t *testing.T, db *gorm.DB, username string, sessions int, eventsMiles, extraMiles float64) {
	t.Helper()
	err := db.Exec(`INSERT INTO user_rollups (platform, username, events_miles, session_count, extra_miles)
	                VALUES ('twitch', ?, ?, ?, ?)`, username, eventsMiles, sessions, extraMiles).Error
	if err != nil {
		t.Fatalf("insert rollup for %s: %v", username, err)
	}
}

func TestLifetimeStatsHandler_Aggregates(t *testing.T) {
	db := testdb.New(t)
	// Postgres stores microseconds; truncate so the round trip compares equal.
	base := time.Now().Add(-2 * time.Hour).UTC().Truncate(time.Microsecond)

	// A sentinel-dated row from the old insert bug: counted in the census,
	// ignored by since.
	sentinel := time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC)
	seedPlainEvent(t, db, "stats_alice", "login", sentinel)
	seedPlainEvent(t, db, "stats_alice", "login", base)
	seedPlainEvent(t, db, "stats_alice", "logout", base.Add(time.Hour))
	seedPlainEvent(t, db, "stats_bob", "login", base.Add(time.Minute))

	seedUser(t, db, "stats_alice", false)
	seedUser(t, db, "stats_bob", false)
	seedUser(t, db, "stats_bot", true)
	seedRollup(t, db, "stats_alice", 5, 10.5, 1.5)
	seedRollup(t, db, "stats_bob", 3, 2.0, 0)
	seedRollup(t, db, "stats_bot", 99, 999, 9)

	inside := 0.0
	seedVideo(t, db, "stats_clip_a", "Utah", "Moab", &inside)
	seedVideo(t, db, "stats_clip_b", "Utah", "", nil)
	seedVideo(t, db, "stats_clip_c", "", "", nil)

	peakAt := base.Add(30 * time.Minute)
	vid := seedVideo(t, db, "stats_clip_peak", "", "", nil)
	seedSample(t, db, vid, 3, base)
	seedSample(t, db, vid, 9, peakAt)
	seedSample(t, db, vid, 9, peakAt.Add(time.Minute)) // tie: the earlier peak wins

	rec := insightsGET(t, "/api/stats/lifetime", lifetimeStatsHandler)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var got lifetimeStatsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v\n%s", err, rec.Body.String())
	}

	if got.Since == nil || !got.Since.Equal(base) {
		t.Errorf("since = %v, want %v (the sentinel row must not win)", got.Since, base)
	}
	if got.EventsTotal != 4 {
		t.Errorf("events_total = %d, want 4 (the census includes sentinel rows)", got.EventsTotal)
	}
	wantKinds := []eventKindCount{{Event: "login", Count: 3}, {Event: "logout", Count: 1}}
	if len(got.EventKinds) != len(wantKinds) {
		t.Fatalf("event_kinds = %+v, want %+v", got.EventKinds, wantKinds)
	}
	for i := range wantKinds {
		if got.EventKinds[i] != wantKinds[i] {
			t.Errorf("event_kinds[%d] = %+v, want %+v", i, got.EventKinds[i], wantKinds[i])
		}
	}

	wantUsers := lifetimeUsers{Known: 2, Bots: 1, Sessions: 8, EventsMiles: 12.5, ExtraMiles: 1.5}
	if got.Users != wantUsers {
		t.Errorf("users = %+v, want %+v (bot rollups excluded, bots counted apart)", got.Users, wantUsers)
	}

	if got.Corpus.Clips != 4 || got.Corpus.States != 1 || got.Corpus.Hours != nil {
		t.Errorf("corpus = %+v, want 4 clips, 1 state, null hours", got.Corpus)
	}

	if got.PeakChatters == nil || got.PeakChatters.Count != 9 || !got.PeakChatters.At.Equal(peakAt) {
		t.Errorf("peak_chatters = %+v, want count 9 at %v", got.PeakChatters, peakAt)
	}
}

func TestPlaybackStatsHandler_Aggregates(t *testing.T) {
	db := testdb.New(t)
	seedUser(t, db, "play_alice", false)
	seedUser(t, db, "play_bob", false)
	seedUser(t, db, "play_bot", true)
	in := time.Now().Add(-1 * time.Hour)
	out := time.Now().Add(-10 * 24 * time.Hour)

	vid := seedVideo(t, db, "stats_clip_play", "Utah", "", nil)
	seedPlay(t, db, vid, in)
	seedPlay(t, db, vid, in.Add(time.Minute))
	seedPlay(t, db, vid, out) // stale: not counted
	// A play whose clip had no DB row still counts as a play, but not as a
	// distinct clip.
	if err := db.Exec(`INSERT INTO video_plays (platform, started_at) VALUES ('twitch', ?)`, in).Error; err != nil {
		t.Fatalf("insert rowless play: %v", err)
	}

	// state_crossing is a system event: empty username, no users row.
	seedEvent(t, db, "", "state_crossing", `{"from":"utah","to":"colorado","sequential":true}`, in)
	seedEvent(t, db, "", "state_crossing", `{"from":"colorado","to":"kansas","sequential":true}`, in.Add(time.Minute))
	seedEvent(t, db, "", "state_crossing", `{"from":"kansas","to":"utah","sequential":false}`, in.Add(2*time.Minute))
	seedEvent(t, db, "", "state_crossing", `{"from":"utah","to":"idaho","sequential":true}`, out) // stale

	seedEvent(t, db, "play_alice", "timewarp", `{}`, in)
	seedEvent(t, db, "play_bot", "timewarp", `{}`, in) // bot: not counted
	seedEvent(t, db, "play_bob", "guess_submitted", `{"guessed":"utah","actual":"utah","correct":true}`, in)
	seedEvent(t, db, "play_alice", "command_run", `{"command":"!guess"}`, in)
	seedEvent(t, db, "play_alice", "command_run", `{"command":"!miles"}`, in.Add(time.Minute))
	seedEvent(t, db, "play_bot", "command_run", `{"command":"!miles"}`, in) // bot: not counted

	rec := insightsGET(t, "/api/stats/playback?days=7", playbackStatsHandler)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var got playbackStatsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v\n%s", err, rec.Body.String())
	}

	want := playbackStatsResponse{Days: 7, Plays: 3, DistinctClips: 1,
		Crossings: 3, SequentialCrossings: 2, Timewarps: 1, Guesses: 1, Commands: 2}
	if got != want {
		t.Errorf("playback = %+v, want %+v", got, want)
	}
}

// seedMilesUser writes a users row with a display-miles total and leaderboard
// eligibility flags.
func seedMilesUser(t *testing.T, db *gorm.DB, username string, miles float64, isBot, optedOut bool) {
	t.Helper()
	err := db.Exec(`INSERT INTO users (username, platform, miles, is_bot, exclude_from_leaderboard)
	                VALUES (?, 'twitch', ?, ?, ?)`, username, miles, isBot, optedOut).Error
	if err != nil {
		t.Fatalf("insert user %s: %v", username, err)
	}
}

func seedSnapshot(t *testing.T, db *gorm.DB, board string, rank int, username string, value float64) {
	t.Helper()
	err := db.Exec(`INSERT INTO scoreboard_snapshots (scoreboard_name, platform, rank, username, value)
	                VALUES (?, 'twitch', ?, ?, ?)`, board, rank, username, value).Error
	if err != nil {
		t.Fatalf("insert snapshot %s rank %d: %v", board, rank, err)
	}
}

func TestCommunityStatsHandler_Aggregates(t *testing.T) {
	db := testdb.New(t)
	seedMilesUser(t, db, "comm_alice", 500.5, false, false)
	seedMilesUser(t, db, "comm_bob", 100, false, false)
	seedMilesUser(t, db, "comm_bot", 999, true, false)    // bot: never listed
	seedMilesUser(t, db, "comm_optout", 998, false, true) // opted out: never listed
	in := time.Now().Add(-1 * time.Hour)
	out := time.Now().Add(-40 * 24 * time.Hour)

	seedPlainEvent(t, db, "comm_alice", "subscribe", in)
	seedPlainEvent(t, db, "comm_alice", "subscribe", in.Add(time.Minute))
	seedPlainEvent(t, db, "comm_bob", "unsubscribe", in)
	seedPlainEvent(t, db, "comm_alice", "correction", in)
	seedPlainEvent(t, db, "comm_bot", "subscribe", in)  // bot: not counted
	seedPlainEvent(t, db, "comm_bob", "subscribe", out) // stale: not counted

	// Two months of snapshots: only 2026_07 (the latest) reports, capped at
	// the top 5 of miles_2026_07's seven ranks.
	seedSnapshot(t, db, "miles_2026_06", 1, "old_winner", 50)
	for rank, entry := range []struct {
		name  string
		value float64
	}{{"comm_alice", 70}, {"comm_bob", 60}, {"g3", 50}, {"g4", 40}, {"g5", 30}, {"g6", 20}, {"g7", 10}} {
		seedSnapshot(t, db, "miles_2026_07", rank+1, entry.name, entry.value)
	}
	seedSnapshot(t, db, "guess_state_2026_07", 1, "comm_bob", 12)
	seedSnapshot(t, db, "guess_state_2026_07", 2, "comm_alice", 3)

	rec := insightsGET(t, "/api/stats/community", communityStatsHandler)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var got communityStatsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v\n%s", err, rec.Body.String())
	}

	if got.Days != 30 || got.Subscribes != 2 || got.Unsubscribes != 1 || got.Corrections != 1 {
		t.Errorf("counts = %+v, want days 30, subscribes 2, unsubscribes 1, corrections 1", got)
	}

	wantTop := []topMilesEntry{
		{Username: "comm_alice", Platform: "twitch", Miles: 500.5},
		{Username: "comm_bob", Platform: "twitch", Miles: 100},
	}
	if len(got.TopMiles) != len(wantTop) {
		t.Fatalf("top_miles = %+v, want %+v (bots and opted-out excluded)", got.TopMiles, wantTop)
	}
	for i := range wantTop {
		if got.TopMiles[i] != wantTop[i] {
			t.Errorf("top_miles[%d] = %+v, want %+v", i, got.TopMiles[i], wantTop[i])
		}
	}

	if len(got.Boards) != 2 {
		t.Fatalf("boards = %+v, want the two 2026_07 boards", got.Boards)
	}
	guess, miles := got.Boards[0], got.Boards[1] // name order: guess_state before miles
	if guess.Board != "guess_state_2026_07" || len(guess.Rows) != 2 {
		t.Errorf("boards[0] = %+v, want guess_state_2026_07 with 2 rows", guess)
	}
	if miles.Board != "miles_2026_07" || len(miles.Rows) != 5 {
		t.Fatalf("boards[1] = %+v, want miles_2026_07 capped at 5 rows", miles)
	}
	wantRow := boardRow{Rank: 1, Username: "comm_alice", Value: 70}
	if miles.Rows[0] != wantRow {
		t.Errorf("miles rows[0] = %+v, want %+v", miles.Rows[0], wantRow)
	}
	if miles.Rows[4].Rank != 5 || miles.Rows[4].Username != "g5" {
		t.Errorf("miles rows[4] = %+v, want rank 5 g5", miles.Rows[4])
	}
}
