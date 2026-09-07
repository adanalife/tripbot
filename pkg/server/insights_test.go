package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/adanalife/tripbot/pkg/database/testdb"
	"github.com/gorilla/mux"
	"gorm.io/gorm"
)

func TestInsightsDays(t *testing.T) {
	cases := []struct {
		query string
		def   int
		want  int
	}{
		{"", 7, 7},                // absent -> default
		{"days=abc", 30, 30},      // unparseable -> default
		{"days=", 7, 7},           // empty value -> default
		{"days=0", 7, 1},          // below range -> min
		{"days=-5", 7, 1},         // negative -> min
		{"days=500", 7, 90},       // above range -> max
		{"days=30", 7, 30},        // in range -> as asked
		{"days=1", 30, 1},         // boundary min
		{"days=90", 30, 90},       // boundary max
		{"days=2.5", 30, 30},      // non-integer -> default
		{"days=7&days=90", 30, 7}, // first value wins, mux-style
	}
	for _, c := range cases {
		r := httptest.NewRequest(http.MethodGet, "/api/insights/commands?"+c.query, nil)
		if got := insightsDays(r, c.def); got != c.want {
			t.Errorf("insightsDays(%q, %d) = %d, want %d", c.query, c.def, got, c.want)
		}
	}
}

func TestClipLabel(t *testing.T) {
	near := 5000.0
	far := 50000.0
	inside := 0.0
	cases := []struct {
		name  string
		state string
		city  string
		cityM *float64
		want  string
	}{
		{"inside a city", "Utah", "Moab", &inside, "Moab, Utah"},
		{"near a city", "Utah", "Moab", &near, "Moab, Utah"},
		{"city too far to claim", "Nevada", "Ely", &far, "Nevada"},
		{"city without a distance", "Nevada", "Ely", nil, "Nevada"},
		{"state only", "Utah", "", nil, "Utah"},
		{"nothing named yet", "", "", nil, ""},
		{"city without a state", "", "Moab", &inside, "Moab"},
	}
	for _, c := range cases {
		if got := clipLabel(c.state, c.city, c.cityM); got != c.want {
			t.Errorf("%s: clipLabel(%q, %q, %v) = %q, want %q",
				c.name, c.state, c.city, c.cityM, got, c.want)
		}
	}
}

// insightsGET routes one GET through the real mux registration shape and
// returns the recorder.
func insightsGET(t *testing.T, path string, handler http.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	r := mux.NewRouter()
	r.Handle(strings.SplitN(path, "?", 2)[0], handler).Methods("GET")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

// The empty-data contract: every list renders as [], never null — the console
// iterates these fields without null checks.
func TestCommandInsightsHandler_EmptyDataShape(t *testing.T) {
	testdb.New(t)
	rec := insightsGET(t, "/api/insights/commands", commandInsightsHandler)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{`"days":7`, `"commands":[]`, `"refusal_reasons":[]`, `"unknown_commands":[]`} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %s: %s", want, body)
		}
	}
}

func TestGuessInsightsHandler_EmptyDataShape(t *testing.T) {
	testdb.New(t)
	rec := insightsGET(t, "/api/insights/guesses", guessInsightsHandler)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{`"days":30`, `"total":0`, `"correct":0`, `"players":0`, `"states":[]`} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %s: %s", want, body)
		}
	}
}

func TestFootageInsightsHandler_EmptyDataShape(t *testing.T) {
	testdb.New(t)
	rec := insightsGET(t, "/api/insights/footage", footageInsightsHandler)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{`"days":7`, `"clips":[]`, `"platforms":[]`} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %s: %s", want, body)
		}
	}
}

// seedUser inserts a users row; the insights queries join through it to
// exclude bots.
func seedUser(t *testing.T, db *gorm.DB, username string, isBot bool) {
	t.Helper()
	err := db.Exec(`INSERT INTO users (username, platform, is_bot) VALUES (?, 'twitch', ?)`,
		username, isBot).Error
	if err != nil {
		t.Fatalf("insert user %s: %v", username, err)
	}
}

// seedEvent inserts one events row with a JSON meta payload. Timestamps are
// explicit, offset from time.Now() — the queries window on now(), which is
// frozen inside the test transaction.
func seedEvent(t *testing.T, db *gorm.DB, username, event, meta string, at time.Time) {
	t.Helper()
	err := db.Exec(`INSERT INTO events (platform, username, event, meta, date_created)
	                VALUES ('twitch', ?, ?, ?, ?)`, username, event, meta, at).Error
	if err != nil {
		t.Fatalf("insert %s event for %s: %v", event, username, err)
	}
}

func TestCommandInsightsHandler_Aggregates(t *testing.T) {
	db := testdb.New(t)
	seedUser(t, db, "cmd_alice", false)
	seedUser(t, db, "cmd_bob", false)
	seedUser(t, db, "cmd_bot", true)
	in := time.Now().Add(-1 * time.Hour)
	out := time.Now().Add(-10 * 24 * time.Hour)

	// !guess: three non-bot runs by two users; the bot's run and the stale run
	// must not count.
	seedEvent(t, db, "cmd_alice", "command_run", `{"command":"!guess","args":"utah"}`, in)
	seedEvent(t, db, "cmd_alice", "command_run", `{"command":"!guess","args":"ohio"}`, in.Add(time.Minute))
	seedEvent(t, db, "cmd_bob", "command_run", `{"command":"!guess","args":"utah"}`, in.Add(2*time.Minute))
	seedEvent(t, db, "cmd_bot", "command_run", `{"command":"!guess","args":"utah"}`, in)
	seedEvent(t, db, "cmd_alice", "command_run", `{"command":"!guess","args":"utah"}`, out)
	// One refusal against the same command...
	seedEvent(t, db, "cmd_alice", "command_refused", `{"command":"!guess","reason":"cooldown"}`, in)
	// ...a command that only ever gets refused (gate-heavy, runs 0)...
	seedEvent(t, db, "cmd_bob", "command_refused", `{"command":"!timewarp","reason":"sub_gate"}`, in)
	// ...and unknown tokens: one stores only the raw token in command, one
	// carries a differing typed form.
	seedEvent(t, db, "cmd_bob", "command_refused", `{"command":"!lurk","reason":"unknown"}`, in)
	seedEvent(t, db, "cmd_bob", "command_refused", `{"command":"!lurk","reason":"unknown"}`, in.Add(time.Minute))
	seedEvent(t, db, "cmd_alice", "command_refused", `{"command":"!points","typed":"!Points","reason":"unknown"}`, in)

	rec := insightsGET(t, "/api/insights/commands?days=7", commandInsightsHandler)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var got commandInsightsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v\n%s", err, rec.Body.String())
	}

	if got.Days != 7 {
		t.Errorf("days = %d, want 7", got.Days)
	}
	wantCommands := []commandUsage{
		{Command: "!guess", Runs: 3, Users: 2, Refusals: 1},
		{Command: "!lurk", Runs: 0, Users: 0, Refusals: 2},
		{Command: "!points", Runs: 0, Users: 0, Refusals: 1},
		{Command: "!timewarp", Runs: 0, Users: 0, Refusals: 1},
	}
	if len(got.Commands) != len(wantCommands) {
		t.Fatalf("commands = %+v, want %+v", got.Commands, wantCommands)
	}
	for i := range wantCommands {
		if got.Commands[i] != wantCommands[i] {
			t.Errorf("commands[%d] = %+v, want %+v", i, got.Commands[i], wantCommands[i])
		}
	}

	wantReasons := []refusalReason{
		{Reason: "unknown", Count: 3},
		{Reason: "cooldown", Count: 1},
		{Reason: "sub_gate", Count: 1},
	}
	if len(got.RefusalReasons) != len(wantReasons) {
		t.Fatalf("refusal_reasons = %+v, want %+v", got.RefusalReasons, wantReasons)
	}
	for i := range wantReasons {
		if got.RefusalReasons[i] != wantReasons[i] {
			t.Errorf("refusal_reasons[%d] = %+v, want %+v", i, got.RefusalReasons[i], wantReasons[i])
		}
	}

	wantUnknown := []unknownCommand{
		{Typed: "!lurk", Count: 2},
		{Typed: "!Points", Count: 1},
	}
	if len(got.UnknownCommands) != len(wantUnknown) {
		t.Fatalf("unknown_commands = %+v, want %+v", got.UnknownCommands, wantUnknown)
	}
	for i := range wantUnknown {
		if got.UnknownCommands[i] != wantUnknown[i] {
			t.Errorf("unknown_commands[%d] = %+v, want %+v", i, got.UnknownCommands[i], wantUnknown[i])
		}
	}
}

func TestGuessInsightsHandler_Aggregates(t *testing.T) {
	db := testdb.New(t)
	seedUser(t, db, "guess_alice", false)
	seedUser(t, db, "guess_bob", false)
	seedUser(t, db, "guess_bot", true)
	in := time.Now().Add(-1 * time.Hour)
	out := time.Now().Add(-40 * 24 * time.Hour)

	// correct is a JSON boolean, the way the writer stores it.
	seedEvent(t, db, "guess_alice", "guess_submitted", `{"guessed":"florida","actual":"florida","correct":true}`, in)
	seedEvent(t, db, "guess_alice", "guess_submitted", `{"guessed":"georgia","actual":"florida","correct":false}`, in.Add(time.Minute))
	seedEvent(t, db, "guess_bob", "guess_submitted", `{"guessed":"utah","actual":"utah","correct":true}`, in)
	// A bot's guess and a stale guess must not count anywhere.
	seedEvent(t, db, "guess_bot", "guess_submitted", `{"guessed":"utah","actual":"utah","correct":true}`, in)
	seedEvent(t, db, "guess_bob", "guess_submitted", `{"guessed":"idaho","actual":"idaho","correct":true}`, out)

	rec := insightsGET(t, "/api/insights/guesses", guessInsightsHandler)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var got guessInsightsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v\n%s", err, rec.Body.String())
	}

	if got.Days != 30 || got.Total != 3 || got.Correct != 2 || got.Players != 2 {
		t.Errorf("totals = %+v, want days 30, total 3, correct 2, players 2", got)
	}
	wantStates := []stateGuesses{
		{State: "florida", Guesses: 2, Correct: 1},
		{State: "utah", Guesses: 1, Correct: 1},
	}
	if len(got.States) != len(wantStates) {
		t.Fatalf("states = %+v, want %+v", got.States, wantStates)
	}
	for i := range wantStates {
		if got.States[i] != wantStates[i] {
			t.Errorf("states[%d] = %+v, want %+v", i, got.States[i], wantStates[i])
		}
	}
}

// seedVideo inserts a videos row with place columns and returns its id.
func seedVideo(t *testing.T, db *gorm.DB, slug, state, city string, cityM *float64) int {
	t.Helper()
	var id int
	err := db.Raw(`INSERT INTO videos (slug, state, city, city_m) VALUES (?, ?, NULLIF(?, ''), ?) RETURNING id`,
		slug, state, city, cityM).Scan(&id).Error
	if err != nil {
		t.Fatalf("insert video %s: %v", slug, err)
	}
	return id
}

func seedSample(t *testing.T, db *gorm.DB, videoID, count int, at time.Time) {
	t.Helper()
	seedSampleOn(t, db, "twitch", videoID, count, at)
}

func seedSampleOn(t *testing.T, db *gorm.DB, platform string, videoID, count int, at time.Time) {
	t.Helper()
	err := db.Exec(`INSERT INTO viewer_samples (platform, count, video_id, sampled_at)
	                VALUES (?, ?, ?, ?)`, platform, count, videoID, at).Error
	if err != nil {
		t.Fatalf("insert %s sample for video %d: %v", platform, videoID, err)
	}
}

func seedPlay(t *testing.T, db *gorm.DB, videoID int, at time.Time) {
	t.Helper()
	err := db.Exec(`INSERT INTO video_plays (platform, video_id, started_at)
	                VALUES ('twitch', ?, ?)`, videoID, at).Error
	if err != nil {
		t.Fatalf("insert play for video %d: %v", videoID, err)
	}
}

func TestFootageInsightsHandler_Aggregates(t *testing.T) {
	db := testdb.New(t)
	in := time.Now().Add(-1 * time.Hour)
	out := time.Now().Add(-10 * 24 * time.Hour)

	inside := 0.0
	moab := seedVideo(t, db, "insights_clip_moab", "Utah", "Moab", &inside)
	nevada := seedVideo(t, db, "insights_clip_nv", "Nevada", "", nil)
	sparse := seedVideo(t, db, "insights_clip_sparse", "Idaho", "", nil)

	// moab: three in-window samples averaging 1.666… (rounds to 1.7); the
	// stale sample must count toward nothing.
	for i, n := range []int{1, 2, 2} {
		seedSample(t, db, moab, n, in.Add(time.Duration(i)*time.Minute))
	}
	seedSample(t, db, moab, 99, out)
	// nevada: four samples, avg 7.0 — sorts first.
	for i, n := range []int{5, 6, 7, 10} {
		seedSample(t, db, nevada, n, in.Add(time.Duration(i)*time.Minute))
	}
	// sparse: two samples, under the minimum — excluded entirely.
	seedSample(t, db, sparse, 3, in)
	seedSample(t, db, sparse, 3, in.Add(time.Minute))

	// moab aired twice in-window (the stale play doesn't count); nevada's
	// plays all predate the window, but its samples still report it.
	seedPlay(t, db, moab, in)
	seedPlay(t, db, moab, in.Add(30*time.Minute))
	seedPlay(t, db, moab, out)
	seedPlay(t, db, nevada, out)

	rec := insightsGET(t, "/api/insights/footage", footageInsightsHandler)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var got footageInsightsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v\n%s", err, rec.Body.String())
	}

	if got.Days != 7 {
		t.Errorf("days = %d, want 7", got.Days)
	}
	want := []clipInsight{
		{VideoID: nevada, Label: "Nevada", Plays: 0, AvgChatters: 7.0, MaxChatters: 10, Samples: 4},
		{VideoID: moab, Label: "Moab, Utah", Plays: 2, AvgChatters: 1.7, MaxChatters: 2, Samples: 3},
	}
	if len(got.Clips) != len(want) {
		t.Fatalf("clips = %+v, want %+v", got.Clips, want)
	}
	for i := range want {
		if got.Clips[i] != want[i] {
			t.Errorf("clips[%d] = %+v, want %+v", i, got.Clips[i], want[i])
		}
	}
}

// TestFootageInsightsHandler_PlatformsAreObserved is the honesty check on the
// chatter figures. They read as fleet-wide and are not: a platform running
// bot-less samples no chatters, so it contributes nothing to the average and
// nothing says so. The list is derived from the rows rather than declared, so
// it cannot go stale the way a hardcoded "twitch-only" note would.
func TestFootageInsightsHandler_PlatformsAreObserved(t *testing.T) {
	db := testdb.New(t)
	in := time.Now().Add(-1 * time.Hour)
	stale := time.Now().Add(-10 * 24 * time.Hour)
	clip := seedVideo(t, db, "insights_clip_platforms", "Utah", "", nil)

	for i, n := range []int{1, 2, 3} {
		seedSample(t, db, clip, n, in.Add(time.Duration(i)*time.Minute))
	}
	// Out of the window: a platform that used to sample must not still be
	// claimed as feeding these numbers.
	seedSampleOn(t, db, "youtube", clip, 4, stale)

	if got := footagePlatforms(t); !slices.Equal(got, []string{"twitch"}) {
		t.Fatalf("platforms = %v, want [twitch] — only twitch sampled in-window", got)
	}

	// One in-window youtube sample, and the list has to grow on its own. It is
	// deliberately below footageMinSamples: the question this answers is which
	// chats were being sampled at all, not which clips cleared the bar.
	seedSampleOn(t, db, "youtube", clip, 4, in)
	if got := footagePlatforms(t); !slices.Equal(got, []string{"twitch", "youtube"}) {
		t.Fatalf("platforms = %v, want [twitch youtube]", got)
	}
}

func footagePlatforms(t *testing.T) []string {
	t.Helper()
	rec := insightsGET(t, "/api/insights/footage", footageInsightsHandler)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var got footageInsightsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v\n%s", err, rec.Body.String())
	}
	return got.Platforms
}

// seedSampleFull inserts a viewer_samples row carrying the migration-047
// audience columns, which seedSample leaves NULL.
func seedSampleFull(t *testing.T, db *gorm.DB, videoID, chatters int, viewers *int, live *bool, at time.Time) {
	t.Helper()
	err := db.Exec(`INSERT INTO viewer_samples (platform, count, video_id, viewers, live, sampled_at)
	                VALUES ('twitch', ?, ?, ?, ?, ?)`, chatters, videoID, viewers, live, at).Error
	if err != nil {
		t.Fatalf("insert full sample for video %d: %v", videoID, err)
	}
}

// seedClosedPlay inserts a video_plays row with an observed end, which is
// what the region rollup can sum airtime from. A nil ended leaves the row
// open, mirroring a restart mid-clip.
func seedClosedPlay(t *testing.T, db *gorm.DB, videoID int, started time.Time, ended *time.Time) {
	t.Helper()
	err := db.Exec(`INSERT INTO video_plays (platform, video_id, started_at, ended_at)
	                VALUES ('twitch', ?, ?, ?)`, videoID, started, ended).Error
	if err != nil {
		t.Fatalf("insert closed play for video %d: %v", videoID, err)
	}
}

// seedAiringEvent inserts a login/logout stamped with the clip that was
// airing — the migration-044 columns that make the churn join a GROUP BY.
func seedAiringEvent(t *testing.T, db *gorm.DB, username, event string, videoID int, at time.Time) {
	t.Helper()
	err := db.Exec(`INSERT INTO events (platform, username, event, video_id, date_created)
	                VALUES ('twitch', ?, ?, ?, ?)`, username, event, videoID, at).Error
	if err != nil {
		t.Fatalf("insert airing %s for %s: %v", event, username, err)
	}
}

func TestRegionInsightsHandler_EmptyDataShape(t *testing.T) {
	testdb.New(t)
	rec := insightsGET(t, "/api/insights/regions", regionInsightsHandler)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{`"days":30`, `"regions":[]`, `"platforms":[]`} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %s: %s", want, body)
		}
	}
}

func regionInsights(t *testing.T, path string) regionInsightsResponse {
	t.Helper()
	rec := insightsGET(t, path, regionInsightsHandler)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var got regionInsightsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v\n%s", err, rec.Body.String())
	}
	return got
}

func TestRegionInsightsHandler_Aggregates(t *testing.T) {
	db := testdb.New(t)
	seedUser(t, db, "reg_alice", false)
	seedUser(t, db, "reg_bob", false)
	seedUser(t, db, "reg_bot", true)

	in := time.Now().Add(-1 * time.Hour)
	out := time.Now().Add(-40 * 24 * time.Hour)

	utah := seedVideo(t, db, "insights_region_ut", "Utah", "", nil)
	nevada := seedVideo(t, db, "insights_region_nv", "Nevada", "", nil)
	idaho := seedVideo(t, db, "insights_region_id", "Idaho", "", nil)
	// A clip the geocode pass has not named yet: it must not become a state
	// called "" in the report.
	unnamed := seedVideo(t, db, "insights_region_none", "", "", nil)

	live, offline := true, false
	ten, twenty, five, huge := 10, 20, 5, 999

	// Utah: twelve live samples averaging 15 viewers, plus one offline tick
	// whose 999 viewers must not touch the average.
	for i := 0; i < 6; i++ {
		seedSampleFull(t, db, utah, 2, &ten, &live, in.Add(time.Duration(i)*time.Minute))
		seedSampleFull(t, db, utah, 2, &twenty, &live, in.Add(time.Duration(6+i)*time.Minute))
	}
	seedSampleFull(t, db, utah, 500, &huge, &offline, in.Add(20*time.Minute))
	// Nevada: twelve live samples at five viewers.
	for i := 0; i < 12; i++ {
		seedSampleFull(t, db, nevada, 1, &five, &live, in.Add(time.Duration(i)*time.Minute))
	}
	// Idaho: under regionMinSamples, so it drops out entirely.
	for i := 0; i < 5; i++ {
		seedSampleFull(t, db, idaho, 9, &twenty, &live, in.Add(time.Duration(i)*time.Minute))
	}
	// The unnamed clip clears the sample bar and still must not appear.
	for i := 0; i < 12; i++ {
		seedSampleFull(t, db, unnamed, 9, &twenty, &live, in.Add(time.Duration(i)*time.Minute))
	}
	// Out of window: must count toward nothing.
	seedSampleFull(t, db, utah, 400, &huge, &live, out)

	// Utah aired two closed hours plus one play still open at the end of the
	// window; Nevada aired two closed hours in one play.
	utahEnd1 := in.Add(time.Hour)
	utahEnd2 := in.Add(2 * time.Hour)
	seedClosedPlay(t, db, utah, in, &utahEnd1)
	seedClosedPlay(t, db, utah, in.Add(time.Hour), &utahEnd2)
	seedClosedPlay(t, db, utah, in.Add(2*time.Hour), nil)
	nevadaEnd := in.Add(2 * time.Hour)
	seedClosedPlay(t, db, nevada, in, &nevadaEnd)
	// A stale play must not add airtime to the window.
	staleEnd := out.Add(time.Hour)
	seedClosedPlay(t, db, nevada, out, &staleEnd)

	// Utah churn: eight joins, two leaves -> net 6 over 2h = 3.00/h.
	for i := 0; i < 8; i++ {
		seedAiringEvent(t, db, "reg_alice", "login", utah, in.Add(time.Duration(i)*time.Minute))
	}
	for i := 0; i < 2; i++ {
		seedAiringEvent(t, db, "reg_alice", "logout", utah, in.Add(time.Duration(30+i)*time.Minute))
	}
	// A bot's arrival is not audience, and a stale join is not in-window.
	seedAiringEvent(t, db, "reg_bot", "login", utah, in)
	seedAiringEvent(t, db, "reg_alice", "login", utah, out)
	// Nevada churn: three joins, one leave -> net 2 over 2h = 1.00/h.
	for i := 0; i < 3; i++ {
		seedAiringEvent(t, db, "reg_bob", "login", nevada, in.Add(time.Duration(i)*time.Minute))
	}
	seedAiringEvent(t, db, "reg_bob", "logout", nevada, in.Add(10*time.Minute))

	got := regionInsights(t, "/api/insights/regions")
	if got.Days != 30 {
		t.Errorf("days = %d, want 30", got.Days)
	}
	if len(got.Regions) != 2 {
		t.Fatalf("regions = %+v, want exactly Utah and Nevada", got.Regions)
	}
	// Ordered by net_per_hour: Utah's 3.00/h beats Nevada's 1.00/h even
	// though both aired the same two hours.
	ut, nv := got.Regions[0], got.Regions[1]
	if ut.State != "Utah" || nv.State != "Nevada" {
		t.Fatalf("order = %q, %q, want Utah then Nevada", ut.State, nv.State)
	}
	if ut.Plays != 3 || ut.ClosedPlays != 2 || ut.AirtimeHours != 2.0 {
		t.Errorf("utah airtime = plays %d closed %d hours %v, want 3/2/2", ut.Plays, ut.ClosedPlays, ut.AirtimeHours)
	}
	if ut.Samples != 13 || ut.LiveSamples != 12 {
		t.Errorf("utah samples = %d (%d live), want 13 (12 live)", ut.Samples, ut.LiveSamples)
	}
	if ut.AvgViewers == nil || *ut.AvgViewers != 15.0 {
		t.Errorf("utah avg_viewers = %v, want 15 — the offline 999 must be excluded", ut.AvgViewers)
	}
	if ut.MaxViewers == nil || *ut.MaxViewers != 20 {
		t.Errorf("utah max_viewers = %v, want 20", ut.MaxViewers)
	}
	if ut.AvgChatters != 2.0 {
		t.Errorf("utah avg_chatters = %v, want 2", ut.AvgChatters)
	}
	if ut.Joins != 8 || ut.Leaves != 2 || ut.Net != 6 {
		t.Errorf("utah churn = %d/%d net %d, want 8/2 net 6 (bot and stale excluded)", ut.Joins, ut.Leaves, ut.Net)
	}
	if ut.NetPerHour == nil || *ut.NetPerHour != 3.0 {
		t.Errorf("utah net_per_hour = %v, want 3", ut.NetPerHour)
	}
	if nv.AirtimeHours != 2.0 || nv.Net != 2 || nv.NetPerHour == nil || *nv.NetPerHour != 1.0 {
		t.Errorf("nevada = hours %v net %d per-hour %v, want 2/2/1", nv.AirtimeHours, nv.Net, nv.NetPerHour)
	}
	if !slices.Equal(got.Platforms, []string{"twitch"}) {
		t.Errorf("platforms = %v, want [twitch]", got.Platforms)
	}
}

// The rollup has to keep working on history written before migration 047 added
// viewer_samples.viewers / .live. Filtering on live strictly would discard
// every earlier tick and return an empty report on a window that reaches back
// past the migration — so the query treats NULL as "nobody reported" rather
// than as "offline", and says so through live_samples. This test is what fails
// if someone tightens `live IS NOT FALSE` to `live`.
func TestRegionInsightsHandler_ReportsRowsPredatingTheAudienceColumns(t *testing.T) {
	db := testdb.New(t)
	in := time.Now().Add(-1 * time.Hour)
	clip := seedVideo(t, db, "insights_region_legacy", "Wyoming", "", nil)

	// Twelve pre-047 ticks: chatters only, no viewers, no live flag.
	for i := 0; i < 12; i++ {
		seedSample(t, db, clip, 3, in.Add(time.Duration(i)*time.Minute))
	}
	end := in.Add(time.Hour)
	seedClosedPlay(t, db, clip, in, &end)

	got := regionInsights(t, "/api/insights/regions")
	if len(got.Regions) != 1 {
		t.Fatalf("regions = %+v, want Wyoming to report from chatter-only history", got.Regions)
	}
	r := got.Regions[0]
	if r.Samples != 12 {
		t.Errorf("samples = %d, want 12", r.Samples)
	}
	if r.LiveSamples != 0 {
		t.Errorf("live_samples = %d, want 0 — no tick vouched for being live", r.LiveSamples)
	}
	if r.AvgViewers != nil || r.MaxViewers != nil {
		t.Errorf("avg/max viewers = %v/%v, want null — no tick carried a viewer count", r.AvgViewers, r.MaxViewers)
	}
	if r.AvgChatters != 3.0 {
		t.Errorf("avg_chatters = %v, want 3 — the one series with history", r.AvgChatters)
	}
}

// A window with samples but no closed play has no airtime to divide by, and
// the rate must read null rather than zero or an infinity.
func TestRegionInsightsHandler_NullRateWithoutClosedAirtime(t *testing.T) {
	db := testdb.New(t)
	seedUser(t, db, "reg_open", false)
	in := time.Now().Add(-1 * time.Hour)
	clip := seedVideo(t, db, "insights_region_open", "Montana", "", nil)

	live := true
	five := 5
	for i := 0; i < 12; i++ {
		seedSampleFull(t, db, clip, 1, &five, &live, in.Add(time.Duration(i)*time.Minute))
	}
	seedClosedPlay(t, db, clip, in, nil) // never observed ending
	seedAiringEvent(t, db, "reg_open", "login", clip, in)

	got := regionInsights(t, "/api/insights/regions")
	if len(got.Regions) != 1 {
		t.Fatalf("regions = %+v, want Montana", got.Regions)
	}
	r := got.Regions[0]
	if r.Plays != 1 || r.ClosedPlays != 0 || r.AirtimeHours != 0 {
		t.Errorf("airtime = plays %d closed %d hours %v, want 1/0/0", r.Plays, r.ClosedPlays, r.AirtimeHours)
	}
	if r.Net != 1 {
		t.Errorf("net = %d, want 1", r.Net)
	}
	if r.NetPerHour != nil {
		t.Errorf("net_per_hour = %v, want null — no airtime to divide by", r.NetPerHour)
	}
}
