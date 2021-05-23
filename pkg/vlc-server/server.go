package vlcServer

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/vlc-server"
	terrors "github.com/adanalife/tripbot/pkg/errors"
	"github.com/adanalife/tripbot/pkg/helpers"
<<<<<<< HEAD
	onscreensServer "github.com/adanalife/tripbot/pkg/onscreens-server"
	"github.com/davecgh/go-spew/spew"
=======
	sentrynegroni "github.com/getsentry/sentry-go/negroni"
>>>>>>> origin/master
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	metrics "github.com/slok/go-http-metrics/metrics/prometheus"
	"github.com/slok/go-http-metrics/middleware"
	negronimiddleware "github.com/slok/go-http-metrics/middleware/negroni"
	"github.com/unrolled/secure"
	"github.com/urfave/negroni"
)

<<<<<<< HEAD
// healthcheck URL, for tools to verify the stream is alive
func healthHandler(w http.ResponseWriter, r *http.Request) {
	//TODO: rewrite this as a handler
	healthCheck(w)
}

func vlcCurrentHandler(w http.ResponseWriter, r *http.Request) {
	// return the currently-playing file
	fmt.Fprintf(w, currentlyPlaying())
}

func vlcPlayHandler(w http.ResponseWriter, r *http.Request) {
	videoFile, ok := r.URL.Query()["video"]
	if !ok || len(videoFile) > 1 {
		//TODO: eventually this could just play instead of hard-requiring a param
		http.Error(w, "417 expectation failed", http.StatusExpectationFailed)
		return
	}

	spew.Dump(videoFile)
	playVideoFile(videoFile[0])

	//TODO: better response
	fmt.Fprintf(w, "OK")
}

func vlcBackHandler(w http.ResponseWriter, r *http.Request) {
	num, ok := r.URL.Query()["n"]
	if !ok || len(num) > 1 {
		back(1)
		return
	}
	i, err := strconv.Atoi(num[0])
	if err != nil {
		terrors.Log(err, "couldn't convert input to int")
		http.Error(w, "422 unprocessable entity", http.StatusUnprocessableEntity)
		return
	}

	back(i)

	//TODO: better response
	fmt.Fprintf(w, "OK")

}

func vlcSkipHandler(w http.ResponseWriter, r *http.Request) {
	num, ok := r.URL.Query()["n"]
	if !ok || len(num) > 1 {
		skip(1)
		return
	}
	i, err := strconv.Atoi(num[0])
	if err != nil {
		terrors.Log(err, "couldn't convert input to int")
		http.Error(w, "422 unprocessable entity", http.StatusUnprocessableEntity)
		return
	}

	skip(i)

	//TODO: better response
	fmt.Fprintf(w, "OK")
}

func vlcRandomHandler(w http.ResponseWriter, r *http.Request) {
	// play a random file
	err := PlayRandom()
	if err != nil {
		http.Error(w, "error playing random", http.StatusInternalServerError)
	}
	fmt.Fprintf(w, "OK")
}

func onscreensFlagShowHandler(w http.ResponseWriter, r *http.Request) {
	base64content, ok := r.URL.Query()["duration"]
	if !ok || len(base64content) > 1 {
		http.Error(w, "417 expectation failed", http.StatusExpectationFailed)
		return
	}
	durStr, err := helpers.Base64Decode(base64content[0])
	if err != nil {
		terrors.Log(err, "unable to decode string")
		http.Error(w, "422 unprocessable entity", http.StatusUnprocessableEntity)
		return
	}
	dur, err := time.ParseDuration(durStr)
	if err != nil {
		http.Error(w, "unable to parse duration", http.StatusInternalServerError)
		return
	}
	onscreensServer.ShowFlag(dur)
	fmt.Fprintf(w, "OK")
}

func onscreensGpsHideHandler(w http.ResponseWriter, r *http.Request) {
	onscreensServer.HideGPSImage()
	fmt.Fprintf(w, "OK")
}

func onscreensGpsShowHandler(w http.ResponseWriter, r *http.Request) {
	onscreensServer.ShowGPSImage()
	fmt.Fprintf(w, "OK")
}

func onscreensTimewarpShowHandler(w http.ResponseWriter, r *http.Request) {
	onscreensServer.ShowTimewarp()
	fmt.Fprintf(w, "OK")
}

func onscreensLeaderboardShowHandler(w http.ResponseWriter, r *http.Request) {
	base64content, ok := r.URL.Query()["content"]
	if !ok || len(base64content) > 1 {
		http.Error(w, "417 expectation failed", http.StatusExpectationFailed)
		return
	}
	spew.Dump(base64content[0])
	content, err := helpers.Base64Decode(base64content[0])
	if err != nil {
		terrors.Log(err, "unable to decode string")
		http.Error(w, "422 unprocessable entity", http.StatusUnprocessableEntity)
		return
	}

	onscreensServer.Leaderboard.Show(content)
	fmt.Fprintf(w, "OK")
}

func onscreensMiddleHideHandler(w http.ResponseWriter, r *http.Request) {
	onscreensServer.MiddleText.Hide()
	fmt.Fprintf(w, "OK")

}

func onscreensMiddleShowHandler(w http.ResponseWriter, r *http.Request) {
	base64content, ok := r.URL.Query()["msg"]
	if !ok || len(base64content) > 1 {
		http.Error(w, "417 expectation failed", http.StatusExpectationFailed)
		return
	}
	msg, err := helpers.Base64Decode(base64content[0])
	if err != nil {
		terrors.Log(err, "unable to decode string")
		http.Error(w, "422 unprocessable entity", http.StatusUnprocessableEntity)
		return
	}
	onscreensServer.MiddleText.Show(msg)
	fmt.Fprintf(w, "OK")
}

func faviconHandler(w http.ResponseWriter, r *http.Request) {
	//	// return a favicon if anyone asks for one
	//} else if r.URL.Path == "/favicon.ico" {
	http.ServeFile(w, r, "assets/favicon.ico")
}

//TODO: use more StatusExpectationFailed instead of http.StatusUnprocessableEntity
func catchAllHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		http.Error(w, "404 not found", http.StatusNotFound)
		log.Println("someone tried hitting", r.URL.Path)
		return

	// someone tried a PUT or a DELETE or something
	default:
		fmt.Fprintf(w, "Only GET methods are supported.\n")
	}
}

=======
>>>>>>> origin/master
// Start starts the web server
func Start() {
	log.Println("Starting VLC web server on host", c.Conf.VlcServerHost)

	r := mux.NewRouter()
<<<<<<< HEAD

	// add the prometheus middleware
	r.Use(helpers.PrometheusMiddleware)
	// make prometheus metrics available
	r.Path("/metrics").Handler(promhttp.Handler())

	r.HandleFunc("/health", healthHandler).Methods("GET")

	// vlc endpoints
	//TODO: consider refactoring into a subrouter
	r.HandleFunc("/vlc/current", vlcCurrentHandler).Methods("GET")
	r.HandleFunc("/vlc/play", vlcPlayHandler).Methods("GET")
	r.HandleFunc("/vlc/back", vlcBackHandler).Methods("GET")
	r.HandleFunc("/vlc/skip", vlcSkipHandler).Methods("GET")
	r.HandleFunc("/vlc/random", vlcRandomHandler).Methods("GET")

	// onscreen endpoints
	//TODO: consider refactoring into a subrouter
	r.HandleFunc("/onscreens/flag/show", onscreensFlagShowHandler).Methods("GET")
	r.HandleFunc("/onscreens/gps/hide", onscreensGpsHideHandler).Methods("GET")
	r.HandleFunc("/onscreens/gps/show", onscreensGpsShowHandler).Methods("GET")
	r.HandleFunc("/onscreens/timewarp/show", onscreensTimewarpShowHandler).Methods("GET")
	r.HandleFunc("/onscreens/leaderboard/show", onscreensLeaderboardShowHandler).Methods("GET")
	r.HandleFunc("/onscreens/middle/hide", onscreensMiddleHideHandler).Methods("GET")
	r.HandleFunc("/onscreens/middle/show", onscreensMiddleShowHandler).Methods("GET")

	//TODO: refactor into static serving
	r.HandleFunc("/favicon.ico", faviconHandler).Methods("GET")

	//TODO: update to be proper catchall(?)
	// r.PathPrefix("/").Handler(catchAllHandler)
	r.HandleFunc("/", catchAllHandler)
	http.Handle("/", r)

	// ListenAndServe() wants a port in the format ":NUM"
=======

	// healthcheck endpoints
	//TODO: handle HEAD requests here too
	hp := r.PathPrefix("/health").Methods("GET").Subrouter()
	hp.HandleFunc("/", healthHandler)
	hp.HandleFunc("/live", healthHandler)
	hp.HandleFunc("/ready", healthHandler)

	// vlc endpoints
	vlc := r.PathPrefix("/vlc").Methods("GET").Subrouter()
	vlc.HandleFunc("/current", vlcCurrentHandler)
	vlc.HandleFunc("/play/{video}", vlcPlayHandler)
	vlc.HandleFunc("/random", vlcRandomHandler)
	vlc.HandleFunc("/back", vlcBackHandler)
	vlc.HandleFunc("/back/{n}", vlcBackHandler)
	vlc.HandleFunc("/skip", vlcSkipHandler)
	vlc.HandleFunc("/skip/{n}", vlcSkipHandler)

	// onscreen endpoints
	osc := r.PathPrefix("/onscreens").Methods("GET").Subrouter()
	//TODO: add state variable
	osc.HandleFunc("/flag/{action}", onscreensFlagHandler)
	osc.HandleFunc("/gps/{action}", onscreensGpsHandler)
	osc.HandleFunc("/gps/{action}", onscreensGpsHandler)
	osc.HandleFunc("/leaderboard/{action}", onscreensLeaderboardHandler)
	osc.HandleFunc("/middle/{action}", onscreensMiddleHandler)
	osc.HandleFunc("/middle/{action}", onscreensMiddleHandler)
	osc.HandleFunc("/timewarp/{action}", onscreensTimewarpHandler)

	// prometheus metrics endpoint
	r.Path("/metrics").Handler(promhttp.Handler())

	// static assets
	r.HandleFunc("/favicon.ico", faviconHandler).Methods("GET")

	// catch everything else
	r.HandleFunc("/", catchAllHandler)

	if c.Conf.Verbose {
		helpers.PrintAllRoutes(r)
	}

	// negroni classic adds panic recovery, logger, and static file middlewares
	// c.p. https://github.com/urfave/negroni
	//TODO: consider adding HTMLPanicFormatter
	app := negroni.Classic()

	// attach http-metrics (prometheus) middleware
	metricsMw := middleware.New(middleware.Config{
		Recorder: metrics.NewRecorder(metrics.Config{}),
		Service:  c.Conf.ServerType,
	})
	app.Use(negronimiddleware.Handler("", metricsMw))

	// attach security middleware
	secureMw := secure.New(secure.Options{
		FrameDeny:     true,
		IsDevelopment: c.Conf.IsDevelopment(),
	})
	app.Use(negroni.HandlerFunc(secureMw.HandlerFuncWithNext))

	// attach Sentry middleware (for reporting exceptions)
	app.Use(sentrynegroni.New(sentrynegroni.Options{}))

	// attaching routes to handler happens last
	app.UseHandler(r)

>>>>>>> origin/master
	//TODO: error if there's no colon to split on
	port := strings.Split(c.Conf.VlcServerHost, ":")[1]

	srv := &http.Server{
		Addr: fmt.Sprintf("0.0.0.0:%s", port),
		// Good practice to set timeouts to avoid Slowloris attacks.
		WriteTimeout:   time.Second * 15,
		ReadTimeout:    time.Second * 15,
		IdleTimeout:    time.Second * 60,
		MaxHeaderBytes: 1 << 20, // 1 MB
		Handler:        app,     // Pass our instance of negroni in
	}

	//TODO: add graceful shutdown
	if err := srv.ListenAndServe(); err != nil {
		terrors.Fatal(err, "couldn't start server")
	}
}
