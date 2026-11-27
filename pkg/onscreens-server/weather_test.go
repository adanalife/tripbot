package onscreensServer

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	rot "github.com/adanalife/tripbot/pkg/rotator"
)

func TestWeatherView(t *testing.T) {
	cases := map[string]onscreenView{
		"":                             {},
		"Partly cloudy, 71°F":          {Content: `<span class="wx-icon">⛅</span> 71°F`, IsShowing: true},
		"Unknown conditions, 71°F":     {Content: "Unknown conditions, 71°F", IsShowing: true},
		"Thunderstorm with hail, 34°F": {Content: `<span class="wx-icon">⛈️</span> 34°F`, IsShowing: true},
	}
	for desc, want := range cases {
		if got := weatherView(rot.Vars{"weather": desc}); got != want {
			t.Errorf("weatherView(%q) = %+v, want %+v", desc, got, want)
		}
	}
	if got := weatherView(nil); got.IsShowing {
		t.Errorf("weatherView(nil) shows: %+v", got)
	}
}

// state.json carries the weather overlay while the clip data is fresh and
// hides it once the feed has gone quiet past locationDataTTL.
func TestStateHandlerWeather(t *testing.T) {
	s := newTestServer(t)
	read := func() onscreenView {
		t.Helper()
		rec := httptest.NewRecorder()
		s.onscreensStateHandler(rec, httptest.NewRequest(http.MethodGet, "/onscreens/state.json", nil))
		var got map[string]onscreenView
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("decoding state.json: %v", err)
		}
		return got[SlugWeather]
	}

	stageClipData(t, clipVars, time.Now())
	if got := read(); !got.IsShowing || got.Content != `<span class="wx-icon">☀️</span> 88°F` {
		t.Errorf("fresh weather = %+v", got)
	}
	stageClipData(t, clipVars, time.Now().Add(-locationDataTTL-time.Second))
	if got := read(); got.IsShowing {
		t.Errorf("stale weather still shows: %+v", got)
	}
}
