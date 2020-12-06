package instrumentation

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	VlcServerHttpDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: "vlc_server_http_duration_seconds",
		Help: "Duration of HTTP requests.",
	}, []string{"path"})
)
