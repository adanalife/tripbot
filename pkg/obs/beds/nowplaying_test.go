package beds

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"
)

// stubSongsURL replaces SongsURL so the fetcher hits a test server. The station
// still reaches the server as a query, so tests can assert which channel was
// asked for.
func stubSongsURL(base string) func(string) string {
	return func(station string) string { return base + "/?station=" + station }
}

func TestNowPlaying_ParsesAndCaches(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		fmt.Fprintln(w, `{"id":"gsclassic","songs":[{"title":"Big Wow","artist":"Steve Cobby","album":"Everliving"}]}`)
	}))
	defer srv.Close()

	np := &nowPlaying{songsURL: stubSongsURL(srv.URL), ttl: time.Minute}

	artist, title, err := np.current(context.Background(), DefaultStation)
	if err != nil {
		t.Fatalf("first current() returned err: %v", err)
	}
	if artist != "Steve Cobby" || title != "Big Wow" {
		t.Errorf("got %q / %q; want Steve Cobby / Big Wow", artist, title)
	}

	if _, _, err := np.current(context.Background(), DefaultStation); err != nil {
		t.Fatalf("second current() returned err: %v", err)
	}
	if hits != 1 {
		t.Errorf("expected 1 HTTP hit due to cache, got %d", hits)
	}
}

func TestNowPlaying_StaleFallbackOnFetchError(t *testing.T) {
	var fail bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if fail {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		fmt.Fprintln(w, `{"id":"gsclassic","songs":[{"title":"Big Wow","artist":"Steve Cobby"}]}`)
	}))
	defer srv.Close()

	np := &nowPlaying{songsURL: stubSongsURL(srv.URL), ttl: time.Nanosecond} // force re-fetch every call

	if _, _, err := np.current(context.Background(), DefaultStation); err != nil {
		t.Fatalf("seed current() returned err: %v", err)
	}

	fail = true
	artist, title, err := np.current(context.Background(), DefaultStation)
	if err != nil {
		t.Fatalf("expected stale fallback on fetch failure, got err: %v", err)
	}
	if artist != "Steve Cobby" || title != "Big Wow" {
		t.Errorf("expected stale values returned; got %q / %q", artist, title)
	}
}

// The cache holds one channel's answer. Retuning and getting the channel we just
// left would name a song nobody is hearing — the same failure the bed check
// prevents one level up.
func TestNowPlaying_RetuningBypassesTheCache(t *testing.T) {
	var asked []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		station := r.URL.Query().Get("station")
		asked = append(asked, station)
		fmt.Fprintf(w, `{"songs":[{"title":"on %s","artist":"Someone"}]}`, station)
	}))
	defer srv.Close()

	np := &nowPlaying{songsURL: stubSongsURL(srv.URL), ttl: time.Minute}

	if _, _, err := np.current(context.Background(), "gsclassic"); err != nil {
		t.Fatal(err)
	}
	_, title, err := np.current(context.Background(), "dronezone")
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

func TestNowPlaying_NoCachedValue_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	np := &nowPlaying{songsURL: stubSongsURL(srv.URL), ttl: time.Minute}

	if _, _, err := np.current(context.Background(), DefaultStation); err == nil {
		t.Error("expected error when no cached value and fetch fails")
	}
}

// The store asks about the channel it has tuned, not the default — a retune has
// to move the answer with it.
func TestStore_SomaFMTrackAsksTheTunedChannel(t *testing.T) {
	var asked []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = append(asked, r.URL.Query().Get("station"))
		fmt.Fprintln(w, `{"songs":[{"title":"Big Wow","artist":"Steve Cobby"}]}`)
	}))
	defer srv.Close()

	s := NewStore(&fakeOBS{}, SomaFM, t.TempDir(), "twitch")
	s.np = &nowPlaying{songsURL: stubSongsURL(srv.URL), ttl: time.Minute}
	if err := s.SetStation(context.Background(), "dronezone"); err != nil {
		t.Fatal(err)
	}

	artist, title, err := s.SomaFMTrack(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if artist != "Steve Cobby" || title != "Big Wow" {
		t.Errorf("got %q / %q", artist, title)
	}
	if !slices.Equal(asked, []string{"dronezone"}) {
		t.Errorf("asked %v; want the tuned channel", asked)
	}
}
