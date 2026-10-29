package beds

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestControlTarget(t *testing.T) {
	for _, tc := range []struct {
		url, base, mount string
		ok               bool
	}{
		{"http://radio.prod-1-supporting.svc.cluster.local:8000/house", "http://radio.prod-1-supporting.svc.cluster.local:8001", "house", true},
		{"http://radio.stage-1-supporting.svc.cluster.local:8000/lofi", "http://radio.stage-1-supporting.svc.cluster.local:8001", "lofi", true},
		{"https://ice.somafm.com/groovesalad", "", "", false},
		{"http://radio.example.com/house", "", "", false},
		{"", "", "", false},
	} {
		base, mount, ok := controlTarget(tc.url)
		if base != tc.base || mount != tc.mount || ok != tc.ok {
			t.Errorf("controlTarget(%q) = (%q, %q, %v), want (%q, %q, %v)", tc.url, base, mount, ok, tc.base, tc.mount, tc.ok)
		}
	}
}

func TestFetchTracksAndPushTrack(t *testing.T) {
	var pushed string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /stations/house/tracks":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"file": "/music/a/One.mp3", "artist": "", "title": "", "requested": false},
				{"file": "/music/b/Two.flac", "artist": "", "title": "", "requested": false},
			})
		case "POST /stations/house/push":
			var body struct{ File string }
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &body)
			if body.File != "/music/b/Two.flac" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			pushed = body.File
			_ = json.NewEncoder(w).Encode(map[string]int{"rid": 7})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	ctx := context.Background()
	files, err := fetchTracks(ctx, srv.URL, "house")
	if err != nil {
		t.Fatalf("fetchTracks: %v", err)
	}
	if len(files) != 2 || files[0] != "/music/a/One.mp3" || files[1] != "/music/b/Two.flac" {
		t.Errorf("fetchTracks = %v", files)
	}
	if err := pushTrack(ctx, srv.URL, "house", "/music/b/Two.flac"); err != nil || pushed != "/music/b/Two.flac" {
		t.Errorf("pushTrack: err %v, pushed %q", err, pushed)
	}
	if err := pushTrack(ctx, srv.URL, "house", "/music/nope.mp3"); err == nil {
		t.Error("pushTrack of a file the station refuses should error")
	}
	if _, err := fetchTracks(ctx, srv.URL, "nope"); err == nil {
		t.Error("fetchTracks on an unknown mount should error")
	}
}

func TestStoreTracks_ThirdPartyStationTakesNoRequests(t *testing.T) {
	prev := Stations
	t.Cleanup(func() { Stations = prev })
	Stations = []Station{{ID: "soma", Name: "SomaFM", URL: "https://ice.somafm.com/groovesalad"}}
	s := &Store{station: "soma"}
	if _, err := s.Tracks(context.Background()); !errors.Is(err, ErrNoRequests) {
		t.Errorf("Tracks err = %v, want ErrNoRequests", err)
	}
	if err := s.Push(context.Background(), "/x.mp3"); !errors.Is(err, ErrNoRequests) {
		t.Errorf("Push err = %v, want ErrNoRequests", err)
	}
}
