package onscreensServer

import (
	"sync"
	"time"

	rot "github.com/adanalife/tripbot/pkg/rotator"
)

// locationDataTTL bounds how long received clip data stays eligible for the
// rotators. tripbot republishes on a ~60s timer, so a value older than this means
// tripbot stopped publishing (the bot crashed, NATS is down) — every $variable
// then reads as unset, and the lines using them drop out of the rotation rather
// than describing footage that has since moved on.
const locationDataTTL = 5 * time.Minute

// liveLocation caches the most recent clip data pushed from tripbot over
// location.update. The rotators resolve their $variables from it. Package scope
// because the rotator loops are package-level; access is mutex-guarded.
var liveLocation = &locationStore{}

type locationStore struct {
	mu        sync.RWMutex
	vars      rot.Vars
	updatedAt time.Time
}

// set caches the latest clip data with the time it arrived.
func (s *locationStore) set(vars rot.Vars, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.vars, s.updatedAt = vars, now
}

// snapshot returns the cached variables when they're still fresh (within
// locationDataTTL of now), or nil when there's nothing recent — which resolves
// every $variable to unset.
func (s *locationStore) snapshot(now time.Time) rot.Vars {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.updatedAt.IsZero() || now.Sub(s.updatedAt) > locationDataTTL {
		return nil
	}
	return s.vars
}
