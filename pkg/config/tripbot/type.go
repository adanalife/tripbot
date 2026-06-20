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
	// OutputChannel is the stream to which the bot will speak
	OutputChannel string
	// BotUsername is the username of the bot
	BotUsername string `required:"true" envconfig:"BOT_USERNAME"`
	// ExternalURL is the where the bot's HTTP server can be reached
	ExternalURL string `required:"true" envconfig:"EXTERNAL_URL"`
	// GoogleMapsAPIKey is the API key with which we access Google Maps.
	// Optional — when unset, geocoder + static-map calls are skipped and
	// callers fall back gracefully (no city/state lookups, no generated
	// maps). The bot continues to run.
	GoogleMapsAPIKey string `envconfig:"GOOGLE_MAPS_API_KEY"`

	// YouTubeClientID / YouTubeClientSecret are the YouTube OAuth app
	// credentials (a GCP-console "Web application" OAuth client whose
	// authorized redirect URIs include <EXTERNAL_URL>/auth/callback).
	// Only set on PLATFORM=youtube instances; optional everywhere else —
	// pkg/youtube returns ErrNotConfigured rather than fataling when absent.
	YouTubeClientID     string `envconfig:"YOUTUBE_CLIENT_ID"`
	YouTubeClientSecret string `envconfig:"YOUTUBE_CLIENT_SECRET"`
	// YouTubeChannelID optionally pins the expected channel identity. When
	// set, the /auth/init?account=youtube consent flow rejects (and does not
	// persist) tokens from any other channel — keeps a prod pod from storing
	// the quiet test channel's token, and vice versa.
	YouTubeChannelID string `envconfig:"YOUTUBE_CHANNEL_ID"`

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

	// YouTubeAPIURL points a PLATFORM=youtube instance's outbound chat sends at
	// the platform-gateway gateway-youtube instance over HTTP, instead of the
	// in-process pkg/youtube path. Empty (the default) keeps the in-process
	// adapter. When set — e.g. http://gateway-youtube.<env>.svc.cluster.local:8080
	// — outbound chat send routes through the gateway's SendChat (which resolves
	// the active live chat itself) unconditionally; there is no runtime flag
	// (unlike Twitch — YouTube cuts straight over). The inbound chat poll stays
	// in-process (no gateway streaming endpoint).
	YouTubeAPIURL string `envconfig:"YOUTUBE_API_URL"`

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
