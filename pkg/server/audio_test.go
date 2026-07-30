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
	bed      beds.Bed
	track    string
	station  string
	setErr   error
	sets     []beds.Bed
	stations []string
}

func (f *fakeBeds) Current() (beds.Bed, string) { return f.bed, f.track }

func (f *fakeBeds) Station() string {
	if f.station == "" {
		return beds.DefaultStation
	}
	return f.station
}

func (f *fakeBeds) Set(_ context.Context, bed beds.Bed) error {
	f.sets = append(f.sets, bed)
	if f.setErr != nil {
		return f.setErr
	}
	f.bed = bed
	return nil
}

func (f *fakeBeds) SetStation(_ context.Context, station string) error {
	f.stations = append(f.stations, station)
	if f.setErr != nil {
		return f.setErr
	}
	f.bed, f.station = beds.SomaFM, station
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
	// Same for the SomaFM channel picker: the lineup lives here, not in the
	// console, so the two can't disagree about what's selectable.
	stations, _ := body["stations"].([]any)
	if len(stations) != len(beds.Stations) {
		t.Fatalf("stations: want %d, got %d", len(beds.Stations), len(stations))
	}
	if body["station"] != beds.DefaultStation {
		t.Fatalf("station: %v", body["station"])
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

// The console posts a station on its own; that has to select the SomaFM bed too,
// or picking a channel from the car-hum bed would change nothing audible.
func TestAudioSetHandler_TunesAStation(t *testing.T) {
	f := &fakeBeds{bed: beds.CarHum}
	w := postAudio(t, &Server{beds: f}, `{"station":"dronezone"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status: %d (%s)", w.Code, w.Body)
	}
	if len(f.sets) != 0 {
		t.Fatalf("a station must not go through Set: %v", f.sets)
	}
	if len(f.stations) != 1 || f.stations[0] != "dronezone" {
		t.Fatalf("stations: %v", f.stations)
	}
	var body map[string]any
	_ = json.NewDecoder(w.Body).Decode(&body)
	if body["bed"] != "somafm" || body["station"] != "dronezone" {
		t.Fatalf("response: %v", body)
	}
}

func TestAudioSetHandler_RejectsUnknownStation(t *testing.T) {
	f := &fakeBeds{bed: beds.SomaFM}
	w := postAudio(t, &Server{beds: f}, `{"station":"wurlitzer-fm"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status: want 400, got %d", w.Code)
	}
	if len(f.stations) != 0 {
		t.Fatalf("an unknown station reached the store: %v", f.stations)
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
