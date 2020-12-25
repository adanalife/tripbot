package instrumentation

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	HttpDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: "vlc_server_http_duration_seconds",
		Help: "Duration of HTTP requests.",
	}, []string{"server_type", "path"})
)
