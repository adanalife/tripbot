package config

type OnscreensServerConfig struct {
	Environment string `required:"true" envconfig:"ENV"`
	ServerType  string `default:"onscreens_server"`

	// Verbose determines output verbosity
	Verbose bool `default:"false" envconfig:"VERBOSE"`

	// RunDir is where temporary-but-important runtime files live.
	// EmptyDir / tmpfs is sufficient in k8s.
	RunDir string `default:"/opt/data/run" envconfig:"RUN_DIR"`

	// OnscreensServerBindAddress is the address (host:port or :port) the
	// onscreens-server HTTP listener binds to. The default stays :8081 — the
	// value from when onscreens-server was co-located with another server's
	// :8080 in one container. The standalone onscreens-server image overrides
	// this to :8080 (the project-wide HTTP convention) via
	// ONSCREENS_SERVER_BIND_ADDRESS, since on its own pod/IP there's nothing
	// to collide with.
	OnscreensServerBindAddress string `default:":8081" envconfig:"ONSCREENS_SERVER_BIND_ADDRESS"`

	// NatsURL is the in-cluster NATS endpoint the subscriber connects to.
	// Format: nats://nats.<env-platform-ns>.svc.cluster.local:4222.
	// Optional — when unset, the subscriber is skipped.
	NatsURL string `envconfig:"NATS_URL"`

	// Platform names the streaming platform this onscreens instance serves
	// ("twitch" / "youtube"). Drives per-platform rotator-message filtering so a
	// YouTube overlay doesn't advertise Twitch-only commands. Defaults to twitch
	// (the primary platform). Set by infra per onscreens-<platform> pod.
	Platform string `default:"twitch" envconfig:"PLATFORM"`

	// YouTubeInboundEnabled mirrors tripbot's gate of the same name for this
	// pipeline's platform. When false on a youtube instance the bot isn't
	// reading chat, so the rotators advertise promotional copy (watch/chat on
	// Twitch, interactivity coming soon) instead of command hints that would
	// no-op. Default true; ignored on twitch. Stamped per-pipeline by infra
	// alongside tripbot's flag so both surfaces flip together.
	YouTubeInboundEnabled bool `default:"true" envconfig:"YOUTUBE_INBOUND_ENABLED"`
}
