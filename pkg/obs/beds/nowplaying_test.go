package beds

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// stubStreamURL replaces StreamURL so the fetcher hits a test server. The
// station still reaches the server as the path, so tests can assert which
// station was asked for.
func stubStreamURL(base string) func(string) string {
	return func(station string) string { return base + "/" + station }
}

// icyMetaint is the audio-block size the stub stations advertise.
const icyMetaint = 64

// writeICY serves one metaint stream: a block of audio bytes, then a metadata
// block carrying streamTitle, padded to a multiple of 16 the way Icecast pads.
func writeICY(t *testing.T, w http.ResponseWriter, r *http.Request, streamTitle string) {
	t.Helper()
	if r.Header.Get("Icy-MetaData") != "1" {
		t.Errorf("request missing Icy-MetaData: 1")
	}
	meta := []byte("StreamTitle='" + streamTitle + "';StreamUrl='';")
	blocks := (len(meta) + 15) / 16
	meta = append(meta, make([]byte, blocks*16-len(meta))...)

	w.Header().Set("icy-metaint", strconv.Itoa(icyMetaint))
	w.Header().Set("Content-Type", "audio/mpeg")
	var buf bytes.Buffer
	buf.Write(bytes.Repeat([]byte{0xff}, icyMetaint))
	buf.WriteByte(byte(blocks))
	buf.Write(meta)
	buf.Write(bytes.Repeat([]byte{0xff}, icyMetaint)) // more audio follows
	_, _ = w.Write(buf.Bytes())
}

func TestNowPlaying_ParsesAndCaches(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		writeICY(t, w, r, "Steve Cobby - Big Wow")
	}))
	defer srv.Close()

	np := &nowPlaying{streamURL: stubStreamURL(srv.URL), ttl: time.Minute}

	artist, title, err := np.current(context.Background(), DefaultStation())
	if err != nil {
		t.Fatalf("first current() returned err: %v", err)
	}
	if artist != "Steve Cobby" || title != "Big Wow" {
		t.Errorf("got %q / %q; want Steve Cobby / Big Wow", artist, title)
	}

	if _, _, err := np.current(context.Background(), DefaultStation()); err != nil {
		t.Fatalf("second current() returned err: %v", err)
	}
	if hits != 1 {
		t.Errorf("expected 1 HTTP hit due to cache, got %d", hits)
	}
}

func TestNowPlaying_StaleFallbackOnFetchError(t *testing.T) {
	var fail bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		writeICY(t, w, r, "Steve Cobby - Big Wow")
	}))
	defer srv.Close()

	np := &nowPlaying{streamURL: stubStreamURL(srv.URL), ttl: time.Nanosecond} // force re-fetch every call

	if _, _, err := np.current(context.Background(), DefaultStation()); err != nil {
		t.Fatalf("seed current() returned err: %v", err)
	}

	fail = true
	artist, title, err := np.current(context.Background(), DefaultStation())
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
		station := strings.TrimPrefix(r.URL.Path, "/")
		asked = append(asked, station)
		writeICY(t, w, r, "Someone - on "+station)
	}))
	defer srv.Close()

	np := &nowPlaying{streamURL: stubStreamURL(srv.URL), ttl: time.Minute}

	if _, _, err := np.current(context.Background(), "ambient"); err != nil {
		t.Fatal(err)
	}
	_, title, err := np.current(context.Background(), "lofi")
	if err != nil {
		t.Fatal(err)
	}
	if title != "on lofi" {
		t.Errorf("title = %q; want the newly tuned channel's track", title)
	}
	if want := []string{"ambient", "lofi"}; !slices.Equal(asked, want) {
		t.Errorf("asked %v; want %v", asked, want)
	}
}

func TestNowPlaying_NoCachedValue_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	np := &nowPlaying{streamURL: stubStreamURL(srv.URL), ttl: time.Minute}

	if _, _, err := np.current(context.Background(), DefaultStation()); err == nil {
		t.Error("expected error when no cached value and fetch fails")
	}
}

// The store asks about the channel it has tuned, not the default — a retune has
// to move the answer with it.
func TestStore_RadioTrackAsksTheTunedChannel(t *testing.T) {
	var asked []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = append(asked, strings.TrimPrefix(r.URL.Path, "/"))
		writeICY(t, w, r, "Steve Cobby - Big Wow")
	}))
	defer srv.Close()

	s := NewStore(&fakeOBS{}, Radio, t.TempDir(), "twitch")
	s.np = &nowPlaying{streamURL: stubStreamURL(srv.URL), ttl: time.Minute}
	if err := s.SetStation(context.Background(), "lofi"); err != nil {
		t.Fatal(err)
	}

	artist, title, err := s.RadioTrack(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if artist != "Steve Cobby" || title != "Big Wow" {
		t.Errorf("got %q / %q", artist, title)
	}
	if !slices.Equal(asked, []string{"lofi"}) {
		t.Errorf("asked %v; want the tuned channel", asked)
	}
}

// A stream that doesn't interleave metadata can't name a song; that's an error,
// not an empty answer.
func TestNowPlaying_NoMetaintIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(bytes.Repeat([]byte{0xff}, 256))
	}))
	defer srv.Close()

	np := &nowPlaying{streamURL: stubStreamURL(srv.URL), ttl: time.Minute}
	if _, _, err := np.current(context.Background(), "any"); err == nil {
		t.Error("expected an error from a stream with no icy-metaint")
	}
}

func TestParseStreamTitle(t *testing.T) {
	cases := []struct {
		meta, artist, title string
		ok                  bool
	}{
		{"StreamTitle='Steve Cobby - Big Wow';", "Steve Cobby", "Big Wow", true},
		{"StreamTitle='A - B - C';StreamUrl='';\x00\x00", "A", "B - C", true},
		{"StreamTitle='Station ID';", "", "Station ID", true},
		{"StreamTitle='Rock 'n' Roll - It's Fine';", "Rock 'n' Roll", "It's Fine", true},
		{"StreamTitle='';", "", "", false},
		{"StreamUrl='x';", "", "", false},
	}
	for _, c := range cases {
		artist, title, err := parseStreamTitle(c.meta)
		if (err == nil) != c.ok || artist != c.artist || title != c.title {
			t.Errorf("parseStreamTitle(%q) = %q, %q, %v; want %q, %q, ok=%v",
				c.meta, artist, title, err, c.artist, c.title, c.ok)
		}
	}
}
