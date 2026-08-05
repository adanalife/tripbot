package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/adanalife/tripbot/pkg/obs/beds"
	"github.com/gorilla/mux"
)

// fakeBeds is a BedStore seam for the /api/audio handlers.
type fakeBeds struct {
	bed          beds.Bed
	track        string
	station      string
	artist       string
	title        string
	feedErr      error
	setErr       error
	sets         []beds.Bed
	stations     []string
	albums       []string // SetAlbum calls, in order
	album        string
	playingAlbum string
	groups       []string
	shuffle      bool
	shuffles     []bool   // SetShuffle calls, in order
	onShare      []string // what Albums() reports; nil means the fan album alone
	feeds        int
	pending      *beds.Switch // a switch waiting out the delay; nil = none
}

func (f *fakeBeds) Current() (beds.Bed, string) { return f.bed, f.track }

func (f *fakeBeds) SomaFMTrack(context.Context) (string, string, error) {
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

func (f *fakeBeds) Album() string { return f.album }

func (f *fakeBeds) PlayingAlbum() string { return f.playingAlbum }

func (f *fakeBeds) Groups() []string { return f.groups }

func (f *fakeBeds) Shuffle() bool { return f.shuffle }

func (f *fakeBeds) SetShuffle(_ context.Context, on bool) error {
	f.shuffles = append(f.shuffles, on)
	if f.setErr != nil {
		return f.setErr
	}
	f.shuffle = on
	return nil
}

func (f *fakeBeds) Albums() []string {
	if f.onShare == nil {
		return []string{"fifty-horizons"}
	}
	return f.onShare
}

func (f *fakeBeds) ValidAlbum(album string) bool {
	if album == "" {
		return false
	}
	if slices.Contains(f.Albums(), album) {
		return true
	}
	for _, a := range f.Albums() {
		if strings.HasPrefix(a, album+"-") {
			return true // a group prefix is a valid selection
		}
	}
	return false
}

func (f *fakeBeds) SetAlbum(_ context.Context, album string) error {
	f.albums = append(f.albums, album)
	if f.setErr != nil {
		return f.setErr
	}
	f.bed, f.album = beds.Album, album
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
	// directory, the extension, or the track number.
	if body["track"] != "New York - Skyline After Midnight" {
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

// The album picker is built from the share, so the list has to travel with the
// state the same way the station lineup does — and be read live, since music
// lands on the NAS without a deploy.
func TestAudioHandler_ReportsTheAlbumAndTheShare(t *testing.T) {
	f := &fakeBeds{
		bed:     beds.Album,
		album:   "lofi-secluded",
		onShare: []string{"fifty-horizons", "ambient-diamonds", "lofi-secluded"},
	}
	_, body := getAudio(t, &Server{beds: f})
	if body["album"] != "lofi-secluded" {
		t.Fatalf("album: %v", body["album"])
	}
	albums, _ := body["albums"].([]any)
	if len(albums) != 3 {
		t.Fatalf("albums: want 3, got %v", body["albums"])
	}
}

func TestAudioSetHandler_SelectsAnAlbum(t *testing.T) {
	f := &fakeBeds{bed: beds.CarHum, onShare: []string{"fifty-horizons", "lofi-secluded"}}
	w := postAudio(t, &Server{beds: f}, `{"album":"lofi-secluded"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status: %d, body %q", w.Code, w.Body.String())
	}
	if len(f.albums) != 1 || f.albums[0] != "lofi-secluded" {
		t.Fatalf("expected one selection of lofi-secluded, got %v", f.albums)
	}
	if len(f.sets) != 0 {
		t.Errorf("an album selects its own bed, no separate switch: %v", f.sets)
	}
}

// A stale console tab POSTing an album that has left the share is the client's
// mistake (400), not the share failing (502) — the two want different UI.
func TestAudioSetHandler_UnknownAlbumIs400(t *testing.T) {
	f := &fakeBeds{bed: beds.CarHum, onShare: []string{"fifty-horizons"}}
	w := postAudio(t, &Server{beds: f}, `{"album":"edm-nocturnal"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status: want 400, got %d (%q)", w.Code, w.Body.String())
	}
	if len(f.albums) != 0 {
		t.Errorf("an unknown album must not reach the store, got %v", f.albums)
	}
}

// "" is a real album selection — the whole share, shuffled together — so it has
// to reach the store rather than falling through to the bed branch and 400ing.
// It's what the console's blank option posts.
func TestAudioSetHandler_EmptyAlbumWidensToTheWholeShare(t *testing.T) {
	f := &fakeBeds{bed: beds.Album, album: "lofi-secluded"}
	w := postAudio(t, &Server{beds: f}, `{"album":""}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d (%q)", w.Code, w.Body.String())
	}
	if len(f.albums) != 1 || f.albums[0] != "" {
		t.Fatalf("expected one widening selection, got %v", f.albums)
	}
}

// Omitting the field entirely is a bed switch, not a widening — otherwise every
// {"bed": ...} POST would silently reset the selected album.
func TestAudioSetHandler_AbsentAlbumIsNotAWidening(t *testing.T) {
	f := &fakeBeds{bed: beds.Album, album: "lofi-secluded"}
	w := postAudio(t, &Server{beds: f}, `{"bed":"carhum"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status: %d (%q)", w.Code, w.Body.String())
	}
	if len(f.albums) != 0 {
		t.Fatalf("a bed switch must not touch the album, got %v", f.albums)
	}
	if len(f.sets) != 1 || f.sets[0] != beds.CarHum {
		t.Fatalf("expected one bed switch to carhum, got %v", f.sets)
	}
}

// A share that goes away mid-switch is a 502: the name was real, applying it
// wasn't possible, and the previous bed keeps playing.
func TestAudioSetHandler_AlbumSwitchFailureIs502(t *testing.T) {
	f := &fakeBeds{
		bed:     beds.CarHum,
		onShare: []string{"lofi-secluded"},
		setErr:  errors.New("no album tracks under /opt/tripbot/assets/music/lofi-secluded"),
	}
	w := postAudio(t, &Server{beds: f}, `{"album":"lofi-secluded"}`)
	if w.Code != http.StatusBadGateway {
		t.Fatalf("status: want 502, got %d (%q)", w.Code, w.Body.String())
	}
}

// On the SomaFM bed the track comes from the feed, so the console renders one
// line from one source whichever bed is live instead of fetching SomaFM itself.
func TestAudioHandler_ReportsTheSomaFMTrack(t *testing.T) {
	f := &fakeBeds{bed: beds.SomaFM, artist: "Steve Cobby", title: "Big Wow"}
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
func TestAudioHandler_SomaFMFeedFailureLeavesNoTrack(t *testing.T) {
	f := &fakeBeds{bed: beds.SomaFM, feedErr: errors.New("somafm unreachable")}
	code, body := getAudio(t, &Server{beds: f})
	if code != http.StatusOK {
		t.Fatalf("status: %d", code)
	}
	if body["track"] != "" || body["bed"] != "somafm" {
		t.Fatalf("want the bed with no track, got %v", body)
	}
}

// The local beds have no feed to consult — asking one would name a song nobody
// is hearing.
func TestAudioHandler_DoesNotConsultTheFeedOffSomaFM(t *testing.T) {
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

// The console builds the picker from albums + groups, and shows the album on air
// rather than the selection — under a group they're different, and the album is
// the one Dana would act on.
func TestAudioHandler_ReportsGroupsPlayingAlbumAndShuffle(t *testing.T) {
	f := &fakeBeds{
		bed:          beds.Album,
		album:        "streambeats",
		playingAlbum: "streambeats-lofi-gold",
		onShare:      []string{"streambeats-lofi-gold", "streambeats-synthwave-rose"},
		groups:       []string{"streambeats"},
		shuffle:      false,
	}
	_, body := getAudio(t, &Server{beds: f})
	if body["album"] != "streambeats" {
		t.Errorf("selection: %v", body["album"])
	}
	if body["playing_album"] != "streambeats-lofi-gold" {
		t.Errorf("playing album: %v", body["playing_album"])
	}
	groups, _ := body["groups"].([]any)
	if len(groups) != 1 {
		t.Errorf("groups: want 1, got %v", body["groups"])
	}
	if body["shuffle"] != false {
		t.Errorf("shuffle: %v", body["shuffle"])
	}
}

func TestAudioSetHandler_SelectsAGroup(t *testing.T) {
	f := &fakeBeds{bed: beds.CarHum, onShare: []string{
		"streambeats-lofi-gold", "streambeats-synthwave-rose",
	}}
	w := postAudio(t, &Server{beds: f}, `{"album":"streambeats"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status: %d (%q)", w.Code, w.Body.String())
	}
	if len(f.albums) != 1 || f.albums[0] != "streambeats" {
		t.Fatalf("expected the group to reach the store, got %v", f.albums)
	}
}

func TestAudioSetHandler_TogglesShuffle(t *testing.T) {
	f := &fakeBeds{bed: beds.Album, shuffle: true}
	w := postAudio(t, &Server{beds: f}, `{"shuffle":false}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status: %d (%q)", w.Code, w.Body.String())
	}
	if !slices.Equal(f.shuffles, []bool{false}) {
		t.Fatalf("expected one shuffle-off, got %v", f.shuffles)
	}
	if f.shuffle {
		t.Error("shuffle should be off")
	}
}

// false is a real instruction, so it must not read as "the caller said nothing".
// A plain bool in the request struct would make shuffle impossible to turn off.
func TestAudioSetHandler_ShuffleFalseIsNotAbsent(t *testing.T) {
	f := &fakeBeds{bed: beds.Album, shuffle: true}
	postAudio(t, &Server{beds: f}, `{"shuffle":false}`)
	if len(f.shuffles) != 1 {
		t.Fatalf("shuffle:false must reach the store, got %v", f.shuffles)
	}
	// ...and omitting it entirely must not touch shuffle at all.
	g := &fakeBeds{bed: beds.Album, shuffle: true, onShare: []string{"fifty-horizons"}}
	postAudio(t, &Server{beds: g}, `{"bed":"carhum"}`)
	if len(g.shuffles) != 0 {
		t.Fatalf("a bed switch must not touch shuffle, got %v", g.shuffles)
	}
}

// The console needs the waiting switch as its own field: bed/station/album still
// describe the audio, so without this a click renders as no change at all.
func TestAudioHandler_ShipsTheWaitingSwitch(t *testing.T) {
	f := &fakeBeds{bed: beds.CarHum, pending: &beds.Switch{
		Bed:     beds.SomaFM,
		Station: "dronezone",
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
	if pending["bed"] != "somafm" || pending["station"] != "dronezone" {
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
