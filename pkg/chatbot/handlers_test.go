package chatbot

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/adanalife/tripbot/pkg/events"
	"github.com/adanalife/tripbot/pkg/users"
	"github.com/adanalife/tripbot/pkg/video"
	"github.com/adanalife/tripbot/pkg/viewstats"
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
	cmd, _, params := builtTestApp.findCommand("!version")
	if cmd == nil {
		t.Fatal("expected a command, got nil")
	}
	if cmd.Trigger != "!version" {
		t.Errorf("got trigger %q, want !version", cmd.Trigger)
	}
	if len(params) != 0 {
		t.Errorf("unexpected params: %v", params)
	}
}

func TestFindCommand_SingleWordAlias(t *testing.T) {
	// "hi" is an alias of "hello"
	cmd, _, _ := builtTestApp.findCommand("hi")
	if cmd == nil {
		t.Fatal("expected a command, got nil")
	}
	if cmd.Trigger != "hello" {
		t.Errorf("got trigger %q, want hello", cmd.Trigger)
	}
}

// "!hello" and bare "hello" deliberately route to different commands — the bang
// lists the command surface, the bare greeting greets back. Nothing about the
// matcher would produce that on its own: fuzzyLookup skips bare-word triggers,
// so "!hello" reached no command at all before it was registered as an alias.
// Pinned as a pair because the split is surprising enough to look like a bug.
func TestFindCommand_BangHelloListsCommands(t *testing.T) {
	cmd, _, _ := builtTestApp.findCommand("!hello")
	if cmd == nil {
		t.Fatal("expected a command, got nil")
	}
	if cmd.Trigger != "!commands" {
		t.Errorf("!hello routed to %q, want !commands", cmd.Trigger)
	}

	bare, _, _ := builtTestApp.findCommand("hello")
	if bare == nil {
		t.Fatal("expected a command for bare hello, got nil")
	}
	if bare.Trigger != "hello" {
		t.Errorf("bare hello routed to %q, want hello", bare.Trigger)
	}
}

// "!help" lists the command surface rather than answering with one rotating
// tip. Pinned because the reply a viewer gets from it is the whole point of the
// alias, and nothing else in the registry would show it moving.
func TestFindCommand_HelpListsCommands(t *testing.T) {
	cmd, _, _ := builtTestApp.findCommand("!help")
	if cmd == nil {
		t.Fatal("expected a command, got nil")
	}
	if cmd.Trigger != "!commands" {
		t.Errorf("!help routed to %q, want !commands", cmd.Trigger)
	}
}

func TestFindCommand_MultiWordAlias(t *testing.T) {
	// "no audio" is an alias of !report
	cmd, _, params := builtTestApp.findCommand("no audio")
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
	cmd, _, params := builtTestApp.findCommand("frozen since yesterday")
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
	cmd, _, _ := builtTestApp.findCommand("¡miles")
	if cmd == nil {
		t.Fatal("expected a command, got nil")
	}
	if cmd.Trigger != "!miles" {
		t.Errorf("got trigger %q, want !miles", cmd.Trigger)
	}
}

func TestFindCommand_DigitOnePrefixRoutes(t *testing.T) {
	// 1location should route to the same command as !location
	cmd, _, _ := builtTestApp.findCommand("1location")
	if cmd == nil {
		t.Fatal("expected a command, got nil")
	}
	if cmd.Trigger != "!location" {
		t.Errorf("got trigger %q, want !location", cmd.Trigger)
	}
}

func TestFindCommand_NumberLeadingMessageDoesNotRoute(t *testing.T) {
	// a message that genuinely starts with a number must not become a command
	if cmd, _, _ := builtTestApp.findCommand("100 miles"); cmd != nil {
		t.Errorf("findCommand(\"100 miles\") = %s, want nil", cmd.Trigger)
	}
}

func TestFindCommand_SpaceSeparatedBang(t *testing.T) {
	// "! location" (with a space) should route to !location
	cmd, _, _ := builtTestApp.findCommand("! location")
	if cmd == nil {
		t.Fatal("expected a command, got nil")
	}
	if cmd.Trigger != "!location" {
		t.Errorf("got trigger %q, want !location", cmd.Trigger)
	}
}

func TestFindCommand_WithParams(t *testing.T) {
	cmd, _, params := builtTestApp.findCommand("!goto 42")
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
	cmd, _, _ := builtTestApp.findCommand("!doesnotexist99")
	if cmd != nil {
		t.Errorf("expected nil for unknown command, got %q", cmd.Trigger)
	}
}

func TestFindCommand_EmptyMessage(t *testing.T) {
	cmd, _, _ := builtTestApp.findCommand("")
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
			cmd, _, _ := builtTestApp.findCommand(in)
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
			cmd, _, _ := builtTestApp.findCommand(in)
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
	cmd, _, params := builtTestApp.findCommand("!MIDDLE Hello World")
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
	cmd, _, params := builtTestApp.findCommand("No Audio Since Yesterday")
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

// A command this bot doesn't have is viewer traffic, not a fault, so it must
// stay under Error — the level pkg/telemetry converts into a Sentry event.
// !watchtime (a trigger from some other channel's bot) reached prod as an error
// and minted its own Sentry issue. The command_refused event carries the "what
// are people reaching for" signal instead.
func TestRunCommand_UnknownCommandStaysBelowError(t *testing.T) {
	app := newTestApp(video.Video{})
	rec := &recordingEvents{}
	app.Events = rec

	var buf bytes.Buffer
	restore := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	defer slog.SetDefault(restore)

	app.runCommand(context.Background(), newTestUser(adminUser), "!watchtime")

	if strings.Contains(buf.String(), "level=ERROR") {
		t.Errorf("unknown command logged at ERROR (reaches Sentry); log = %q", buf.String())
	}
	if !strings.Contains(buf.String(), "!watchtime") {
		t.Errorf("unknown command not logged at all; log = %q", buf.String())
	}
	if len(rec.Refusals) != 1 {
		t.Fatalf("refusals = %d, want 1", len(rec.Refusals))
	}
	if got := rec.Refusals[0]; got.Reason != events.RefusedUnknown || got.Command != "!watchtime" {
		t.Errorf("refusal = %+v, want command !watchtime reason %q", got, events.RefusedUnknown)
	}
}

// A command that exists but isn't indexed here misses findCommand exactly like a
// typo does. They're different questions — a typo may seed a new command, while
// an unreachable one argues for enabling it on this platform — so the reason has
// to separate them rather than lumping both under "unknown".
func TestRunCommand_PlatformGatedCommandRefusesAsWrongPlatform(t *testing.T) {
	app := newTestApp(video.Video{})
	app.Platform = platformFacebook
	app.indexCommands()
	rec := &recordingEvents{}
	app.Events = rec
	out, says := captureSay(t, app)

	// !somafm is in the registry but outside the v1 allowlist Facebook runs.
	app.runCommand(context.Background(), newTestUser(adminUser), "!somafm")

	if len(rec.Refusals) != 1 {
		t.Fatalf("refusals = %d, want 1", len(rec.Refusals))
	}
	if got := rec.Refusals[0].Reason; got != events.RefusedWrongPlatform {
		t.Errorf("reason = %q, want %q", got, events.RefusedWrongPlatform)
	}
	// The viewer typed a real trigger, so they hear why nothing happened.
	if !strings.Contains(out(), "!somafm") {
		t.Errorf("say = %q, want it to name !somafm", out())
	}
	if says() != 1 {
		t.Errorf("expected exactly one Say() call, got %d", says())
	}
}

// A typo names no command at all, so there is nothing to explain — answering
// it would let anyone make the bot echo arbitrary tokens into chat.
func TestRunCommand_UnknownCommandStaysSilent(t *testing.T) {
	app := newTestApp(video.Video{})
	rec := &recordingEvents{}
	app.Events = rec
	out, _ := captureSay(t, app)

	app.runCommand(context.Background(), newTestUser("viewer1"), "!definitelynotacommand")

	if out() != "" {
		t.Errorf("expected silence for an unknown command, got %q", out())
	}
}

// A refusal stamps the airing clip, which is the whole point of recording it in
// events rather than a counter: "which commands do people reach for during which
// footage" is a join, and it's only possible if the clip is on the row.
func TestRunCommand_RefusalStampsAiringClip(t *testing.T) {
	app := newTestApp(video.Video{ID: 77})
	rec := &recordingEvents{}
	app.Events = rec

	app.runCommand(context.Background(), newTestUser(adminUser), "!watchtime extra args")

	if len(rec.Refusals) != 1 {
		t.Fatalf("refusals = %d, want 1", len(rec.Refusals))
	}
	got := rec.Refusals[0]
	if got.VideoID != 77 {
		t.Errorf("video id = %d, want 77", got.VideoID)
	}
	if got.TsSec == nil {
		t.Error("ts_sec = nil, want the playhead stamped")
	}
	if got.Args != "extra args" {
		t.Errorf("args = %q, want %q", got.Args, "extra args")
	}
}

// stubChatterSource is the minimum ChatterSource that lets a test build a real
// users.Sessions: nobody is in chat, follows, or subscribes. Enough to drive
// dispatch's access gates, which read a Sessions rather than the injectable
// chatUser the checkAccess unit tests use.
type stubChatterSource struct{}

func (stubChatterSource) UpdateChatters()               {}
func (stubChatterSource) Chatters() map[string]struct{} { return map[string]struct{}{} }
func (stubChatterSource) ChatterCount() int             { return 0 }
func (stubChatterSource) IsSubscriber(_ string) bool    { return false }
func (stubChatterSource) SubscriberTier(_ string) int   { return 0 }
func (stubChatterSource) IsFollower(_ string) bool      { return false }
func (stubChatterSource) UpdateAudience()               {}
func (stubChatterSource) Audience() viewstats.Audience  { return viewstats.Audience{} }

// A gate refusal is a refusal too — the command existed and was reachable, and
// still didn't run. Leaving the gates unrecorded would make any refusal rate
// computed from the log quietly wrong, so this covers the dispatch emit site
// (the subscriber gate is the one a fresh test user can actually fail; the
// follow gate always passes for a user with no command history).
func TestDispatch_SubscriberGateRecordsRefusal(t *testing.T) {
	app := newTestApp(video.Video{})
	app.UserSessions = users.New(testConf, stubChatterSource{})
	rec := &recordingEvents{}
	app.Events = rec

	var ran bool
	cmd := &Command{
		Trigger:            "!test",
		RequiresSubscriber: true,
		Handler:            func(context.Context, *users.User, []string) { ran = true },
	}
	app.dispatch(context.Background(), cmd, "!test", newTestUser("somenonsubscriber"), []string{"arg"})

	if ran {
		t.Error("handler ran despite the subscriber gate")
	}
	if len(rec.Refusals) != 1 {
		t.Fatalf("refusals = %d, want 1", len(rec.Refusals))
	}
	if got := rec.Refusals[0]; got.Reason != events.RefusedSubGate || got.Command != "!test" {
		t.Errorf("refusal = %+v, want command !test reason %q", got, events.RefusedSubGate)
	}
	// A refused command must not also count as a run — each attempt lands in
	// exactly one of the two kinds.
	if len(rec.Runs) != 0 {
		t.Errorf("runs = %+v, want none for a refused command", rec.Runs)
	}
}

// --- command_run emit site ---

// A dispatched command records exactly one command_run row, stamped with the
// airing clip and playhead. The canonical trigger typed as-is carries no typed
// field — that key only appears when the viewer reached for something else.
func TestRunCommand_RecordsCommandRun(t *testing.T) {
	app := newTestApp(video.Video{ID: 77})
	rec := &recordingEvents{}
	app.Events = rec

	app.runCommand(context.Background(), newTestUser("viewer1"), "!version")

	if len(rec.Runs) != 1 {
		t.Fatalf("runs = %d, want 1", len(rec.Runs))
	}
	got := rec.Runs[0]
	if got.Command != "!version" || got.Username != "viewer1" {
		t.Errorf("run = %+v, want command !version by viewer1", got)
	}
	if got.Typed != "" {
		t.Errorf("typed = %q, want empty when the canonical trigger was typed", got.Typed)
	}
	if got.VideoID != 77 {
		t.Errorf("video id = %d, want 77", got.VideoID)
	}
	if got.TsSec == nil {
		t.Error("ts_sec = nil, want the playhead stamped")
	}
	if len(rec.Refusals) != 0 {
		t.Errorf("refusals = %+v, want none for a successful run", rec.Refusals)
	}
}

// An alias run keeps the token the viewer actually reached for — the signal
// that says which aliases are worth promoting.
func TestRunCommand_AliasRunKeepsTypedToken(t *testing.T) {
	app := newTestApp(video.Video{})
	rec := &recordingEvents{}
	app.Events = rec

	// "hi" is an alias of the "hello" trigger.
	app.runCommand(context.Background(), newTestUser("viewer1"), "hi")

	if len(rec.Runs) != 1 {
		t.Fatalf("runs = %d, want 1", len(rec.Runs))
	}
	got := rec.Runs[0]
	if got.Command != "hello" || got.Typed != "hi" {
		t.Errorf("run = %+v, want command hello typed hi", got)
	}
}

// A state shortcut runs !guess, and the run keeps the shortcut as typed with
// the state as args — so "!florida" traffic is attributable to the shortcut
// rather than blending into plain !guess usage.
func TestRunCommand_StateShortcutRunKeepsTypedToken(t *testing.T) {
	app := newTestApp(video.Video{})
	rec := &recordingEvents{}
	app.Events = rec

	app.runCommand(context.Background(), newTestUser("viewer1"), "!florida")

	if len(rec.Runs) != 1 {
		t.Fatalf("runs = %d, want 1", len(rec.Runs))
	}
	got := rec.Runs[0]
	if got.Command != "!guess" || got.Typed != "!florida" || got.Args != "florida" {
		t.Errorf("run = %+v, want command !guess typed !florida args florida", got)
	}
}

// An unknown token records its refusal and nothing else — no command ran, so
// no command_run row.
func TestRunCommand_UnknownCommandRecordsNoRun(t *testing.T) {
	app := newTestApp(video.Video{})
	rec := &recordingEvents{}
	app.Events = rec

	app.runCommand(context.Background(), newTestUser("viewer1"), "!watchtime")

	if len(rec.Refusals) != 1 {
		t.Fatalf("refusals = %d, want 1", len(rec.Refusals))
	}
	if len(rec.Runs) != 0 {
		t.Errorf("runs = %+v, want none for an unknown command", rec.Runs)
	}
}

// The !guess cooldown refuses from inside the handler, after dispatch has
// already committed to running it. The refusal mark is what keeps the one-row
// invariant there: the attempt records the cooldown refusal and no run.
func TestRunCommand_CooldownRefusalRecordsNoRun(t *testing.T) {
	app := newTestApp(video.Video{})
	rec := &recordingEvents{}
	app.Events = rec

	// The cooldown is live when the user's last answer is newer than the last
	// timewarp; pin the warp clock into the past so this test doesn't depend
	// on package state left by other tests.
	prevWarp := lastTimewarpTime
	lastTimewarpTime = time.Now().Add(-time.Hour)
	t.Cleanup(func() { lastTimewarpTime = prevWarp })

	user := newTestUser("viewer1")
	user.SetLastLocationTime()

	app.runCommand(context.Background(), user, "!guess CO")

	if len(rec.Refusals) != 1 {
		t.Fatalf("refusals = %d, want 1", len(rec.Refusals))
	}
	if got := rec.Refusals[0]; got.Reason != events.RefusedCooldown {
		t.Errorf("reason = %q, want %q", got.Reason, events.RefusedCooldown)
	}
	if len(rec.Runs) != 0 {
		t.Errorf("runs = %+v, want none for a cooldown-refused guess", rec.Runs)
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
	admin      bool
}

func (f *fakeUser) HasCommandAvailable(_ context.Context) bool { return f.follower }
func (f *fakeUser) IsSubscriber() bool                         { return f.subscriber }
func (f *fakeUser) IsAdmin() bool                              { return f.admin }

// sessionUser (a *users.User + the installed *Sessions) is the production
// chatUser; this asserts it satisfies the seam.
var _ chatUser = sessionUser{}

func TestCheckAccess_NoRestrictions(t *testing.T) {
	cmd := &Command{Trigger: "!test"}
	var said string
	if ok, _ := cmd.checkAccess(context.Background(), platformTwitch, &fakeUser{}, func(msg string) { said = msg }); !ok {
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
	ok, refused := cmd.checkAccess(context.Background(), platformTwitch, &fakeUser{follower: false}, func(msg string) { said = msg })
	if ok {
		t.Error("expected false for non-follower")
	}
	// The reason names the gate for the command_refused event, so a rollup can
	// tell a follow-gated refusal from a typo.
	if refused != events.RefusedFollowGate {
		t.Errorf("refused = %q, want %q", refused, events.RefusedFollowGate)
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
	if ok, _ := cmd.checkAccess(context.Background(), platformTwitch, &fakeUser{follower: true}, func(msg string) { said = msg }); !ok {
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
	if ok, _ := cmd.checkAccess(context.Background(), platformTwitch, &fakeUser{follower: false}, func(msg string) { said = msg }); !ok {
		t.Error("expected true for non-follower when gating disabled")
	}
	if said != "" {
		t.Errorf("expected no message when gating disabled, got %q", said)
	}
}

func TestCheckAccess_RequiresSubscriber_NonSubscriber(t *testing.T) {
	cmd := &Command{Trigger: "!test", RequiresSubscriber: true}
	var said string
	ok, refused := cmd.checkAccess(context.Background(), platformTwitch, &fakeUser{subscriber: false}, func(msg string) { said = msg })
	if ok {
		t.Error("expected false for non-subscriber")
	}
	if refused != events.RefusedSubGate {
		t.Errorf("refused = %q, want %q", refused, events.RefusedSubGate)
	}
	if said != subscriberMsg {
		t.Errorf("got %q, want subscriberMsg", said)
	}
}

func TestCheckAccess_RequiresSubscriber_Subscriber(t *testing.T) {
	cmd := &Command{Trigger: "!test", RequiresSubscriber: true}
	var said string
	if ok, _ := cmd.checkAccess(context.Background(), platformTwitch, &fakeUser{subscriber: true}, func(msg string) { said = msg }); !ok {
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
		if ok, _ := cmd.checkAccess(context.Background(), platform, &fakeUser{subscriber: false}, func(msg string) { said = msg }); !ok {
			t.Errorf("%s: expected the subscriber gate to be skipped", platform)
		}
		if said != "" {
			t.Errorf("%s: expected no denial message, got %q", platform, said)
		}
	}
}

// The admin gate declines in silence by default — an admin command shouldn't
// advertise itself to the chat that can't run it.
func TestCheckAccess_RequiresAdmin_NonAdmin(t *testing.T) {
	cmd := &Command{Trigger: "!test", RequiresAdmin: true}
	var said string
	ok, refused := cmd.checkAccess(context.Background(), platformTwitch, &fakeUser{admin: false}, func(msg string) { said = msg })
	if ok {
		t.Error("expected false for non-admin")
	}
	if refused != events.RefusedAdminGate {
		t.Errorf("refused = %q, want %q", refused, events.RefusedAdminGate)
	}
	if said != "" {
		t.Errorf("expected no message, got %q", said)
	}
}

func TestCheckAccess_RequiresAdmin_AdminDeniedMsg(t *testing.T) {
	cmd := &Command{Trigger: "!test", RequiresAdmin: true, AdminDeniedMsg: "Nice try bucko"}
	var said string
	if ok, _ := cmd.checkAccess(context.Background(), platformTwitch, &fakeUser{admin: false}, func(msg string) { said = msg }); ok {
		t.Error("expected false for non-admin")
	}
	if said != "Nice try bucko" {
		t.Errorf("got %q, want the command's AdminDeniedMsg", said)
	}
}

func TestCheckAccess_RequiresAdmin_Admin(t *testing.T) {
	cmd := &Command{Trigger: "!test", RequiresAdmin: true, AdminDeniedMsg: "Nice try bucko"}
	var said string
	if ok, _ := cmd.checkAccess(context.Background(), platformTwitch, &fakeUser{admin: true}, func(msg string) { said = msg }); !ok {
		t.Error("expected true for admin")
	}
	if said != "" {
		t.Errorf("expected no message, got %q", said)
	}
}

// Every command that used to gate itself inside its handler now declares it,
// so a new admin command that forgets the field fails here rather than
// shipping wide open.
func TestRegistry_AdminCommandsDeclareRequiresAdmin(t *testing.T) {
	want := map[string]bool{
		"!shutdown": true, "!refreshoverlays": true, "!secretinfo": true,
		"!middle": true, "!makebot": true, "!unbot": true, "!givemiles": true,
	}
	for _, cmd := range (&App{}).buildRegistry() {
		if want[cmd.Trigger] && !cmd.RequiresAdmin {
			t.Errorf("%s: RequiresAdmin = false, want true", cmd.Trigger)
		}
		delete(want, cmd.Trigger)
	}
	for trigger := range want {
		t.Errorf("%s: missing from the registry", trigger)
	}
}

// The admin gate has to bite in dispatch, not just read true in the registry:
// a non-admin gets no chat, no side effect, and a recorded refusal.
func TestDispatch_AdminGateRefusesAndRecords(t *testing.T) {
	app := newTestApp(video.Video{})
	app.UserSessions = users.New(testConf, stubChatterSource{})
	rec := &recordingEvents{}
	app.Events = rec
	out, _ := captureSay(t, app)

	var ran bool
	cmd := &Command{
		Trigger:       "!test",
		RequiresAdmin: true,
		Handler:       func(context.Context, *users.User, []string) { ran = true },
	}
	app.dispatch(context.Background(), cmd, "!test", newTestUser("viewer1"), nil)

	if ran {
		t.Error("handler ran despite the admin gate")
	}
	if out() != "" {
		t.Errorf("expected silence for a non-admin, got %q", out())
	}
	if len(rec.Refusals) != 1 || rec.Refusals[0].Reason != events.RefusedAdminGate {
		t.Errorf("refusals = %+v, want one %q", rec.Refusals, events.RefusedAdminGate)
	}
}
