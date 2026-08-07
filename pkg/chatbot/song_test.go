package chatbot

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/adanalife/tripbot/pkg/obs/beds"
	"github.com/adanalife/tripbot/pkg/video"
)

func TestSongCmd_RendersCurrentTrack_ViaIRC(t *testing.T) {
	app := newTestApp(video.Video{})
	rec := &recordingChat{}
	app.Chat = rec
	app.Beds = &fakeBeds{bed: beds.SomaFM, artist: "Steve Cobby", title: "Big Wow"}

	app.songCmd(context.Background(), newTestUser("viewer1"), nil)

	if len(rec.Says) != 1 {
		t.Fatalf("expected exactly one Say(), got %d: %v", len(rec.Says), rec.Says)
	}
	if !strings.Contains(rec.Says[0], "Big Wow") || !strings.Contains(rec.Says[0], "Steve Cobby") {
		t.Errorf("expected title + artist in output, got %q", rec.Says[0])
	}
}

// The bed decides the answer: SomaFM's feed describes SomaFM, so consulting it
// while another bed is on air names a track nobody is hearing. TikTok boots on
// the album, so this is the common case, not the exotic one.
func TestSongCmd_OffSomaFM_ReportsTheLiveBedNotTheFeed(t *testing.T) {
	for _, tc := range []struct {
		bed   beds.Bed
		track string
		want  string
	}{
		{beds.Album, testTrack, "Colorado Sunrise"},
		{beds.CarHum, "", bedDescs[beds.CarHum]},
	} {
		t.Run(string(tc.bed), func(t *testing.T) {
			app := newTestApp(video.Video{})
			fake := &fakeBeds{bed: tc.bed, track: tc.track, artist: "Steve Cobby", title: "Big Wow"}
			app.Beds = fake
			out := captureSay(t, app)

			app.songCmd(context.Background(), newTestUser("viewer1"), nil)

			if fake.feeds != 0 {
				t.Errorf("must not consult the SomaFM feed on the %s bed", tc.bed)
			}
			got := out()
			if !strings.Contains(got, tc.want) {
				t.Errorf("expected %q in the report, got %q", tc.want, got)
			}
			if strings.Contains(got, "Big Wow") {
				t.Errorf("reported a SomaFM track while the %s bed was playing: %q", tc.bed, got)
			}
		})
	}
}

// On the SomaFM bed the feed is the only thing that knows the track, and the
// channel it came from is half the answer to "what is this".
func TestSongCmd_OnSomaFM_NamesTheTunedChannel(t *testing.T) {
	app := newTestApp(video.Video{})
	app.Beds = &fakeBeds{
		bed:     beds.SomaFM,
		station: "dronezone",
		artist:  "Steve Cobby",
		title:   "Big Wow",
	}
	out := captureSay(t, app)

	app.songCmd(context.Background(), newTestUser("viewer1"), nil)

	got := out()
	if !strings.Contains(got, "Big Wow") {
		t.Errorf("expected the SomaFM track, got %q", got)
	}
	if !strings.Contains(got, "Drone Zone") {
		t.Errorf("expected the channel named in the report, got %q", got)
	}
}

func TestSongCmd_FetchError_FallsBackToApology(t *testing.T) {
	app := newTestApp(video.Video{})
	rec := &recordingChat{}
	app.Chat = rec
	app.Beds = &fakeBeds{bed: beds.SomaFM, feedErr: errors.New("network unreachable")}

	app.songCmd(context.Background(), newTestUser("viewer1"), nil)

	if len(rec.Says) != 1 {
		t.Fatalf("expected exactly one Say(), got %d: %v", len(rec.Says), rec.Says)
	}
	if !strings.Contains(strings.ToLower(rec.Says[0]), "couldn't") {
		t.Errorf("expected apology message on fetch error, got %q", rec.Says[0])
	}
}

// Without a bed store there is no station and no feed, so the honest answer is
// that the surface isn't there — not silence, and not a guess at a channel.
func TestSongCmd_NoBedStoreSaysSo(t *testing.T) {
	app := newTestApp(video.Video{})
	app.Beds = nil
	out := captureSay(t, app)

	app.songCmd(context.Background(), newTestUser("viewer1"), nil)

	if got := out(); got == "" {
		t.Error("expected a message rather than silence")
	}
}
