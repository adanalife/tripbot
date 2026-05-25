package server

import (
	"context"
	"encoding/json"
	"net/http"
)

// somaFMNowPlayingURL is the per-channel now-playing JSON feed for the
// Groove Salad Classic stream that runs as the OBS background audio
// source. Duplicates the URL constant from pkg/chatbot/song.go on
// purpose — the cleanest consolidation would be a shared pkg/song
// package, but that's a separate refactor; the duplication is two
// lines + a 4-field struct.
const somaFMNowPlayingURL = "https://somafm.com/songs/gsclassic.json"

// nowPlayingTrack carries the current SomaFM track for the admin
// template. Both fields empty when the fetch fails or returns nothing.
type nowPlayingTrack struct {
	Artist string
	Title  string
}

// fetchNowPlaying GETs the SomaFM JSON feed and returns the current
// track. Best-effort: any failure (timeout, non-2xx, empty list) yields
// a zero track + nil error so the template just omits the audio line.
// Uses the existing 2s healthClient — same pattern as the sibling
// /health probes; the page never blocks on SomaFM being slow.
func fetchNowPlaying(ctx context.Context) nowPlayingTrack {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, somaFMNowPlayingURL, nil)
	if err != nil {
		return nowPlayingTrack{}
	}
	resp, err := healthClient.Do(req)
	if err != nil {
		return nowPlayingTrack{}
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nowPlayingTrack{}
	}
	var body struct {
		Songs []struct {
			Artist string `json:"artist"`
			Title  string `json:"title"`
		} `json:"songs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nowPlayingTrack{}
	}
	if len(body.Songs) == 0 {
		return nowPlayingTrack{}
	}
	return nowPlayingTrack{Artist: body.Songs[0].Artist, Title: body.Songs[0].Title}
}

// nowPlayingFetcher is overridable in tests so we don't hit SomaFM
// from a unit test. Default fetches live.
var nowPlayingFetcher = fetchNowPlaying
