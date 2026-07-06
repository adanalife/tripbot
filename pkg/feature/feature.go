// Package feature evaluates boolean feature flags against per-username and
// per-role allowlists, layered over a global default.
//
// The FlagClient interface is the only thing call sites should depend on;
// implementations include an in-process map (for tests and fallback) and a
// Postgres-backed client with a periodic refresh loop. The interface is
// shaped to match OpenFeature's bool-evaluation surface so a future swap to
// the OpenFeature SDK is mechanical.
package feature

import (
	"context"
	"slices"
	"sort"
	"time"
)

// FlagClient evaluates feature flag values for the application.
//
// Unknown flag keys evaluate to false — a flag-gated feature stays off until
// the flag exists in the backing store. This is a safety property: a typo
// in a key won't accidentally expose a feature.
//
// Snapshot returns the current set of known flags for admin / diagnostic
// surfaces, sorted by key. It's distinct from the hot Bool path: Bool to
// evaluate a single key, Snapshot to enumerate.
type FlagClient interface {
	Bool(ctx context.Context, key string, evalCtx EvalContext) bool
	Snapshot(ctx context.Context) []Flag
}

// FlagToggler is the admin write surface: flip a flag's global default on or
// off and make the change live immediately. It's kept separate from
// FlagClient so the read path stays OpenFeature-shaped (a future SDK swap
// only has to satisfy Bool/Snapshot); only the Postgres-backed client
// implements it. Callers that want the toggle surface type-assert their
// FlagClient to FlagToggler and degrade gracefully when it's absent (e.g.
// the in-memory fallback used before the DB client is wired).
type FlagToggler interface {
	// SetEnabled persists the new global-default state for key and refreshes
	// the in-memory snapshot so the change takes effect without waiting for
	// the next background poll. Returns an error if key doesn't exist.
	SetEnabled(ctx context.Context, key string, enabled bool) error
}

// EvalContext carries the targeting attributes a flag is evaluated against.
// The App is responsible for populating it (Channel, Env from config;
// Username and Roles from the user being acted on, if any).
type EvalContext struct {
	Username string   // Twitch login (lowercase). Empty for system-level evals.
	Roles    []string // {"mod","vip","sub","regular"} — multi-valued.
	Channel  string
	Env      string
}

// Flag is the resolved targeting shape for one flag, as held by clients in
// their in-memory snapshot. Description and TargetRemovalDate are loaded
// for the admin-panel surface; evaluate ignores them.
type Flag struct {
	Key                 string
	Description         string
	Enabled             bool
	EnabledForUsernames []string
	EnabledForRoles     []string
	TargetRemovalDate   time.Time
}

// sortFlags orders a slice of flags by key in place — stable order is what
// the admin panel renders, and what tests assert against.
func sortFlags(fs []Flag) {
	sort.Slice(fs, func(i, j int) bool { return fs[i].Key < fs[j].Key })
}

// evaluate applies the v1 targeting rule:
//
//	username allowlist > role allowlist > global default.
//
// Empty Username never matches the username allowlist (so a system eval
// against a username-targeted flag falls through to roles/default).
func evaluate(f Flag, ctx EvalContext) bool {
	if ctx.Username != "" {
		if slices.Contains(f.EnabledForUsernames, ctx.Username) {
			return true
		}
	}
	for _, want := range f.EnabledForRoles {
		for _, have := range ctx.Roles {
			if want == have {
				return true
			}
		}
	}
	return f.Enabled
}
