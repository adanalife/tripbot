package chatbot

import (
	"strings"

	"github.com/adanalife/tripbot/pkg/users"
)

var commands = []Command{
	{
		Trigger: "!help",
		Handler: helpCmd,
	},
	{
		Trigger: "hello",
		Aliases: []string{"hi", "hey", "hallo", "!bot"},
		Handler: helloCmd,
	},
	{
		Trigger: "!flag",
		Handler: flagCmd,
	},
	{
		Trigger: "!version",
		Handler: versionCmd,
	},
	{
		Trigger: "!uptime",
		Handler: uptimeCmd,
	},
	{
		Trigger:        "!timewarp",
		Aliases:        []string{"!timewrap", "!timeskip", "!tw", "!timewqrp", "!warp"},
		Handler:        timewarpCmd,
		RequiresFollow: true,
	},
	{
		Trigger:        "!goto",
		Aliases:        []string{"!jump"},
		Handler:        jumpCmd,
		RequiresFollow: true,
	},
	{
		Trigger:        "!skip",
		Handler:        skipCmd,
		RequiresFollow: true,
	},
	{
		Trigger:        "!back",
		Handler:        backCmd,
		RequiresFollow: true,
	},
	{
		Trigger: "!shutdown",
		Handler: shutdownCmd,
	},
	{
		Trigger: "!socialmedia",
		Aliases: []string{"!social", "!socials"},
		Handler: func(_ *users.User, _ []string) {
			Say("Find me outside of Twitch: !twitter, !instagram, !facebook, !youtube")
		},
	},
	{
		Trigger: "!commands",
		Aliases: []string{"!command", "¡command", "¡commands", "!commads", "!controls", "!commande"},
		Handler: func(_ *users.User, _ []string) {
			Say("You can try: !location, !guess, !date, !state, !sunset, !timewarp, !miles, !leaderboard, and many other hidden commands!")
		},
	},
	{
		Trigger:            "!bonusmiles",
		Handler:            bonusMilesCmd,
		RequiresSubscriber: true,
	},
	{
		Trigger:        "!sunset",
		Aliases:        []string{"!sunet"},
		Handler:        sunsetCmd,
		RequiresFollow: true,
	},
	{
		Trigger:        "!time",
		Aliases:        []string{"!timr"},
		Handler:        timeCmd,
		RequiresFollow: true,
	},
	{
		Trigger:        "!date",
		Aliases:        []string{"!datw"},
		Handler:        dateCmd,
		RequiresFollow: true,
	},
	{
		Trigger:        "!guess",
		Aliases:        []string{"!guss", "guess", "!gusss", "!guees", "!gues", "!quess", "!guis"},
		Handler:        guessCmd,
		RequiresFollow: true,
	},
	{
		Trigger:        "!state",
		Handler:        stateCmd,
		RequiresFollow: true,
	},
	{
		Trigger: "!secretinfo",
		Handler: secretInfoCmd,
	},
	{
		Trigger: "!gas",
		Aliases: []string{"!fuel", "!petrol"},
		Handler: func(_ *users.User, _ []string) {
			Say("About full, thanks for asking")
		},
	},
	{
		Trigger: "!middle",
		Handler: middleCmd,
	},
	{
		Trigger:        "!miles",
		Aliases:        []string{"!points"},
		Handler:        milesCmd,
		RequiresFollow: true,
	},
	{
		Trigger:        "!km",
		Aliases:        []string{"!kilometres", "!kilometers"},
		Handler:        kilometresCmd,
		RequiresFollow: true,
	},
	{
		Trigger:        "!location",
		Aliases:        []string{"!tripbot", "!city", "!town", "!where", "!loacation", "!loation", "!loc", "!locatioin", "!locatoion", "!locaton", "!loclistion", "!locton", "1location", "¡location", "!locatiom", "!location!", "!locatio", "!lcoation"},
		Handler:        locationCmd,
		RequiresFollow: true,
	},
	{
		Trigger:        "!leaderboard",
		Aliases:        []string{"!monthlyleaderboard", "!lb", "!mlb", "!leaderbord", "!ldb", "!ldbd"},
		Handler:        monthlyMilesLeaderboardCmd,
		RequiresFollow: true,
	},
	{
		Trigger:        "!totalleaderboard",
		Aliases:        []string{"!lifetimeleaderboard", "!tlb", "!llb"},
		Handler:        lifetimeMilesLeaderboardCmd,
		RequiresFollow: true,
	},
	{
		Trigger:        "!guessleaderboard",
		Aliases:        []string{"!glb"},
		Handler:        monthlyGuessLeaderboardCmd,
		RequiresFollow: true,
	},
	{
		Trigger:        "!report",
		Aliases:        []string{"no audio", "no sound", "no music", "frozen"},
		Handler:        reportCmd,
		RequiresFollow: true,
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
