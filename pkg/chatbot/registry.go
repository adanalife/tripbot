package chatbot

import (
	"context"
	"strings"

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
			Aliases:        []string{"!timewrap", "!timeskip", "!tw", "!timewqrp", "!warp"},
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
				a.IRC.Say("Find me outside of Twitch: !youtube, !tiktok, !instagram, !bluesky")
			},
		},
		{
			Trigger: "!discord",
			Handler: func(_ context.Context, _ *users.User, _ []string) {
				a.IRC.Say("Join us on Discord: https://discord.gg/hKvNgZrk52")
			},
		},
		{
			Trigger: "!twitter",
			Handler: func(_ context.Context, _ *users.User, _ []string) {
				a.IRC.Say("Follow on Twitter: https://twitter.com/adanalife_")
			},
		},
		{
			Trigger: "!instagram",
			Aliases: []string{"!ig", "!insta"},
			Handler: func(_ context.Context, _ *users.User, _ []string) {
				a.IRC.Say("Follow on Instagram: https://instagram.com/adanalife_")
			},
		},
		{
			Trigger: "!facebook",
			Aliases: []string{"!fb"},
			Handler: func(_ context.Context, _ *users.User, _ []string) {
				a.IRC.Say("Follow on Facebook: https://www.facebook.com/adanalifeblog")
			},
		},
		{
			Trigger: "!youtube",
			Aliases: []string{"!yt"},
			Handler: func(_ context.Context, _ *users.User, _ []string) {
				a.IRC.Say("Subscribe on YouTube: https://www.youtube.com/channel/UC8Q7uFC1Xyr2ZnTWOk9Aizg")
			},
		},
		{
			Trigger: "!tiktok",
			Handler: func(_ context.Context, _ *users.User, _ []string) {
				a.IRC.Say("Follow on TikTok: https://tiktok.com/@adanalife")
			},
		},
		{
			Trigger: "!bluesky",
			Aliases: []string{"!bsky"},
			Handler: func(_ context.Context, _ *users.User, _ []string) {
				a.IRC.Say("Follow on Bluesky: https://bsky.app/profile/dana.lol")
			},
		},
		{
			Trigger: "!commands",
			Aliases: []string{"!command", "¡command", "¡commands", "!commads", "!controls", "!commande"},
			Handler: func(_ context.Context, _ *users.User, _ []string) {
				a.IRC.Say("You can try: !location, !guess, !date, !state, !sunset, !timewarp, !miles, !leaderboard, !song, and many other hidden commands!")
			},
		},
		{
			Trigger:            "!bonusmiles",
			Handler:            a.bonusMilesCmd,
			RequiresSubscriber: true,
		},
		{
			Trigger:        "!sunset",
			Aliases:        []string{"!sunet"},
			Handler:        a.sunsetCmd,
			RequiresFollow: true,
		},
		{
			Trigger:        "!time",
			Aliases:        []string{"!timr"},
			Handler:        a.timeCmd,
			RequiresFollow: true,
		},
		{
			Trigger:        "!date",
			Aliases:        []string{"!datw", "is this live", "is this live?"},
			Handler:        a.dateCmd,
			RequiresFollow: true,
		},
		{
			Trigger:        "!guess",
			Aliases:        []string{"!guss", "guess", "!gusss", "!guees", "!gues", "!quess", "!guis"},
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
				a.IRC.Say("About full, thanks for asking")
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
			Trigger:        "!location",
			Aliases:        []string{"!tripbot", "!city", "!town", "!where", "!loacation", "!loation", "!loc", "!locatioin", "!locatoion", "!locaton", "!loclistion", "!locton", "1location", "¡location", "!locatiom", "!location!", "!locatio", "!lcoation"},
			Handler:        a.locationCmd,
			RequiresFollow: true,
		},
		{
			Trigger:        "!leaderboard",
			Aliases:        []string{"!monthlyleaderboard", "!lb", "!mlb", "!leaderbord", "!leaderborad", "!ldb", "!ldbd"},
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
				a.IRC.Say("Stream music by SomaFM — https://somafm.com")
			},
		},
	}
}

// indexCommands builds a.commands from a.buildRegistry() and indexes it into
// a.singleWordLookup / a.multiWordLookup by trigger and alias. Call once after
// the App is constructed (its deps don't need to be set — buildRegistry only
// binds handler method values to a).
func (a *App) indexCommands() {
	a.commands = a.buildRegistry()
	a.singleWordLookup = make(map[string]*Command)
	a.multiWordLookup = make(map[string]*Command)
	for i := range a.commands {
		cmd := &a.commands[i]
		a.registerTrigger(cmd.Trigger, cmd)
		for _, alias := range cmd.Aliases {
			a.registerTrigger(alias, cmd)
		}
	}
}

func (a *App) registerTrigger(trigger string, cmd *Command) {
	if strings.Contains(trigger, " ") {
		a.multiWordLookup[trigger] = cmd
	} else {
		a.singleWordLookup[trigger] = cmd
	}
}
