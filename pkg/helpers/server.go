package helpers

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/adanalife/tripbot/pkg/config"
	"github.com/adanalife/tripbot/pkg/instrumentation"
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus"
)

// PrometheusMiddleware implements mux.MiddlewareFunc.
// func PrometheusMiddleware(next http.Handler) http.Handler {
// 	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 		route := mux.CurrentRoute(r)
// 		path, _ := route.GetPathTemplate()
// 		timer := prometheus.NewTimer(instrumentation.HttpDuration.WithLabelValues(config.ServerType, path))
// 		next.ServeHTTP(w, r)
// 		timer.ObserveDuration()
// 	})
// }

func PrometheusMiddleware(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	route := mux.CurrentRoute(r)
	path, _ := route.GetPathTemplate()
	timer := prometheus.NewTimer(instrumentation.HttpDuration.WithLabelValues(config.ServerType, path))
	next(rw, r)
	timer.ObserveDuration()
}

func PrintAllRoutes(r *mux.Router) {
	err := r.Walk(func(route *mux.Route, router *mux.Router, ancestors []*mux.Route) error {
		pathTemplate, err := route.GetPathTemplate()
		if err == nil {
			fmt.Println("ROUTE:", pathTemplate)
		}
		pathRegexp, err := route.GetPathRegexp()
		if err == nil {
			fmt.Println("Path regexp:", pathRegexp)
		}
		queriesTemplates, err := route.GetQueriesTemplates()
		if err == nil {
			fmt.Println("Queries templates:", strings.Join(queriesTemplates, ","))
		}
		queriesRegexps, err := route.GetQueriesRegexp()
		if err == nil {
			fmt.Println("Queries regexps:", strings.Join(queriesRegexps, ","))
		}
		methods, err := route.GetMethods()
		if err == nil {
			fmt.Println("Methods:", strings.Join(methods, ","))
		}
		fmt.Println()
		return nil
	})

	if err != nil {
		fmt.Println(err)
	}
}
