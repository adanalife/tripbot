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
	// Tune per-host without recompiling. Lower = faster between-clip
	// startup (smaller frame gap that OBS's ffmpeg_source has to ride
	// out before disconnect/reconnect kicks in); higher = more headroom
	// for bursty disk/NFS read latency. 300ms is a working baseline for
	// local-NFS-backed playback; the prior 1111 was unscientific tuning
	// from years ago that left a >1s frame gap on each clip transition.
	VlcFileCaching int `default:"300" envconfig:"VLC_FILE_CACHING"`
	// VlcAvcodecHw is the libvlc --avcodec-hw value (Linux-only). One of
	// none, vdpau_avcodec, cuda. Only applied on Linux hosts; ignored on
	// Windows/Darwin (matches today's platform-specific flag layering).
	VlcAvcodecHw string `default:"vdpau_avcodec" envconfig:"VLC_AVCODEC_HW"`
	// VlcVout is the libvlc --vout value (Linux-only). Defaults to dummy
	// (headless container, no X server). For local-display modes on
	// Linux, set VLC_VOUT=x11. Only applied on Linux hosts; ignored on
	// Windows/Darwin (libvlc picks the platform default vout).
	VlcVout string `default:"dummy" envconfig:"VLC_VOUT"`
	// VlcOutput selects what vlc-server emits frames to:
	//   rtsp   — RTSP listener only, no local window (the container default).
	//   window — local libvlc window only, no RTSP listener.
	//   both   — duplicate to both window and RTSP listener.
	// `window` and `both` need a vout module that can actually open a
	// window (Linux: VLC_VOUT=x11; Windows: libvlc default).
	VlcOutput string `default:"rtsp" envconfig:"VLC_OUTPUT"`
	// VlcCanvasWidth / VlcCanvasHeight set both the libvlc --width/--height
	// and --canvas-width/--canvas-height. Defaults are today's hardcoded
	// 1920x1080.
	VlcCanvasWidth  int `default:"1920" envconfig:"VLC_CANVAS_WIDTH"`
	VlcCanvasHeight int `default:"1080" envconfig:"VLC_CANVAS_HEIGHT"`

	// VLCPidFile is where the vlc-server PID file lives
	VLCPidFile string `default:"/opt/data/run/vlc-server.pid" envconfig:"VLC_PIDFILE"`

	// VlcServerBindAddress is the address (host:port or :port) the
	// vlc-server HTTP listener binds to. Defaults to :8080 so the pod
	// boots without being told its own address; override when running
	// multiple instances on one host or to pin to a specific interface.
	VlcServerBindAddress string `default:":8080" envconfig:"VLC_SERVER_BIND_ADDRESS"`
}
