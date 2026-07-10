package config

type TripbotConfig struct {
	Environment string `required:"true" envconfig:"ENV"`
	ServerType  string `default:"tripbot"`
	// Platform selects which streaming platform this bot instance serves.
	// "twitch" (the default) runs the full command surface; "youtube" runs the
	// restricted v1 allowlist (see pkg/chatbot/registry.go). One instance per
	// platform, selected by config — same binary, blast-radius isolated.
	// Reads the same STREAM_PLATFORM env key the OBS image uses
	// (contract.EnvKeyStreamPlatform), so the cdk8s factory stamps one platform
	// value across every component of a pipeline.
	Platform string `default:"twitch" envconfig:"STREAM_PLATFORM"`

	// ChannelName is the username of the stream
	ChannelName string `required:"true" envconfig:"CHANNEL_NAME"`
	// BotUsername is the username of the bot
	BotUsername string `required:"true" envconfig:"BOT_USERNAME"`
	// CompedSubscribers are usernames treated as subscribers for
	// subscriber-only commands without an actual sub (comped friends/VIPs).
	// Comma-separated, case-insensitive. Empty by default.
	CompedSubscribers []string `envconfig:"COMPED_SUBSCRIBERS"`
	// ExternalURL is the where the bot's HTTP server can be reached
	ExternalURL string `required:"true" envconfig:"EXTERNAL_URL"`
	// GoogleMapsAPIKey is the API key with which we access Google Maps.
	// Optional — when unset, geocoder + static-map calls are skipped and
	// callers fall back gracefully (no city/state lookups, no generated
	// maps). The bot continues to run.
	GoogleMapsAPIKey string `envconfig:"GOOGLE_MAPS_API_KEY"`

	// ReadOnly is used to prevent writing some things to the DB
	ReadOnly bool `default:"false" envconfig:"READ_ONLY"`
	// Verbose determines output verbosity
	Verbose bool `default:"false" envconfig:"VERBOSE"`

	// VideoDir is where the videos live
	VideoDir string `default:"/opt/data/Dashcam/_all" envconfig:"VIDEO_DIR"`

	// MapsOutputDir is where generated maps will be stored
	MapsOutputDir string `default:"/opt/data/maps" envconfig:"MAPS_OUTPUT_DIR"`

	// TripbotPidFile is where the tripbot PID is written
	TripbotPidFile string `default:"/opt/data/run/tripbot.pid" envconfig:"TRIPBOT_PIDFILE"`

	// TripbotServerPort is used to specify the port on which the webserver runs
	TripbotServerPort string `default:"8080" envconfig:"TRIPBOT_SERVER_PORT"`
	// VlcServerHost is used to specify the host for the VLC webserver
	VlcServerHost string `required:"true" envconfig:"VLC_SERVER_HOST"`
	// OnscreensServerHost is the host:port for the onscreens-server HTTP
	// API (state.json, render/, asset/, plus the show/hide endpoints the
	// chatbot drives).
	OnscreensServerHost string `required:"true" envconfig:"ONSCREENS_SERVER_HOST"`
	// ObsServerHost is the host:port of obs-server — the Flask process
	// baked into the OBS image that exposes /health/ready, /version,
	// and POST /admin/shutdown on the same shape the Go services use.
	// Named for symmetry with vlc-server / onscreens-server. The admin
	// panel probes it for the OBS row + posts to its /admin/shutdown
	// for the "restart obs" button. Optional — blank skips the OBS row.
	ObsServerHost string `envconfig:"OBS_SERVER_HOST"`

	// TwitchAPIURL points the chatbot's command-time Twitch Helix calls at
	// the platform-gateway gateway-twitch instance over HTTP, instead of the
	// in-process pkg/twitch path. Empty (the default) keeps the in-process
	// adapter, so existing envs are unaffected. When set — e.g.
	// http://gateway-twitch.<env>.svc.cluster.local:8080 — App.Twitch becomes an
	// HTTP client behind the same interface, with no command code changes.
	TwitchAPIURL string `envconfig:"TWITCH_API_URL"`

	// YouTubeAPIURL points a PLATFORM=youtube instance at the platform-gateway
	// gateway-youtube instance over HTTP — e.g.
	// http://gateway-youtube.<env>.svc.cluster.local:8080. Both chat directions
	// flow through the gateway: outbound via its SendChat (which resolves the
	// active live chat itself), inbound via its GET /v1/chat/inbound poll. The
	// gateway also owns the YouTube OAuth token, so tripbot holds none at
	// runtime. Required on a youtube instance — the in-process YouTube client is
	// gone, so with this empty the instance comes up without YouTube chat.
	YouTubeAPIURL string `envconfig:"YOUTUBE_API_URL"`

	// YouTubeInboundEnabled gates the gateway-youtube inbound chat poll on a
	// PLATFORM=youtube instance. Default true. Set false for a "bot-less"
	// YouTube presence: outbound posting (rotators) and the background jobs keep
	// running, but nothing reads chat, so no command can respond. When false the
	// chatbot also swaps its rotating Chatter/!help copy from command ads to
	// promotional lines (see pkg/chatbot enabledHelpMessages) — advertising a
	// command nobody can run reads as a broken bot. Flip to true the day the
	// YouTube Data API quota extension lands. No effect on Twitch.
	YouTubeInboundEnabled bool `default:"true" envconfig:"YOUTUBE_INBOUND_ENABLED"`

	// FacebookAPIURL points a PLATFORM=facebook instance at the platform-gateway
	// gateway-facebook instance over HTTP — e.g.
	// http://gateway-facebook.<env>.svc.cluster.local:8080. Both chat directions
	// flow through the gateway: outbound via its SendChat (which comments on the
	// Page's active live video as the Page), inbound via its GET /v1/chat/inbound
	// poll. The gateway also owns the Page access token, so tripbot holds none at
	// runtime. Required on a facebook instance — with this empty the instance
	// comes up without Facebook chat.
	FacebookAPIURL string `envconfig:"FACEBOOK_API_URL"`

	// InstagramAPIURL points a PLATFORM=instagram instance at the
	// platform-gateway gateway-instagram instance over HTTP — e.g.
	// http://gateway-instagram.<env>.svc.cluster.local:8080. Inbound-only: the
	// gateway polls live-broadcast comments off the Graph API; the Graph API
	// cannot create IG comments, so outbound Say is dropped (viewers get
	// responses via onscreens / playback effects). Required on an instagram
	// instance — with this empty the instance comes up without Instagram chat.
	InstagramAPIURL string `envconfig:"INSTAGRAM_API_URL"`

	// NatsURL is the in-cluster NATS endpoint used for fire-and-forget
	// inter-component events. Format:
	// nats://nats.<env-platform-ns>.svc.cluster.local:4222.
	// Optional — when unset, NATS publishes no-op silently. Lets local
	// dev / tests skip NATS entirely.
	NatsURL string `envconfig:"NATS_URL"`

	// DiscordAlertsWebhook is the Discord webhook URL that !report posts
	// viewer reports to. Optional — when unset, !report falls through to
	// slog/Sentry only and the bot keeps running.
	DiscordAlertsWebhook string `envconfig:"DISCORD_ALERTS_WEBHOOK"`

	// DiscordBotToken authenticates the live Discord bot session that
	// serves slash commands. Optional — when unset, missing, or still at
	// the AWS Secrets Manager placeholder value, pkg/discord skips
	// session init entirely and the rest of the bot runs normally.
	DiscordBotToken string `envconfig:"DISCORD_BOT_TOKEN"`
	// DiscordGuildID is the Discord server snowflake the bot registers
	// its slash commands against. Optional — leaving it empty in an
	// env's ConfigMap is the supported way to keep the Discord session
	// gated off without having to remove the token wiring.
	DiscordGuildID string `envconfig:"DISCORD_GUILD_ID"`
}
