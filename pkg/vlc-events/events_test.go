package vlcEvents

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
		{"play.random", PlayRandomSubject("staging"), "tripbot.staging.vlc.play.random"},
		{"play.file", PlayFileSubject("prod"), "tripbot.prod.vlc.play.file"},
		{"skip", SkipSubject("development"), "tripbot.development.vlc.skip"},
		{"back", BackSubject("staging"), "tripbot.staging.vlc.back"},
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

func TestNewEnvelopeStampsTime(t *testing.T) {
	if NewEnvelope().EmittedAt == "" {
		t.Error("NewEnvelope did not stamp EmittedAt")
	}
}
