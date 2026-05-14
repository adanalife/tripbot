package vlcServer

import (
	"embed"
	"encoding/json"
	"html/template"
	"net/http"
	"path/filepath"

	terrors "github.com/adanalife/tripbot/pkg/errors"
	"github.com/adanalife/tripbot/pkg/helpers"
	onscreensServer "github.com/adanalife/tripbot/pkg/onscreens-server"
	"github.com/gorilla/mux"
)

// flagPlaceholderPNG is a 1×1 transparent PNG served by the flag asset
// endpoint while the state-driven flag swap is disabled (see
// onscreens-server/flag.go's TODO). The browser source's <img> tag
// fetches this URL even when the onscreen is hidden, so we serve a
// valid PNG to keep the request quiet rather than 404.
var flagPlaceholderPNG = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
	0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
	0x89, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x44, 0x41,
	0x54, 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00,
	0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00,
	0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, 0xae,
	0x42, 0x60, 0x82,
}

//go:embed templates/onscreen.html.tmpl
var onscreenTemplates embed.FS

var onscreenTmpl = template.Must(template.ParseFS(onscreenTemplates, "templates/onscreen.html.tmpl"))

// onscreenStyle controls how a single onscreen renders in its OBS browser source.
// Keep these in sync with the dimensions / fonts that the previous text_ft2_source
// and image_source entries in infra/docker/obs/config/Tripbot.json.tmpl used.
//
// For text onscreens whose browser-source viewport overhangs a smaller on-canvas
// overlay box (the bottom-strip rotators sit on a 640px third but their grey-box
// underlay is narrower — 564px for left, 369px for right), AnchorXPx + FitWidthPx
// describe the desired "centering window" within the viewport. When both are set,
// the template switches to an anchored layout and runs a shrink-to-fit pass in JS:
// content renders at FontSizePx and shrinks down to MinFontSizePx (1px steps) until
// it fits inside FitWidthPx on a single line, then falls back to wrapping if the
// floor doesn't fit.
type onscreenStyle struct {
	Name          string       // URL slug; matches the key in /onscreens/state.json
	IsImage       bool         // image vs. text source
	FontCSS       template.CSS // CSS font-family (only meaningful for text)
	FontSizePx    int          // default / max font-size in px (only meaningful for text)
	MinFontSizePx int          // shrink-to-fit floor (only used when FitWidthPx > 0)
	ColorCSS      template.CSS // CSS color (only meaningful for text)
	DropShadow    bool         // text-shadow on/off
	AnchorXPx     int          // center-x within the browser-source viewport (0 = use flex-center fallback)
	FitWidthPx    int          // single-line width budget for shrink-to-fit (0 = no fit pass)
	get           func() *onscreensServer.Onscreen
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
		Name: "left-message", FontCSS: `"Trebuchet MS", sans-serif`, FontSizePx: 28, MinFontSizePx: 18, ColorCSS: "#ffffff",
		AnchorXPx: 282, FitWidthPx: 564,
		get: func() *onscreensServer.Onscreen { return onscreensServer.LeftRotator },
	},
	"right-message": {
		Name: "right-message", FontCSS: `"Trebuchet MS", sans-serif`, FontSizePx: 28, MinFontSizePx: 18, ColorCSS: "#ffffff",
		AnchorXPx: 456, FitWidthPx: 369,
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
	out := make(map[string]*onscreensServer.Onscreen, len(onscreenRegistry))
	for name, style := range onscreenRegistry {
		out[name] = style.get()
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
// `gps` resolves to the checked-in GPS overlay; `flag` returns a 1×1
// transparent placeholder while the state-driven flag swap is offline.
func onscreensAssetHandler(w http.ResponseWriter, r *http.Request) {
	switch mux.Vars(r)["name"] {
	case "gps":
		http.ServeFile(w, r, filepath.Join(helpers.ProjectRoot(), "assets", "GPS.png"))
	case "flag":
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "no-store")
		if _, err := w.Write(flagPlaceholderPNG); err != nil {
			terrors.Log(err, "writing flag placeholder")
		}
	default:
		http.NotFound(w, r)
	}
}
