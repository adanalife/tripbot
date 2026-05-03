package main

type Command struct {
	Trigger         string
	Aliases         []string
	Category        string
	ExpectsBotReply bool
	OnscreenEffect  string
	RequiresFollow  bool
	SampleParams    string
}

var Catalog = []Command{
	{
		Trigger:         "!help",
		Category:        "info",
		ExpectsBotReply: true,
	},
	{
		Trigger:         "hello",
		Aliases:         []string{"hi", "hey", "hallo", "!bot"},
		Category:        "info",
		ExpectsBotReply: true,
	},
	{
		Trigger:         "!flag",
		Category:        "info",
		ExpectsBotReply: true,
		OnscreenEffect:  "flag",
	},
	{
		Trigger:         "!version",
		Category:        "info",
		ExpectsBotReply: true,
	},
	{
		Trigger:         "!uptime",
		Category:        "info",
		ExpectsBotReply: true,
	},
	{
		Trigger:         "!timewarp",
		Aliases:         []string{"!timewrap", "!timeskip", "!tw", "!timewqrp", "!warp"},
		Category:        "follower-gated",
		ExpectsBotReply: true,
		OnscreenEffect:  "timewarp",
		RequiresFollow:  true,
	},
	{
		Trigger:         "!goto",
		Aliases:         []string{"!jump"},
		Category:        "follower-gated",
		ExpectsBotReply: true,
		OnscreenEffect:  "flag",
		RequiresFollow:  true,
		SampleParams:    "MO",
	},
	{
		Trigger:         "!skip",
		Category:        "follower-gated",
		ExpectsBotReply: true,
		RequiresFollow:  true,
		SampleParams:    "5",
	},
	{
		Trigger:         "!back",
		Category:        "follower-gated",
		ExpectsBotReply: true,
		RequiresFollow:  true,
		SampleParams:    "5",
	},
	{
		Trigger:         "!shutdown",
		Category:        "admin",
		ExpectsBotReply: false,
	},
	{
		Trigger:         "!socialmedia",
		Aliases:         []string{"!social", "!socials"},
		Category:        "info",
		ExpectsBotReply: true,
	},
	{
		Trigger:         "!commands",
		Aliases:         []string{"!command", "¡command", "¡commands", "!commads", "!controls", "!commande"},
		Category:        "info",
		ExpectsBotReply: true,
	},
	{
		Trigger:         "!bonusmiles",
		Category:        "subscriber-gated",
		ExpectsBotReply: true,
	},
	{
		Trigger:         "!sunset",
		Aliases:         []string{"!sunet"},
		Category:        "follower-gated",
		ExpectsBotReply: true,
		RequiresFollow:  true,
	},
	{
		Trigger:         "!time",
		Aliases:         []string{"!timr"},
		Category:        "follower-gated",
		ExpectsBotReply: true,
		RequiresFollow:  true,
	},
	{
		Trigger:         "!date",
		Aliases:         []string{"!datw"},
		Category:        "follower-gated",
		ExpectsBotReply: true,
		RequiresFollow:  true,
	},
	{
		Trigger:         "!guess",
		Aliases:         []string{"!guss", "guess", "!gusss", "!guees", "!gues", "!quess", "!guis"},
		Category:        "follower-gated",
		ExpectsBotReply: true,
		OnscreenEffect:  "flag",
		RequiresFollow:  true,
		SampleParams:    "MO",
	},
	{
		Trigger:         "!state",
		Category:        "follower-gated",
		ExpectsBotReply: true,
		OnscreenEffect:  "flag",
		RequiresFollow:  true,
	},
	{
		Trigger:         "!secretinfo",
		Category:        "admin",
		ExpectsBotReply: true,
	},
	{
		Trigger:         "!gas",
		Aliases:         []string{"!fuel", "!petrol"},
		Category:        "info",
		ExpectsBotReply: true,
	},
	{
		Trigger:         "!middle",
		Category:        "info",
		ExpectsBotReply: false,
		OnscreenEffect:  "middle-text",
		SampleParams:    "hello from streamtest",
	},
	{
		Trigger:         "!miles",
		Aliases:         []string{"!points"},
		Category:        "follower-gated",
		ExpectsBotReply: true,
		RequiresFollow:  true,
	},
	{
		Trigger:         "!km",
		Aliases:         []string{"!kilometres", "!kilometers"},
		Category:        "follower-gated",
		ExpectsBotReply: true,
		RequiresFollow:  true,
	},
	{
		Trigger:         "!location",
		Aliases:         []string{"!tripbot", "!city", "!town", "!where", "!loacation", "!loation", "!loc", "!locatioin", "!locatoion", "!locaton", "!loclistion", "!locton", "1location", "¡location", "!locatiom", "!location!", "!locatio", "!lcoation"},
		Category:        "follower-gated",
		ExpectsBotReply: true,
		RequiresFollow:  true,
	},
	{
		Trigger:         "!leaderboard",
		Aliases:         []string{"!monthlyleaderboard", "!lb", "!mlb", "!leaderbord", "!ldb", "!ldbd"},
		Category:        "follower-gated",
		ExpectsBotReply: true,
		OnscreenEffect:  "leaderboard",
		RequiresFollow:  true,
	},
	{
		Trigger:         "!totalleaderboard",
		Aliases:         []string{"!lifetimeleaderboard", "!tlb", "!llb"},
		Category:        "follower-gated",
		ExpectsBotReply: true,
		OnscreenEffect:  "leaderboard",
		RequiresFollow:  true,
	},
	{
		Trigger:         "!guessleaderboard",
		Aliases:         []string{"!glb"},
		Category:        "follower-gated",
		ExpectsBotReply: true,
		OnscreenEffect:  "leaderboard",
		RequiresFollow:  true,
	},
	{
		Trigger:         "!report",
		Aliases:         []string{"no audio", "no sound", "no music", "frozen"},
		Category:        "follower-gated",
		ExpectsBotReply: true,
		RequiresFollow:  true,
	},
}
