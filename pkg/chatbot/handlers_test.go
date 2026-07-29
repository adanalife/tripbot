package chatbot

import (
	"context"
	"testing"

	"github.com/adanalife/tripbot/pkg/video"
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
			name: "leading 1 before a letter is rewritten to !",
			in:   "1location",
			want: "!location",
		},
		{
			name: "leading 1 with params is rewritten to !",
			in:   "1goto 42",
			want: "!goto 42",
		},
		{
			name: "number-leading message is untouched",
			in:   "100 miles",
			want: "100 miles",
		},
		{
			name: "1 followed by punctuation is untouched",
			in:   "10/10",
			want: "10/10",
		},
		{
			name: "bare 1 is untouched",
			in:   "1",
			want: "1",
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
	cmd, params := builtTestApp.findCommand("!help")
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
	cmd, _ := builtTestApp.findCommand("hi")
	if cmd == nil {
		t.Fatal("expected a command, got nil")
	}
	if cmd.Trigger != "hello" {
		t.Errorf("got trigger %q, want hello", cmd.Trigger)
	}
}

func TestFindCommand_MultiWordAlias(t *testing.T) {
	// "no audio" is an alias of !report
	cmd, params := builtTestApp.findCommand("no audio")
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
	cmd, params := builtTestApp.findCommand("frozen since yesterday")
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
	cmd, _ := builtTestApp.findCommand("¡miles")
	if cmd == nil {
		t.Fatal("expected a command, got nil")
	}
	if cmd.Trigger != "!miles" {
		t.Errorf("got trigger %q, want !miles", cmd.Trigger)
	}
}

func TestFindCommand_DigitOnePrefixRoutes(t *testing.T) {
	// 1location should route to the same command as !location
	cmd, _ := builtTestApp.findCommand("1location")
	if cmd == nil {
		t.Fatal("expected a command, got nil")
	}
	if cmd.Trigger != "!location" {
		t.Errorf("got trigger %q, want !location", cmd.Trigger)
	}
}

func TestFindCommand_NumberLeadingMessageDoesNotRoute(t *testing.T) {
	// a message that genuinely starts with a number must not become a command
	if cmd, _ := builtTestApp.findCommand("100 miles"); cmd != nil {
		t.Errorf("findCommand(\"100 miles\") = %s, want nil", cmd.Trigger)
	}
}

func TestFindCommand_SpaceSeparatedBang(t *testing.T) {
	// "! location" (with a space) should route to !location
	cmd, _ := builtTestApp.findCommand("! location")
	if cmd == nil {
		t.Fatal("expected a command, got nil")
	}
	if cmd.Trigger != "!location" {
		t.Errorf("got trigger %q, want !location", cmd.Trigger)
	}
}

func TestFindCommand_WithParams(t *testing.T) {
	cmd, params := builtTestApp.findCommand("!goto 42")
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
	cmd, _ := builtTestApp.findCommand("!doesnotexist99")
	if cmd != nil {
		t.Errorf("expected nil for unknown command, got %q", cmd.Trigger)
	}
}

func TestFindCommand_EmptyMessage(t *testing.T) {
	cmd, _ := builtTestApp.findCommand("")
	if cmd != nil {
		t.Errorf("expected nil for empty message, got %q", cmd.Trigger)
	}
}

// --- trigger case-folding / param casing ---

// TestFindCommand_TriggerCaseInsensitive covers the trigger side of the split:
// the token is folded before lookup, so shouty and mixed-case triggers route.
func TestFindCommand_TriggerCaseInsensitive(t *testing.T) {
	for _, in := range []string{"!MILES", "!Miles", "!mIlEs", "HI", "! LOCATION"} {
		t.Run(in, func(t *testing.T) {
			cmd, _ := builtTestApp.findCommand(in)
			if cmd == nil {
				t.Fatalf("findCommand(%q) = nil, want a command", in)
			}
		})
	}
}

// TestFindCommand_LeadingWhitespace asserts a command typed with surrounding
// whitespace still dispatches.
func TestFindCommand_LeadingWhitespace(t *testing.T) {
	for _, in := range []string{" !miles", "\t!miles", "  !MILES  "} {
		t.Run(in, func(t *testing.T) {
			cmd, _ := builtTestApp.findCommand(in)
			if cmd == nil {
				t.Fatalf("findCommand(%q) = nil, want !miles", in)
			}
			if cmd.Trigger != "!miles" {
				t.Errorf("findCommand(%q) = %q, want !miles", in, cmd.Trigger)
			}
		})
	}
}

// TestFindCommand_ParamsKeepOriginalCase is the param side of the split: free
// text handed to a command survives verbatim, so !middle can set capitalized
// on-screen text.
func TestFindCommand_ParamsKeepOriginalCase(t *testing.T) {
	cmd, params := builtTestApp.findCommand("!MIDDLE Hello World")
	if cmd == nil {
		t.Fatal("expected a command, got nil")
	}
	if cmd.Trigger != "!middle" {
		t.Errorf("got trigger %q, want !middle", cmd.Trigger)
	}
	if len(params) != 2 || params[0] != "Hello" || params[1] != "World" {
		t.Errorf("params = %v, want [Hello World]", params)
	}
}

// TestFindCommand_MultiWordAliasKeepsParamCase covers the same for the
// multi-word alias path, which matches the alias across whole words.
func TestFindCommand_MultiWordAliasKeepsParamCase(t *testing.T) {
	cmd, params := builtTestApp.findCommand("No Audio Since Yesterday")
	if cmd == nil {
		t.Fatal("expected a command, got nil")
	}
	if cmd.Trigger != "!report" {
		t.Errorf("got trigger %q, want !report", cmd.Trigger)
	}
	if len(params) != 2 || params[0] != "Since" || params[1] != "Yesterday" {
		t.Errorf("params = %v, want [Since Yesterday]", params)
	}
}

// TestRunCommand_MiddleTextKeepsCapitalization is the end-to-end proof through
// the shared dispatcher both platform transports call: a shouty trigger routes
// and the free text lands on the overlay with its capitals.
func TestRunCommand_MiddleTextKeepsCapitalization(t *testing.T) {
	app := newTestApp(video.Video{})
	rec := &recordingOnscreens{}
	app.Onscreens = rec

	app.runCommand(context.Background(), newTestUser(adminUser), " !MIDDLE Hello World ")

	want := `ShowMiddleText("Hello World")`
	if len(rec.Calls) != 1 || rec.Calls[0] != want {
		t.Errorf("overlay calls = %v, want [%s]", rec.Calls, want)
	}
}

// TestRunCommand_MiddleHideStillHides guards the handlers that fold param case
// themselves: the dispatcher passes params through verbatim, so !middle's
// "hide" keyword has to still hide when typed shouty.
func TestRunCommand_MiddleHideStillHides(t *testing.T) {
	app := newTestApp(video.Video{})
	rec := &recordingOnscreens{}
	app.Onscreens = rec

	app.runCommand(context.Background(), newTestUser(adminUser), "!middle HIDE")

	if len(rec.Calls) != 1 || rec.Calls[0] != "HideMiddleText()" {
		t.Errorf("overlay calls = %v, want [HideMiddleText()]", rec.Calls)
	}
}

func TestTargetUsername(t *testing.T) {
	for in, want := range map[string]string{
		"@DanaMerrick": "danamerrick",
		"DanaMerrick":  "danamerrick",
		"@viewer1":     "viewer1",
	} {
		if got := targetUsername(in); got != want {
			t.Errorf("targetUsername(%q) = %q, want %q", in, got, want)
		}
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
	if !cmd.checkAccess(context.Background(), platformTwitch, &fakeUser{}, func(msg string) { said = msg }) {
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
	if cmd.checkAccess(context.Background(), platformTwitch, &fakeUser{follower: false}, func(msg string) { said = msg }) {
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
	if !cmd.checkAccess(context.Background(), platformTwitch, &fakeUser{follower: true}, func(msg string) { said = msg }) {
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
	if !cmd.checkAccess(context.Background(), platformTwitch, &fakeUser{follower: false}, func(msg string) { said = msg }) {
		t.Error("expected true for non-follower when gating disabled")
	}
	if said != "" {
		t.Errorf("expected no message when gating disabled, got %q", said)
	}
}

func TestCheckAccess_RequiresSubscriber_NonSubscriber(t *testing.T) {
	cmd := &Command{Trigger: "!test", RequiresSubscriber: true}
	var said string
	if cmd.checkAccess(context.Background(), platformTwitch, &fakeUser{subscriber: false}, func(msg string) { said = msg }) {
		t.Error("expected false for non-subscriber")
	}
	if said != subscriberMsg {
		t.Errorf("got %q, want subscriberMsg", said)
	}
}

func TestCheckAccess_RequiresSubscriber_Subscriber(t *testing.T) {
	cmd := &Command{Trigger: "!test", RequiresSubscriber: true}
	var said string
	if !cmd.checkAccess(context.Background(), platformTwitch, &fakeUser{subscriber: true}, func(msg string) { said = msg }) {
		t.Error("expected true for subscriber")
	}
	if said != "" {
		t.Errorf("expected no message, got %q", said)
	}
}

// A platform with no subscriber signal can't answer the subscriber gate, so it
// doesn't apply there — the alternative is a check no viewer could ever pass.
func TestCheckAccess_RequiresSubscriber_PlatformWithoutSubscribers(t *testing.T) {
	cmd := &Command{Trigger: "!test", RequiresSubscriber: true}
	for _, platform := range []string{platformTikTok, platformInstagram, platformYouTube, "kick"} {
		var said string
		if !cmd.checkAccess(context.Background(), platform, &fakeUser{subscriber: false}, func(msg string) { said = msg }) {
			t.Errorf("%s: expected the subscriber gate to be skipped", platform)
		}
		if said != "" {
			t.Errorf("%s: expected no denial message, got %q", platform, said)
		}
	}
}
