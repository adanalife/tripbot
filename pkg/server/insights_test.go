package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	for _, want := range []string{`"days":7`, `"clips":[]`} {
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
	err := db.Exec(`INSERT INTO viewer_samples (platform, count, video_id, sampled_at)
	                VALUES ('twitch', ?, ?, ?)`, count, videoID, at).Error
	if err != nil {
		t.Fatalf("insert sample for video %d: %v", videoID, err)
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
