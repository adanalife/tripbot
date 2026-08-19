package main

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/adanalife/tripbot/pkg/video"
)

// A run that never resolved a clip must not name a file: Video.File() formats
// the slug into "<slug>.MP4", so a zero-value Video reports ".MP4" rather than
// nothing at all. Shutdown logs are what gets read after a crashloop, so the
// two cases have to be distinguishable.
func TestLogLastPlayed(t *testing.T) {
	for _, tc := range []struct {
		name       string
		slug       string
		wantSubstr string
		notSubstr  string
	}{
		{"nothing played", "", "no video played this run", "last played video"},
		{"a clip played", "2018_04_02_12_00_00", `last played video`, "no video played"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))

			tb := &Tripbot{player: &video.Player{CurrentlyPlaying: video.Video{Slug: tc.slug}}}
			tb.logLastPlayed()

			got := buf.String()
			if !strings.Contains(got, tc.wantSubstr) {
				t.Errorf("log %q missing %q", got, tc.wantSubstr)
			}
			if strings.Contains(got, tc.notSubstr) {
				t.Errorf("log %q should not contain %q", got, tc.notSubstr)
			}
			if tc.slug != "" && !strings.Contains(got, `file=`+tc.slug+`.MP4`) {
				t.Errorf("log %q missing file attribute", got)
			}
		})
	}
}
