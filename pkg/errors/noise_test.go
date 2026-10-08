package errors

import (
	"testing"

	"github.com/getsentry/sentry-go"
)

// The cases are the exception values of live prod issues, so a pattern that
// stops matching what the cluster actually emits fails here rather than in
// Sentry three weeks later.
func TestClassifyNoise(t *testing.T) {
	cases := []struct {
		name, value, want string
	}{
		{"gateway pod restarting", `gateway request: Get "http://gateway-twitch.prod-1.svc.cluster.local:8080/v1/chat/inbound": dial tcp 10.100.127.157:8080: connect: connection refused`, noisePeerUnavailable},
		{"postgres cycling", "dial tcp 10.97.92.89:5432: connect: connection refused", noisePeerUnavailable},
		{"twitch dropped the websocket", "could not read message: failed to get reader: failed to read frame header: read tcp 10.244.0.40:38790->18.161.21.121:443: read: connection reset by peer", noisePeerUnavailable},
		{"nats hung up", "nats: unexpected EOF", noisePeerUnavailable},
		{"facebook broadcast never answered", `gateway request: Get "http://gateway-facebook.prod-1.svc.cluster.local:8080/v1/broadcast": context deadline exceeded (Client.Timeout exceeded while awaiting headers)`, noiseTimeout},
		{"dns unavailable", "[Errno -3] Temporary failure in name resolution", noiseDNS},
		{"gateway's own upstream failed", "gateway /v1/broadcast: unexpected status 502", noiseUpstream},

		{"a real defect stays untagged", `pq: duplicate key value violates unique constraint "users_platform_user_id_key"`, ""},
		{"an unauthorized token is a defect, not churn", `could not subscribe to event: 401 Unauthorized: {"error":"Unauthorized"}`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ev := &sentry.Event{Exception: []sentry.Exception{{Value: tc.value}}}
			if got := classifyNoise(ev); got != tc.want {
				t.Errorf("classifyNoise(%q) = %q, want %q", tc.value, got, tc.want)
			}
		})
	}
}

// A reset is a dead peer, not a slow one — the looser timeout patterns must not
// claim an error the peer-unavailable list already answers.
func TestClassifyNoisePrefersTheMoreSpecificPattern(t *testing.T) {
	ev := &sentry.Event{
		Message:   "gateway inbound poll failed",
		Exception: []sentry.Exception{{Value: "read: connection reset by peer (i/o timeout on retry)"}},
	}
	if got := classifyNoise(ev); got != noisePeerUnavailable {
		t.Errorf("classifyNoise = %q, want %q", got, noisePeerUnavailable)
	}
}

func TestTagNoise(t *testing.T) {
	t.Run("stamps the class", func(t *testing.T) {
		ev := &sentry.Event{Exception: []sentry.Exception{{Value: "dial tcp: connect: connection refused"}}}
		tagNoise(ev)
		if ev.Tags[noiseTagKey] != noisePeerUnavailable {
			t.Errorf("tag = %q, want %q", ev.Tags[noiseTagKey], noisePeerUnavailable)
		}
	})

	t.Run("leaves a defect untagged", func(t *testing.T) {
		ev := &sentry.Event{Message: "couldn't record platform user id"}
		tagNoise(ev)
		if _, ok := ev.Tags[noiseTagKey]; ok {
			t.Errorf("tag = %q, want none", ev.Tags[noiseTagKey])
		}
	})

	t.Run("keeps a caller's own classification", func(t *testing.T) {
		ev := &sentry.Event{
			Tags:      map[string]string{noiseTagKey: "deliberate"},
			Exception: []sentry.Exception{{Value: "connect: connection refused"}},
		}
		tagNoise(ev)
		if ev.Tags[noiseTagKey] != "deliberate" {
			t.Errorf("tag = %q, want it left alone", ev.Tags[noiseTagKey])
		}
	})

	t.Run("nil event does not panic", func(t *testing.T) { tagNoise(nil) })
}

// The tag has to survive the path events actually take, not just the helper.
func TestThrottleTagsSurvivingEvents(t *testing.T) {
	hook := throttle(fakeConfig{prod: true})
	ev := hook(&sentry.Event{
		Message:   "gateway inbound poll failed",
		Exception: []sentry.Exception{{Value: "dial tcp 10.100.127.157:8080: connect: connection refused"}},
	}, nil)
	if ev == nil {
		t.Fatal("prod should send")
	}
	if ev.Tags[noiseTagKey] != noisePeerUnavailable {
		t.Errorf("tag = %q, want %q", ev.Tags[noiseTagKey], noisePeerUnavailable)
	}
}
