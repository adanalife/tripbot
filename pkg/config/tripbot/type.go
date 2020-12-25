package config

type TripbotConfig struct {
	//TODO: is this right?
	Environment string `required:"true" envconfig:"ENV"`
	ServerType  string `default:"tripbot"`
	// ChannelName is the username of the stream
	ChannelName string `required:"true" envconfig:"CHANNEL_NAME"`
	// OutputChannel is the stream to which the bot will speak
	OutputChannel string
	// BotUsername is the username of the bot
	BotUsername string `required:"true" envconfig:"BOT_USERNAME"`
	// ExternalURL is the where the bot's HTTP server can be reached
	ExternalURL string `required:"true" envconfig:"EXTERNAL_URL"`
	// GoogleProjectID is the Google Cloud project ID
	GoogleProjectID string `required:"true" envconfig:"GOOGLE_APPS_PROJECT_ID"`
	// GoogleMapsAPIKey is the API key with which we access Google Maps
	GoogleMapsAPIKey string `required:"true" envconfig:"GOOGLE_MAPS_API_KEY"`
	// ReadOnly is used to prevent writing some things to the DB
	ReadOnly bool `default:"false" envconfig:"READ_ONLY"`
	// Verbose determines output verbosity
	Verbose bool `default:"false" envconfig:"VERBOSE"`

	// DashcamDir contains the dashcam footage
	DashcamDir string `default:"/opt/data/Dashcam" envconfig:"DASHCAM_DIR"`
	// VideoDir is where the videos live
	VideoDir string `default:"/opt/data/Dashcam/_all" envconfig:"VIDEO_DIR"`
	// // MapsOutputDir is where generated maps will be stored
	// MapsOutputDir string `default:"/opt/data/maps" envconfig:"MAPS_OUTPUT_DIR"`
	// // CroppedCornersDir is where we store the cropped versions of screencaps (to OCR them)
	// CroppedCornersDir string `default:"/opt/data/cropped-corner" envconfig:"CROPPED_CORNERS_DIR"`
	// ScreencapDir is where we store full screenshots from the videos
	ScreencapDir string `default:"/opt/data/screencaps" envconfig:"SCREENCAP_DIR"`
	// RunDir is where temporary-but-important runtime files live (such as pidfiles and onscreen content)
	RunDir string `default:"/opt/data/run" envconfig:"RUN_DIR"`

	// VLCPidFile     string `default:"/opt/data/run/vlc-server.pid" envconfig:"VLC_PIDFILE"`
	// OBSPidFile     string `default:"/opt/data/run/OBS.pid" envconfig:"OBS_PIDFILE"`
	TripbotPidFile string `default:"/opt/data/run/tripbot.pid" envconfig:"TRIPBOT_PIDFILE"`

	// DisableTwitchWebhooks disables receiving webhooks from Twitch (new followers for instance)
	DisableTwitchWebhooks bool `default:"false" envconfig:"DISABLE_TWITCH_WEBHOOKS"`
	// DisableMusic disables MPD completely
	//TODO: rename to disable_audio
	DisableMusic bool `default:"false" envconfig:"DISABLE_MUSIC"`
	// DisableMusicAutoplay disables the auto-play for MPD
	DisableMusicAutoplay bool `default:"false" envconfig:"DISABLE_MUSIC_AUTOPLAY"`

	// TripbotHttpAuth is used to authenticate users to the HTTP server
	TripbotHttpAuth string `required:"true" envconfig:"TRIPBOT_HTTP_AUTH"`
	// TripbotServerPort is used to specify the port on which the webserver runs
	TripbotServerPort string `default:"8080" envconfig:"TRIPBOT_SERVER_PORT"`
	// VlcServerHost is used to specify the host for the VLC webserver
	VlcServerHost string `required:"true" envconfig:"VLC_SERVER_HOST"`
	MpdServerHost string `required:"true" envconfig:"MPD_SERVER_HOST"`
}
