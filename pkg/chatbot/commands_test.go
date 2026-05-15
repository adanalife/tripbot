package chatbot

import (
	"strings"
	"testing"
	"time"

	"github.com/adanalife/tripbot/pkg/users"
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

// --- helpCmd ---

func TestHelpCmd_SaysSomething(t *testing.T) {
	out, restore := captureSay(t)
	defer restore()

	helpCmd(newTestUser("viewer1"), nil)

	if out() == "" {
		t.Fatal("expected a help message, got empty output")
	}
}

func TestHelpCmd_MessageContainsCount(t *testing.T) {
	out, restore := captureSay(t)
	defer restore()

	helpCmd(newTestUser("viewer1"), nil)

	// message format: "<help text> (N of M)"
	if !strings.Contains(out(), " of ") {
		t.Errorf("expected count like '(N of M)', got %q", out())
	}
}

func TestHelpCmd_AdvancesIndex(t *testing.T) {
	out, restore := captureSay(t)
	defer restore()

	helpCmd(newTestUser("viewer1"), nil)
	first := out()

	helpCmd(newTestUser("viewer1"), nil)
	second := out()

	if first == second {
		t.Errorf("expected different messages on successive calls, got %q twice", first)
	}
}

// --- uptimeCmd ---

func TestUptimeCmd_SaysRunningFor(t *testing.T) {
	Uptime = time.Now().Add(-5 * time.Minute)
	out, restore := captureSay(t)
	defer restore()

	uptimeCmd(newTestUser("viewer1"), nil)

	if !strings.HasPrefix(out(), "I have been running for") {
		t.Errorf("unexpected uptime message: %q", out())
	}
}

// --- helloCmd ---

func TestHelloCmd_GreetsNewViewer(t *testing.T) {
	lastHelloTime = time.Time{} // clear rate limiter
	out, restore := captureSay(t)
	defer restore()

	// a fresh user with 0 miles gets the newcomer hint appended
	helloCmd(newTestUser("newviewer"), nil)

	msg := out()
	if msg == "" {
		t.Fatal("expected a greeting, got empty output")
	}
	if !strings.Contains(msg, "Tripbot") {
		t.Errorf("expected newcomer hint in greeting, got %q", msg)
	}
}

func TestHelloCmd_RateLimitSilencesSecondCall(t *testing.T) {
	lastHelloTime = time.Now() // simulate a very recent greeting
	out, restore := captureSay(t)
	defer restore()

	helloCmd(newTestUser("viewer1"), nil)

	if out() != "" {
		t.Errorf("expected silence due to rate limit, got %q", out())
	}
}

func TestHelloCmd_IgnoresMessageWithParams(t *testing.T) {
	lastHelloTime = time.Time{} // not rate limited
	out, restore := captureSay(t)
	defer restore()

	// "hello world" — has params so the bot stays quiet
	helloCmd(newTestUser("viewer1"), []string{"world"})

	if out() != "" {
		t.Errorf("expected silence for greeting with params, got %q", out())
	}
}

// --- kilometresCmd ---

func TestKilometresCmd_ConvertsCorrectly(t *testing.T) {
	out, restore := captureSay(t)
	defer restore()

	user := &users.User{Username: "viewer1", Miles: 10}
	kilometresCmd(user, nil)

	// 10 miles * 1.609344 = 16.09344, formatted as "16.09"
	if !strings.Contains(out(), "16.09") {
		t.Errorf("expected km conversion in output, got %q", out())
	}
}

func TestKilometresCmd_IncludesUsername(t *testing.T) {
	out, restore := captureSay(t)
	defer restore()

	user := &users.User{Username: "testviewer", Miles: 5}
	kilometresCmd(user, nil)

	if !strings.Contains(out(), "@testviewer") {
		t.Errorf("expected @username in output, got %q", out())
	}
}

func TestKilometresCmd_ZeroMiles(t *testing.T) {
	out, restore := captureSay(t)
	defer restore()

	user := &users.User{Username: "newbie", Miles: 0}
	kilometresCmd(user, nil)

	if !strings.Contains(out(), "0.00") {
		t.Errorf("expected zero km in output, got %q", out())
	}
}

// --- versionCmd ---

func TestVersionCmd_UsesCachedVersion(t *testing.T) {
	currentVersion = "v1.2.3-test"
	defer func() { currentVersion = "" }()

	out, restore := captureSay(t)
	defer restore()

	versionCmd(newTestUser("viewer1"), nil)

	if !strings.Contains(out(), "v1.2.3-test") {
		t.Errorf("expected cached version in output, got %q", out())
	}
}

func TestVersionCmd_MessageFormat(t *testing.T) {
	currentVersion = "v1.2.3-test"
	defer func() { currentVersion = "" }()

	out, restore := captureSay(t)
	defer restore()

	versionCmd(newTestUser("viewer1"), nil)

	if !strings.HasPrefix(out(), "Current version is ") {
		t.Errorf("unexpected message format: %q", out())
	}
}
