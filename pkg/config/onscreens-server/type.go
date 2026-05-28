package config

type OnscreensServerConfig struct {
	Environment string `required:"true" envconfig:"ENV"`
	ServerType  string `default:"onscreens_server"`

	// Verbose determines output verbosity
	Verbose bool `default:"false" envconfig:"VERBOSE"`

	// RunDir is where temporary-but-important runtime files (the
	// onscreens-server pidfile) live. EmptyDir / tmpfs is sufficient in k8s.
	RunDir string `default:"/opt/data/run" envconfig:"RUN_DIR"`

	// OnscreensPidFile is where the onscreens-server PID file lives
	OnscreensPidFile string `default:"/opt/data/run/onscreens-server.pid" envconfig:"ONSCREENS_PIDFILE"`

	// OnscreensServerBindAddress is the address (host:port or :port) the
	// onscreens-server HTTP listener binds to. Defaults to :8081 so it sits
	// adjacent to vlc-server's :8080 when they run in the same pod/container.
	OnscreensServerBindAddress string `default:":8081" envconfig:"ONSCREENS_SERVER_BIND_ADDRESS"`

	// NatsURL is the in-cluster NATS endpoint the subscriber connects to
	// (phase 1: tripbot.<env>.onscreens.middle.show). Format:
	// nats://nats.<env-platform-ns>.svc.cluster.local:4222. Optional —
	// when unset, the subscriber is skipped and HTTP is the sole transport.
	NatsURL string `envconfig:"NATS_URL"`
}
