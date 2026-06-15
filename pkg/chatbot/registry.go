package chatbot

import (
	"context"
	"math/rand"
	"slices"
	"strings"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/users"
)

// buildRegistry constructs the command slice with handlers bound to a.
func (a *App) buildRegistry() []Command {
	return []Command{
		{
			Trigger: "!help",
			Handler: a.helpCmd,
		},
		{
			Trigger: "hello",
			Aliases: []string{"hi", "hey", "hallo", "!bot"},
			Handler: a.helloCmd,
		},
		{
			Trigger: "!flag",
			Handler: a.flagCmd,
		},
		{
			Trigger:   "!carsound",
			Aliases:   []string{"!carhum"},
			Handler:   a.carSoundCmd,
			Platforms: []string{platformYouTube}, // repoints the YouTube-only "Car Hum" OBS source
		},
		{
			Trigger: "!version",
			Handler: a.versionCmd,
		},
		{
			Trigger: "!uptime",
			Handler: a.uptimeCmd,
		},
		{
			Trigger: "!followage",
			Aliases: []string{"!followtime"},
			Handler: a.followageCmd,
		},
		{
			Trigger:        "!timewarp",
			Aliases:        []string{"!timeskip", "!tw", "!warp"},
			Handler:        a.timewarpCmd,
			RequiresFollow: true,
		},
		{
			Trigger:        "!goto",
			Aliases:        []string{"!jump"},
			Handler:        a.jumpCmd,
			RequiresFollow: true,
		},
		{
			Trigger:        "!skip",
			Handler:        a.skipCmd,
			RequiresFollow: true,
		},
		{
			Trigger:        "!back",
			Handler:        a.backCmd,
			RequiresFollow: true,
		},
		{
			Trigger: "!shutdown",
			Handler: a.shutdownCmd,
		},
		{
			Trigger: "!socialmedia",
			Aliases: []string{"!social", "!socials"},
			Handler: func(_ context.Context, _ *users.User, _ []string) {
				a.Chat.Say("Find me outside of Twitch: !youtube, !tiktok, !instagram, !bluesky")
			},
		},
		{
			Trigger: "!discord",
			Handler: func(_ context.Context, _ *users.User, _ []string) {
				a.Chat.Say("Join us on Discord: https://discord.gg/hKvNgZrk52")
			},
		},
		{
			Trigger: "!twitter",
			Handler: func(_ context.Context, _ *users.User, _ []string) {
				a.Chat.Say("Follow on Twitter: https://twitter.com/adanalife_")
			},
		},
		{
			Trigger: "!instagram",
			Aliases: []string{"!ig", "!insta"},
			Handler: func(_ context.Context, _ *users.User, _ []string) {
				a.Chat.Say("Follow on Instagram: https://instagram.com/adanalife_")
			},
		},
		{
			Trigger: "!facebook",
			Aliases: []string{"!fb"},
			Handler: func(_ context.Context, _ *users.User, _ []string) {
				a.Chat.Say("Follow on Facebook: https://www.facebook.com/adanalifeblog")
			},
		},
		{
			Trigger: "!youtube",
			Aliases: []string{"!yt"},
			Handler: func(_ context.Context, _ *users.User, _ []string) {
				a.Chat.Say("Subscribe on YouTube: https://www.youtube.com/channel/UC8Q7uFC1Xyr2ZnTWOk9Aizg")
			},
		},
		{
			Trigger: "!tiktok",
			Handler: func(_ context.Context, _ *users.User, _ []string) {
				a.Chat.Say("Follow on TikTok: https://tiktok.com/@adanalife")
			},
		},
		{
			Trigger: "!bluesky",
			Aliases: []string{"!bsky"},
			Handler: func(_ context.Context, _ *users.User, _ []string) {
				a.Chat.Say("Follow on Bluesky: https://bsky.app/profile/dana.lol")
			},
		},
		{
			Trigger: "!commands",
			Aliases: []string{"!command", "!controls"},
			Handler: a.commandsCmd,
		},
		{
			Trigger:            "!bonusmiles",
			Handler:            a.bonusMilesCmd,
			RequiresSubscriber: true,
		},
		{
			Trigger:        "!sunset",
			Handler:        a.sunsetCmd,
			RequiresFollow: true,
		},
		{
			Trigger:        "!weather",
			Aliases:        []string{"!meteo"},
			Handler:        a.weatherCmd,
			RequiresFollow: true,
		},
		{
			Trigger:        "!time",
			Handler:        a.timeCmd,
			RequiresFollow: true,
		},
		{
			Trigger:        "!date",
			Aliases:        []string{"is this live", "is this live?"},
			Handler:        a.dateCmd,
			RequiresFollow: true,
		},
		{
			Trigger: "!guess",
			// "!guis" stays: it's 2 edits from !guess, beyond fuzzyLookup's
			// reach at that length (max 1 edit for inputs of 4-6 runes)
			Aliases:        []string{"guess", "!guis"},
			Handler:        a.guessCmd,
			RequiresFollow: true,
		},
		{
			Trigger:        "!state",
			Handler:        a.stateCmd,
			RequiresFollow: true,
		},
		{
			Trigger: "!secretinfo",
			Handler: a.secretInfoCmd,
		},
		{
			Trigger: "!gas",
			Aliases: []string{"!fuel", "!petrol"},
			Handler: func(_ context.Context, _ *users.User, _ []string) {
				a.Chat.Say("About full, thanks for asking")
			},
		},
		{
			Trigger: "!middle",
			Handler: a.middleCmd,
		},
		{
			Trigger: "!makebot",
			Handler: a.makeBotCmd,
		},
		{
			Trigger: "!unbot",
			Handler: a.unBotCmd,
		},
		{
			Trigger:        "!miles",
			Aliases:        []string{"!points"},
			Handler:        a.milesCmd,
			RequiresFollow: true,
		},
		{
			Trigger:        "!km",
			Aliases:        []string{"!kilometres", "!kilometers"},
			Handler:        a.kilometresCmd,
			RequiresFollow: true,
		},
		{
			Trigger: "!location",
			// "!loclistion" stays: 3 edits from !location, beyond
			// fuzzyLookup's max of 2
			Aliases:        []string{"!tripbot", "!city", "!town", "!where", "!loc", "!loclistion"},
			Handler:        a.locationCmd,
			RequiresFollow: true,
		},
		{
			Trigger:        "!leaderboard",
			Aliases:        []string{"!monthlyleaderboard", "!lb", "!mlb", "!ldb", "!ldbd"},
			Handler:        a.monthlyMilesLeaderboardCmd,
			RequiresFollow: true,
		},
		{
			Trigger:        "!totalleaderboard",
			Aliases:        []string{"!lifetimeleaderboard", "!tlb", "!llb"},
			Handler:        a.lifetimeMilesLeaderboardCmd,
			RequiresFollow: true,
		},
		{
			Trigger:        "!guessleaderboard",
			Aliases:        []string{"!glb"},
			Handler:        a.monthlyGuessLeaderboardCmd,
			RequiresFollow: true,
		},
		{
			Trigger:        "!report",
			Aliases:        []string{"no audio", "no sound", "no music", "frozen"},
			Handler:        a.reportCmd,
			RequiresFollow: false,
		},
		{
			Trigger: "!song",
			Aliases: []string{"!music"},
			Handler: a.songCmd,
		},
		{
			Trigger: "!somafm",
			Handler: func(_ context.Context, _ *users.User, _ []string) {
				a.Chat.Say("Stream music by SomaFM — https://somafm.com")
			},
		},
	}
}

// Platform names for App.Platform. Add a constant here when a new streaming
// platform (Kick, TikTok, …) comes online; platform-specific commands then
// reference it via Command.Platforms.
const (
	platformTwitch  = "twitch"
	platformYouTube = "youtube"
)

// platform returns this App's platform, normalizing the empty/unset value to
// Twitch. Twitch was the original (and is still the most mature) platform, so
// an App constructed without an explicit platform behaves as Twitch. This is
// the single place that "empty defaults to Twitch" lives — change it here if
// that assumption ever needs to move.
func (a *App) platform() string {
	if a.Platform == "" {
		return platformTwitch
	}
	return a.Platform
}

// youtubeCommands is the v1 allowlist of triggers a YouTube instance runs — the
// "info + playback control" subset, plus the !state/!location info commands.
// Identity/miles commands (!miles, !leaderboard, !guess, …), the Twitch-only
// !followage, and the admin commands (!middle, !secretinfo, !shutdown, !makebot,
// !unbot) are excluded: those are per-user identity/score state. The now-playing /
// SomaFM commands (!song, !music, !somafm) are also deferred for now — the
// background-audio source is Twitch-stream-specific. Aliases come along with
// their trigger, so only triggers are listed. See the YouTube provider plan.
var youtubeCommands = map[string]bool{
	"!help": true, "!version": true, "!uptime": true, "!commands": true,
	"!gas": true, "!report": true, "!flag": true,
	// info (read current-video state only)
	"!weather": true, "!time": true, "!date": true, "!sunset": true,
	"!state": true, "!location": true,
	// playback control (drives this platform's vlc pipeline)
	"!timewarp": true, "!goto": true, "!skip": true, "!back": true,
	// socials / static links
	"!socialmedia": true, "!discord": true, "!twitter": true, "!instagram": true,
	"!facebook": true, "!youtube": true, "!tiktok": true, "!bluesky": true,
}

// commandEnabled reports whether cmd should be indexed for dispatch on this
// App's platform. Two orthogonal concerns, in order:
//
//  1. Platform-specific commands declare their scope via Command.Platforms. A
//     command with a non-nil Platforms is governed solely by it — indexed on
//     exactly the listed platforms, on every platform. This is symmetric: no
//     platform is special, and a new Kick/TikTok-only command just lists itself.
//  2. Cross-platform commands (Platforms == nil): YouTube is still a v1
//     rollout, so it runs only the vetted youtubeCommands allowlist; mature
//     platforms (Twitch, and any future fully-rolled-out platform) run them all.
//     As more platforms graduate from v1 this allowlist gate may invert, but
//     that's a future call.
func (a *App) commandEnabled(cmd *Command) bool {
	if len(cmd.Platforms) > 0 {
		return slices.Contains(cmd.Platforms, a.platform())
	}
	if a.platform() == platformYouTube {
		return youtubeCommands[cmd.Trigger]
	}
	return true
}

// indexCommands builds a.commands from a.buildRegistry() and indexes it into
// a.singleWordLookup / a.multiWordLookup by trigger and alias. Call once after
// the App is constructed (its deps don't need to be set — buildRegistry only
// binds handler method values to a). Commands not enabled for a.Platform
// (commandEnabled) stay in a.commands but are never indexed, so they don't
// dispatch on that platform.
func (a *App) indexCommands() {
	a.commands = a.buildRegistry()
	a.singleWordLookup = make(map[string]*Command)
	a.multiWordLookup = make(map[string]*Command)
	for i := range a.commands {
		cmd := &a.commands[i]
		if !a.commandEnabled(cmd) {
			continue
		}
		a.registerTrigger(cmd.Trigger, cmd)
		for _, alias := range cmd.Aliases {
			a.registerTrigger(alias, cmd)
		}
	}
	// Filter the rotating help lines to this platform, then start on a random
	// one (so each restart opens differently). Must run after the lookups are
	// built — enabledHelpMessages reads singleWordLookup.
	a.helpMessages = a.enabledHelpMessages()
	if len(a.helpMessages) > 0 {
		a.helpIndex = rand.Intn(len(a.helpMessages))
	}
}

// enabledHelpMessages returns c.HelpMessages minus any line whose leading
// "!command" token isn't dispatchable on this platform — so a YouTube instance
// never advertises a command that would silently no-op. A line that doesn't
// start with a "!command" token is always kept.
func (a *App) enabledHelpMessages() []string {
	out := make([]string, 0, len(c.HelpMessages))
	for _, msg := range c.HelpMessages {
		fields := strings.Fields(msg)
		if len(fields) > 0 {
			token := strings.TrimRight(fields[0], ":")
			if strings.HasPrefix(token, "!") {
				if _, ok := a.singleWordLookup[token]; !ok {
					continue // command disabled on this platform
				}
			}
		}
		out = append(out, msg)
	}
	return out
}

func (a *App) registerTrigger(trigger string, cmd *Command) {
	if strings.Contains(trigger, " ") {
		a.multiWordLookup[trigger] = cmd
	} else {
		a.singleWordLookup[trigger] = cmd
	}
}
