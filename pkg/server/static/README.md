# Vendored frontend assets

Embedded into the tripbot binary via `//go:embed static` (see `../static.go`)
and served at `/static/`. Vendored rather than CDN-loaded so the admin panel is
self-contained — it runs in-cluster behind the tailnet and shouldn't depend on
an external host being reachable at page-load.

| File | Package | Version | Source |
|---|---|---|---|
| `htmx.min.js` | htmx.org | 2.0.10 | https://unpkg.com/htmx.org@2.0.10/dist/htmx.min.js |
| `sse.js` | htmx-ext-sse | latest @ 2026-05-29 | https://unpkg.com/htmx-ext-sse/sse.js |
| `leaflet.js` / `leaflet.css` | leaflet | 1.9.4 | https://unpkg.com/leaflet@1.9.4/dist/ |

Leaflet powers the live location map. Map tiles load from OpenStreetMap at render
time (no API key); the 🚐 marker is an emoji `divIcon`, so Leaflet's default
marker images aren't needed and aren't vendored.

## Updating

Per the use-latest-stable-when-adding ADR, bump to the current stable release:

```sh
curl -sL -o htmx.min.js https://unpkg.com/htmx.org/dist/htmx.min.js
curl -sL -o sse.js      https://unpkg.com/htmx-ext-sse/sse.js
```

Then update the version column above (the resolved version is in `htmx.min.js`'s
header comment / the unpkg redirect `Location`).
