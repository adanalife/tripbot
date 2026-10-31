package beds

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	nowPlayingCacheTTL    = 30 * time.Second
	nowPlayingHTTPTimeout = 5 * time.Second
)

// nowPlaying fetches and caches the current song on a radio station. The audio
// arrives as a bare stream, so the station's in-band ICY metadata is the only
// thing that knows what is playing — which is why it lives beside the station
// rather than in whichever surface asks.
//
// The cache serves every asker from one fetch per nowPlayingCacheTTL (chat spam
// and the console's poll both land here) and falls back to the last known answer
// when a fetch fails. It holds ONE station's answer: a retune must not be
// answered with the station we just left, so a different station is a miss.
type nowPlaying struct {
	streamURL func(station string) string // StreamURL; tests point it at a stub
	ttl       time.Duration

	mu        sync.Mutex
	station   string
	artist    string
	title     string
	fetchedAt time.Time
}

func newNowPlaying() *nowPlaying {
	return &nowPlaying{streamURL: StreamURL, ttl: nowPlayingCacheTTL}
}

func (n *nowPlaying) current(ctx context.Context, station string) (string, string, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	cached := !n.fetchedAt.IsZero() && n.station == station
	if cached && time.Since(n.fetchedAt) < n.ttl {
		return n.artist, n.title, nil
	}

	artist, title, err := fetchICYTitle(ctx, n.streamURL(station))
	if err != nil {
		if cached {
			return n.artist, n.title, nil
		}
		return "", "", err
	}
	n.station, n.artist, n.title, n.fetchedAt = station, artist, title, time.Now()
	return artist, title, nil
}

// fetchICYTitle reads the first in-band metadata block off an Icecast/Shoutcast
// stream: asked for with Icy-MetaData, the server interleaves a metadata block
// after every icy-metaint audio bytes, its length one byte ×16.
//
// shortcut: Go's HTTP client rejects a bare "ICY 200 OK" status line, so a
// legacy Shoutcast v1 server can't answer; wrap the conn if a station needs it.
func fetchICYTitle(ctx context.Context, url string) (string, string, error) {
	if url == "" {
		return "", "", errors.New("radio station has no stream url")
	}
	ctx, cancel := context.WithTimeout(ctx, nowPlayingHTTPTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Icy-MetaData", "1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("radio stream returned status %d", resp.StatusCode)
	}
	metaint, err := strconv.Atoi(resp.Header.Get("icy-metaint"))
	if err != nil || metaint <= 0 {
		return "", "", errors.New("radio stream sends no icy-metaint")
	}

	body := bufio.NewReader(resp.Body)
	if _, err := io.CopyN(io.Discard, body, int64(metaint)); err != nil {
		return "", "", fmt.Errorf("read radio audio: %w", err)
	}
	n, err := body.ReadByte()
	if err != nil {
		return "", "", fmt.Errorf("read radio metadata length: %w", err)
	}
	meta := make([]byte, int(n)*16)
	if _, err := io.ReadFull(body, meta); err != nil {
		return "", "", fmt.Errorf("read radio metadata: %w", err)
	}
	return parseStreamTitle(string(meta))
}

// parseStreamTitle pulls StreamTitle='…'; out of an ICY metadata block and
// splits it into artist and title on the first " - ", the convention stations
// follow; a title without one is all title.
func parseStreamTitle(meta string) (string, string, error) {
	const key = "StreamTitle='"
	_, rest, ok := strings.Cut(meta, key)
	if !ok {
		return "", "", errors.New("radio metadata has no StreamTitle")
	}
	// The value can itself hold a quote, so it ends at the quote before ';'.
	streamTitle, _, ok := strings.Cut(rest, "';")
	if !ok {
		streamTitle = strings.TrimRight(rest, "\x00'")
	}
	streamTitle = strings.TrimSpace(streamTitle)
	if streamTitle == "" {
		return "", "", errors.New("radio metadata has an empty StreamTitle")
	}
	if artist, title, ok := strings.Cut(streamTitle, " - "); ok {
		return strings.TrimSpace(artist), strings.TrimSpace(title), nil
	}
	return "", streamTitle, nil
}
