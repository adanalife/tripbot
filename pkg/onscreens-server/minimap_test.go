package onscreensServer

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/nats-io/nats.go"
)

func TestProject(t *testing.T) {
	// Opposite corners of the country land in opposite corners of the frame.
	seaX, seaY, ok := project(47.61, -122.33)
	if !ok {
		t.Fatal("Seattle is off the map")
	}
	miaX, miaY, ok := project(25.76, -80.19)
	if !ok {
		t.Fatal("Miami is off the map")
	}
	if seaX >= miaX || seaY >= miaY {
		t.Errorf("Seattle (%.0f,%.0f) should sit up and left of Miami (%.0f,%.0f)", seaX, seaY, miaX, miaY)
	}
	for _, p := range [][2]float64{{seaX, seaY}, {miaX, miaY}} {
		if p[0] < 0 || p[0] > minimapWidth || p[1] < 0 || p[1] > minimapHeight {
			t.Errorf("projected point %v is outside the %vx%v frame", p, minimapWidth, minimapHeight)
		}
	}
	for name, c := range map[string][2]float64{
		"Anchorage": {61.22, -149.9}, "null island": {0, 0}, "Vancouver": {49.6, -123.1},
	} {
		if _, _, ok := project(c[0], c[1]); ok {
			t.Errorf("%s projected onto a map of the lower 48", name)
		}
	}
}

func TestStatePathsCoverTheLower48(t *testing.T) {
	paths := statePaths()
	if len(paths) != 49 { // 48 states + DC
		t.Fatalf("got %d outlines, want 49", len(paths))
	}
	for _, p := range paths {
		if !strings.HasPrefix(p.d, "M") || !strings.HasSuffix(p.d, "Z") {
			t.Errorf("%s: path data isn't a closed outline: %.40q", p.name, p.d)
		}
	}
}

// locationMsg is a location.update for a clip filmed in Moab.
func locationMsg(body string) *nats.Msg {
	return &nats.Msg{Subject: "tripbot.testing.onscreens.location.update.twitch", Data: []byte(body)}
}

const moabUpdate = `{"state":"Utah","lat":38.5733,"lng":-109.5498}`

func TestMinimapStaysHiddenUntilSwitchedOn(t *testing.T) {
	s := newTestServer(t) // twitch: off by default, a map would spoil !guess
	s.handleLocationUpdate(locationMsg(moabUpdate))
	if s.Minimap.IsShowing() {
		t.Fatal("minimap showed on twitch without a minimap.show")
	}

	// Switching on shows the position already reported, without waiting a tick.
	s.handleMinimapShow(emptyMsg("minimap.show"))
	if !s.Minimap.IsShowing() {
		t.Fatal("minimap.show left the minimap hidden")
	}
	if !strings.HasSuffix(s.Minimap.Content(), ",Utah") {
		t.Errorf("content = %q, want the plotted fix", s.Minimap.Content())
	}

	s.handleMinimapHide(emptyMsg("minimap.hide"))
	s.handleLocationUpdate(locationMsg(moabUpdate))
	if s.Minimap.IsShowing() {
		t.Error("a location update re-showed a minimap that was switched off")
	}
}

func TestMinimapDefaultsOnWhereChatCantAsk(t *testing.T) {
	for platform, want := range map[string]bool{
		platformTwitch: false, platformTikTok: true, platformInstagram: true,
	} {
		s := New(Config{Conf: rotatorConf(platform, true)})
		s.handleLocationUpdate(locationMsg(moabUpdate))
		if got := s.Minimap.IsShowing(); got != want {
			t.Errorf("%s: showing = %v, want %v", platform, got, want)
		}
		_ = s.Shutdown(t.Context())
	}
}

func TestMinimapHidesOffTheMapAndOnAStaleFix(t *testing.T) {
	s := newTestServer(t)
	s.handleMinimapShow(emptyMsg("minimap.show"))
	s.handleLocationUpdate(locationMsg(moabUpdate))

	// An update with no coordinate (an older tripbot, or footage outside the
	// lower 48) takes the dot down rather than leaving it in Utah.
	s.handleLocationUpdate(locationMsg(`{"state":"Alaska","lat":61.22,"lng":-149.9}`))
	if s.Minimap.IsShowing() {
		t.Error("minimap kept showing for a coordinate off the map")
	}

	// A fix older than the feed's TTL is not worth showing on a switch-on.
	s.setMinimap(false, time.Now())
	s.plotMinimap(38.5733, -109.5498, "Utah", time.Now().Add(-2*locationDataTTL))
	s.setMinimap(true, time.Now())
	if s.Minimap.IsShowing() {
		t.Error("switching on showed a fix older than the location TTL")
	}
}

func TestRefreshResetsTheExpiry(t *testing.T) {
	osc := newOnscreen(defaultSleepInterval)
	t.Cleanup(osc.stop)
	osc.Refresh("a", time.Hour)
	osc.Refresh("b", time.Minute)
	osc.mu.RLock()
	left := time.Until(osc.expires)
	osc.mu.RUnlock()
	if left > time.Minute {
		t.Errorf("expiry is %v out; a second Refresh should replace the first, not add to it", left)
	}
	if osc.Content() != "b" || !osc.IsShowing() {
		t.Errorf("content=%q showing=%v, want the refreshed content showing", osc.Content(), osc.IsShowing())
	}
}

func TestMinimapAssetDrawsTheDotAndState(t *testing.T) {
	s := newTestServer(t)
	get := func() string {
		req := mux.SetURLVars(httptest.NewRequest(http.MethodGet, "/onscreens/asset/minimap", nil), map[string]string{"name": SlugMinimap})
		rec := httptest.NewRecorder()
		s.onscreensAssetHandler(rec, req)
		if ct := rec.Header().Get("Content-Type"); ct != "image/svg+xml" {
			t.Fatalf("Content-Type = %q", ct)
		}
		return rec.Body.String()
	}

	if bare := get(); strings.Contains(bare, "<circle") || strings.Contains(bare, "#ffd166") {
		t.Error("the map drew a dot or a highlighted state before any position arrived")
	}
	s.handleLocationUpdate(locationMsg(moabUpdate))
	svg := get()
	if strings.Count(svg, "<circle") != 2 {
		t.Errorf("want the dot and its ring, got %d circles", strings.Count(svg, "<circle"))
	}
	if strings.Count(svg, "#ffd166") != 1 {
		t.Errorf("want exactly one highlighted state, got %d", strings.Count(svg, "#ffd166"))
	}
}
