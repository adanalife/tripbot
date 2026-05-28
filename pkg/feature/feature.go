// Package feature evaluates boolean feature flags against per-username and
// per-role allowlists, layered over a global default.
//
// The FlagClient interface is the only thing call sites should depend on;
// implementations include an in-process map (for tests and fallback) and a
// Postgres-backed client with a periodic refresh loop. The interface is
// shaped to match OpenFeature's bool-evaluation surface so a future swap to
// the OpenFeature SDK is mechanical.
package feature

import "context"

// FlagClient evaluates feature flag values for the application.
//
// Unknown flag keys evaluate to false — a flag-gated feature stays off until
// the flag exists in the backing store. This is a safety property: a typo
// in a key won't accidentally expose a feature.
type FlagClient interface {
	Bool(ctx context.Context, key string, evalCtx EvalContext) bool
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
// their in-memory snapshot.
type Flag struct {
	Key                 string
	Enabled             bool
	EnabledForUsernames []string
	EnabledForRoles     []string
}

// evaluate applies the v1 targeting rule:
//
//	username allowlist > role allowlist > global default.
//
// Empty Username never matches the username allowlist (so a system eval
// against a username-targeted flag falls through to roles/default).
func evaluate(f Flag, ctx EvalContext) bool {
	if ctx.Username != "" {
		for _, u := range f.EnabledForUsernames {
			if u == ctx.Username {
				return true
			}
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
