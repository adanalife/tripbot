package config

const (
	// BotUsername is the name of the bot
	BotUsername = "TripBot4000"
	// ChannelName is the channel to join
	ChannelName = "adanalife_"

	// GetCurrentVidScript is a script that figures out the currently-playing video
	//TODO this should use the ProjectRoot helper
	GetCurrentVidScript = "/Users/dmerrick/other_projects/danalol-stream/bin/current-file.sh"

	//TODO move these off usbshare1
	ScreencapDir  = "/Volumes/usbshare1/screencaps"
	VideoDir      = "/Volumes/usbshare1/Dashcam/_all"
	MapsOutputDir = "/Volumes/usbshare1/maps"
	CroppedPath   = "/Volumes/usbshare1/cropped-corners"

	DbPath            = "tripbot.db"
	UserJoinsBucket   = "user_joins"
	UserWatchedBucket = "user_watched"
)

//TODO: this should load from a config file
// IgnoredUsers are users who shouldn't be in the running for miles
// https://twitchinsights.net/bots
var IgnoredUsers = []string{
	"adanalife_",
	"tripbot4000",
	"nightbot",
	"anotherttvviewer",
	"apricotdrupefruit",
	"avocadobadado",
	"commanderroot",
	"communityshowcase",
	"electricallongboard",
	"eubyt",
	"feuerwehr",
	"freddyybot",
	"jobi_essen",
	"logviewer",
	"lurxx",
	"p0lizei_",
	"slocool",
	"taormina2600",
	"unixchat",
	"v_and_k",
	"virgoproz",
	"zanekyber",
}

// HelpMessages are all of the different things !help can return
var HelpMessages = []string{
	"!tripbot: Get the current location (beta)",
	"!map: Show a map of the whole trip",
	"!info: Get more details on the footage",
	"!song: Get the current music",
	"!miles: See your current miles",
}
