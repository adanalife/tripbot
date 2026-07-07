package onscreensEvents

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
		{"middle.show", MiddleShowSubject("staging", "twitch"), "tripbot.staging.onscreens.middle.show.twitch"},
		{"middle.hide", MiddleHideSubject("staging", "twitch"), "tripbot.staging.onscreens.middle.hide.twitch"},
		{"middle.state", MiddleStateSubject("staging", "youtube"), "tripbot.staging.onscreens.middle.state.youtube"},
		{"middle.state.wildcard", MiddleStateWildcard("staging"), "tripbot.staging.onscreens.middle.state.*"},
		{"leaderboard.show", LeaderboardShowSubject("prod", "twitch"), "tripbot.prod.onscreens.leaderboard.show.twitch"},
		{"leaderboard.hide", LeaderboardHideSubject("prod", "twitch"), "tripbot.prod.onscreens.leaderboard.hide.twitch"},
		{"timewarp.show", TimewarpShowSubject("development", "twitch"), "tripbot.development.onscreens.timewarp.show.twitch"},
		{"timewarp.hide", TimewarpHideSubject("development", "twitch"), "tripbot.development.onscreens.timewarp.hide.twitch"},
		{"gps.show", GPSShowSubject("staging", "twitch"), "tripbot.staging.onscreens.gps.show.twitch"},
		{"gps.hide", GPSHideSubject("staging", "twitch"), "tripbot.staging.onscreens.gps.hide.twitch"},
		{"location.update", LocationUpdateSubject("prod", "youtube"), "tripbot.prod.onscreens.location.update.youtube"},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, tc.got, tc.want)
		}
	}
}

func TestMiddleShowRoundTrip(t *testing.T) {
	in := MiddleShow{Envelope: NewEnvelope(), Msg: "hello"}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out MiddleShow
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Msg != in.Msg {
		t.Errorf("msg: got %q, want %q", out.Msg, in.Msg)
	}
	if out.EmittedAt == "" {
		t.Error("emitted_at empty after round-trip")
	}
}

func TestLeaderboardShowRoundTrip(t *testing.T) {
	in := LeaderboardShow{
		Envelope: NewEnvelope(),
		Title:    "Monthly Miles",
		Rows:     [][]string{{"alice", "42"}, {"bob", "17"}},
	}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out LeaderboardShow
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Title != in.Title {
		t.Errorf("title: got %q, want %q", out.Title, in.Title)
	}
	if len(out.Rows) != 2 || out.Rows[0][0] != "alice" || out.Rows[1][1] != "17" {
		t.Errorf("rows round-trip mismatch: %v", out.Rows)
	}
}

func TestNewEnvelopeStampsTime(t *testing.T) {
	if NewEnvelope().EmittedAt == "" {
		t.Error("NewEnvelope did not stamp EmittedAt")
	}
}
