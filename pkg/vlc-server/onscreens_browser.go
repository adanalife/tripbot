package vlcServer

import (
	"embed"
	"encoding/json"
	"html/template"
	"net/http"
	"path/filepath"

	c "github.com/adanalife/tripbot/pkg/config/vlc-server"
	terrors "github.com/adanalife/tripbot/pkg/errors"
	"github.com/adanalife/tripbot/pkg/helpers"
	onscreensServer "github.com/adanalife/tripbot/pkg/onscreens-server"
	"github.com/gorilla/mux"
)

//go:embed templates/onscreen.html.tmpl
var onscreenTemplates embed.FS

var onscreenTmpl = template.Must(template.ParseFS(onscreenTemplates, "templates/onscreen.html.tmpl"))

// onscreenStyle controls how a single onscreen renders in its OBS browser source.
// Keep these in sync with the dimensions / fonts that the previous text_ft2_source
// and image_source entries in infra/docker/obs/config/Tripbot.json.tmpl used.
type onscreenStyle struct {
	Name       string       // URL slug; matches the key in /onscreens/state.json
	IsImage    bool         // image vs. text source
	FontCSS    template.CSS // CSS font-family (only meaningful for text)
	FontSizePx int          // CSS font-size in px (only meaningful for text)
	ColorCSS   template.CSS // CSS color (only meaningful for text)
	DropShadow bool         // text-shadow on/off
	get        func() *onscreensServer.Onscreen
}

var onscreenRegistry = map[string]onscreenStyle{
	"middle-text": {
		Name: "middle-text", FontCSS: `"Trebuchet MS", sans-serif`, FontSizePx: 18, ColorCSS: "#ffffff",
		get: func() *onscreensServer.Onscreen { return onscreensServer.MiddleText },
	},
	"leaderboard": {
		Name: "leaderboard", FontCSS: `"Trebuchet MS", sans-serif`, FontSizePx: 18, ColorCSS: "#ffffff",
		get: func() *onscreensServer.Onscreen { return onscreensServer.Leaderboard },
	},
	"left-message": {
		Name: "left-message", FontCSS: `"Trebuchet MS", sans-serif`, FontSizePx: 28, ColorCSS: "#ffffff",
		get: func() *onscreensServer.Onscreen { return onscreensServer.LeftRotator },
	},
	"right-message": {
		Name: "right-message", FontCSS: `"Trebuchet MS", sans-serif`, FontSizePx: 28, ColorCSS: "#ffffff",
		get: func() *onscreensServer.Onscreen { return onscreensServer.RightRotator },
	},
	"timewarp": {
		Name: "timewarp", FontCSS: `sans-serif`, FontSizePx: 72, ColorCSS: "#ffffff", DropShadow: true,
		get: func() *onscreensServer.Onscreen { return onscreensServer.Timewarp },
	},
	"gps": {
		Name: "gps", IsImage: true,
		get: func() *onscreensServer.Onscreen { return onscreensServer.GPSImage },
	},
	"flag": {
		Name: "flag", IsImage: true,
		get: func() *onscreensServer.Onscreen { return onscreensServer.FlagImage },
	},
}

// onscreensStateHandler returns a JSON snapshot of every onscreen's current
// state. The OBS browser-source HTML pages poll this endpoint and re-render.
func onscreensStateHandler(w http.ResponseWriter, r *http.Request) {
	out := make(map[string]onscreensServer.Snapshot, len(onscreenRegistry))
	for name, style := range onscreenRegistry {
		out[name] = style.get().Snapshot()
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(out); err != nil {
		terrors.Log(err, "encoding onscreens state")
	}
}

// onscreensRenderHandler returns the HTML page that an OBS browser source
// loads for a single onscreen. The page polls /onscreens/state.json and
// updates its DOM in place.
func onscreensRenderHandler(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	style, ok := onscreenRegistry[name]
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := onscreenTmpl.Execute(w, style); err != nil {
		terrors.Log(err, "rendering onscreen template")
	}
}

// onscreensAssetHandler serves the raw image bytes for image-type onscreens.
// Today only `gps` and `flag` resolve; both pull from the on-disk source the
// onscreens-server package already manages.
func onscreensAssetHandler(w http.ResponseWriter, r *http.Request) {
	switch mux.Vars(r)["name"] {
	case "gps":
		http.ServeFile(w, r, filepath.Join(helpers.ProjectRoot(), "assets", "GPS.png"))
	case "flag":
		http.ServeFile(w, r, filepath.Join(c.Conf.RunDir, "flag.png"))
	default:
		http.NotFound(w, r)
	}
}
