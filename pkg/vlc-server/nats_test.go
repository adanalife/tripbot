package vlcServer

import (
	"testing"

	ve "github.com/adanalife/tripbot/pkg/vlc-events"
	"github.com/nats-io/nats.go"
)

// The handlers act on playback now (they drive libvlc), so the valid-payload
// path can't be unit-tested against a zero Server. What stays testable is the
// decode-error guard: a malformed body is logged and dropped before any
// playback call, so it's safe on a zero Server and proves bad input never
// reaches libvlc. play.random has no decode step, so it's exercised in the
// live burn-in, not here.

func TestNATSHandlersTolerateMalformedPayloads(t *testing.T) {
	s := &Server{}
	bad := []byte("{not json")

	// Malformed bodies are logged and dropped, not panicked on, and never
	// reach s.skip / s.back / s.PlayVideoFile.
	s.handleSkip(&nats.Msg{Subject: ve.SkipSubject("test", "twitch"), Data: bad})
	s.handleBack(&nats.Msg{Subject: ve.BackSubject("test", "twitch"), Data: bad})
	s.handlePlayFile(&nats.Msg{Subject: ve.PlayFileSubject("test", "twitch"), Data: bad})
	s.handlePlayFileAt(&nats.Msg{Subject: ve.PlayFileAtSubject("test", "twitch"), Data: bad})
}

// handlePlayFile / handlePlayFileAt with an empty filename is a publisher bug —
// dropped before the PlayVideoFile call (so safe on a zero Server).
func TestPlayFileEmptyFilenameDropped(t *testing.T) {
	s := &Server{}
	empty := []byte(`{"emitted_at":"2026-01-01T00:00:00Z","file":""}`)
	s.handlePlayFile(&nats.Msg{Subject: ve.PlayFileSubject("test", "twitch"), Data: empty})
	s.handlePlayFileAt(&nats.Msg{Subject: ve.PlayFileAtSubject("test", "twitch"), Data: empty})
}
