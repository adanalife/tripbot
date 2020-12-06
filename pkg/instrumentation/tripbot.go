package instrumentation

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	ChatMessages = promauto.NewCounter(prometheus.CounterOpts{
		Name: "tripbot_chat_messages_total",
		Help: "The total number of chat messages",
	})
	ChatCommands = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "tripbot_chat_commands_total",
		Help: "The total number of chat commands",
	}, []string{"command"},
	)
	// TripbotServerHttpDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
	// 	Name: "tripbot_server_http_duration_seconds",
	// 	Help: "Duration of HTTP requests.",
	// }, []string{"path"})
)
