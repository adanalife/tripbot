package chatbot

import (
	"context"
	"testing"
)

func TestNormalizeCommandPrefix(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "leading inverted bang is rewritten to !",
			in:   "¡miles",
			want: "!miles",
		},
		{
			name: "inverted bang with params is rewritten to !",
			in:   "¡goto 42",
			want: "!goto 42",
		},
		{
			name: "regular ! prefix is untouched",
			in:   "!miles",
			want: "!miles",
		},
		{
			name: "bare-word (no prefix) is untouched",
			in:   "hello",
			want: "hello",
		},
		{
			name: "inverted bang not at the start is untouched",
			in:   "say ¡hola",
			want: "say ¡hola",
		},
		{
			name: "empty string is untouched",
			in:   "",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeCommandPrefix(tt.in)
			if got != tt.want {
				t.Errorf("normalizeCommandPrefix(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestNormalizeCommandPrefix_DispatchEquivalence asserts the same downstream
// "command" token (i.e. the first whitespace-separated word of the normalized
// message) is produced for `¡foo` and `!foo`. This is the property the
// dispatcher in runCommand() relies on for both prefixes to route to the same
// switch case.
func TestNormalizeCommandPrefix_DispatchEquivalence(t *testing.T) {
	cases := []string{"miles", "location", "leaderboard", "goto 42"}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			gotBang := normalizeCommandPrefix("!" + c)
			gotInverted := normalizeCommandPrefix("¡" + c)
			if gotBang != gotInverted {
				t.Errorf("dispatch divergence: !%s -> %q vs ¡%s -> %q",
					c, gotBang, c, gotInverted)
			}
		})
	}
}

// --- findCommand routing tests ---

func TestFindCommand_SingleWordTrigger(t *testing.T) {
	cmd, params := findCommand("!help")
	if cmd == nil {
		t.Fatal("expected a command, got nil")
	}
	if cmd.Trigger != "!help" {
		t.Errorf("got trigger %q, want !help", cmd.Trigger)
	}
	if len(params) != 0 {
		t.Errorf("unexpected params: %v", params)
	}
}

func TestFindCommand_SingleWordAlias(t *testing.T) {
	// "hi" is an alias of "hello"
	cmd, _ := findCommand("hi")
	if cmd == nil {
		t.Fatal("expected a command, got nil")
	}
	if cmd.Trigger != "hello" {
		t.Errorf("got trigger %q, want hello", cmd.Trigger)
	}
}

func TestFindCommand_MultiWordAlias(t *testing.T) {
	// "no audio" is an alias of !report
	cmd, params := findCommand("no audio")
	if cmd == nil {
		t.Fatal("expected a command, got nil")
	}
	if cmd.Trigger != "!report" {
		t.Errorf("got trigger %q, want !report", cmd.Trigger)
	}
	if len(params) != 0 {
		t.Errorf("unexpected params: %v", params)
	}
}

func TestFindCommand_MultiWordAliasWithTrailingText(t *testing.T) {
	// "frozen since yesterday" — starts with the "frozen" alias
	cmd, params := findCommand("frozen since yesterday")
	if cmd == nil {
		t.Fatal("expected a command, got nil")
	}
	if cmd.Trigger != "!report" {
		t.Errorf("got trigger %q, want !report", cmd.Trigger)
	}
	if len(params) != 2 || params[0] != "since" || params[1] != "yesterday" {
		t.Errorf("unexpected params: %v", params)
	}
}

func TestFindCommand_InvertedBangRoutes(t *testing.T) {
	// ¡miles should route to the same command as !miles
	cmd, _ := findCommand("¡miles")
	if cmd == nil {
		t.Fatal("expected a command, got nil")
	}
	if cmd.Trigger != "!miles" {
		t.Errorf("got trigger %q, want !miles", cmd.Trigger)
	}
}

func TestFindCommand_SpaceSeparatedBang(t *testing.T) {
	// "! location" (with a space) should route to !location
	cmd, _ := findCommand("! location")
	if cmd == nil {
		t.Fatal("expected a command, got nil")
	}
	if cmd.Trigger != "!location" {
		t.Errorf("got trigger %q, want !location", cmd.Trigger)
	}
}

func TestFindCommand_WithParams(t *testing.T) {
	cmd, params := findCommand("!goto 42")
	if cmd == nil {
		t.Fatal("expected a command, got nil")
	}
	if cmd.Trigger != "!goto" {
		t.Errorf("got trigger %q, want !goto", cmd.Trigger)
	}
	if len(params) != 1 || params[0] != "42" {
		t.Errorf("unexpected params: %v", params)
	}
}

func TestFindCommand_UnknownCommand(t *testing.T) {
	cmd, _ := findCommand("!doesnotexist99")
	if cmd != nil {
		t.Errorf("expected nil for unknown command, got %q", cmd.Trigger)
	}
}

func TestFindCommand_EmptyMessage(t *testing.T) {
	cmd, _ := findCommand("")
	if cmd != nil {
		t.Errorf("expected nil for empty message, got %q", cmd.Trigger)
	}
}

// --- checkAccess gating tests ---

// fakeUser implements chatUser for testing without a real DB or Twitch API.
type fakeUser struct {
	follower   bool
	subscriber bool
}

func (f *fakeUser) HasCommandAvailable(_ context.Context) bool { return f.follower }
func (f *fakeUser) IsSubscriber() bool                         { return f.subscriber }

// sessionUser (a *users.User + the installed *Sessions) is the production
// chatUser; this asserts it satisfies the seam.
var _ chatUser = sessionUser{}

func TestCheckAccess_NoRestrictions(t *testing.T) {
	cmd := &Command{Trigger: "!test"}
	var said string
	if !cmd.checkAccess(context.Background(), &fakeUser{}, func(msg string) { said = msg }) {
		t.Error("expected true for unrestricted command")
	}
	if said != "" {
		t.Errorf("expected no message, got %q", said)
	}
}

func TestCheckAccess_RequiresFollow_NonFollower(t *testing.T) {
	prev := followerGatingEnabled
	followerGatingEnabled = true
	t.Cleanup(func() { followerGatingEnabled = prev })

	cmd := &Command{Trigger: "!test", RequiresFollow: true}
	var said string
	if cmd.checkAccess(context.Background(), &fakeUser{follower: false}, func(msg string) { said = msg }) {
		t.Error("expected false for non-follower")
	}
	if said != followerMsg {
		t.Errorf("got %q, want followerMsg", said)
	}
}

func TestCheckAccess_RequiresFollow_Follower(t *testing.T) {
	prev := followerGatingEnabled
	followerGatingEnabled = true
	t.Cleanup(func() { followerGatingEnabled = prev })

	cmd := &Command{Trigger: "!test", RequiresFollow: true}
	var said string
	if !cmd.checkAccess(context.Background(), &fakeUser{follower: true}, func(msg string) { said = msg }) {
		t.Error("expected true for follower")
	}
	if said != "" {
		t.Errorf("expected no message, got %q", said)
	}
}

func TestCheckAccess_RequiresFollow_GatingDisabled(t *testing.T) {
	prev := followerGatingEnabled
	followerGatingEnabled = false
	t.Cleanup(func() { followerGatingEnabled = prev })

	cmd := &Command{Trigger: "!test", RequiresFollow: true}
	var said string
	if !cmd.checkAccess(context.Background(), &fakeUser{follower: false}, func(msg string) { said = msg }) {
		t.Error("expected true for non-follower when gating disabled")
	}
	if said != "" {
		t.Errorf("expected no message when gating disabled, got %q", said)
	}
}

func TestCheckAccess_RequiresSubscriber_NonSubscriber(t *testing.T) {
	cmd := &Command{Trigger: "!test", RequiresSubscriber: true}
	var said string
	if cmd.checkAccess(context.Background(), &fakeUser{subscriber: false}, func(msg string) { said = msg }) {
		t.Error("expected false for non-subscriber")
	}
	if said != subscriberMsg {
		t.Errorf("got %q, want subscriberMsg", said)
	}
}

func TestCheckAccess_RequiresSubscriber_Subscriber(t *testing.T) {
	cmd := &Command{Trigger: "!test", RequiresSubscriber: true}
	var said string
	if !cmd.checkAccess(context.Background(), &fakeUser{subscriber: true}, func(msg string) { said = msg }) {
		t.Error("expected true for subscriber")
	}
	if said != "" {
		t.Errorf("expected no message, got %q", said)
	}
}
