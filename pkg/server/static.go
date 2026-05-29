package server

import (
	"embed"
	"net/http"
)

// staticFS holds the vendored frontend assets (htmx + its SSE extension),
// embedded so the binary is self-contained — no CDN dependency on a panel that
// runs in-cluster behind the tailnet. Pinned versions live in static/; bump per
// the use-latest-stable-when-adding ADR. See static/README.md.
//
//go:embed static
var staticFS embed.FS

// staticHandler serves the embedded assets. The embed keeps the "static/"
// prefix, so a request for /static/htmx.min.js resolves directly with no
// StripPrefix. Mounted at /static/ in server.Start.
func staticHandler() http.Handler {
	return http.FileServer(http.FS(staticFS))
}
