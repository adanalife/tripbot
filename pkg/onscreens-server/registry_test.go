package onscreensServer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHideUngraduatedBlanksOnlyNewOverlays(t *testing.T) {
	views := map[string]onscreenView{
		SlugLeaderboard: {Content: "board", IsShowing: true},
		"brand-new":     {Content: "wip", IsShowing: true},
	}
	hideUngraduated(views)
	if got := views[SlugLeaderboard]; !got.IsShowing || got.Content != "board" {
		t.Errorf("a graduated overlay was changed: %+v", got)
	}
	if got := views["brand-new"]; got.IsShowing || got.Content != "" {
		t.Errorf("an ungraduated overlay still shows: %+v", got)
	}
}

// The overlays already on the stream keep showing on production once the gate
// is in the state feed.
func TestProductionStateKeepsTheGraduatedOverlays(t *testing.T) {
	conf := *testConf
	conf.Environment = "production"
	s := New(Config{Version: "test", Conf: &conf})
	t.Cleanup(func() { _ = s.Shutdown(context.Background()) })
	for _, osc := range s.all() {
		osc.Show("on")
	}

	rec := httptest.NewRecorder()
	s.onscreensStateHandler(rec, httptest.NewRequest(http.MethodGet, "/onscreens/state.json", nil))
	var got map[string]onscreenView
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding state.json: %v", err)
	}
	for _, slug := range []string{SlugMiddleText, SlugLeaderboard, SlugLeftMessage, SlugRightMessage, SlugTimewarp, SlugGPS} {
		if !got[slug].IsShowing {
			t.Errorf("%s is hidden on production", slug)
		}
	}
}
