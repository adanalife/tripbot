package config

const (
	BotUsername = "TripBot4000"
	ChannelName = "adanalife_"

	//TODO this should use the ProjectRoot helper
	GetCurrentVidScript = "/Users/dmerrick/other_projects/danalol-stream/bin/current-file.sh"

	//TODO move these off usbshare1
	ScreencapDir  = "/Volumes/usbshare1/first frame of every video"
	MapsOutputDir = "/Volumes/usbshare1/maps"
	CroppedPath   = "/Volumes/usbshare1/cropped-corners"

	DbPath            = "tripbot.db"
	UserJoinsBucket   = "user_joins"
	UserWatchedBucket = "user_watched"
)

// https://twitchinsights.net/bots
var IgnoredUsers = []string{
	"adanalife_",
	"tripbot4000",
	"nightbot",
	"anotherttvviewer",
	"apricotdrupefruit",
	"commanderroot",
	"communityshowcase",
	"electricallongboard",
	"logviewer",
	"lurxx",
	"p0lizei_",
	"slocool",
	"unixchat",
	"v_and_k",
	"virgoproz",
	"zanekyber",
	"feuerwehr",
	"jobi_essen",
	"freddyybot",
	"taormina2600",
	"avocadobadado",
	"eubyt",
}

var HelpMessages = []string{
	"!tripbot: Get the current location (beta)",
	"!map: Show a map of the whole trip",
	"!info: Get more details on the footage",
	"!song: Get the current music",
	"!miles: See your current miles",
}
