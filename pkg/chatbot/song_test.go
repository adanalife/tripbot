package chatbot

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/adanalife/tripbot/pkg/obs/beds"
	"github.com/adanalife/tripbot/pkg/video"
)

// noopNowPlaying satisfies NowPlaying for tests that don't care about the
// chat-side now-playing surface — it returns a fixed track without I/O.
type noopNowPlaying struct{}

func (noopNowPlaying) Current(_ context.Context, _ string) (string, string, error) {
	return "Test Artist", "Test Title", nil
}

// recordingNowPlaying captures every Current() call and returns
// configurable values so tests can assert the chatbot called it and
// rendered the response correctly.
type recordingNowPlaying struct {
	mu       sync.Mutex
	Calls    int
	Stations []string
	Artist   string
	Title    string
	Err      error
}

func (r *recordingNowPlaying) Current(_ context.Context, station string) (string, string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Calls++
	r.Stations = append(r.Stations, station)
	return r.Artist, r.Title, r.Err
}

func TestSongCmd_RendersCurrentTrack_ViaIRC(t *testing.T) {
	app := newTestApp(video.Video{})
	rec := &recordingChat{}
	app.Chat = rec
	app.NowPlaying = &recordingNowPlaying{Artist: "Steve Cobby", Title: "Big Wow"}

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
			feed := &recordingNowPlaying{Artist: "Steve Cobby", Title: "Big Wow"}
			app.NowPlaying = feed
			app.Beds = &fakeBeds{bed: tc.bed, track: tc.track}
			out := captureSay(t, app)

			app.songCmd(context.Background(), newTestUser("viewer1"), nil)

			if feed.Calls != 0 {
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

// On the SomaFM bed the feed is the only thing that knows the track — and it
// has to be asked about the tuned channel, since each publishes its own.
func TestSongCmd_OnSomaFM_ReadsTheTunedChannelsFeed(t *testing.T) {
	app := newTestApp(video.Video{})
	feed := &recordingNowPlaying{Artist: "Steve Cobby", Title: "Big Wow"}
	app.NowPlaying = feed
	app.Beds = &fakeBeds{bed: beds.SomaFM, station: "dronezone"}
	out := captureSay(t, app)

	app.songCmd(context.Background(), newTestUser("viewer1"), nil)

	if !slices.Equal(feed.Stations, []string{"dronezone"}) {
		t.Errorf("feed asked for %v; want the tuned channel", feed.Stations)
	}
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
	app.NowPlaying = &recordingNowPlaying{Err: errors.New("network unreachable")}

	app.songCmd(context.Background(), newTestUser("viewer1"), nil)

	if len(rec.Says) != 1 {
		t.Fatalf("expected exactly one Say(), got %d: %v", len(rec.Says), rec.Says)
	}
	if !strings.Contains(strings.ToLower(rec.Says[0]), "couldn't") {
		t.Errorf("expected apology message on fetch error, got %q", rec.Says[0])
	}
}

// stubSongsURL replaces beds.SongsURL so the fetcher hits a test server. The
// station still reaches the server as a query, so tests can assert which
// channel was asked for.
func stubSongsURL(base string) func(string) string {
	return func(station string) string { return base + "/?station=" + station }
}

func TestRealNowPlaying_ParsesAndCaches(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		fmt.Fprintln(w, `{"id":"gsclassic","songs":[{"title":"Big Wow","artist":"Steve Cobby","album":"Everliving"}]}`)
	}))
	defer srv.Close()

	np := &realNowPlaying{songsURL: stubSongsURL(srv.URL), ttl: time.Minute}

	artist, title, err := np.Current(context.Background(), beds.DefaultStation)
	if err != nil {
		t.Fatalf("first Current() returned err: %v", err)
	}
	if artist != "Steve Cobby" || title != "Big Wow" {
		t.Errorf("got %q / %q; want Steve Cobby / Big Wow", artist, title)
	}

	if _, _, err := np.Current(context.Background(), beds.DefaultStation); err != nil {
		t.Fatalf("second Current() returned err: %v", err)
	}
	if hits != 1 {
		t.Errorf("expected 1 HTTP hit due to cache, got %d", hits)
	}
}

func TestRealNowPlaying_StaleFallbackOnFetchError(t *testing.T) {
	var fail bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if fail {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		fmt.Fprintln(w, `{"id":"gsclassic","songs":[{"title":"Big Wow","artist":"Steve Cobby"}]}`)
	}))
	defer srv.Close()

	np := &realNowPlaying{songsURL: stubSongsURL(srv.URL), ttl: time.Nanosecond} // force re-fetch every call

	if _, _, err := np.Current(context.Background(), beds.DefaultStation); err != nil {
		t.Fatalf("seed Current() returned err: %v", err)
	}

	fail = true
	artist, title, err := np.Current(context.Background(), beds.DefaultStation)
	if err != nil {
		t.Fatalf("expected stale fallback on fetch failure, got err: %v", err)
	}
	if artist != "Steve Cobby" || title != "Big Wow" {
		t.Errorf("expected stale values returned; got %q / %q", artist, title)
	}
}

// The cache holds one channel's answer. Retuning and getting the channel we
// just left would name a track nobody is hearing — the exact bug the bed check
// in songCmd exists to prevent, one level down.
func TestRealNowPlaying_RetuningBypassesTheCache(t *testing.T) {
	var asked []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		station := r.URL.Query().Get("station")
		asked = append(asked, station)
		fmt.Fprintf(w, `{"songs":[{"title":"on %s","artist":"Someone"}]}`, station)
	}))
	defer srv.Close()

	np := &realNowPlaying{songsURL: stubSongsURL(srv.URL), ttl: time.Minute}

	if _, _, err := np.Current(context.Background(), "gsclassic"); err != nil {
		t.Fatal(err)
	}
	_, title, err := np.Current(context.Background(), "dronezone")
	if err != nil {
		t.Fatal(err)
	}
	if title != "on dronezone" {
		t.Errorf("title = %q; want the newly tuned channel's track", title)
	}
	if want := []string{"gsclassic", "dronezone"}; !slices.Equal(asked, want) {
		t.Errorf("asked %v; want %v", asked, want)
	}
}

func TestRealNowPlaying_NoCachedValue_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	np := &realNowPlaying{songsURL: stubSongsURL(srv.URL), ttl: time.Minute}

	if _, _, err := np.Current(context.Background(), beds.DefaultStation); err == nil {
		t.Error("expected error when no cached value and fetch fails")
	}
}
