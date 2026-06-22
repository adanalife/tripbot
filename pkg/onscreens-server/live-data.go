package onscreensServer

import (
	"sync"
	"time"
)

// locationDataTTL bounds how long a received location/date stays eligible for
// the rotators. tripbot republishes on a ~60s timer, so a value older than this
// means tripbot stopped publishing (the feature's off, the bot crashed, or this
// isn't a bot-less youtube pipeline) — the rotators then fall back to their
// static lines rather than showing stale info.
const locationDataTTL = 5 * time.Minute

// liveLocation caches the most recent location/date pushed from tripbot over
// location.update. The rotator content funcs read it to surface the info the
// !location / !date commands would return on a bot-less YouTube stream. Package
// scope because the rotator loops + content funcs are package-level; access is
// mutex-guarded.
var liveLocation = &locationStore{}

type locationStore struct {
	mu        sync.RWMutex
	location  string
	date      string
	updatedAt time.Time
}

// set caches the latest location/date with the time it arrived.
func (s *locationStore) set(location, date string, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.location, s.date, s.updatedAt = location, date, now
}

// snapshot returns the cached location/date when it's still fresh (within
// locationDataTTL of now); ok is false when there's nothing recent to show.
func (s *locationStore) snapshot(now time.Time) (location, date string, ok bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.updatedAt.IsZero() || now.Sub(s.updatedAt) > locationDataTTL {
		return "", "", false
	}
	return s.location, s.date, true
}
