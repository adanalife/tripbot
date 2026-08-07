package beds

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

const (
	nowPlayingCacheTTL    = 30 * time.Second
	nowPlayingHTTPTimeout = 5 * time.Second
)

// nowPlaying fetches and caches the current song on a SomaFM channel. SomaFM's
// audio arrives as a bare stream, so this feed is the only thing that knows what
// is playing — which is why it lives beside the station rather than in whichever
// surface asks.
//
// The cache serves every asker from one fetch per nowPlayingCacheTTL (chat spam
// and the console's poll both land here) and falls back to the last known answer
// when a fetch fails. It holds ONE channel's answer: a retune must not be
// answered with the channel we just left, so a different station is a miss.
type nowPlaying struct {
	songsURL func(station string) string // SongsURL; tests point it at a stub
	ttl      time.Duration

	mu        sync.Mutex
	station   string
	artist    string
	title     string
	fetchedAt time.Time
}

func newNowPlaying() *nowPlaying {
	return &nowPlaying{songsURL: SongsURL, ttl: nowPlayingCacheTTL}
}

func (n *nowPlaying) current(ctx context.Context, station string) (string, string, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	cached := !n.fetchedAt.IsZero() && n.station == station
	if cached && time.Since(n.fetchedAt) < n.ttl {
		return n.artist, n.title, nil
	}

	artist, title, err := fetchSomaFMCurrent(ctx, n.songsURL(station))
	if err != nil {
		if cached {
			return n.artist, n.title, nil
		}
		return "", "", err
	}
	n.station, n.artist, n.title, n.fetchedAt = station, artist, title, time.Now()
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
