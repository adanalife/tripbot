// Package tripbot holds what the repo root carries for Go code to read: the
// fleet-wide supported-platform set, synced from platform-gateway into
// platforms.json by `task platforms:sync`.
package tripbot

import (
	_ "embed"
	"encoding/json"
	"slices"
)

//go:embed platforms.json
var platformsJSON []byte

var platforms = func() []string {
	var f struct {
		Platforms []string `json:"platforms"`
	}
	if err := json.Unmarshal(platformsJSON, &f); err != nil || len(f.Platforms) == 0 {
		panic("platforms.json: no platform list")
	}
	return f.Platforms
}()

// Platforms returns every streaming platform the fleet supports, in
// platforms.json order. Code and tests that need the whole set range over this
// instead of restating it, so a platform the gateway adds shows up here on the
// next sync.
func Platforms() []string { return slices.Clone(platforms) }
