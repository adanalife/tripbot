package eventsub

import (
	"context"
	"strings"
	"testing"
)

func TestRun_RejectsEmptyConfig(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
	}{
		{"empty everything", Config{}},
		{"missing ClientID", Config{BroadcasterToken: "t", BroadcasterUserID: "u"}},
		{"missing BroadcasterToken", Config{ClientID: "c", BroadcasterUserID: "u"}},
		{"missing BroadcasterUserID", Config{ClientID: "c", BroadcasterToken: "t"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := Run(context.Background(), tc.cfg, Handlers{})
			if err == nil {
				t.Fatal("Run with incomplete Config should return error; got nil")
			}
			if !strings.Contains(err.Error(), "Config") {
				t.Errorf("err message %q should mention Config", err.Error())
			}
		})
	}
}
