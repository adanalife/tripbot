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
	// RunDir is where temporary-but-important runtime files (the
	// vlc-server pidfile) live. EmptyDir / tmpfs is sufficient in k8s.
	RunDir string `default:"/opt/data/run" envconfig:"RUN_DIR"`

	// VlcFileCaching is the libvlc --file-caching value in milliseconds.
	// Defaults to today's hardcoded value; tune per-host without recompiling.
	VlcFileCaching int `default:"1111" envconfig:"VLC_FILE_CACHING"`
	// VlcAvcodecHw is the libvlc --avcodec-hw value (Linux-only). One of
	// none, vdpau_avcodec, cuda. Only applied on Linux hosts; ignored on
	// Windows/Darwin (matches today's platform-specific flag layering).
	VlcAvcodecHw string `default:"vdpau_avcodec" envconfig:"VLC_AVCODEC_HW"`
	// VlcVout is the libvlc --vout value (Linux-only). Defaults to dummy
	// (headless); the container no longer ships an X server. Set to x11
	// on a non-containerized host if local on-screen render is wanted.
	// Only applied on Linux hosts; ignored on Windows/Darwin.
	VlcVout string `default:"dummy" envconfig:"VLC_VOUT"`
	// VlcCanvasWidth / VlcCanvasHeight set both the libvlc --width/--height
	// and --canvas-width/--canvas-height. Defaults are today's hardcoded
	// 1920x1080.
	VlcCanvasWidth  int `default:"1920" envconfig:"VLC_CANVAS_WIDTH"`
	VlcCanvasHeight int `default:"1080" envconfig:"VLC_CANVAS_HEIGHT"`

	// VLCPidFile is where the vlc-server PID file lives
	VLCPidFile string `default:"/opt/data/run/vlc-server.pid" envconfig:"VLC_PIDFILE"`

	// VlcServerHost is used to specify the host for the VLC webserver
	VlcServerHost string `required:"true" envconfig:"VLC_SERVER_HOST"`
}
