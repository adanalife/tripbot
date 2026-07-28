package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	rot "github.com/adanalife/tripbot/pkg/rotator"
	"github.com/gorilla/mux"
)

// fakeRotatorStore is an in-memory RotatorStore. putErr / getErr let a test
// drive the failure branches without a database.
type fakeRotatorStore struct {
	saved   map[string]rot.Config
	deleted []string
	putErr  error
	getErr  error
}

func newFakeStore() *fakeRotatorStore {
	return &fakeRotatorStore{saved: map[string]rot.Config{}}
}

func (f *fakeRotatorStore) GetOrDefault(_ context.Context, platform string) (rot.Config, bool, error) {
	if f.getErr != nil {
		return rot.DefaultConfigFor(platform), false, f.getErr
	}
	if cfg, ok := f.saved[platform]; ok {
		return cfg, true, nil
	}
	return rot.DefaultConfigFor(platform), false, nil
}

func (f *fakeRotatorStore) Put(_ context.Context, platform string, cfg rot.Config) error {
	if f.putErr != nil {
		return f.putErr
	}
	f.saved[platform] = cfg
	return nil
}

func (f *fakeRotatorStore) Delete(_ context.Context, platform string) error {
	f.deleted = append(f.deleted, platform)
	delete(f.saved, platform)
	return nil
}

// fakeRotatorPublisher records what would have gone onto NATS.
type fakeRotatorPublisher struct {
	published map[string]rot.Config
	err       error
}

func newFakePublisher() *fakeRotatorPublisher {
	return &fakeRotatorPublisher{published: map[string]rot.Config{}}
}

func (f *fakeRotatorPublisher) PublishRotatorConfig(_ context.Context, platform string, cfg rot.Config) error {
	if f.err != nil {
		return f.err
	}
	f.published[platform] = cfg
	return nil
}

// rotatorTestServer wires a Server with the rotator surface installed and
// returns a router so mux path vars resolve exactly as in production.
func rotatorTestServer(t *testing.T) (*fakeRotatorStore, *fakeRotatorPublisher, http.Handler) {
	t.Helper()
	store, pub := newFakeStore(), newFakePublisher()
	s := &Server{cfg: &c.TripbotConfig{}, versionTag: "test"}
	s.SetRotators(store, pub)

	r := mux.NewRouter()
	r.HandleFunc("/api/rotators/{platform}", s.rotatorsGetHandler).Methods("GET")
	r.HandleFunc("/api/rotators/{platform}", s.rotatorsPutHandler).Methods("PUT")
	r.HandleFunc("/api/rotators/{platform}", s.rotatorsResetHandler).Methods("DELETE")
	return store, pub, r
}

func doJSON(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var rdr *strings.Reader
	if body == "" {
		rdr = strings.NewReader("")
	} else {
		rdr = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, rdr)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

// An unedited platform serves the compiled-in defaults with stored=false, plus
// the corner budgets the console measures copy against.
func TestRotatorsGetServesDefaultsWithBudgets(t *testing.T) {
	_, _, h := rotatorTestServer(t)

	w := doJSON(t, h, http.MethodGet, "/api/rotators/youtube", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body)
	}
	var got rotatorConfigDTO
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Stored {
		t.Error("stored = true, want false for an unedited platform")
	}
	if len(got.Config.Left.Messages) == 0 {
		t.Error("expected the prefill to carry left-corner copy")
	}
	if len(got.Budgets) != 2 {
		t.Fatalf("budgets = %d, want 2 (one per corner)", len(got.Budgets))
	}
	// The asymmetry is the point: the console needs both to warn per corner.
	var left, right rot.Budget
	for _, b := range got.Budgets {
		switch b.Side {
		case rot.SideLeft:
			left = b
		case rot.SideRight:
			right = b
		}
	}
	if right.FitWidthPx >= left.FitWidthPx {
		t.Errorf("right budget %dpx should be narrower than left %dpx", right.FitWidthPx, left.FitWidthPx)
	}
	if right.FontFamilyCSS == "" {
		t.Error("budget missing the font stack the console measures with")
	}
}

// The editor generates its variable palette from this rather than hardcoding a
// second list, so every entry has to arrive complete.
func TestRotatorsGetDeclaresVariables(t *testing.T) {
	_, _, h := rotatorTestServer(t)

	w := doJSON(t, h, http.MethodGet, "/api/rotators/twitch", "")
	var got rotatorConfigDTO
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Variables) != len(rot.Variables()) {
		t.Fatalf("variables = %d, want the %d declared in pkg/rotator",
			len(got.Variables), len(rot.Variables()))
	}
	for _, v := range got.Variables {
		if v.Name == "" || v.Description == "" || v.Example == "" {
			t.Errorf("variable arrived incomplete: %+v", v)
		}
	}
}

// A misspelled variable is rejected with the offending token named, the same 422
// path a too-long line takes — the console shows the body verbatim.
func TestRotatorsPutRejectsUnknownVariable(t *testing.T) {
	store, pub, h := rotatorTestServer(t)

	w := doJSON(t, h, http.MethodPut, "/api/rotators/twitch",
		`{"left":{"messages":[{"text":"driving through $loction"}],"promo_messages":[]},`+
			`"right":{"messages":[],"promo_messages":[]}}`)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422: %s", w.Code, w.Body)
	}
	if !strings.Contains(w.Body.String(), "$loction") {
		t.Errorf("body %q should name the offending token", w.Body.String())
	}
	if len(store.saved) != 0 {
		t.Error("a rejected save should not reach the store")
	}
	if len(pub.published) != 0 {
		t.Error("a rejected save should not publish")
	}
}

func TestRotatorsPutSavesAndPublishes(t *testing.T) {
	store, pub, h := rotatorTestServer(t)

	w := doJSON(t, h, http.MethodPut, "/api/rotators/twitch",
		`{"left":{"messages":[{"text":"hello","weight":2}],"promo_messages":[]},
		  "right":{"messages":[{"text":"there"}],"promo_messages":[]},
		  "rare_message":"rare"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body)
	}
	saved, ok := store.saved["twitch"]
	if !ok {
		t.Fatal("config was not saved")
	}
	if len(saved.Left.Messages) != 1 || saved.Left.Messages[0].Text != "hello" {
		t.Errorf("saved left = %+v", saved.Left.Messages)
	}
	if _, ok := pub.published["twitch"]; !ok {
		t.Error("saved config was not published to onscreens-server")
	}
}

// Copy is stored before it's published, so a NATS outage still persists the
// edit — losing the live push is recoverable, losing the text isn't.
func TestRotatorsPutSavesEvenWhenPublishFails(t *testing.T) {
	store, pub, h := rotatorTestServer(t)
	pub.err = errors.New("nats down")

	w := doJSON(t, h, http.MethodPut, "/api/rotators/twitch",
		`{"left":{"messages":[{"text":"kept"}]}}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 despite the publish failure: %s", w.Code, w.Body)
	}
	if _, ok := store.saved["twitch"]; !ok {
		t.Error("edit was not persisted when the publish failed")
	}
}

// A failed write must not report success — the console would otherwise show
// copy that isn't stored anywhere.
func TestRotatorsPutReportsSaveFailure(t *testing.T) {
	store, pub, h := rotatorTestServer(t)
	store.putErr = errors.New("db down")

	w := doJSON(t, h, http.MethodPut, "/api/rotators/twitch", `{"left":{"messages":[{"text":"x"}]}}`)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
	if len(pub.published) != 0 {
		t.Error("a failed save must not publish")
	}
}

// Oversized copy is rejected with the line named, so the editor can point at it.
func TestRotatorsPutRejectsOversizedLine(t *testing.T) {
	store, _, h := rotatorTestServer(t)
	long := strings.Repeat("x", rot.BudgetFor(rot.SideRight).HardMaxRunes()+1)

	body, err := json.Marshal(rot.Config{Right: rot.Corner{Messages: []rot.Message{{Text: long}}}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	w := doJSON(t, h, http.MethodPut, "/api/rotators/twitch", string(body))
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422: %s", w.Code, w.Body)
	}
	if !strings.Contains(w.Body.String(), "right") {
		t.Errorf("error should name the offending corner: %s", w.Body)
	}
	if len(store.saved) != 0 {
		t.Error("rejected copy must not be saved")
	}
}

// Sanitizing happens server-side too, not just in the browser.
func TestRotatorsPutTrimsAndDropsBlankRows(t *testing.T) {
	store, _, h := rotatorTestServer(t)

	w := doJSON(t, h, http.MethodPut, "/api/rotators/twitch",
		`{"left":{"messages":[{"text":"  padded  "},{"text":"   "}]}}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", w.Code, w.Body)
	}
	msgs := store.saved["twitch"].Left.Messages
	if len(msgs) != 1 || msgs[0].Text != "padded" {
		t.Errorf("saved left = %+v, want one trimmed line", msgs)
	}
}

func TestRotatorsResetDeletesAndPublishesDefaults(t *testing.T) {
	store, pub, h := rotatorTestServer(t)
	store.saved["tiktok"] = rot.Config{Left: rot.Corner{Messages: []rot.Message{{Text: "edited"}}}}

	w := doJSON(t, h, http.MethodDelete, "/api/rotators/tiktok", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body)
	}
	if len(store.deleted) != 1 || store.deleted[0] != "tiktok" {
		t.Errorf("deleted = %v, want [tiktok]", store.deleted)
	}
	// The overlays shouldn't have to wait for a restart to show the defaults.
	published, ok := pub.published["tiktok"]
	if !ok {
		t.Fatal("reset did not publish the defaults")
	}
	if len(published.Left.Messages) == 0 {
		t.Error("published defaults carry no left-corner copy")
	}
}

// The platform is a primary key and a NATS subject leaf, so a typo must 404
// rather than write a row nothing reads.
func TestRotatorsRejectUnknownPlatform(t *testing.T) {
	store, _, h := rotatorTestServer(t)

	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		w := doJSON(t, h, method, "/api/rotators/myspace", `{}`)
		if w.Code != http.StatusNotFound {
			t.Errorf("%s status = %d, want 404", method, w.Code)
		}
	}
	if len(store.saved) != 0 || len(store.deleted) != 0 {
		t.Error("an unknown platform must not touch the store")
	}
}

func TestRotatorsPutRejectsMalformedBody(t *testing.T) {
	_, _, h := rotatorTestServer(t)

	w := doJSON(t, h, http.MethodPut, "/api/rotators/twitch", `not json`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

// Before SetRotators runs the endpoints degrade to 503, the same contract the
// flag endpoints follow before their client loads.
func TestRotatorsUnavailableWithoutWiring(t *testing.T) {
	s := &Server{cfg: &c.TripbotConfig{}, versionTag: "test"}
	r := mux.NewRouter()
	r.HandleFunc("/api/rotators/{platform}", s.rotatorsGetHandler).Methods("GET")

	w := doJSON(t, r, http.MethodGet, "/api/rotators/twitch", "")
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", w.Code)
	}
}

// A read error still serves usable defaults rather than failing the page load.
func TestRotatorsGetServesDefaultsOnStoreError(t *testing.T) {
	store, _, h := rotatorTestServer(t)
	store.getErr = errors.New("db down")

	w := doJSON(t, h, http.MethodGet, "/api/rotators/twitch", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body)
	}
	var got rotatorConfigDTO
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Config.Left.Messages) == 0 {
		t.Error("expected defaults to be served despite the read error")
	}
}

// Every response carries the compiled-in defaults for the platform, which is how
// the editor tells a shipped line from one authored in the console. Without it
// there's no provenance to show, since the stored document doesn't record it.
func TestRotatorsResponsesCarryDefaults(t *testing.T) {
	store, _, h := rotatorTestServer(t)
	store.saved["twitch"] = rot.Config{Left: rot.Corner{Messages: []rot.Message{{Text: "mine"}}}}

	for _, tc := range []struct{ method, path, body string }{
		{http.MethodGet, "/api/rotators/twitch", ""},
		{http.MethodPut, "/api/rotators/twitch", `{"left":{"messages":[{"text":"mine"}]}}`},
		{http.MethodDelete, "/api/rotators/twitch", ""},
	} {
		t.Run(tc.method, func(t *testing.T) {
			w := doJSON(t, h, tc.method, tc.path, tc.body)
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d: %s", w.Code, w.Body)
			}
			var got rotatorConfigDTO
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if len(got.Defaults.Left.Messages) == 0 {
				t.Error("response carries no default left-corner copy")
			}
			// Defaults are the platform-filtered set, so provenance can't be
			// judged against copy that would never render here.
			for _, m := range got.Defaults.Left.Messages {
				if len(m.Platforms) != 0 {
					t.Errorf("defaults kept platform scoping on %q", m.Text)
				}
			}
		})
	}
}

// The defaults are per platform, so a YouTube editor judges provenance against
// YouTube's shipped copy rather than Twitch's.
func TestRotatorsDefaultsArePlatformSpecific(t *testing.T) {
	_, _, h := rotatorTestServer(t)

	texts := func(platform string) map[string]bool {
		w := doJSON(t, h, http.MethodGet, "/api/rotators/"+platform, "")
		var got rotatorConfigDTO
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		out := map[string]bool{}
		for _, m := range got.Defaults.Left.Messages {
			out[m.Text] = true
		}
		return out
	}

	twitch, youtube := texts("twitch"), texts("youtube")
	var twitchOnly string
	for text := range twitch {
		if strings.Contains(text, "!miles") {
			twitchOnly = text
		}
	}
	if twitchOnly == "" {
		t.Fatal("expected a Twitch-only default line to compare against")
	}
	if youtube[twitchOnly] {
		t.Errorf("YouTube defaults included the Twitch-only line %q", twitchOnly)
	}
}
