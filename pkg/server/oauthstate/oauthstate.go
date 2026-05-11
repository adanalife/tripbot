// Package oauthstate generates and validates the `state` query parameter for
// the OAuth Authorization Code flow. Generators stash a random token before
// redirecting to the IdP; validators consume it on the callback.
//
// State entries auto-expire after a short TTL and are single-use (deleted on
// successful Validate). The in-memory store is process-local — fine for the
// callback flow where the same process serves /auth/init and /auth/callback,
// not designed for distribution.
package oauthstate

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// TTL is how long a state token remains valid after New.
const TTL = 5 * time.Minute

var (
	mu    sync.Mutex
	store = map[string]time.Time{} // state -> expiry
	now   = time.Now              // overridable in tests
)

// New mints a fresh state token, stores it with TTL, and returns it.
func New() string {
	var buf [32]byte
	if _, err := rand.Read(buf[:]); err != nil {
		// crypto/rand failure is a process-wide problem; panic is acceptable.
		panic("oauthstate: crypto/rand: " + err.Error())
	}
	state := hex.EncodeToString(buf[:])
	mu.Lock()
	store[state] = now().Add(TTL)
	mu.Unlock()
	return state
}

// Validate consumes the state token. Returns true exactly once per New().
// Subsequent calls (or expired/unknown states) return false.
func Validate(state string) bool {
	if state == "" {
		return false
	}
	mu.Lock()
	defer mu.Unlock()
	sweepLocked()
	exp, ok := store[state]
	if !ok {
		return false
	}
	delete(store, state)
	return now().Before(exp)
}

// sweepLocked drops expired entries. Called from Validate so we never grow
// the store unboundedly when state tokens go unused.
func sweepLocked() {
	t := now()
	for k, exp := range store {
		if !t.Before(exp) {
			delete(store, k)
		}
	}
}
