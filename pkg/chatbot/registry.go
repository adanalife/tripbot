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
			Trigger: "hello",
			Help:    "Say hi and I'll say hi back",
			Aliases: []string{"hi", "hey", "hallo", "!bot"},
			Handler: a.helloCmd,
		},
		{
			Trigger: "!audio",
			Help:    "What's playing on the background audio; admins switch the bed or tune a SomaFM channel",
			Aliases: []string{"!bed", "!carsound", "!carhum", "!radio"},
			Handler: a.audioCmd,
		},
		{
			Trigger: "!version",
			Help:    "The bot's build version",
			Handler: a.versionCmd,
		},
		{
			Trigger: "!uptime",
			Help:    "How long the stream and the bot have been up",
			Handler: a.uptimeCmd,
		},
		{
			Trigger: "!followage",
			Help:    "How long you've been following the channel",
			Aliases: []string{"!followtime"},
			Handler: a.followageCmd,
		},
		{
			Trigger:        "!timewarp",
			Help:           "Jump to a random different video",
			Aliases:        []string{"!timeskip", "!tw", "!warp"},
			Handler:        a.timewarpCmd,
			RequiresFollow: true,
		},
		{
			Trigger:        "!goto",
			Help:           "Jump to footage from a specific state: !goto [state]",
			Aliases:        []string{"!jump"},
			Handler:        a.jumpCmd,
			RequiresFollow: true,
		},
		{
			Trigger:            "!find",
			Help:               "Search the footage for a scene and jump to it: !find <a tunnel at sunset>",
			Aliases:            []string{"!search"},
			Handler:            a.findCmd,
			RequiresSubscriber: true,
		},
		{
			Trigger:        "!skip",
			Help:           "Skip to the next video",
			Handler:        a.skipCmd,
			RequiresFollow: true,
		},
		{
			Trigger:        "!back",
			Help:           "Go back to the previous video",
			Handler:        a.backCmd,
			RequiresFollow: true,
		},
		{
			Trigger:        "!daytime",
			Help:           "Jump ahead to the next daytime footage",
			Aliases:        []string{"!daylight", "!morning"},
			Handler:        a.daytimeCmd,
			RequiresFollow: true,
		},
		{
			Trigger:        "!shutdown",
			Help:           "Shut the bot down",
			Handler:        a.shutdownCmd,
			RequiresAdmin:  true,
			AdminDeniedMsg: "Nice try bucko",
		},
		{
			Trigger:       "!refreshoverlays",
			Help:          "Reload the on-screen overlays",
			Aliases:       []string{"!refreshoverlay"},
			Handler:       a.refreshOverlaysCmd,
			RequiresAdmin: true,
		},
		{
			Trigger: "!socialmedia",
			Help:    "All the places to find me off-stream",
			Aliases: []string{"!social", "!socials"},
			Handler: func(_ context.Context, _ *users.User, _ []string) {
				a.Chat.Say("Find me outside of Twitch: !youtube, !tiktok, !instagram, !bluesky — or play the dashcam guessing game at " + guessrGameURL)
			},
		},
		{
			Trigger: "!discord",
			Help:    "The Discord invite link",
			Handler: func(_ context.Context, _ *users.User, _ []string) {
				a.Chat.Say("Join us on Discord: https://discord.gg/hKvNgZrk52")
			},
		},
		{
			Trigger: "!twitter",
			Help:    "The Twitter link",
			Handler: func(_ context.Context, _ *users.User, _ []string) {
				a.Chat.Say("Follow on Twitter: https://twitter.com/adanalife_")
			},
		},
		{
			Trigger: "!instagram",
			Help:    "The Instagram link",
			Aliases: []string{"!ig", "!insta"},
			Handler: func(_ context.Context, _ *users.User, _ []string) {
				a.Chat.Say("Follow on Instagram: https://instagram.com/adanalife_")
			},
		},
		{
			Trigger: "!facebook",
			Help:    "The Facebook link",
			Aliases: []string{"!fb"},
			Handler: func(_ context.Context, _ *users.User, _ []string) {
				a.Chat.Say("Follow on Facebook: https://www.facebook.com/adanalifeunderscore")
			},
		},
		{
			Trigger: "!youtube",
			Help:    "The YouTube channel link",
			Aliases: []string{"!yt"},
			Handler: func(_ context.Context, _ *users.User, _ []string) {
				a.Chat.Say("Subscribe on YouTube: https://www.youtube.com/channel/UC8Q7uFC1Xyr2ZnTWOk9Aizg")
			},
		},
		{
			Trigger: "!tiktok",
			Help:    "The TikTok link",
			Handler: func(_ context.Context, _ *users.User, _ []string) {
				a.Chat.Say("Follow on TikTok: https://tiktok.com/@adanalife")
			},
		},
		{
			Trigger: "!bluesky",
			Help:    "The Bluesky link",
			Aliases: []string{"!bsky"},
			Handler: func(_ context.Context, _ *users.User, _ []string) {
				a.Chat.Say("Follow on Bluesky: https://bsky.app/profile/dana.lol")
			},
		},
		{
			Trigger: "!commands",
			Help:    "List the commands you can try; !help <command> explains one",
			// "!hello" lists commands rather than greeting: a viewer who types
			// the bang is addressing the bot, and what they want next is the
			// command surface. The bare "hello" trigger above still greets, so
			// an ordinary greeting in chat is unaffected.
			//
			// "!help" is here rather than on its own handler because a viewer
			// asking for help wants the command surface, not one rotating tip
			// out of a set they cannot page through. The rotation still runs on
			// the Chatter timer, where being one line at a time is the point.
			Aliases: []string{"!command", "!controls", "!hello", "!help"},
			Handler: a.commandsCmd,
		},
		{
			Trigger:            "!bonusmiles",
			Help:               "Your bonus miles earned this session, a subscriber perk",
			Handler:            a.bonusMilesCmd,
			RequiresSubscriber: true,
		},
		{
			Trigger:        "!sunset",
			Help:           "Sunset time at the current location",
			Handler:        a.sunsetCmd,
			RequiresFollow: true,
		},
		{
			Trigger:        "!weather",
			Help:           "The weather at the current location",
			Aliases:        []string{"!meteo"},
			Handler:        a.weatherCmd,
			RequiresFollow: true,
		},
		{
			Trigger:        "!time",
			Help:           "Local time at the current location",
			Handler:        a.timeCmd,
			RequiresFollow: true,
		},
		{
			Trigger:        "!date",
			Help:           "The date this footage was recorded",
			Aliases:        []string{"is this live", "is this live?"},
			Handler:        a.dateCmd,
			RequiresFollow: true,
		},
		{
			Trigger: "!guess",
			Help:    "Guess which state the van is in: !guess Utah",
			// "!guis" stays: it's 2 edits from !guess, beyond fuzzyLookup's
			// reach at that length (max 1 edit for inputs of 4-6 runes).
			// "!guesss"/"!guesr" are equidistant from !guess and !guessr, so
			// fuzzyLookup calls them ambiguous and answers nothing; the state
			// guess is the far more likely intent at that spelling.
			Aliases:        []string{"guess", "!guis", "!guesss", "!guesr"},
			Handler:        a.guessCmd,
			RequiresFollow: true,
		},
		{
			Trigger:        "!state",
			Help:           "The state the van is in right now",
			Handler:        a.stateCmd,
			RequiresFollow: true,
		},
		{
			Trigger:       "!secretinfo",
			Help:          "Dump internal state to chat",
			Handler:       a.secretInfoCmd,
			RequiresAdmin: true,
		},
		{
			Trigger: "!gas",
			Help:    "How's the tank?",
			Aliases: []string{"!fuel", "!petrol"},
			Handler: func(_ context.Context, _ *users.User, _ []string) {
				a.Chat.Say("About full, thanks for asking")
			},
		},
		{
			Trigger:       "!middle",
			Help:          "Set the bottom-middle overlay text",
			Handler:       a.middleCmd,
			RequiresAdmin: true,
		},
		{
			Trigger:       "!makebot",
			Help:          "Flag a user as a bot so they leave the leaderboards",
			Handler:       a.makeBotCmd,
			RequiresAdmin: true,
		},
		{
			Trigger:       "!unbot",
			Help:          "Unflag a user as a bot",
			Handler:       a.unBotCmd,
			RequiresAdmin: true,
		},
		{
			Trigger:        "!miles",
			Help:           "Your accumulated miles",
			Aliases:        []string{"!points"},
			Handler:        a.milesCmd,
			RequiresFollow: true,
		},
		{
			Trigger:       "!givemiles",
			Help:          "Grant a viewer miles: !givemiles @user <miles>",
			Handler:       a.giveMilesCmd,
			RequiresAdmin: true,
		},
		{
			Trigger:        "!km",
			Help:           "Your miles, in kilometres",
			Aliases:        []string{"!kilometres", "!kilometers"},
			Handler:        a.kilometresCmd,
			RequiresFollow: true,
		},
		{
			Trigger: "!location",
			Help:    "Where the van is: the town and state, with a map link",
			// "!loclistion" stays: 3 edits from !location, beyond
			// fuzzyLookup's max of 2
			Aliases:        []string{"!tripbot", "!city", "!town", "!where", "!loc", "!loclistion"},
			Handler:        a.locationCmd,
			RequiresFollow: true,
		},
		{
			Trigger:        "!leaderboard",
			Help:           "This month's top 10 by miles",
			Aliases:        []string{"!monthlyleaderboard", "!lb", "!mlb", "!ldb", "!ldbd", "!top"},
			Handler:        a.monthlyMilesLeaderboardCmd,
			RequiresFollow: true,
		},
		{
			Trigger:        "!totalleaderboard",
			Help:           "The all-time top 10 by miles",
			Aliases:        []string{"!lifetimeleaderboard", "!tlb", "!llb"},
			Handler:        a.lifetimeMilesLeaderboardCmd,
			RequiresFollow: true,
		},
		{
			Trigger:        "!guessleaderboard",
			Help:           "This month's top 10 by correct state guesses",
			Aliases:        []string{"!glb", "!guesslb"},
			Handler:        a.monthlyGuessLeaderboardCmd,
			RequiresFollow: true,
		},
		{
			Trigger:        "!guessr",
			Help:           "The dashcam guessing game leaderboard; add 'monthly' for the running total",
			Aliases:        []string{"!guessrleaderboard", "!grlb"},
			Handler:        a.guessrLeaderboardCmd,
			RequiresFollow: true,
		},
		{
			Trigger:        "!report",
			Help:           "Report a stream problem, like no audio or a frozen picture",
			Aliases:        []string{"no audio", "no sound", "no music", "frozen"},
			Handler:        a.reportCmd,
			RequiresFollow: false,
		},
		{
			Trigger: "!song",
			Help:    "What's playing on the background audio right now",
			Aliases: []string{"!music"},
			Handler: a.songCmd,
		},
		{
			Trigger: "!somafm",
			Help:    "The SomaFM link, whose channels are the background audio",
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
	platformTwitch    = "twitch"
	platformYouTube   = "youtube"
	platformFacebook  = "facebook"
	platformInstagram = "instagram"
	platformTikTok    = "tiktok"
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

// v1Commands is the allowlist of triggers a v1-rollout platform instance
// (YouTube, Facebook, Instagram, TikTok) runs — the "info + playback control" subset, plus the
// !state/!location info commands.
// Identity/miles commands (!miles, !leaderboard, !guess, …), the Twitch-only
// !followage, and the admin commands (!middle, !secretinfo, !shutdown, !makebot,
// !unbot) are excluded: those are per-user identity/score state. !somafm is
// excluded too — it credits a bed that only Twitch defaults to. Aliases come
// along with their trigger, so only triggers are listed. See the YouTube
// provider plan.
var v1Commands = map[string]bool{
	"!version": true, "!uptime": true, "!commands": true,
	"!gas": true, "!report": true,
	// background audio (every platform's scene ships the one bed source)
	"!song": true, "!audio": true,
	// info (read current-video state only)
	"!weather": true, "!time": true, "!date": true, "!sunset": true,
	"!state": true, "!location": true,
	// playback control (drives this platform's playout pipeline)
	"!timewarp": true, "!goto": true, "!skip": true, "!back": true, "!daytime": true,
	"!find": true,
	// socials / static links
	"!socialmedia": true, "!discord": true, "!twitter": true, "!instagram": true,
	"!facebook": true, "!youtube": true, "!tiktok": true, "!bluesky": true,
}

// commandScope is how much of the cross-platform command surface a platform
// runs — the per-platform capability the command gate keys off, a declared
// property of the platform rather than a hardcoded name check.
type commandScope int

const (
	// scopeV1 runs only the vetted v1Commands allowlist. It is the zero value,
	// so any platform absent from platformCommandScope defaults to it: a newly
	// added platform (Kick, …) proves itself through the allowlist rather than
	// silently inheriting the full command surface.
	scopeV1 commandScope = iota
	// scopeFull runs every cross-platform command — a mature, fully-rolled-out
	// platform.
	scopeFull
)

// platformCommandScope declares each known platform's command-surface scope.
// A platform absent from the map gets the zero value (scopeV1), so the gate is
// driven by this capability declaration and an unrecognized STREAM_PLATFORM can
// never fall through to the full surface. Graduating a platform is a one-line
// change here (scopeV1 → scopeFull), symmetric across platforms — none is
// special-cased by name.
var platformCommandScope = map[string]commandScope{
	platformTwitch:    scopeFull,
	platformYouTube:   scopeV1,
	platformFacebook:  scopeV1,
	platformInstagram: scopeV1,
	platformTikTok:    scopeV1,
}

// platformPersistsUsers declares which platforms give a chatter a persisted
// identity — a users row, a session, miles, a login/logout lifecycle. Twitch
// does; the gateway platforms punt identity for v1, so their chatters reach the
// command path as a transient user carrying only a display name.
//
// It is what decides whether an inbound message logs its sender in, so the two
// halves stay consistent by construction: a platform that persists users runs
// the identity commands (scopeFull) and can answer a subscriber check, and one
// that doesn't is bounded by the v1 allowlist, which reads nothing user-specific
// beyond the name. Graduating a platform means flipping it here and in
// platformCommandScope together — flipping only the scope would hand
// identity-reading commands a user with no rows behind it, which answers 0
// miles rather than failing.
var platformPersistsUsers = map[string]bool{
	platformTwitch: true,
}

// platformHasSubscribers declares which platforms expose a subscriber signal
// tripbot can actually check. Twitch does; the gateway platforms give viewers
// no persisted identity at all (v1 hands the command path a transient user), so
// there is nothing there for RequiresSubscriber to read.
//
// A RequiresSubscriber command is therefore ungated on a platform with no
// subscriber signal — see (*Command).checkAccess. The alternative is a gate
// nobody can ever pass, which to a viewer is indistinguishable from a broken
// command. The zero value (absent → no subscriber signal) is the right default
// for a newly added platform: it starts out with no identity plumbing, and the
// v1 allowlist is what bounds its command surface.
var platformHasSubscribers = map[string]bool{
	platformTwitch: true,
}

// commandEnabled reports whether cmd should be indexed for dispatch on this
// App's platform. Two orthogonal concerns, in order:
//
//  1. Platform-specific commands declare their scope via Command.Platforms. A
//     command with a non-nil Platforms is governed solely by it — indexed on
//     exactly the listed platforms, on every platform. This is symmetric: no
//     platform is special, and a new Kick/TikTok-only command just lists itself.
//  2. Cross-platform commands (Platforms == nil) are gated by the platform's
//     declared commandScope: a scopeFull platform (Twitch today) runs them all;
//     every other platform runs only the vetted v1Commands allowlist. The
//     conservative default (scopeV1 for any undeclared platform) means a new
//     platform is restricted to the allowlist, never handed the full surface.
func (a *App) commandEnabled(cmd *Command) bool {
	if len(cmd.Platforms) > 0 {
		return slices.Contains(cmd.Platforms, a.platform())
	}
	if platformCommandScope[a.platform()] == scopeFull {
		return true
	}
	return v1Commands[cmd.Trigger]
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
	// A bot-less instance (YouTube with inbound chat disabled) can't answer any
	// command, so advertise promo copy pointing at Twitch instead of command
	// hints that would silently no-op. These lines carry no "!command" token, so
	// they skip the per-platform command filtering below.
	if a.botless {
		return slices.Clone(c.YouTubeBotlessHelpMessages)
	}
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
