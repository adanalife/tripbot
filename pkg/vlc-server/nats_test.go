package vlcServer

import (
	"encoding/json"
	"testing"

	ve "github.com/adanalife/tripbot/pkg/vlc-events"
	"github.com/nats-io/nats.go"
)

// The observe-only handlers only decode + log, so these tests assert the
// decode path tolerates both well-formed and malformed payloads without
// panicking. They don't touch libvlc (a zero Server is enough — the handlers
// read no server state), which keeps them honest about being side-effect-free.

func TestNATSHandlersDecodeValidPayloads(t *testing.T) {
	s := &Server{}

	skip, _ := json.Marshal(ve.Skip{Envelope: ve.NewEnvelope(), N: 3})
	back, _ := json.Marshal(ve.Back{Envelope: ve.NewEnvelope(), N: 2})
	file, _ := json.Marshal(ve.PlayFile{Envelope: ve.NewEnvelope(), File: "x.mp4"})
	cmd, _ := json.Marshal(ve.Command{Envelope: ve.NewEnvelope()})

	// None of these should panic or act on playback.
	s.handleSkip(&nats.Msg{Subject: ve.SkipSubject("test"), Data: skip})
	s.handleBack(&nats.Msg{Subject: ve.BackSubject("test"), Data: back})
	s.handlePlayFile(&nats.Msg{Subject: ve.PlayFileSubject("test"), Data: file})
	s.handlePlayRandom(&nats.Msg{Subject: ve.PlayRandomSubject("test"), Data: cmd})
}

func TestNATSHandlersTolerateMalformedPayloads(t *testing.T) {
	s := &Server{}
	bad := []byte("{not json")

	// Malformed bodies are logged and dropped, not panicked on.
	s.handleSkip(&nats.Msg{Subject: ve.SkipSubject("test"), Data: bad})
	s.handleBack(&nats.Msg{Subject: ve.BackSubject("test"), Data: bad})
	s.handlePlayFile(&nats.Msg{Subject: ve.PlayFileSubject("test"), Data: bad})
}
