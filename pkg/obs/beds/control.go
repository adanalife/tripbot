package beds

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// controlPort is the radio pod's JSON control API, on the same host as the
// stream. Anyone else's station is listen-only.
const controlPort = "8001"

const controlHTTPTimeout = 5 * time.Second

// ErrNoRequests is the answer for a station that isn't ours: only the fleet's
// own Liquidsoap airs a request queue.
var ErrNoRequests = errors.New("station takes no requests")

// controlTarget is (control API base, mount) for a station of ours, ok false
// for a third party's.
func controlTarget(streamURL string) (base, mount string, ok bool) {
	u, err := url.Parse(streamURL)
	if err != nil {
		return "", "", false
	}
	host := u.Hostname()
	if !strings.HasPrefix(host, "radio.") || !strings.HasSuffix(host, ".svc.cluster.local") {
		return "", "", false
	}
	return "http://" + host + ":" + controlPort, strings.TrimPrefix(u.Path, "/"), true
}

// Tracks lists the files the tuned station plays — the only things it can be
// asked for. ErrNoRequests for a station that isn't ours.
func (s *Store) Tracks(ctx context.Context) ([]string, error) {
	base, mount, ok := controlTarget(StreamURL(s.Station()))
	if !ok {
		return nil, ErrNoRequests
	}
	return fetchTracks(ctx, base, mount)
}

// Push queues file to play next on the tuned station. file must be one Tracks
// listed; the station refuses anything else.
func (s *Store) Push(ctx context.Context, file string) error {
	base, mount, ok := controlTarget(StreamURL(s.Station()))
	if !ok {
		return ErrNoRequests
	}
	return pushTrack(ctx, base, mount, file)
}

func fetchTracks(ctx context.Context, base, mount string) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, controlHTTPTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/stations/"+mount+"/tracks", nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("radio control returned status %d", resp.StatusCode)
	}
	var entries []struct {
		File string `json:"file"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, fmt.Errorf("parse radio tracks: %w", err)
	}
	files := make([]string, 0, len(entries))
	for _, e := range entries {
		files = append(files, e.File)
	}
	return files, nil
}

func pushTrack(ctx context.Context, base, mount, file string) error {
	ctx, cancel := context.WithTimeout(ctx, controlHTTPTimeout)
	defer cancel()
	body, _ := json.Marshal(map[string]string{"file": file})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/stations/"+mount+"/push", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("radio control returned status %d", resp.StatusCode)
	}
	return nil
}
