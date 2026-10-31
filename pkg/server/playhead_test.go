package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/adanalife/tripbot/pkg/video"
)

type fixedPlayhead struct {
	vid video.Video
	at  time.Duration
}

func (f fixedPlayhead) Playhead(context.Context) (video.Video, time.Duration) { return f.vid, f.at }

func withClipTrack(t *testing.T, pts []video.TrackPoint) {
	t.Helper()
	saved := clipTrack
	t.Cleanup(func() { clipTrack = saved })
	clipTrack = func(context.Context, video.Video) []video.TrackPoint { return pts }
}

func getPlayhead(t *testing.T, s *Server) (int, playheadResponse) {
	t.Helper()
	rec := httptest.NewRecorder()
	s.mapPlayheadHandler(rec, httptest.NewRequest(http.MethodGet, "/api/map/playhead", nil))
	var body playheadResponse
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode %q: %v", rec.Body.String(), err)
		}
	}
	return rec.Code, body
}

func TestMapPlayheadHandler(t *testing.T) {
	withClipTrack(t, []video.TrackPoint{{Sec: 0, Lat: 40, Lng: -110}, {Sec: 2, Lat: 40.1, Lng: -110.1}})
	s := New(nil)
	s.SetPlayhead(fixedPlayhead{video.Video{Slug: "2018_0514_224801_080"}, 75500 * time.Millisecond})

	code, got := getPlayhead(t, s)
	want := playheadResponse{
		File:    "2018_0514_224801_080.MP4",
		OffsetS: 75.5,
		Track:   [][3]float64{{0, 40, -110}, {2, 40.1, -110.1}},
	}
	if code != http.StatusOK || !reflect.DeepEqual(got, want) {
		t.Errorf("got %d %+v, want 200 %+v", code, got, want)
	}
}

// A clip with no believable track still answers, with an empty list rather
// than null, so the console can tell "no track" from a malformed reply.
func TestMapPlayheadHandler_NoTrack(t *testing.T) {
	withClipTrack(t, nil)
	s := New(nil)
	s.SetPlayhead(fixedPlayhead{video.Video{Slug: "2018_0514_224801_080"}, 0})

	rec := httptest.NewRecorder()
	s.mapPlayheadHandler(rec, httptest.NewRequest(http.MethodGet, "/api/map/playhead", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if string(raw["track"]) != "[]" {
		t.Errorf("track = %s, want []", raw["track"])
	}
}

func TestMapPlayheadHandler_NoPlayer(t *testing.T) {
	if code, _ := getPlayhead(t, New(nil)); code != http.StatusServiceUnavailable {
		t.Errorf("code = %d, want 503", code)
	}
}
