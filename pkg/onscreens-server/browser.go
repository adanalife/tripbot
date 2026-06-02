package onscreensServer

import (
	"embed"
	"encoding/json"
	"html/template"
	"log/slog"
	"net/http"

	"github.com/gorilla/mux"
)

// flagPlaceholderPNG is a 1×1 transparent PNG served by the flag asset
// endpoint while the state-driven flag swap is disabled (see flag.go's
// TODO). The browser source's <img> tag fetches this URL even when the
// onscreen is hidden, so we serve a valid PNG to keep the request quiet
// rather than 404.
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

// gpsPNG is the GPS map overlay, embedded so the binary is self-contained and
// has no runtime dependency on the source tree (helpers.ProjectRoot() resolves
// to a compile-time path that doesn't exist in the slim runtime image). Kept in
// sync with the canonical assets/GPS.png at the repo root.
//
//go:embed assets/GPS.png
var gpsPNG []byte

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
	Name          string       // URL slug; matches a Slug constant in this package
	IsImage       bool         // image vs. text source
	FontCSS       template.CSS // CSS font-family (only meaningful for text)
	FontSizePx    int          // default / max font-size in px (only meaningful for text)
	MinFontSizePx int          // shrink-to-fit floor (only used when FitWidthPx > 0)
	ColorCSS      template.CSS // CSS color (only meaningful for text)
	DropShadow    bool         // text-shadow on/off
	AnchorXPx     int          // center-x within the browser-source viewport (0 = use flex-center fallback)
	FitWidthPx    int          // single-line width budget for shrink-to-fit (0 = no fit pass)
	RenderAsHTML  bool         // inject content via innerHTML instead of textContent (server emits HTML for this onscreen)
}

var onscreenRegistry = map[string]onscreenStyle{
	SlugMiddleText: {
		Name: SlugMiddleText, FontCSS: `"Trebuchet MS", sans-serif`, FontSizePx: 18, ColorCSS: "#ffffff",
	},
	SlugLeaderboard: {
		// Server emits an HTML grid (see renderLeaderboard) so the
		// score column aligns via CSS rather than space-padding — any font
		// works.
		Name: SlugLeaderboard, FontCSS: `"Trebuchet MS", sans-serif`, FontSizePx: 18, ColorCSS: "#ffffff",
		RenderAsHTML: true,
	},
	SlugLeftMessage: {
		Name: SlugLeftMessage, FontCSS: `"Trebuchet MS", sans-serif`, FontSizePx: 28, MinFontSizePx: 18, ColorCSS: "#ffffff",
		AnchorXPx: 282, FitWidthPx: 564,
	},
	SlugRightMessage: {
		Name: SlugRightMessage, FontCSS: `"Trebuchet MS", sans-serif`, FontSizePx: 28, MinFontSizePx: 18, ColorCSS: "#ffffff",
		AnchorXPx: 456, FitWidthPx: 369,
	},
	SlugTimewarp: {
		Name: SlugTimewarp, FontCSS: `sans-serif`, FontSizePx: 72, ColorCSS: "#ffffff", DropShadow: true,
	},
	SlugGPS: {
		Name: SlugGPS, IsImage: true,
	},
	SlugFlag: {
		Name: SlugFlag, IsImage: true,
	},
}

// onscreensStateHandler returns a JSON snapshot of every onscreen's current
// state. The OBS browser-source HTML pages poll this endpoint and re-render.
func (s *Server) onscreensStateHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(s.Snapshot()); err != nil {
		slog.ErrorContext(r.Context(), "encoding onscreens state", "err", err)
	}
}

// onscreensRenderHandler returns the HTML page that an OBS browser source
// loads for a single onscreen. The page polls /onscreens/state.json and
// updates its DOM in place.
func (s *Server) onscreensRenderHandler(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	style, ok := onscreenRegistry[name]
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := onscreenTmpl.Execute(w, style); err != nil {
		slog.ErrorContext(r.Context(), "rendering onscreen template", "err", err)
	}
}

// onscreensAssetHandler serves the raw image bytes for image-type onscreens.
// `gps` resolves to the embedded GPS overlay; `flag` returns a 1×1
// transparent placeholder while the state-driven flag swap is offline.
func (s *Server) onscreensAssetHandler(w http.ResponseWriter, r *http.Request) {
	switch mux.Vars(r)["name"] {
	case SlugGPS:
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "no-store")
		if _, err := w.Write(gpsPNG); err != nil {
			slog.ErrorContext(r.Context(), "writing gps image", "err", err)
		}
	case SlugFlag:
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "no-store")
		if _, err := w.Write(flagPlaceholderPNG); err != nil {
			slog.ErrorContext(r.Context(), "writing flag placeholder", "err", err)
		}
	default:
		http.NotFound(w, r)
	}
}
