package chatbot

import (
	"strings"
	"testing"
	"time"

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

// newTestApp returns an App with CurrentVideo returning vid.
// For commands that don't use CurrentVideo, pass a zero-value video.Video.
func newTestApp(vid video.Video) *App {
	return &App{CurrentVideo: func() video.Video { return vid }}
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
