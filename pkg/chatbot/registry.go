package chatbot

import (
	"strings"

	"github.com/adanalife/tripbot/pkg/users"
)

var commands = []Command{
	{
		Trigger:  "!help",
		Handler:  helpCmd,
		Category: "info",
	},
	{
		Trigger:  "hello",
		Aliases:  []string{"hi", "hey", "hallo", "!bot"},
		Handler:  helloCmd,
		Category: "info",
	},
	{
		Trigger:  "!flag",
		Handler:  flagCmd,
		Category: "info",
	},
	{
		Trigger:  "!version",
		Handler:  versionCmd,
		Category: "info",
	},
	{
		Trigger:  "!uptime",
		Handler:  uptimeCmd,
		Category: "info",
	},
	{
		Trigger:        "!timewarp",
		Aliases:        []string{"!timewrap", "!timeskip", "!tw", "!timewqrp", "!warp"},
		Handler:        timewarpCmd,
		RequiresFollow: true,
		Category:       "follower-gated",
	},
	{
		Trigger:        "!goto",
		Aliases:        []string{"!jump"},
		Handler:        jumpCmd,
		RequiresFollow: true,
		Category:       "follower-gated",
	},
	{
		Trigger:        "!skip",
		Handler:        skipCmd,
		RequiresFollow: true,
		Category:       "follower-gated",
	},
	{
		Trigger:        "!back",
		Handler:        backCmd,
		RequiresFollow: true,
		Category:       "follower-gated",
	},
	{
		Trigger:  "!shutdown",
		Handler:  shutdownCmd,
		Category: "admin",
	},
	{
		Trigger: "!socialmedia",
		Aliases: []string{"!social", "!socials"},
		Handler: func(_ *users.User, _ []string) {
			Say("Find me outside of Twitch: !twitter, !instagram, !facebook, !youtube")
		},
		Category: "info",
	},
	{
		Trigger: "!commands",
		Aliases: []string{"!command", "¡command", "¡commands", "!commads", "!controls", "!commande"},
		Handler: func(_ *users.User, _ []string) {
			Say("You can try: !location, !guess, !date, !state, !sunset, !timewarp, !miles, !leaderboard, and many other hidden commands!")
		},
		Category: "info",
	},
	{
		Trigger:            "!bonusmiles",
		Handler:            bonusMilesCmd,
		RequiresSubscriber: true,
		Category:           "subscriber-gated",
	},
	{
		Trigger:        "!sunset",
		Aliases:        []string{"!sunet"},
		Handler:        sunsetCmd,
		RequiresFollow: true,
		Category:       "follower-gated",
	},
	{
		Trigger:        "!time",
		Aliases:        []string{"!timr"},
		Handler:        timeCmd,
		RequiresFollow: true,
		Category:       "follower-gated",
	},
	{
		Trigger:        "!date",
		Aliases:        []string{"!datw"},
		Handler:        dateCmd,
		RequiresFollow: true,
		Category:       "follower-gated",
	},
	{
		Trigger:        "!guess",
		Aliases:        []string{"!guss", "guess", "!gusss", "!guees", "!gues", "!quess", "!guis"},
		Handler:        guessCmd,
		RequiresFollow: true,
		Category:       "follower-gated",
	},
	{
		Trigger:        "!state",
		Handler:        stateCmd,
		RequiresFollow: true,
		Category:       "follower-gated",
	},
	{
		Trigger:  "!secretinfo",
		Handler:  secretInfoCmd,
		Category: "admin",
	},
	{
		Trigger: "!gas",
		Aliases: []string{"!fuel", "!petrol"},
		Handler: func(_ *users.User, _ []string) {
			Say("About full, thanks for asking")
		},
		Category: "info",
	},
	{
		Trigger:  "!middle",
		Handler:  middleCmd,
		Category: "admin",
	},
	{
		Trigger:        "!miles",
		Aliases:        []string{"!points"},
		Handler:        milesCmd,
		RequiresFollow: true,
		Category:       "follower-gated",
	},
	{
		Trigger:        "!km",
		Aliases:        []string{"!kilometres", "!kilometers"},
		Handler:        kilometresCmd,
		RequiresFollow: true,
		Category:       "follower-gated",
	},
	{
		Trigger:        "!location",
		Aliases:        []string{"!tripbot", "!city", "!town", "!where", "!loacation", "!loation", "!loc", "!locatioin", "!locatoion", "!locaton", "!loclistion", "!locton", "1location", "¡location", "!locatiom", "!location!", "!locatio", "!lcoation"},
		Handler:        locationCmd,
		RequiresFollow: true,
		Category:       "follower-gated",
	},
	{
		Trigger:        "!leaderboard",
		Aliases:        []string{"!monthlyleaderboard", "!lb", "!mlb", "!leaderbord", "!ldb", "!ldbd"},
		Handler:        monthlyMilesLeaderboardCmd,
		RequiresFollow: true,
		Category:       "follower-gated",
	},
	{
		Trigger:        "!totalleaderboard",
		Aliases:        []string{"!lifetimeleaderboard", "!tlb", "!llb"},
		Handler:        lifetimeMilesLeaderboardCmd,
		RequiresFollow: true,
		Category:       "follower-gated",
	},
	{
		Trigger:        "!guessleaderboard",
		Aliases:        []string{"!glb"},
		Handler:        monthlyGuessLeaderboardCmd,
		RequiresFollow: true,
		Category:       "follower-gated",
	},
	{
		Trigger:        "!report",
		Aliases:        []string{"no audio", "no sound", "no music", "frozen"},
		Handler:        reportCmd,
		RequiresFollow: true,
		Category:       "follower-gated",
	},
}

// singleWordLookup maps single-word triggers and aliases to their Command.
// multiWordLookup maps space-containing triggers and aliases to their Command.
var singleWordLookup map[string]*Command
var multiWordLookup map[string]*Command

func init() {
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
