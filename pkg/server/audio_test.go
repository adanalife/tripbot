package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/adanalife/tripbot/pkg/obs/beds"
	"github.com/gorilla/mux"
)

// fakeBeds is a BedStore seam for the /api/audio handlers.
type fakeBeds struct {
	bed    beds.Bed
	track  string
	setErr error
	sets   []beds.Bed
}

func (f *fakeBeds) Current() (beds.Bed, string) { return f.bed, f.track }

func (f *fakeBeds) Set(_ context.Context, bed beds.Bed) error {
	f.sets = append(f.sets, bed)
	if f.setErr != nil {
		return f.setErr
	}
	f.bed = bed
	return nil
}

func audioRouter(s *Server) *mux.Router {
	r := mux.NewRouter()
	r.Handle("/api/audio", http.HandlerFunc(s.audioHandler)).Methods("GET")
	r.Handle("/api/audio", http.HandlerFunc(s.audioSetHandler)).Methods("POST")
	return r
}

func getAudio(t *testing.T, s *Server) (int, map[string]any) {
	t.Helper()
	w := httptest.NewRecorder()
	audioRouter(s).ServeHTTP(w, httptest.NewRequest("GET", "/api/audio", nil))
	var body map[string]any
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return w.Code, body
}

func postAudio(t *testing.T, s *Server, payload string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/audio", strings.NewReader(payload))
	audioRouter(s).ServeHTTP(w, req)
	return w
}

func TestAudioHandler_ReportsBedAndOptions(t *testing.T) {
	s := &Server{beds: &fakeBeds{bed: beds.Album, track: "/opt/tripbot/assets/music/007 New York - Skyline After Midnight.mp3"}}
	code, body := getAudio(t, s)
	if code != http.StatusOK {
		t.Fatalf("status: %d", code)
	}
	if body["ok"] != true {
		t.Fatalf("ok: %v", body["ok"])
	}
	if body["bed"] != "album" {
		t.Fatalf("bed: %v", body["bed"])
	}
	// The console renders the track name as-is, so it must arrive without the
	// directory or the extension.
	if body["track"] != "007 New York - Skyline After Midnight" {
		t.Fatalf("track: %v", body["track"])
	}
	// The console builds its dropdown from this list rather than hardcoding it.
	opts, _ := body["beds"].([]any)
	if len(opts) != len(beds.All) {
		t.Fatalf("beds: want %d options, got %v", len(beds.All), body["beds"])
	}
}

func TestAudioHandler_UnavailableWithoutAStore(t *testing.T) {
	// A tripbot with no OBS pairing still answers, so the console can render
	// the panel as unavailable instead of erroring the whole page.
	code, body := getAudio(t, &Server{})
	if code != http.StatusOK {
		t.Fatalf("status: %d", code)
	}
	if body["ok"] != false {
		t.Fatalf("ok: want false, got %v", body["ok"])
	}
}

func TestAudioSetHandler_SwitchesBed(t *testing.T) {
	f := &fakeBeds{bed: beds.SomaFM}
	w := postAudio(t, &Server{beds: f}, `{"bed":"carhum"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status: %d (%s)", w.Code, w.Body)
	}
	if len(f.sets) != 1 || f.sets[0] != beds.CarHum {
		t.Fatalf("sets: %v", f.sets)
	}
	var body map[string]any
	_ = json.NewDecoder(w.Body).Decode(&body)
	// The response echoes the bed actually live now, so the console can
	// re-render from the write instead of racing a follow-up read.
	if body["bed"] != "carhum" {
		t.Fatalf("bed: %v", body["bed"])
	}
}

func TestAudioSetHandler_RejectsUnknownBed(t *testing.T) {
	f := &fakeBeds{bed: beds.SomaFM}
	w := postAudio(t, &Server{beds: f}, `{"bed":"wurlitzer"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status: want 400, got %d", w.Code)
	}
	if len(f.sets) != 0 {
		t.Fatalf("an unknown bed reached the store: %v", f.sets)
	}
}

func TestAudioSetHandler_SurfacesAFailedSwitch(t *testing.T) {
	// A switch OBS rejects (or an album with no share mounted) must be visible
	// as an error, not swallowed into a success the UI would render as done.
	f := &fakeBeds{bed: beds.SomaFM, setErr: errors.New("no album tracks")}
	w := postAudio(t, &Server{beds: f}, `{"bed":"album"}`)
	if w.Code != http.StatusBadGateway {
		t.Fatalf("status: want 502, got %d", w.Code)
	}
}

func TestAudioSetHandler_UnavailableWithoutAStore(t *testing.T) {
	w := postAudio(t, &Server{}, `{"bed":"carhum"}`)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status: want 503, got %d", w.Code)
	}
}
