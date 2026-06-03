package chatbot

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/adanalife/tripbot/pkg/video"
)

// noopNowPlaying satisfies NowPlaying for tests that don't care about the
// chat-side now-playing surface — it returns a fixed track without I/O.
type noopNowPlaying struct{}

func (noopNowPlaying) Current(_ context.Context) (string, string, error) {
	return "Test Artist", "Test Title", nil
}

// recordingNowPlaying captures every Current() call and returns
// configurable values so tests can assert the chatbot called it and
// rendered the response correctly.
type recordingNowPlaying struct {
	mu     sync.Mutex
	Calls  int
	Artist string
	Title  string
	Err    error
}

func (r *recordingNowPlaying) Current(_ context.Context) (string, string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Calls++
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

func TestRealNowPlaying_ParsesAndCaches(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		fmt.Fprintln(w, `{"id":"gsclassic","songs":[{"title":"Big Wow","artist":"Steve Cobby","album":"Everliving"}]}`)
	}))
	defer srv.Close()

	np := &realNowPlaying{url: srv.URL, ttl: time.Minute}

	artist, title, err := np.Current(context.Background())
	if err != nil {
		t.Fatalf("first Current() returned err: %v", err)
	}
	if artist != "Steve Cobby" || title != "Big Wow" {
		t.Errorf("got %q / %q; want Steve Cobby / Big Wow", artist, title)
	}

	if _, _, err := np.Current(context.Background()); err != nil {
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

	np := &realNowPlaying{url: srv.URL, ttl: time.Nanosecond} // force re-fetch every call

	if _, _, err := np.Current(context.Background()); err != nil {
		t.Fatalf("seed Current() returned err: %v", err)
	}

	fail = true
	artist, title, err := np.Current(context.Background())
	if err != nil {
		t.Fatalf("expected stale fallback on fetch failure, got err: %v", err)
	}
	if artist != "Steve Cobby" || title != "Big Wow" {
		t.Errorf("expected stale values returned; got %q / %q", artist, title)
	}
}

func TestRealNowPlaying_NoCachedValue_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	np := &realNowPlaying{url: srv.URL, ttl: time.Minute}

	if _, _, err := np.Current(context.Background()); err == nil {
		t.Error("expected error when no cached value and fetch fails")
	}
}
