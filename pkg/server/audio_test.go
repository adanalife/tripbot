package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/adanalife/tripbot/pkg/obs/beds"
	"github.com/gorilla/mux"
)

// The radio bed's station list is mounted at runtime; the tests tune this one.
func init() {
	beds.Stations = []beds.Station{{ID: "lofi", Name: "Example Lo-Fi", URL: "https://radio.example/lofi"}}
}

// fakeBeds is a BedStore seam for the /api/audio handlers.
type fakeBeds struct {
	bed beds.Bed
	// fallback is the bed the watchdog has on air while bed stays selected; ""
	// when the selection is what's playing.
	fallback     beds.Bed
	track        string
	station      string
	voicing      string
	voicings     []string // SetVoicing calls, in order
	artist       string
	title        string
	feedErr      error
	setErr       error
	sets         []beds.Bed
	stations     []string
	playingAlbum string
	feeds        int
	pending      *beds.Switch // a switch waiting out the delay; nil = none
}

func (f *fakeBeds) Current() (beds.Bed, string) { return f.bed, f.track }

func (f *fakeBeds) Playing() (beds.Bed, string) {
	if f.fallback != "" {
		return f.fallback, f.track
	}
	return f.bed, f.track
}

func (f *fakeBeds) RadioTrack(context.Context) (string, string, error) {
	f.feeds++
	return f.artist, f.title, f.feedErr
}

// Pending models the real store's switch delay: nil is a store applying switches
// inline (the tests below drive the effects directly), and a test that cares
// about the waiting state sets it.
func (f *fakeBeds) Pending() (beds.Switch, bool) {
	if f.pending == nil {
		return beds.Switch{}, false
	}
	return *f.pending, true
}

func (f *fakeBeds) Station() string {
	if f.station == "" {
		return beds.DefaultStation()
	}
	return f.station
}

func (f *fakeBeds) Voicing() string {
	if f.voicing == "" {
		return beds.DefaultVoicing
	}
	return f.voicing
}

func (f *fakeBeds) SetVoicing(_ context.Context, voicing string) error {
	f.voicings = append(f.voicings, voicing)
	if f.setErr != nil {
		return f.setErr
	}
	f.bed, f.voicing = beds.CarHum, voicing
	return nil
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
	f.bed, f.station = beds.Radio, station
	return nil
}

func (f *fakeBeds) PlayingAlbum() string { return f.playingAlbum }

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
	s := &Server{beds: &fakeBeds{bed: beds.Radio, fallback: beds.Album, track: "/opt/tripbot/assets/music/007 New York - Skyline After Midnight.mp3"}}
	code, body := getAudio(t, s)
	if code != http.StatusOK {
		t.Fatalf("status: %d", code)
	}
	if body["ok"] != true {
		t.Fatalf("ok: %v", body["ok"])
	}
	if body["bed"] != "radio" {
		t.Fatalf("bed: %v", body["bed"])
	}
	// The album is the radio's fallback, never a selection, so nothing about
	// choosing one ships.
	for _, k := range []string{"album", "albums", "groups", "shuffle"} {
		if _, ok := body[k]; ok {
			t.Errorf("%s shipped: %v", k, body[k])
		}
	}
	// The console renders the track name as-is, so it must arrive without the
	// directory, the extension, or the track number.
	if body["track"] != "New York - Skyline After Midnight" {
		t.Fatalf("track: %v", body["track"])
	}
	// The console builds its dropdown from this list rather than hardcoding it.
	opts, _ := body["beds"].([]any)
	if len(opts) != len(beds.All) {
		t.Fatalf("beds: want %d options, got %v", len(beds.All), body["beds"])
	}
	// Same for the radio station picker: the lineup lives here, not in the
	// console, so the two can't disagree about what's selectable.
	stations, _ := body["stations"].([]any)
	if len(stations) != len(beds.Stations) {
		t.Fatalf("stations: want %d, got %d", len(beds.Stations), len(stations))
	}
	if body["station"] != beds.DefaultStation() {
		t.Fatalf("station: %v", body["station"])
	}
}

// On the radio bed the track comes from the feed, so the console renders one
// line from one source whichever bed is live instead of fetching the radio itself.
func TestAudioHandler_ReportsTheRadioTrack(t *testing.T) {
	f := &fakeBeds{bed: beds.Radio, artist: "Steve Cobby", title: "Big Wow"}
	_, body := getAudio(t, &Server{beds: f})
	if body["track"] != "Steve Cobby — Big Wow" {
		t.Fatalf("track: %v", body["track"])
	}
	if f.feeds != 1 {
		t.Fatalf("feed reads: %d, want 1", f.feeds)
	}
}

// A feed that won't answer is a missing line, not an error: the panel still has
// to render the bed, and the next poll picks the track up.
func TestAudioHandler_RadioFeedFailureLeavesNoTrack(t *testing.T) {
	f := &fakeBeds{bed: beds.Radio, feedErr: errors.New("radio unreachable")}
	code, body := getAudio(t, &Server{beds: f})
	if code != http.StatusOK {
		t.Fatalf("status: %d", code)
	}
	if body["track"] != "" || body["bed"] != "radio" {
		t.Fatalf("want the bed with no track, got %v", body)
	}
}

// The local beds have no feed to consult — asking one would name a song nobody
// is hearing.
func TestAudioHandler_DoesNotConsultTheFeedOffRadio(t *testing.T) {
	f := &fakeBeds{bed: beds.CarHum, artist: "Steve Cobby", title: "Big Wow"}
	_, body := getAudio(t, &Server{beds: f})
	if f.feeds != 0 {
		t.Fatalf("feed read on the %s bed", f.bed)
	}
	if body["track"] != "" {
		t.Fatalf("track: %v", body["track"])
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
	f := &fakeBeds{bed: beds.Radio}
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

// The console posts a station on its own; that has to select the radio bed too,
// or picking a channel from the car-hum bed would change nothing audible.
func TestAudioSetHandler_TunesAStation(t *testing.T) {
	f := &fakeBeds{bed: beds.CarHum}
	w := postAudio(t, &Server{beds: f}, `{"station":"lofi"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status: %d (%s)", w.Code, w.Body)
	}
	if len(f.sets) != 0 {
		t.Fatalf("a station must not go through Set: %v", f.sets)
	}
	if len(f.stations) != 1 || f.stations[0] != "lofi" {
		t.Fatalf("stations: %v", f.stations)
	}
	var body map[string]any
	_ = json.NewDecoder(w.Body).Decode(&body)
	if body["bed"] != "radio" || body["station"] != "lofi" {
		t.Fatalf("response: %v", body)
	}
}

func TestAudioSetHandler_RejectsUnknownStation(t *testing.T) {
	f := &fakeBeds{bed: beds.Radio}
	w := postAudio(t, &Server{beds: f}, `{"station":"wurlitzer-fm"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status: want 400, got %d", w.Code)
	}
	if len(f.stations) != 0 {
		t.Fatalf("an unknown station reached the store: %v", f.stations)
	}
}

func TestAudioSetHandler_RejectsUnknownBed(t *testing.T) {
	f := &fakeBeds{bed: beds.Radio}
	w := postAudio(t, &Server{beds: f}, `{"bed":"wurlitzer"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status: want 400, got %d", w.Code)
	}
	if len(f.sets) != 0 {
		t.Fatalf("an unknown bed reached the store: %v", f.sets)
	}
}

func TestAudioSetHandler_SurfacesAFailedSwitch(t *testing.T) {
	// A switch OBS rejects must be visible as an error, not swallowed into a
	// success the UI would render as done.
	f := &fakeBeds{bed: beds.Radio, setErr: errors.New("obs unreachable")}
	w := postAudio(t, &Server{beds: f}, `{"bed":"carhum"}`)
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

// The console needs the waiting switch as its own field: bed/station/album still
// describe the audio, so without this a click renders as no change at all.
func TestAudioHandler_ShipsTheWaitingSwitch(t *testing.T) {
	f := &fakeBeds{bed: beds.CarHum, pending: &beds.Switch{
		Bed:     beds.Radio,
		Station: "lofi",
		At:      time.Now().Add(5 * time.Second),
	}}
	_, body := getAudio(t, &Server{beds: f})

	if body["bed"] != "carhum" {
		t.Errorf("the live bed must still be the car hum, got %v", body["bed"])
	}
	pending, ok := body["pending"].(map[string]any)
	if !ok {
		t.Fatalf("pending: %v", body["pending"])
	}
	if pending["bed"] != "radio" || pending["station"] != "lofi" {
		t.Errorf("pending = %v", pending)
	}
	// A countdown, so it has to arrive as a number of seconds left rather than a
	// deadline the console would have to trust its own clock against.
	if in, _ := pending["in_seconds"].(float64); in < 1 || in > 5 {
		t.Errorf("in_seconds = %v, want the remaining wait", pending["in_seconds"])
	}
}

// Nothing waiting means no field at all, so the console's countdown markup can
// key off its presence.
func TestAudioHandler_OmitsPendingWhenNothingIsWaiting(t *testing.T) {
	_, body := getAudio(t, &Server{beds: &fakeBeds{bed: beds.CarHum}})
	if _, ok := body["pending"]; ok {
		t.Errorf("pending shipped with nothing waiting: %v", body["pending"])
	}
}

// Mid-outage the watchdog has the album on air with the radio still selected. The
// picker still has to highlight the selection, but the now-playing line must
// come off the audio: reading the selection here fetches the station's metadata and
// prints a track nobody is hearing.
func TestAudioHandler_OnFallback_ReportsTheAudioNotTheSelection(t *testing.T) {
	f := &fakeBeds{
		bed:          beds.Radio,
		fallback:     beds.Album,
		track:        "/opt/tripbot/assets/music/fifty-horizons/007 New York - Skyline After Midnight.mp3",
		playingAlbum: "fifty-horizons",
		artist:       "Steve Cobby",
		title:        "Big Wow",
	}
	_, body := getAudio(t, &Server{beds: f})
	if body["bed"] != "radio" {
		t.Fatalf("bed: want the selection radio, got %v", body["bed"])
	}
	if body["on_fallback"] != true {
		t.Fatalf("on_fallback: %v", body["on_fallback"])
	}
	if body["track"] != "New York - Skyline After Midnight" {
		t.Fatalf("track: %v", body["track"])
	}
	if f.feeds != 0 {
		t.Fatal("must not consult the station metadata while another bed is on air")
	}
}

func TestAudioSetHandler_SelectsAVoicing(t *testing.T) {
	f := &fakeBeds{bed: beds.Radio}
	w := postAudio(t, &Server{beds: f}, `{"voicing":"highway"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status: %d, body %q", w.Code, w.Body.String())
	}
	if len(f.voicings) != 1 || f.voicings[0] != "highway" {
		t.Fatalf("expected one selection of highway, got %v", f.voicings)
	}
	if len(f.sets) != 0 {
		t.Errorf("a voicing selects its own bed, no separate switch: %v", f.sets)
	}
}

// A voicing the paired OBS image never rendered is a path nothing can open, so
// it stops at the handler rather than reaching OBS and going silent.
func TestAudioSetHandler_UnknownVoicingIs400(t *testing.T) {
	f := &fakeBeds{bed: beds.CarHum}
	w := postAudio(t, &Server{beds: f}, `{"voicing":"spaceship"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status: want 400, got %d (%q)", w.Code, w.Body.String())
	}
	if len(f.voicings) != 0 {
		t.Errorf("an unknown voicing must not reach the store, got %v", f.voicings)
	}
}

func TestAudioHandler_ReportsTheVoicingAndItsOptions(t *testing.T) {
	_, body := getAudio(t, &Server{beds: &fakeBeds{bed: beds.CarHum, voicing: "mountain"}})
	if body["voicing"] != "mountain" {
		t.Errorf("voicing: want mountain, got %v", body["voicing"])
	}
	// The picker is built from what this tripbot will accept, so the option
	// list has to travel with the state the way the stations do.
	got, _ := body["voicings"].([]any)
	if len(got) != len(beds.Voicings) {
		t.Fatalf("voicings: want %d options, got %v", len(beds.Voicings), body["voicings"])
	}
	for i, want := range beds.Voicings {
		if got[i] != want {
			t.Errorf("voicings[%d]: want %s, got %v", i, want, got[i])
		}
	}
}

// A private station ships in the list with its flag, for the clients that list
// it, but the console can't select it as the stream's bed.
func TestAudio_PrivateStationListedButRefused(t *testing.T) {
	prev := beds.Stations
	beds.Stations = append([]beds.Station{{ID: "home", Name: "Home", URL: "https://radio.example/home", Private: true}}, prev...)
	t.Cleanup(func() { beds.Stations = prev })

	f := &fakeBeds{bed: beds.Radio}
	_, body := getAudio(t, &Server{beds: f})
	stations, _ := body["stations"].([]any)
	first, _ := stations[0].(map[string]any)
	if len(stations) != 2 || first["id"] != "home" || first["private"] != true {
		t.Fatalf("stations: %v; want home listed first with private=true", body["stations"])
	}

	w := postAudio(t, &Server{beds: f}, `{"station":"home"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status: want 400, got %d", w.Code)
	}
	if len(f.stations) != 0 {
		t.Fatalf("a private station reached the store: %v", f.stations)
	}
}

// The album is the radio's fallback, not a bed anyone selects.
func TestAudioSetHandler_AlbumIsNotABed(t *testing.T) {
	f := &fakeBeds{bed: beds.Radio}
	w := postAudio(t, &Server{beds: f}, `{"bed":"album"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status: want 400, got %d", w.Code)
	}
	if len(f.sets) != 0 {
		t.Fatalf("the album reached the store: %v", f.sets)
	}
}
