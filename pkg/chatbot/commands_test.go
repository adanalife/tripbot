package chatbot

import (
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

// newTestApp returns an App with CurrentVideo returning vid and a no-op
// Onscreens fake. For commands that don't use CurrentVideo, pass a zero-value
// video.Video. To assert on Onscreens calls, replace app.Onscreens with a
// recording fake (see fakeOnscreens in onscreens_test.go).
func newTestApp(vid video.Video) *App {
	return &App{
		CurrentVideo: func() video.Video { return vid },
		Onscreens:    noopOnscreens{},
	}
}

// --- helpCmd ---

func TestHelpCmd_SaysSomething(t *testing.T) {
	app := newTestApp(video.Video{})
	out, restore := captureSay(t)
	defer restore()

	app.helpCmd(newTestUser("viewer1"), nil)

	if out() == "" {
		t.Fatal("expected a help message, got empty output")
	}
}

func TestHelpCmd_MessageContainsCount(t *testing.T) {
	app := newTestApp(video.Video{})
	out, restore := captureSay(t)
	defer restore()

	app.helpCmd(newTestUser("viewer1"), nil)

	// message format: "<help text> (N of M)"
	if !strings.Contains(out(), " of ") {
		t.Errorf("expected count like '(N of M)', got %q", out())
	}
}

func TestHelpCmd_AdvancesIndex(t *testing.T) {
	app := newTestApp(video.Video{})
	out, restore := captureSay(t)
	defer restore()

	app.helpCmd(newTestUser("viewer1"), nil)
	first := out()

	app.helpCmd(newTestUser("viewer1"), nil)
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

	app.uptimeCmd(newTestUser("viewer1"), nil)

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
	app.helloCmd(newTestUser("newviewer"), nil)

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

	app.helloCmd(newTestUser("viewer1"), nil)

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
	app.helloCmd(newTestUser("viewer1"), []string{"world"})

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
	app.kilometresCmd(user, nil)

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
	app.kilometresCmd(user, nil)

	if !strings.Contains(out(), "@testviewer") {
		t.Errorf("expected @username in output, got %q", out())
	}
}

func TestKilometresCmd_ZeroMiles(t *testing.T) {
	app := newTestApp(video.Video{})
	out, restore := captureSay(t)
	defer restore()

	user := &users.User{Username: "newbie", Miles: 0}
	app.kilometresCmd(user, nil)

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

	app.versionCmd(newTestUser("viewer1"), nil)

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

	app.versionCmd(newTestUser("viewer1"), nil)

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

	app.stateCmd(newTestUser("viewer1"), nil)

	if !strings.Contains(out(), "Colorado") {
		t.Errorf("expected state name in output, got %q", out())
	}
}

func TestStateCmd_MessageFormat(t *testing.T) {
	vid := newTestVideo("Utah", 40.0, -111.0, time.Now())
	app := newTestApp(vid)
	out, restore := captureSay(t)
	defer restore()

	app.stateCmd(newTestUser("viewer1"), nil)

	if !strings.HasPrefix(out(), "We're in ") {
		t.Errorf("unexpected state message format: %q", out())
	}
}

// --- dateCmd ---

func TestDateCmd_SaysThisMomentWas(t *testing.T) {
	date := time.Date(2019, 6, 15, 18, 30, 0, 0, time.UTC)
	vid := newTestVideo("Colorado", 39.5, -105.0, date)
	app := newTestApp(vid)
	out, restore := captureSay(t)
	defer restore()

	app.dateCmd(newTestUser("viewer1"), nil)

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

	app.dateCmd(newTestUser("viewer1"), nil)

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

	app.timeCmd(newTestUser("viewer1"), nil)

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

	app.timeCmd(newTestUser("viewer1"), nil)

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

	app.sunsetCmd(newTestUser("viewer1"), nil)

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

	app.guessCmd(newTestUser("viewer1"), nil)

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
	app.guessCmd(newTestUser("viewer1"), []string{"Wyoming"})

	if !strings.Contains(out(), "Try again") {
		t.Errorf("expected try-again in output, got %q", out())
	}
}

// --- middleCmd ---

// adminUser matches CHANNEL_NAME in .env.testing, satisfying c.UserIsAdmin.
const adminUser = "test"

func TestMiddleCmd_NonAdminIsSilent(t *testing.T) {
	app := newTestApp(video.Video{})
	out, restore := captureSay(t)
	defer restore()

	app.middleCmd(newTestUser("viewer1"), []string{"hello"})

	if out() != "" {
		t.Errorf("expected silence for non-admin, got %q", out())
	}
}

func TestMiddleCmd_NoParams_PromptsForText(t *testing.T) {
	app := newTestApp(video.Video{})
	out, restore := captureSay(t)
	defer restore()

	app.middleCmd(newTestUser(adminUser), nil)

	if !strings.Contains(out(), "What do you want to say") {
		t.Errorf("expected prompt, got %q", out())
	}
}

// --- lifetimeMilesLeaderboardCmd ---
//
// Reads users.LifetimeMilesLeaderboard, which is a package-level [][]string
// hydrated at startup by users.InitLeaderboard. No DB on the read path.

func TestLifetimeMilesLeaderboardCmd_Empty(t *testing.T) {
	app := newTestApp(video.Video{})
	prev := users.LifetimeMilesLeaderboard
	users.LifetimeMilesLeaderboard = nil
	defer func() { users.LifetimeMilesLeaderboard = prev }()

	out, restore := captureSay(t)
	defer restore()

	app.lifetimeMilesLeaderboardCmd(newTestUser("viewer1"), nil)

	msg := out()
	if !strings.Contains(msg, "Top 0 lifetime miles") {
		t.Errorf("expected zero-size leaderboard header, got %q", msg)
	}
}

func TestLifetimeMilesLeaderboardCmd_WithUsers(t *testing.T) {
	app := newTestApp(video.Video{})
	rec := &recordingOnscreens{}
	app.Onscreens = rec

	prev := users.LifetimeMilesLeaderboard
	users.LifetimeMilesLeaderboard = [][]string{
		{"viewer1", "200.0"}, {"viewer2", "150.5"}, {"viewer3", "10.0"},
	}
	defer func() { users.LifetimeMilesLeaderboard = prev }()

	out, restore := captureSay(t)
	defer restore()

	app.lifetimeMilesLeaderboardCmd(newTestUser("caller"), nil)

	msg := out()
	if !strings.Contains(msg, "viewer1") || !strings.Contains(msg, "200.0mi") {
		t.Errorf("expected viewer1 with 200.0mi in output, got %q", msg)
	}
	if !strings.Contains(msg, "Top 3 lifetime miles") {
		t.Errorf("expected 'Top 3 lifetime miles' header, got %q", msg)
	}

	// confirm the overlay surface was driven with the same title + row count
	if len(rec.Calls) != 1 || !strings.Contains(rec.Calls[0], `ShowLeaderboard("Total Miles", 3 rows)`) {
		t.Errorf("expected single ShowLeaderboard overlay call, got %v", rec.Calls)
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

	app.monthlyMilesLeaderboardCmd(newTestUser("caller"), nil)

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

	app.monthlyGuessLeaderboardCmd(newTestUser("caller"), nil)

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

	app.monthlyGuessLeaderboardCmd(newTestUser("caller"), nil)

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
// other-user path adds a users.Find on top.

func TestMilesCmd_OtherUser_NotInDB(t *testing.T) {
	mock := installMockDB(t)
	app := newTestApp(video.Video{})

	// users.Find runs a SELECT; returning no rows triggers gorm.ErrRecordNotFound
	// which Find translates into User{ID: 0}.
	mock.ExpectQuery(`SELECT \* FROM "users" WHERE username = `).
		WithArgs("ghost", 1). // GORM appends the LIMIT 1 arg
		WillReturnRows(sqlmock.NewRows([]string{"id", "username"}))

	out, restore := captureSay(t)
	defer restore()

	app.milesCmd(newTestUser("caller"), []string{"ghost"})

	if !strings.Contains(out(), "I don't know them") {
		t.Errorf("expected unknown-user message, got %q", out())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
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
	app.milesCmd(user, nil)

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
	app.milesCmd(user, nil)

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

	// 1. users.Find — returns one row with miles=120
	mock.ExpectQuery(`SELECT \* FROM "users" WHERE username = `).
		WithArgs("viewer1", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "miles"}).
			AddRow(42, "viewer1", 120.0))

	// 2. scoreboards.getUserIDByName — raw SELECT id by username
	mock.ExpectQuery(`SELECT id FROM users WHERE username = `).
		WithArgs("viewer1").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(42))

	// 3. scoreboards.findOrCreateScoreboard — FirstOrCreate SELECT
	mock.ExpectQuery(`SELECT \* FROM "scoreboards" WHERE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(7, "miles_2026_05"))

	// 4. scoreboards.findOrCreateScore — FirstOrCreate SELECT for the score row
	mock.ExpectQuery(`SELECT \* FROM "scores" WHERE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "scoreboard_id", "value"}).
			AddRow(99, 42, 7, 15.5))

	out, restore := captureSay(t)
	defer restore()

	app.milesCmd(newTestUser("caller"), []string{"viewer1"})

	msg := out()
	// monthly is 15.5, lifetime 120 → both should appear (rounded to int for total)
	if !strings.Contains(msg, "@viewer1 has 15.50mi this month") {
		t.Errorf("expected monthly miles line, got %q", msg)
	}
	if !strings.Contains(msg, "(120mi total)") {
		t.Errorf("expected lifetime miles in parens, got %q", msg)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestMilesCmd_OtherUser_StripsAtSign(t *testing.T) {
	mock := installMockDB(t)
	app := newTestApp(video.Video{})

	mock.ExpectQuery(`SELECT \* FROM "users" WHERE username = `).
		WithArgs("ghost", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "username"}))

	out, restore := captureSay(t)
	defer restore()

	// "@ghost" should be normalized to "ghost" before the lookup
	app.milesCmd(newTestUser("caller"), []string{"@ghost"})

	if !strings.Contains(out(), "I don't know them") {
		t.Errorf("expected unknown-user message, got %q", out())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}
