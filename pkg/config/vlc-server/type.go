package config

type VlcServerConfig struct {
	environment string `required:"true" envconfig:"ENV"`
	ServerType  string `default:"vlc_server"`

	// Verbose determines output verbosity
	Verbose bool `default:"false" envconfig:"VERBOSE"`
	// VlcVerbose adds extra VLC output
	VlcVerbose bool `default:"false" envconfig:"VLC_VERBOSE"`

	// VideoDir is where the videos live
	VideoDir string `default:"/opt/data/Dashcam/_all" envconfig:"VIDEO_DIR"`
	// RunDir is where temporary-but-important runtime files live (such as pidfiles and onscreen content)
	RunDir string `default:"/opt/data/run" envconfig:"RUN_DIR"`

	// VLCPidFile is where the vlc-server PID file lives
	VLCPidFile string `default:"/opt/data/run/vlc-server.pid" envconfig:"VLC_PIDFILE"`
	// OBSPidFile is where the OBS PID file lives
	OBSPidFile string `default:"/opt/data/run/OBS.pid" envconfig:"OBS_PIDFILE"`

	// VlcServerHost is used to specify the host for the VLC webserver
	VlcServerHost string `required:"true" envconfig:"VLC_SERVER_HOST"`
}
