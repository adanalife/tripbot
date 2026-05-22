package chatbot

import (
	"context"
	"strings"

	"github.com/adanalife/tripbot/pkg/users"
)

var commands []Command
var singleWordLookup map[string]*Command
var multiWordLookup map[string]*Command

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
				sayFn("Find me outside of Twitch: !twitter, !instagram, !facebook, !youtube")
			},
		},
		{
			Trigger: "!discord",
			Handler: func(_ context.Context, _ *users.User, _ []string) {
				sayFn("Join us on Discord: https://discord.gg/hKvNgZrk52")
			},
		},
		{
			Trigger: "!commands",
			Aliases: []string{"!command", "¡command", "¡commands", "!commads", "!controls", "!commande"},
			Handler: func(_ context.Context, _ *users.User, _ []string) {
				sayFn("You can try: !location, !guess, !date, !state, !sunset, !timewarp, !miles, !leaderboard, and many other hidden commands!")
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
				sayFn("About full, thanks for asking")
			},
		},
		{
			Trigger: "!middle",
			Handler: a.middleCmd,
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
			Aliases:        []string{"!monthlyleaderboard", "!lb", "!mlb", "!leaderbord", "!ldb", "!ldbd"},
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
	}
}

func init() {
	commands = defaultApp.buildRegistry()
	singleWordLookup = make(map[string]*Command)
	multiWordLookup = make(map[string]*Command)
	for i := range commands {
		cmd := &commands[i]
		registerTrigger(cmd.Trigger, cmd)
		for _, alias := range cmd.Aliases {
			registerTrigger(alias, cmd)
		}
	}
}

func registerTrigger(trigger string, cmd *Command) {
	if strings.Contains(trigger, " ") {
		multiWordLookup[trigger] = cmd
	} else {
		singleWordLookup[trigger] = cmd
	}
}
