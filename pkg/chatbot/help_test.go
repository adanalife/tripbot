package chatbot

import (
	"context"
	"strings"
	"testing"
)

// Every command carries a Help line, so "!help <trigger>" can never answer
// "I don't know" for a command that exists.
func TestRegistryEveryCommandHasHelp(t *testing.T) {
	for _, cmd := range (&App{}).buildRegistry() {
		if cmd.Help == "" {
			t.Errorf("%s has no Help line", cmd.Trigger)
		}
	}
}

func TestHelpForCommand(t *testing.T) {
	twitch := &App{}
	twitch.indexCommands()
	yt := &App{Platform: platformYouTube}
	yt.indexCommands()

	cases := []struct {
		app  *App
		name string
		want string
	}{
		{twitch, "timewarp", "!timewarp: Jump to a random different video"},
		{twitch, "!TW", "!timewarp:"}, // bang and case are both optional; aliases resolve
		{twitch, "timewarp", "also !timeskip, !tw, !warp"},
		{twitch, "find", "(subscribers)"},
		{twitch, "nope", "I don't know !nope"},
		{twitch, "shutdown", "I don't know !shutdown"}, // admin-only stays unadvertised
		{yt, "miles", "I don't know !miles"},           // not dispatchable on YouTube
		{yt, "find", "!find:"},                         // ungated there, so no suffix
	}
	for _, tc := range cases {
		got := tc.app.helpFor(nil, tc.name)
		if !strings.Contains(got, tc.want) {
			t.Errorf("helpFor(%q) on %s = %q, want it to contain %q", tc.name, tc.app.platform(), got, tc.want)
		}
	}
	if got := yt.helpFor(nil, "find"); strings.Contains(got, "(subscribers)") {
		t.Errorf("YouTube has no subscriber signal, so !find should read ungated: %q", got)
	}

	// Through the real handler: "!help timewarp" answers with the man page,
	// bare "!help" still lists the surface.
	chat := &recordingChat{}
	twitch.Chat = chat
	twitch.commandsCmd(context.Background(), nil, []string{"timewarp"})
	if out := chat.Output(); !strings.Contains(out, "!timewarp: Jump") {
		t.Errorf("!help timewarp = %q", out)
	}
	chat2 := &recordingChat{}
	twitch.Chat = chat2
	twitch.commandsCmd(context.Background(), nil, nil)
	if out := chat2.Output(); !strings.Contains(out, "You can try:") {
		t.Errorf("bare !help = %q", out)
	}
}
