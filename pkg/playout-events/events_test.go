package playoutEvents

import (
	"encoding/json"
	"testing"
)

func TestSubjects(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"play.random", PlayRandomSubject("staging", "twitch"), "tripbot.staging.vlc.play.random.twitch"},
		{"play.file", PlayFileSubject("prod", "youtube"), "tripbot.prod.vlc.play.file.youtube"},
		{"play.at", PlayFileAtSubject("prod", "twitch"), "tripbot.prod.vlc.play.at.twitch"},
		{"skip", SkipSubject("development", "youtube"), "tripbot.development.vlc.skip.youtube"},
		{"back", BackSubject("staging", "twitch"), "tripbot.staging.vlc.back.twitch"},
		{"seek", SeekSubject("production", "twitch"), "tripbot.production.vlc.seek.twitch"},
		{"lastplayed", LastPlayedSubject("prod", "twitch"), "tripbot.prod.vlc.lastplayed.twitch"},
		{"lastplayed wildcard", LastPlayedWildcard("prod"), "tripbot.prod.vlc.lastplayed.*"},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, tc.got, tc.want)
		}
	}
}

func TestSkipRoundTrip(t *testing.T) {
	in := Skip{Envelope: NewEnvelope(), N: 5}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out Skip
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.N != in.N {
		t.Errorf("n: got %d, want %d", out.N, in.N)
	}
	if out.EmittedAt == "" {
		t.Error("emitted_at empty after round-trip")
	}
}

func TestSeekRoundTrip(t *testing.T) {
	in := Seek{Envelope: NewEnvelope(), DeltaMs: -600_000}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out Seek
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.DeltaMs != in.DeltaMs {
		t.Errorf("delta_ms: got %d, want %d", out.DeltaMs, in.DeltaMs)
	}
	if out.EmittedAt == "" {
		t.Error("emitted_at empty after round-trip")
	}
}

func TestPlayFileRoundTrip(t *testing.T) {
	in := PlayFile{Envelope: NewEnvelope(), File: "2020-01-02-1234.mp4"}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out PlayFile
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.File != in.File {
		t.Errorf("file: got %q, want %q", out.File, in.File)
	}
}

func TestPlayFileAtRoundTrip(t *testing.T) {
	in := PlayFileAt{Envelope: NewEnvelope(), File: "2020-01-02-1234.mp4", PositionMs: 163_000}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out PlayFileAt
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.File != in.File {
		t.Errorf("file: got %q, want %q", out.File, in.File)
	}
	if out.PositionMs != in.PositionMs {
		t.Errorf("position_ms: got %d, want %d", out.PositionMs, in.PositionMs)
	}
}

func TestLastPlayedRoundTrip(t *testing.T) {
	in := LastPlayed{Envelope: NewEnvelope(), File: "2020-01-02-1234.mp4", PositionMs: 42_500}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out LastPlayed
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.File != in.File {
		t.Errorf("file: got %q, want %q", out.File, in.File)
	}
	if out.PositionMs != in.PositionMs {
		t.Errorf("position_ms: got %d, want %d", out.PositionMs, in.PositionMs)
	}
}

func TestLastPlayedPositionlessDecodesToClipStart(t *testing.T) {
	// Messages published before position_ms existed must decode as 0 —
	// start-of-clip — not error.
	var out LastPlayed
	if err := json.Unmarshal([]byte(`{"emitted_at":"2026-01-01T00:00:00Z","file":"a.MP4"}`), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.PositionMs != 0 {
		t.Errorf("position_ms: got %d, want 0", out.PositionMs)
	}
}

func TestNewEnvelopeStampsTime(t *testing.T) {
	if NewEnvelope().EmittedAt == "" {
		t.Error("NewEnvelope did not stamp EmittedAt")
	}
}
