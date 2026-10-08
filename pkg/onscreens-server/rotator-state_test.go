package onscreensServer

import (
	"encoding/json"
	"testing"

	oe "github.com/adanalife/tripbot/pkg/onscreens-events"
	rot "github.com/adanalife/tripbot/pkg/rotator"
	"github.com/nats-io/nats.go"
)

// rotatorTestServer builds a Server with just the two corners wired — enough to
// exercise the copy-application path without binding sockets or NATS.
func rotatorTestServer(platform string, inbound bool) *Server {
	cfg := rotatorConf(platform, inbound)
	return &Server{cfg: cfg, left: leftRotator(cfg), right: rightRotator(cfg)}
}

// TestHandleRotatorConfigAppliesPayload is the console-save path end to end:
// tripbot's published payload decodes and replaces both corners' pools.
func TestHandleRotatorConfigAppliesPayload(t *testing.T) {
	s := rotatorTestServer(platformTwitch, true)
	payload, err := json.Marshal(oe.RotatorConfig{
		Envelope: oe.NewEnvelope(),
		Left: rot.Corner{
			Messages:      []rot.Message{{Text: "left edited", Weight: 3}},
			PromoMessages: []rot.Message{{Text: "left promo"}},
		},
		Right:       rot.Corner{Messages: []rot.Message{{Text: "right edited"}}},
		RareMessage: "rare edited",
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	s.handleRotatorConfig(&nats.Msg{Subject: "test", Data: payload})

	left := s.left.copy.Load()
	if len(left.messages) != 1 || left.messages[0].Text != "left edited" {
		t.Errorf("left messages = %+v, want the single edited line", left.messages)
	}
	if left.messages[0].Weight != 3 {
		t.Errorf("left weight = %d, want 3 (weights must survive the wire)", left.messages[0].Weight)
	}
	if len(left.promoMessages) != 1 || left.promoMessages[0].Text != "left promo" {
		t.Errorf("left promo = %+v, want the single promo line", left.promoMessages)
	}
	if left.rareMessage != "rare edited" {
		t.Errorf("left rare = %q, want the edited rare line", left.rareMessage)
	}
	if got := s.right.copy.Load().messages[0].Text; got != "right edited" {
		t.Errorf("right messages = %q, want right edited", got)
	}
}

// A malformed body must leave the live pools alone rather than blanking the
// overlays — a bad publish is a publisher bug, not an intent to show nothing.
func TestHandleRotatorConfigIgnoresGarbage(t *testing.T) {
	s := rotatorTestServer(platformTwitch, true)
	before := s.left.copy.Load()

	s.handleRotatorConfig(&nats.Msg{Subject: "test", Data: []byte("not json")})

	if s.left.copy.Load() != before {
		t.Error("malformed payload replaced the live copy")
	}
}

// TestHandleRotatorConfigAcceptsEmptyPools covers a deliberate "show nothing in
// this corner" edit: it applies, and rendering degrades to an empty string
// rather than panicking on an empty pool.
func TestHandleRotatorConfigAcceptsEmptyPools(t *testing.T) {
	s := rotatorTestServer(platformTwitch, true)
	payload, err := json.Marshal(oe.RotatorConfig{Envelope: oe.NewEnvelope()})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	s.handleRotatorConfig(&nats.Msg{Subject: "test", Data: payload})

	if got := len(s.left.copy.Load().messages); got != 0 {
		t.Errorf("left messages = %d, want 0 after an empty edit", got)
	}
	if got := s.left.content(); got != "" {
		t.Errorf("content() = %q, want empty for an empty pool", got)
	}
}

// The restore path is best-effort: with no NATS configured (nil JetStream) it
// must leave the compiled-in copy in place instead of failing the boot.
func TestRestoreRotatorCopyWithoutNATS(t *testing.T) {
	s := rotatorTestServer(platformTwitch, true)
	before := s.left.copy.Load()

	s.RestoreRotatorCopy(t.Context())

	if s.left.copy.Load() != before {
		t.Error("restore without NATS should leave the compiled-in copy alone")
	}
}
