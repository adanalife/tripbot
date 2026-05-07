package config

type VlcServerConfig struct {
	Environment string `required:"true" envconfig:"ENV"`
	ServerType  string `default:"vlc_server"`

	// Verbose determines output verbosity
	Verbose bool `default:"false" envconfig:"VERBOSE"`
	// VlcVerbose adds extra VLC output
	VlcVerbose bool `default:"false" envconfig:"VLC_VERBOSE"`

	// VideoDir is where the videos live
	VideoDir string `default:"/opt/data/Dashcam/_all" envconfig:"VIDEO_DIR"`
	// RunDir is where temporary-but-important runtime files live (the
	// vlc-server pidfile, today). Onscreen state used to live here too
	// before the OBS browser-source migration; the directory is now
	// pidfile-only and works fine on an EmptyDir / tmpfs in k8s.
	RunDir string `default:"/opt/data/run" envconfig:"RUN_DIR"`

	// VLCPidFile is where the vlc-server PID file lives
	VLCPidFile string `default:"/opt/data/run/vlc-server.pid" envconfig:"VLC_PIDFILE"`

	// VlcServerHost is used to specify the host for the VLC webserver
	VlcServerHost string `required:"true" envconfig:"VLC_SERVER_HOST"`
}
