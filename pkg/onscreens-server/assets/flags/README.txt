US state flag PNGs (lowercase state abbreviation), downloaded from
https://usa.flagpedia.net/download. Embedded by pkg/onscreens-server
(see browser.go's flagsFS) and served per-state by the flag onscreen.
Relocated here from the repo-root assets/flags/ so Go's //go:embed can
reach them (embed can't cross above the package directory).
