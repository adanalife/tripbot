package chatbot

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/adanalife/tripbot/pkg/users"
	"github.com/adanalife/tripbot/pkg/video"
)

// captureSay swaps sayFn for a recorder and returns helpers to read the
// captured output and restore the original. Always call restore() as a defer.
// Each call to output() returns messages since the last call and resets the
// buffer, so multiple calls within one test don't accumulate across rounds.
func captureSay(t *testing.T) (output func() string, restore func()) {
	t.Helper()
	var msgs []string
	orig := sayFn
	sayFn = func(msg string) { msgs = append(msgs, msg) }
	out := func() string {
		result := strings.Join(msgs, "\n")
		msgs = nil
		return result
	}
	return out, func() { sayFn = orig }
}

func newTestUser(name string) *users.User {
	return &users.User{Username: name}
}

// newTestVideo returns a non-flagged Video with the given fields set.
func newTestVideo(state string, lat, lng float64, date time.Time) video.Video {
	return video.Video{State: state, Lat: lat, Lng: lng, DateFilmed: date}
}

// newTestApp returns an App with CurrentVideo returning vid, plus no-op
// Onscreens, VLC, Video, IRC, and Sessions fakes. For commands that
// don't use CurrentVideo, pass a zero-value video.Video. To assert on
// any of those surfaces, replace the corresponding field with a
// recording fake (recordingOnscreens / recordingVLC / recordingVideo /
// recordingIRC / recordingSessions).
func newTestApp(vid video.Video) *App {
	return &App{
		CurrentVideo: func() video.Video { return vid },
		Onscreens:    noopOnscreens{},
		VLC:          noopVLC{},
		Video:        noopVideo{},
		IRC:          noopIRC{},
		Sessions:     noopSessions{},
	}
}

// --- App.IRC seam ---
//
// These tests exercise the new App.IRC injection point introduced alongside
// the legacy sayFn-based captureSay() helper. Pick a command that's been
// migrated to a.IRC.Say(...) and assert via a recordingIRC. Once all command
// callsites flow through a.IRC, the captureSay()-based tests above can be
// rewritten in this shape and the global Say()/sayFn collapsed.

func TestHelpCmd_SaysSomething_ViaIRC(t *testing.T) {
	app := newTestApp(video.Video{})
	rec := &recordingIRC{}
	app.IRC = rec

	app.helpCmd(context.Background(), newTestUser("viewer1"), nil)

	if len(rec.Says) == 0 {
		t.Fatal("expected a help message via IRC, got none")
	}
	if !strings.Contains(rec.Says[0], " of ") {
		t.Errorf("expected count like '(N of M)' in help message, got %q", rec.Says[0])
	}
}

func TestUptimeCmd_SaysRunningFor_ViaIRC(t *testing.T) {
	app := newTestApp(video.Video{})
	rec := &recordingIRC{}
	app.IRC = rec
	Uptime = time.Now().Add(-5 * time.Minute)

	app.uptimeCmd(context.Background(), newTestUser("viewer1"), nil)

	if len(rec.Says) != 1 {
		t.Fatalf("expected exactly one Say() call, got %d: %v", len(rec.Says), rec.Says)
	}
	if !strings.HasPrefix(rec.Says[0], "I have been running for") {
		t.Errorf("unexpected uptime message via IRC: %q", rec.Says[0])
	}
}

func TestKilometresCmd_SaysViaIRC(t *testing.T) {
	app := newTestApp(video.Video{})
	rec := &recordingIRC{}
	app.IRC = rec

	user := &users.User{Username: "viewer1", Miles: 10}
	app.kilometresCmd(context.Background(), user, nil)

	if len(rec.Says) != 1 {
		t.Fatalf("expected exactly one Say() call, got %d: %v", len(rec.Says), rec.Says)
	}
	// 10 miles * 1.609344 = 16.09344, formatted as "16.09"
	if !strings.Contains(rec.Says[0], "16.09") {
		t.Errorf("expected km conversion in IRC output, got %q", rec.Says[0])
	}
	if !strings.Contains(rec.Says[0], "@viewer1") {
		t.Errorf("expected @username in IRC output, got %q", rec.Says[0])
	}
}

// --- App.Sessions seam ---
//
// These tests exercise the new App.Sessions injection point. Pick a command
// that calls a.Sessions.<method>(...) and assert via a recordingSessions.
// The DB-backed chain that follows a successful Find lookup (the GetScore
// query trio) is still exercised via sqlmock in the broader miles tests
// below — these tests focus on the Sessions surface itself.

func TestMilesCmd_OtherUser_QueriesSessionsFind(t *testing.T) {
	// Confirm a.Sessions.Find is the lookup path for the !miles <user> form.
	// Stage an unknown user (default FindResult is User{ID: 0}) so the
	// command short-circuits before any DB-backed score chain — keeps the
	// test focused on the Sessions seam itself.
	app := newTestApp(video.Video{})
	rec := &recordingSessions{}
	app.Sessions = rec

	_, restore := captureSay(t)
	defer restore()

	app.milesCmd(context.Background(), newTestUser("caller"), []string{"viewer1"})

	if len(rec.Calls) != 1 || rec.Calls[0] != `Find("viewer1")` {
		t.Errorf("expected single Find(\"viewer1\") via Sessions, got %v", rec.Calls)
	}
}

func TestLifetimeMilesLeaderboardCmd_ReadsSessions(t *testing.T) {
	// Confirm lifetimeMilesLeaderboardCmd reads Sessions.LifetimeLeaderboard
	// (not the global users.LifetimeMilesLeaderboard package var).
	app := newTestApp(video.Video{})
	rec := &recordingSessions{
		Leaderboard: [][]string{{"alice", "300.0"}, {"bob", "100.0"}},
	}
	app.Sessions = rec

	out, restore := captureSay(t)
	defer restore()

	app.lifetimeMilesLeaderboardCmd(context.Background(), newTestUser("caller"), nil)

	if len(rec.Calls) != 1 || rec.Calls[0] != "LifetimeLeaderboard()" {
		t.Errorf("expected single LifetimeLeaderboard() via Sessions, got %v", rec.Calls)
	}
	msg := out()
	if !strings.Contains(msg, "alice") || !strings.Contains(msg, "300.0mi") {
		t.Errorf("expected staged leaderboard data in chat output, got %q", msg)
	}
}

// shutdownCmd ultimately calls os.Exit(0), so we can't drive the whole
// command end-to-end in a unit test. The Sessions.Shutdown wiring is
// covered indirectly: realSessions.Shutdown is a thin adapter, and the
// recordingSessions implementation is exercised here as a contract check
// so future refactors of !shutdown can pivot to it without re-deriving
// the call shape.
func TestRecordingSessions_ShutdownIsRecorded(t *testing.T) {
	rec := &recordingSessions{}
	rec.Shutdown(context.Background())

	if len(rec.Calls) != 1 || rec.Calls[0] != "Shutdown()" {
		t.Errorf("expected single Shutdown() recording, got %v", rec.Calls)
	}
}

// --- helpCmd ---

func TestHelpCmd_SaysSomething(t *testing.T) {
	app := newTestApp(video.Video{})
	out, restore := captureSay(t)
	defer restore()

	app.helpCmd(context.Background(), newTestUser("viewer1"), nil)

	if out() == "" {
		t.Fatal("expected a help message, got empty output")
	}
}

func TestHelpCmd_MessageContainsCount(t *testing.T) {
	app := newTestApp(video.Video{})
	out, restore := captureSay(t)
	defer restore()

	app.helpCmd(context.Background(), newTestUser("viewer1"), nil)

	// message format: "<help text> (N of M)"
	if !strings.Contains(out(), " of ") {
		t.Errorf("expected count like '(N of M)', got %q", out())
	}
}

func TestHelpCmd_AdvancesIndex(t *testing.T) {
	app := newTestApp(video.Video{})
	out, restore := captureSay(t)
	defer restore()

	app.helpCmd(context.Background(), newTestUser("viewer1"), nil)
	first := out()

	app.helpCmd(context.Background(), newTestUser("viewer1"), nil)
	second := out()

	if first == second {
		t.Errorf("expected different messages on successive calls, got %q twice", first)
	}
}

// --- uptimeCmd ---

func TestUptimeCmd_SaysRunningFor(t *testing.T) {
	app := newTestApp(video.Video{})
	Uptime = time.Now().Add(-5 * time.Minute)
	out, restore := captureSay(t)
	defer restore()

	app.uptimeCmd(context.Background(), newTestUser("viewer1"), nil)

	if !strings.HasPrefix(out(), "I have been running for") {
		t.Errorf("unexpected uptime message: %q", out())
	}
}

// --- helloCmd ---

func TestHelloCmd_GreetsNewViewer(t *testing.T) {
	app := newTestApp(video.Video{})
	lastHelloTime = time.Time{} // clear rate limiter
	out, restore := captureSay(t)
	defer restore()

	// a fresh user with 0 miles gets the newcomer hint appended
	app.helloCmd(context.Background(), newTestUser("newviewer"), nil)

	msg := out()
	if msg == "" {
		t.Fatal("expected a greeting, got empty output")
	}
	if !strings.Contains(msg, "Tripbot") {
		t.Errorf("expected newcomer hint in greeting, got %q", msg)
	}
}

func TestHelloCmd_RateLimitSilencesSecondCall(t *testing.T) {
	app := newTestApp(video.Video{})
	lastHelloTime = time.Now() // simulate a very recent greeting
	out, restore := captureSay(t)
	defer restore()

	app.helloCmd(context.Background(), newTestUser("viewer1"), nil)

	if out() != "" {
		t.Errorf("expected silence due to rate limit, got %q", out())
	}
}

func TestHelloCmd_IgnoresMessageWithParams(t *testing.T) {
	app := newTestApp(video.Video{})
	lastHelloTime = time.Time{} // not rate limited
	out, restore := captureSay(t)
	defer restore()

	// "hello world" — has params so the bot stays quiet
	app.helloCmd(context.Background(), newTestUser("viewer1"), []string{"world"})

	if out() != "" {
		t.Errorf("expected silence for greeting with params, got %q", out())
	}
}

// --- kilometresCmd ---

func TestKilometresCmd_ConvertsCorrectly(t *testing.T) {
	app := newTestApp(video.Video{})
	out, restore := captureSay(t)
	defer restore()

	user := &users.User{Username: "viewer1", Miles: 10}
	app.kilometresCmd(context.Background(), user, nil)

	// 10 miles * 1.609344 = 16.09344, formatted as "16.09"
	if !strings.Contains(out(), "16.09") {
		t.Errorf("expected km conversion in output, got %q", out())
	}
}

func TestKilometresCmd_IncludesUsername(t *testing.T) {
	app := newTestApp(video.Video{})
	out, restore := captureSay(t)
	defer restore()

	user := &users.User{Username: "testviewer", Miles: 5}
	app.kilometresCmd(context.Background(), user, nil)

	if !strings.Contains(out(), "@testviewer") {
		t.Errorf("expected @username in output, got %q", out())
	}
}

func TestKilometresCmd_ZeroMiles(t *testing.T) {
	app := newTestApp(video.Video{})
	out, restore := captureSay(t)
	defer restore()

	user := &users.User{Username: "newbie", Miles: 0}
	app.kilometresCmd(context.Background(), user, nil)

	if !strings.Contains(out(), "0.00") {
		t.Errorf("expected zero km in output, got %q", out())
	}
}

// --- versionCmd ---

func TestVersionCmd_UsesCachedVersion(t *testing.T) {
	app := newTestApp(video.Video{})
	currentVersion = "v1.2.3-test"
	defer func() { currentVersion = "" }()

	out, restore := captureSay(t)
	defer restore()

	app.versionCmd(context.Background(), newTestUser("viewer1"), nil)

	if !strings.Contains(out(), "v1.2.3-test") {
		t.Errorf("expected cached version in output, got %q", out())
	}
}

func TestVersionCmd_MessageFormat(t *testing.T) {
	app := newTestApp(video.Video{})
	currentVersion = "v1.2.3-test"
	defer func() { currentVersion = "" }()

	out, restore := captureSay(t)
	defer restore()

	app.versionCmd(context.Background(), newTestUser("viewer1"), nil)

	if !strings.HasPrefix(out(), "Current version is ") {
		t.Errorf("unexpected message format: %q", out())
	}
}

// --- stateCmd ---

func TestStateCmd_SaysCurrentState(t *testing.T) {
	vid := newTestVideo("Colorado", 39.5, -105.0, time.Now())
	app := newTestApp(vid)
	out, restore := captureSay(t)
	defer restore()

	app.stateCmd(context.Background(), newTestUser("viewer1"), nil)

	if !strings.Contains(out(), "Colorado") {
		t.Errorf("expected state name in output, got %q", out())
	}
}

func TestStateCmd_MessageFormat(t *testing.T) {
	vid := newTestVideo("Utah", 40.0, -111.0, time.Now())
	app := newTestApp(vid)
	out, restore := captureSay(t)
	defer restore()

	app.stateCmd(context.Background(), newTestUser("viewer1"), nil)

	if !strings.HasPrefix(out(), "We're in ") {
		t.Errorf("unexpected state message format: %q", out())
	}
}

func TestStateCmd_DrivesShowFlagOverlay(t *testing.T) {
	vid := newTestVideo("Wyoming", 43.0, -107.0, time.Now())
	app := newTestApp(vid)
	rec := &recordingOnscreens{}
	app.Onscreens = rec

	_, restore := captureSay(t)
	defer restore()

	app.stateCmd(context.Background(), newTestUser("viewer1"), nil)

	if len(rec.Calls) != 1 || !strings.HasPrefix(rec.Calls[0], "ShowFlag(") {
		t.Errorf("expected one ShowFlag overlay call, got %v", rec.Calls)
	}
}

// --- flagCmd ---

func TestFlagCmd_DrivesShowFlagOverlay(t *testing.T) {
	app := newTestApp(video.Video{})
	rec := &recordingOnscreens{}
	app.Onscreens = rec

	_, restore := captureSay(t)
	defer restore()

	app.flagCmd(context.Background(), newTestUser("viewer1"), nil)

	if len(rec.Calls) != 1 || rec.Calls[0] != "ShowFlag(10s)" {
		t.Errorf("expected ShowFlag(10s) overlay call, got %v", rec.Calls)
	}
}

func TestFlagCmd_DoesNotSayInChat(t *testing.T) {
	app := newTestApp(video.Video{})
	out, restore := captureSay(t)
	defer restore()

	app.flagCmd(context.Background(), newTestUser("viewer1"), nil)

	if out() != "" {
		t.Errorf("expected flagCmd to be silent in chat, got %q", out())
	}
}

// --- dateCmd ---

func TestDateCmd_SaysThisMomentWas(t *testing.T) {
	date := time.Date(2019, 6, 15, 18, 30, 0, 0, time.UTC)
	vid := newTestVideo("Colorado", 39.5, -105.0, date)
	app := newTestApp(vid)
	out, restore := captureSay(t)
	defer restore()

	app.dateCmd(context.Background(), newTestUser("viewer1"), nil)

	msg := out()
	if !strings.HasPrefix(msg, "This moment was") {
		t.Errorf("unexpected date message: %q", msg)
	}
}

func TestDateCmd_IncludesYear(t *testing.T) {
	date := time.Date(2019, 6, 15, 18, 30, 0, 0, time.UTC)
	vid := newTestVideo("Colorado", 39.5, -105.0, date)
	app := newTestApp(vid)
	out, restore := captureSay(t)
	defer restore()

	app.dateCmd(context.Background(), newTestUser("viewer1"), nil)

	if !strings.Contains(out(), "2019") {
		t.Errorf("expected year 2019 in output, got %q", out())
	}
}

// --- timeCmd ---

func TestTimeCmd_SaysThisMomentWas(t *testing.T) {
	date := time.Date(2019, 6, 15, 18, 30, 0, 0, time.UTC)
	vid := newTestVideo("Colorado", 39.5, -105.0, date)
	app := newTestApp(vid)
	out, restore := captureSay(t)
	defer restore()

	app.timeCmd(context.Background(), newTestUser("viewer1"), nil)

	if !strings.HasPrefix(out(), "This moment was") {
		t.Errorf("unexpected time message: %q", out())
	}
}

func TestTimeCmd_IncludesAMPM(t *testing.T) {
	date := time.Date(2019, 6, 15, 18, 30, 0, 0, time.UTC)
	vid := newTestVideo("Colorado", 39.5, -105.0, date)
	app := newTestApp(vid)
	out, restore := captureSay(t)
	defer restore()

	app.timeCmd(context.Background(), newTestUser("viewer1"), nil)

	msg := out()
	if !strings.Contains(msg, "am") && !strings.Contains(msg, "pm") {
		t.Errorf("expected am/pm in time output, got %q", msg)
	}
}

// --- sunsetCmd ---

func TestSunsetCmd_SaysSunset(t *testing.T) {
	// 2pm UTC in Colorado — sunset hasn't happened yet
	date := time.Date(2019, 6, 15, 20, 0, 0, 0, time.UTC)
	vid := newTestVideo("Colorado", 39.5, -105.0, date)
	app := newTestApp(vid)
	out, restore := captureSay(t)
	defer restore()

	app.sunsetCmd(context.Background(), newTestUser("viewer1"), nil)

	if !strings.Contains(out(), "Sunset on this day") {
		t.Errorf("unexpected sunset message: %q", out())
	}
}

// --- guessCmd ---

func TestGuessCmd_NoParams_PromptsGuess(t *testing.T) {
	vid := newTestVideo("Colorado", 39.5, -105.0, time.Now())
	app := newTestApp(vid)
	out, restore := captureSay(t)
	defer restore()

	app.guessCmd(context.Background(), newTestUser("viewer1"), nil)

	if !strings.Contains(out(), "guess") {
		t.Errorf("expected guess prompt in output, got %q", out())
	}
}

func TestGuessCmd_WrongGuess_SaysTryAgain(t *testing.T) {
	vid := newTestVideo("Colorado", 39.5, -105.0, time.Now())
	app := newTestApp(vid)
	out, restore := captureSay(t)
	defer restore()

	// Wyoming != Colorado
	app.guessCmd(context.Background(), newTestUser("viewer1"), []string{"Wyoming"})

	if !strings.Contains(out(), "Try again") {
		t.Errorf("expected try-again in output, got %q", out())
	}
}

// expectAddToScoreChain queues sqlmock expectations for one user.AddToScore
// call: getUserIDByName + findOrCreateScoreboard + findOrCreateScore + the
// UPDATE on Score.save. AddToScore fires twice on a correct guess (once for
// the lifetime "guess_state_total" scoreboard, once for the monthly one), so
// callers queue it twice.
func expectAddToScoreChain(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(`SELECT id FROM users WHERE username = `).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(42))
	mock.ExpectQuery(`SELECT \* FROM "scoreboards" WHERE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(7, "guess_sb"))
	mock.ExpectQuery(`SELECT \* FROM "scores" WHERE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "scoreboard_id", "value"}).
			AddRow(99, 42, 7, 5.0))
	mock.ExpectExec(`UPDATE "scores" SET`).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

func TestGuessCmd_CorrectGuess_DrivesOverlayAndPlayback(t *testing.T) {
	mock := installMockDB(t)
	vid := newTestVideo("Colorado", 39.5, -105.0, time.Now())
	app := newTestApp(vid)
	recOverlay := &recordingOnscreens{}
	recVLC := &recordingVLC{}
	app.Onscreens = recOverlay
	app.VLC = recVLC

	// Two AddToScore calls — lifetime ("guess_state_total") + monthly.
	expectAddToScoreChain(mock)
	expectAddToScoreChain(mock)

	out, restore := captureSay(t)
	defer restore()

	app.guessCmd(context.Background(), newTestUser("viewer1"), []string{"Colorado"})

	msg := out()
	if !strings.Contains(msg, "@viewer1 got it") || !strings.Contains(msg, "Colorado") {
		t.Errorf("expected correct-guess chat message, got %q", msg)
	}

	// Overlay sequence: ShowFlag (state flag) then ShowTimewarp (from a.timewarp()).
	wantOverlay := []string{"ShowFlag(10s)", "ShowTimewarp()"}
	if len(recOverlay.Calls) != len(wantOverlay) {
		t.Fatalf("expected %d overlay calls, got %d: %v", len(wantOverlay), len(recOverlay.Calls), recOverlay.Calls)
	}
	for i, want := range wantOverlay {
		if recOverlay.Calls[i] != want {
			t.Errorf("overlay call %d: want %q, got %q", i, want, recOverlay.Calls[i])
		}
	}

	// VLC: PlayRandom fires inside a.timewarp().
	if len(recVLC.Calls) != 1 || recVLC.Calls[0] != "PlayRandom()" {
		t.Errorf("expected single PlayRandom call, got %v", recVLC.Calls)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestGuessCmd_CorrectGuess_FullStateName(t *testing.T) {
	// guessCmd converts 2-letter codes to long-form before comparing; pass
	// the long form directly to confirm the equality branch works without
	// the abbrev lookup.
	mock := installMockDB(t)
	vid := newTestVideo("Massachusetts", 42.3, -71.0, time.Now())
	app := newTestApp(vid)

	expectAddToScoreChain(mock)
	expectAddToScoreChain(mock)

	out, restore := captureSay(t)
	defer restore()

	app.guessCmd(context.Background(), newTestUser("viewer1"), []string{"Massachusetts"})

	if !strings.Contains(out(), "got it") {
		t.Errorf("expected correct-guess msg, got %q", out())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestGuessCmd_CorrectGuess_TwoLetterCode(t *testing.T) {
	// Two-letter codes get expanded via helpers.StateAbbrevToState.
	mock := installMockDB(t)
	vid := newTestVideo("California", 36.7, -119.4, time.Now())
	app := newTestApp(vid)

	expectAddToScoreChain(mock)
	expectAddToScoreChain(mock)

	out, restore := captureSay(t)
	defer restore()

	app.guessCmd(context.Background(), newTestUser("viewer1"), []string{"CA"})

	if !strings.Contains(out(), "got it") {
		t.Errorf("expected correct-guess msg from CA, got %q", out())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// --- middleCmd ---

// adminUser matches CHANNEL_NAME in .env.testing, satisfying c.UserIsAdmin.
const adminUser = "test"

func TestMiddleCmd_NonAdminIsSilent(t *testing.T) {
	app := newTestApp(video.Video{})
	out, restore := captureSay(t)
	defer restore()

	app.middleCmd(context.Background(), newTestUser("viewer1"), []string{"hello"})

	if out() != "" {
		t.Errorf("expected silence for non-admin, got %q", out())
	}
}

func TestMiddleCmd_NoParams_PromptsForText(t *testing.T) {
	app := newTestApp(video.Video{})
	out, restore := captureSay(t)
	defer restore()

	app.middleCmd(context.Background(), newTestUser(adminUser), nil)

	if !strings.Contains(out(), "What do you want to say") {
		t.Errorf("expected prompt, got %q", out())
	}
}

func TestMiddleCmd_Hide_DrivesHideOverlay(t *testing.T) {
	app := newTestApp(video.Video{})
	rec := &recordingOnscreens{}
	app.Onscreens = rec

	out, restore := captureSay(t)
	defer restore()

	app.middleCmd(context.Background(), newTestUser(adminUser), []string{"hide"})

	if !strings.Contains(out(), "Hiding the message") {
		t.Errorf("expected hide confirmation in chat, got %q", out())
	}
	if len(rec.Calls) != 1 || rec.Calls[0] != "HideMiddleText()" {
		t.Errorf("expected one HideMiddleText overlay call, got %v", rec.Calls)
	}
}

func TestMiddleCmd_Hide_CaseInsensitive(t *testing.T) {
	// "HIDE" should be normalized to lowercase before the branch check.
	app := newTestApp(video.Video{})
	rec := &recordingOnscreens{}
	app.Onscreens = rec

	_, restore := captureSay(t)
	defer restore()

	app.middleCmd(context.Background(), newTestUser(adminUser), []string{"HIDE"})

	if len(rec.Calls) != 1 || rec.Calls[0] != "HideMiddleText()" {
		t.Errorf("expected one HideMiddleText overlay call for 'HIDE', got %v", rec.Calls)
	}
}

func TestMiddleCmd_Text_DrivesShowOverlay(t *testing.T) {
	app := newTestApp(video.Video{})
	rec := &recordingOnscreens{}
	app.Onscreens = rec

	_, restore := captureSay(t)
	defer restore()

	// Multiple words get joined with a space into the overlay text.
	app.middleCmd(context.Background(), newTestUser(adminUser), []string{"hello", "everyone"})

	if len(rec.Calls) != 1 || rec.Calls[0] != `ShowMiddleText("hello everyone")` {
		t.Errorf("expected ShowMiddleText with joined text, got %v", rec.Calls)
	}
}

func TestMiddleCmd_NonAdmin_DoesNotDriveOverlay(t *testing.T) {
	app := newTestApp(video.Video{})
	rec := &recordingOnscreens{}
	app.Onscreens = rec

	_, restore := captureSay(t)
	defer restore()

	// A non-admin's params should be ignored — no chat, no overlay call.
	app.middleCmd(context.Background(), newTestUser("viewer1"), []string{"hide"})
	app.middleCmd(context.Background(), newTestUser("viewer1"), []string{"hello"})

	if len(rec.Calls) != 0 {
		t.Errorf("expected no overlay calls for non-admin, got %v", rec.Calls)
	}
}

// --- lifetimeMilesLeaderboardCmd ---
//
// Reads the lifetime-miles leaderboard via Sessions.LifetimeLeaderboard().
// In production realSessions returns users.LifetimeMilesLeaderboard (the
// package-level [][]string hydrated at startup by users.InitLeaderboard);
// in tests recordingSessions.Leaderboard stages whatever data the test
// wants. No DB on the read path.

func TestLifetimeMilesLeaderboardCmd_Empty(t *testing.T) {
	app := newTestApp(video.Video{})
	// noopSessions's LifetimeLeaderboard returns nil — the test asserts
	// the empty-leaderboard header still renders cleanly.

	out, restore := captureSay(t)
	defer restore()

	app.lifetimeMilesLeaderboardCmd(context.Background(), newTestUser("viewer1"), nil)

	msg := out()
	if !strings.Contains(msg, "Top 0 lifetime miles") {
		t.Errorf("expected zero-size leaderboard header, got %q", msg)
	}
}

func TestLifetimeMilesLeaderboardCmd_WithUsers(t *testing.T) {
	app := newTestApp(video.Video{})
	recOverlay := &recordingOnscreens{}
	app.Onscreens = recOverlay

	// Stage the leaderboard via Sessions instead of mutating the package var.
	recSessions := &recordingSessions{
		Leaderboard: [][]string{
			{"viewer1", "200.0"}, {"viewer2", "150.5"}, {"viewer3", "10.0"},
		},
	}
	app.Sessions = recSessions

	out, restore := captureSay(t)
	defer restore()

	app.lifetimeMilesLeaderboardCmd(context.Background(), newTestUser("caller"), nil)

	msg := out()
	if !strings.Contains(msg, "viewer1") || !strings.Contains(msg, "200.0mi") {
		t.Errorf("expected viewer1 with 200.0mi in output, got %q", msg)
	}
	if !strings.Contains(msg, "Top 3 lifetime miles") {
		t.Errorf("expected 'Top 3 lifetime miles' header, got %q", msg)
	}

	// confirm Sessions.LifetimeLeaderboard was the source
	if len(recSessions.Calls) != 1 || recSessions.Calls[0] != "LifetimeLeaderboard()" {
		t.Errorf("expected single LifetimeLeaderboard() call, got %v", recSessions.Calls)
	}

	// confirm the overlay surface was driven with the same title + row count
	if len(recOverlay.Calls) != 1 || !strings.Contains(recOverlay.Calls[0], `ShowLeaderboard("Total Miles", 3 rows)`) {
		t.Errorf("expected single ShowLeaderboard overlay call, got %v", recOverlay.Calls)
	}
}

// --- monthlyMilesLeaderboardCmd ---
//
// scoreboards.TopUsers emits a JOIN across scores, scoreboards, and users.
// With sqlmock we just need to honor the one query the command makes.

func TestMonthlyMilesLeaderboardCmd_RendersTopUsers(t *testing.T) {
	mock := installMockDB(t)
	app := newTestApp(video.Video{})
	rec := &recordingOnscreens{}
	app.Onscreens = rec

	rows := sqlmock.NewRows([]string{"username", "value"}).
		AddRow("viewer1", 42.5).
		AddRow("viewer2", 12.0)
	mock.ExpectQuery(`SELECT users\.username, scores\.value FROM "scores"`).
		WillReturnRows(rows)

	out, restore := captureSay(t)
	defer restore()

	app.monthlyMilesLeaderboardCmd(context.Background(), newTestUser("caller"), nil)

	msg := out()
	if !strings.Contains(msg, "viewer1") || !strings.Contains(msg, "42.5") {
		t.Errorf("expected leaderboard row in output, got %q", msg)
	}
	if !strings.HasPrefix(msg, "Top 2 miles this month:") {
		t.Errorf("expected 'Top 2 miles this month:' prefix, got %q", msg)
	}
	if len(rec.Calls) != 1 || !strings.Contains(rec.Calls[0], `ShowLeaderboard("Monthly Miles", 2 rows)`) {
		t.Errorf("expected one ShowLeaderboard overlay call with 2 rows, got %v", rec.Calls)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// --- monthlyGuessLeaderboardCmd ---

func TestMonthlyGuessLeaderboardCmd_Empty_SaysNoneYet(t *testing.T) {
	mock := installMockDB(t)
	app := newTestApp(video.Video{})
	rec := &recordingOnscreens{}
	app.Onscreens = rec

	// scoreboards.TopUsers returns an empty result set
	rows := sqlmock.NewRows([]string{"username", "value"})
	mock.ExpectQuery(`SELECT users\.username, scores\.value FROM "scores"`).
		WillReturnRows(rows)

	out, restore := captureSay(t)
	defer restore()

	app.monthlyGuessLeaderboardCmd(context.Background(), newTestUser("caller"), nil)

	if !strings.Contains(out(), "No one is on that leaderboard yet") {
		t.Errorf("expected empty-leaderboard message, got %q", out())
	}
	if len(rec.Calls) != 0 {
		t.Errorf("expected no overlay call when leaderboard empty, got %v", rec.Calls)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestMonthlyGuessLeaderboardCmd_WithGuesses_StripsDecimals(t *testing.T) {
	mock := installMockDB(t)
	app := newTestApp(video.Video{})
	rec := &recordingOnscreens{}
	app.Onscreens = rec

	rows := sqlmock.NewRows([]string{"username", "value"}).
		AddRow("viewer1", 7.0).
		AddRow("viewer2", 3.0)
	mock.ExpectQuery(`SELECT users\.username, scores\.value FROM "scores"`).
		WillReturnRows(rows)

	out, restore := captureSay(t)
	defer restore()

	app.monthlyGuessLeaderboardCmd(context.Background(), newTestUser("caller"), nil)

	msg := out()
	// guess scores are formatted as integers in the chat message
	if !strings.Contains(msg, "1. viewer1 (7)") {
		t.Errorf("expected integer-formatted guess count, got %q", msg)
	}
	if strings.Contains(msg, "7.0") {
		t.Errorf("decimals should be stripped, but found '7.0' in %q", msg)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// --- milesCmd ---
//
// Self-lookup (no params) and other-user lookup (with params) both end up
// calling user.CurrentMonthlyMiles, which runs the 3-query GetScore chain
// (getUserIDByName → findOrCreateScoreboard → findOrCreateScore). The
// other-user path adds a Sessions.Find on top — staged via recordingSessions.

func TestMilesCmd_OtherUser_NotInDB(t *testing.T) {
	app := newTestApp(video.Video{})

	// recordingSessions.FindResult defaults to users.User{} (ID == 0),
	// which mirrors pkg/users.Find's "not found" contract.
	rec := &recordingSessions{}
	app.Sessions = rec

	out, restore := captureSay(t)
	defer restore()

	app.milesCmd(context.Background(), newTestUser("caller"), []string{"ghost"})

	if !strings.Contains(out(), "I don't know them") {
		t.Errorf("expected unknown-user message, got %q", out())
	}
	if len(rec.Calls) != 1 || rec.Calls[0] != `Find("ghost")` {
		t.Errorf("expected Sessions.Find(\"ghost\") call, got %v", rec.Calls)
	}
}

func TestMilesCmd_Self_WithMiles(t *testing.T) {
	mock := installMockDB(t)
	app := newTestApp(video.Video{})

	mock.ExpectQuery(`SELECT id FROM users WHERE username = `).
		WithArgs("viewer1").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(42))
	mock.ExpectQuery(`SELECT \* FROM "scoreboards" WHERE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(7, "miles_2026_05"))
	mock.ExpectQuery(`SELECT \* FROM "scores" WHERE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "scoreboard_id", "value"}).
			AddRow(99, 42, 7, 8.0))

	out, restore := captureSay(t)
	defer restore()

	user := &users.User{Username: "viewer1", Miles: 50.0}
	app.milesCmd(context.Background(), user, nil)

	msg := out()
	if !strings.Contains(msg, "@viewer1 has 8.00mi this month") {
		t.Errorf("expected monthly miles in self-lookup, got %q", msg)
	}
	if !strings.Contains(msg, "(50mi total)") {
		t.Errorf("expected lifetime total in self-lookup, got %q", msg)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestMilesCmd_Self_NewcomerHint(t *testing.T) {
	mock := installMockDB(t)
	app := newTestApp(video.Video{})

	// Brand-new user: monthly = 0, lifetime = 0 → triggers both newcomer hints.
	mock.ExpectQuery(`SELECT id FROM users WHERE username = `).
		WithArgs("newbie").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(99))
	mock.ExpectQuery(`SELECT \* FROM "scoreboards" WHERE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(7, "miles_2026_05"))
	mock.ExpectQuery(`SELECT \* FROM "scores" WHERE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "scoreboard_id", "value"}).
			AddRow(100, 99, 7, 0.0))

	out, restore := captureSay(t)
	defer restore()

	user := &users.User{Username: "newbie", Miles: 0.0}
	app.milesCmd(context.Background(), user, nil)

	msg := out()
	if !strings.Contains(msg, "You'll earn more miles") {
		t.Errorf("expected newcomer hint, got %q", msg)
	}
	if !strings.Contains(msg, "takes a bit for me to notice you") {
		t.Errorf("expected zero-miles-specific hint, got %q", msg)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestMilesCmd_OtherUser_Found(t *testing.T) {
	mock := installMockDB(t)
	app := newTestApp(video.Video{})

	// Stage Sessions.Find to return a known user (replaces the old
	// sqlmock SELECT * FROM users expectation).
	rec := &recordingSessions{
		FindResult: users.User{ID: 42, Username: "viewer1", Miles: 120.0},
	}
	app.Sessions = rec

	// 1. scoreboards.getUserIDByName — raw SELECT id by username
	mock.ExpectQuery(`SELECT id FROM users WHERE username = `).
		WithArgs("viewer1").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(42))

	// 2. scoreboards.findOrCreateScoreboard — FirstOrCreate SELECT
	mock.ExpectQuery(`SELECT \* FROM "scoreboards" WHERE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(7, "miles_2026_05"))

	// 3. scoreboards.findOrCreateScore — FirstOrCreate SELECT for the score row
	mock.ExpectQuery(`SELECT \* FROM "scores" WHERE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "scoreboard_id", "value"}).
			AddRow(99, 42, 7, 15.5))

	out, restore := captureSay(t)
	defer restore()

	app.milesCmd(context.Background(), newTestUser("caller"), []string{"viewer1"})

	msg := out()
	// monthly is 15.5, lifetime 120 → both should appear (rounded to int for total)
	if !strings.Contains(msg, "@viewer1 has 15.50mi this month") {
		t.Errorf("expected monthly miles line, got %q", msg)
	}
	if !strings.Contains(msg, "(120mi total)") {
		t.Errorf("expected lifetime miles in parens, got %q", msg)
	}
	if len(rec.Calls) != 1 || rec.Calls[0] != `Find("viewer1")` {
		t.Errorf("expected Sessions.Find(\"viewer1\") call, got %v", rec.Calls)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestMilesCmd_OtherUser_StripsAtSign(t *testing.T) {
	app := newTestApp(video.Video{})

	// recordingSessions.FindResult defaults to User{} (ID == 0) — the
	// "@ghost" arg should be normalized to "ghost" before the Find call.
	rec := &recordingSessions{}
	app.Sessions = rec

	out, restore := captureSay(t)
	defer restore()

	app.milesCmd(context.Background(), newTestUser("caller"), []string{"@ghost"})

	if !strings.Contains(out(), "I don't know them") {
		t.Errorf("expected unknown-user message, got %q", out())
	}
	if len(rec.Calls) != 1 || rec.Calls[0] != `Find("ghost")` {
		t.Errorf("expected Sessions.Find(\"ghost\") with @ stripped, got %v", rec.Calls)
	}
}
