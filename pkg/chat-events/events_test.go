package chatEvents

import (
	"encoding/json"
	"testing"
)

func TestSendSubject(t *testing.T) {
	cases := []struct {
		env      string
		platform string
		want     string
	}{
		{"staging", PlatformTwitch, "tripbot.staging.chat.send.twitch"},
		{"prod", PlatformTwitch, "tripbot.prod.chat.send.twitch"},
		{"prod", PlatformYouTube, "tripbot.prod.chat.send.youtube"},
		{"development", PlatformTwitch, "tripbot.development.chat.send.twitch"},
	}
	for _, tc := range cases {
		if got := SendSubject(tc.env, tc.platform); got != tc.want {
			t.Errorf("SendSubject(%q, %q): got %q, want %q", tc.env, tc.platform, got, tc.want)
		}
	}
}

func TestSendRoundTrip(t *testing.T) {
	in := Send{Envelope: NewEnvelope(), Identity: IdentityBroadcaster, Text: "hello chat"}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out Send
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Identity != IdentityBroadcaster {
		t.Errorf("identity: got %q, want %q", out.Identity, IdentityBroadcaster)
	}
	if out.Text != in.Text {
		t.Errorf("text: got %q, want %q", out.Text, in.Text)
	}
	if out.EmittedAt == "" {
		t.Error("emitted_at empty after round-trip")
	}
}

func TestNewEnvelopeStampsTime(t *testing.T) {
	if NewEnvelope().EmittedAt == "" {
		t.Error("NewEnvelope did not stamp EmittedAt")
	}
}
