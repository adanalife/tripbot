package chatbot

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/adanalife/tripbot/pkg/obs/beds"
	"github.com/adanalife/tripbot/pkg/users"
)

// NowPlaying reports the currently-playing track on the stream's background
// audio source. Tests inject a fake; production uses realNowPlaying which
// polls SomaFM's per-channel now-playing JSON feed.
type NowPlaying interface {
	Current(ctx context.Context) (artist, title string, err error)
}

const (
	somaFMNowPlayingURL   = "https://somafm.com/songs/gsclassic.json"
	nowPlayingCacheTTL    = 30 * time.Second
	nowPlayingHTTPTimeout = 5 * time.Second
)

// realNowPlaying fetches and caches the current song from SomaFM. The cache
// avoids hammering SomaFM (one fetch per nowPlayingCacheTTL even under chat
// spam) and provides a stale-fallback if a fetch fails.
type realNowPlaying struct {
	url string
	ttl time.Duration

	mu        sync.Mutex
	artist    string
	title     string
	fetchedAt time.Time
}

func newRealNowPlaying() *realNowPlaying {
	return &realNowPlaying{url: somaFMNowPlayingURL, ttl: nowPlayingCacheTTL}
}

func (r *realNowPlaying) Current(ctx context.Context) (string, string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.fetchedAt.IsZero() && time.Since(r.fetchedAt) < r.ttl {
		return r.artist, r.title, nil
	}

	artist, title, err := fetchSomaFMCurrent(ctx, r.url)
	if err != nil {
		if !r.fetchedAt.IsZero() {
			return r.artist, r.title, nil
		}
		return "", "", err
	}
	r.artist, r.title, r.fetchedAt = artist, title, time.Now()
	return artist, title, nil
}

type somaFMResponse struct {
	Songs []struct {
		Artist string `json:"artist"`
		Title  string `json:"title"`
	} `json:"songs"`
}

func fetchSomaFMCurrent(ctx context.Context, url string) (string, string, error) {
	ctx, cancel := context.WithTimeout(ctx, nowPlayingHTTPTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("somafm returned status %d", resp.StatusCode)
	}

	var parsed somaFMResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", "", err
	}
	if len(parsed.Songs) == 0 {
		return "", "", fmt.Errorf("somafm returned no songs")
	}
	return parsed.Songs[0].Artist, parsed.Songs[0].Title, nil
}

// songCmd answers "what is this music". Which answer is right depends on the
// live background-audio bed: only SomaFM has an external now-playing feed, so
// reading that feed while the album or the car hum is on air names a track
// nobody is hearing. The console's now-playing line asks tripbot for the same
// reason.
//
// With no bed store wired (an instance with no OBS pairing) the SomaFM feed is
// the only answer available, so it stands.
func (a *App) songCmd(ctx context.Context, user *users.User, _ []string) {
	slog.InfoContext(ctx, "ran !song", "username", user.Username)

	if a.Beds != nil {
		if bed, track := a.Beds.Current(); bed != beds.SomaFM {
			a.Chat.Say("♪ Now playing: " + describeBed(bed, track))
			return
		}
	}

	artist, title, err := a.NowPlaying.Current(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "now-playing fetch failed", "err", err)
		a.Chat.Say("Couldn't reach the music source for the current track, sorry!")
		return
	}
	a.Chat.Say(fmt.Sprintf("♪ Now playing: %s — %s", title, artist))
}
