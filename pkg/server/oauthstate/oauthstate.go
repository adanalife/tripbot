// Package oauthstate generates and validates the `state` query parameter for
// the OAuth Authorization Code flow. Generators stash a random token before
// redirecting to the IdP; validators consume it on the callback.
//
// State entries can carry an Account selector — the identity the flow was
// initiated for ("bot" or "broadcaster"). Callbacks read the account back
// on Validate so they can sanity-check the discovered identity against the
// expectation set at /auth/init time.
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

// Account is the identity selector carried through an OAuth flow.
// Empty value = flow doesn't enforce an identity check (legacy).
type Account string

const (
	AccountUnchecked   Account = ""
	AccountBot         Account = "bot"
	AccountBroadcaster Account = "broadcaster"
)

type entry struct {
	expiry  time.Time
	account Account
}

var (
	mu    sync.Mutex
	store = map[string]entry{}
	now   = time.Now // overridable in tests
)

// New mints a fresh state token, stashes the account selector + TTL, and
// returns the token. Pass AccountUnchecked when no identity check is wanted.
func New(account Account) string {
	var buf [32]byte
	if _, err := rand.Read(buf[:]); err != nil {
		// crypto/rand failure is a process-wide problem; panic is acceptable.
		panic("oauthstate: crypto/rand: " + err.Error())
	}
	state := hex.EncodeToString(buf[:])
	mu.Lock()
	store[state] = entry{expiry: now().Add(TTL), account: account}
	mu.Unlock()
	return state
}

// Validate consumes the state token. Returns the stashed Account and true
// exactly once per New(). Subsequent calls (or expired/unknown states)
// return AccountUnchecked, false.
func Validate(state string) (Account, bool) {
	if state == "" {
		return AccountUnchecked, false
	}
	mu.Lock()
	defer mu.Unlock()
	sweepLocked()
	e, ok := store[state]
	if !ok {
		return AccountUnchecked, false
	}
	delete(store, state)
	if !now().Before(e.expiry) {
		return AccountUnchecked, false
	}
	return e.account, true
}

// sweepLocked drops expired entries. Called from Validate so we never grow
// the store unboundedly when state tokens go unused.
func sweepLocked() {
	t := now()
	for k, e := range store {
		if !t.Before(e.expiry) {
			delete(store, k)
		}
	}
}
