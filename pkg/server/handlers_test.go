package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/adanalife/tripbot/pkg/server/oauthstate"
)

func TestLiveHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	rec := httptest.NewRecorder()

	liveHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusOK)
	}
	if rec.Body.String() != "OK" {
		t.Fatalf("got body %q, want %q", rec.Body.String(), "OK")
	}
}

func TestReadyHandler(t *testing.T) {
	defer SetReady(false) // reset package state for other tests

	// not ready by default: 503
	SetReady(false)
	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	rec := httptest.NewRecorder()
	readyHandler(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("not-ready: got status %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}

	// ready once Twitch connection is established: 200
	SetReady(true)
	rec = httptest.NewRecorder()
	readyHandler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("ready: got status %d, want %d", rec.Code, http.StatusOK)
	}
	if rec.Body.String() != "OK" {
		t.Fatalf("ready: got body %q, want %q", rec.Body.String(), "OK")
	}
}

func TestVersionHandlerReturnsInjectedTag(t *testing.T) {
	saved := versionTag
	defer func() { versionTag = saved }()
	SetVersion("v9.9.9-test")

	req := httptest.NewRequest(http.MethodGet, "/version", nil)
	rec := httptest.NewRecorder()

	versionHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("got Content-Type %q, want %q", got, "application/json")
	}

	var body struct {
		Tag     string `json:"tag"`
		Sha     string `json:"sha"`
		BuiltAt string `json:"built_at"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("couldn't decode response: %v", err)
	}
	if body.Tag != "v9.9.9-test" {
		t.Fatalf("got tag %q, want %q", body.Tag, "v9.9.9-test")
	}
}

// withStubGenerateUserAccessToken swaps the package-level generator so the
// /auth/callback handler can be tested without round-tripping to Twitch.
func withStubGenerateUserAccessToken(t *testing.T, stub func(string, string) error) {
	t.Helper()
	saved := generateUserAccessToken
	generateUserAccessToken = stub
	t.Cleanup(func() { generateUserAccessToken = saved })
}

func TestAuthCallbackHandler_NoStateReturns400(t *testing.T) {
	withStubGenerateUserAccessToken(t, func(string, string) error {
		t.Fatal("generator should not be called when state is missing")
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/auth/callback?code=anything", nil)
	rec := httptest.NewRecorder()
	authCallbackHandler(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestAuthCallbackHandler_BadStateReturns400(t *testing.T) {
	withStubGenerateUserAccessToken(t, func(string, string) error {
		t.Fatal("generator should not be called when state is invalid")
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/auth/callback?state=not-real&code=anything", nil)
	rec := httptest.NewRecorder()
	authCallbackHandler(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestAuthCallbackHandler_NoCodeReturns400(t *testing.T) {
	state := oauthstate.New(oauthstate.AccountBot)
	withStubGenerateUserAccessToken(t, func(string, string) error {
		t.Fatal("generator should not be called when code is missing")
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/auth/callback?state="+state, nil)
	rec := httptest.NewRecorder()
	authCallbackHandler(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestAuthCallbackHandler_HappyPath(t *testing.T) {
	state := oauthstate.New(oauthstate.AccountBot)
	var gotCode, gotExpected string
	withStubGenerateUserAccessToken(t, func(code, expected string) error {
		gotCode = code
		gotExpected = expected
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/auth/callback?state="+state+"&code=the-code", nil)
	rec := httptest.NewRecorder()
	authCallbackHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusOK)
	}
	if gotCode != "the-code" {
		t.Errorf("generator got code %q, want %q", gotCode, "the-code")
	}
	// expected login should be BotUsername (from c.Conf) since the state
	// stashed AccountBot. Empty string here means the routing didn't fire.
	if gotExpected == "" {
		t.Errorf("generator got empty expected login; want BotUsername-derived value")
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "text/html") {
		t.Errorf("Content-Type %q is not html", rec.Header().Get("Content-Type"))
	}
	if !strings.Contains(rec.Body.String(), "Success") {
		t.Errorf("body should contain 'Success'; got %q", rec.Body.String())
	}
}

func TestAuthCallbackHandler_GeneratorErrorReturns500(t *testing.T) {
	state := oauthstate.New(oauthstate.AccountBot)
	withStubGenerateUserAccessToken(t, func(string, string) error {
		return errors.New("twitch broke")
	})

	req := httptest.NewRequest(http.MethodGet, "/auth/callback?state="+state+"&code=anything", nil)
	rec := httptest.NewRecorder()
	authCallbackHandler(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestAuthCallbackHandler_StateIsSingleUse(t *testing.T) {
	state := oauthstate.New(oauthstate.AccountBot)
	withStubGenerateUserAccessToken(t, func(string, string) error { return nil })

	// First call consumes the state and succeeds.
	req1 := httptest.NewRequest(http.MethodGet, "/auth/callback?state="+state+"&code=x", nil)
	rec1 := httptest.NewRecorder()
	authCallbackHandler(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("first call: got status %d, want %d", rec1.Code, http.StatusOK)
	}

	// Second call with the same state should 400.
	req2 := httptest.NewRequest(http.MethodGet, "/auth/callback?state="+state+"&code=x", nil)
	rec2 := httptest.NewRecorder()
	authCallbackHandler(rec2, req2)
	if rec2.Code != http.StatusBadRequest {
		t.Fatalf("second call: got status %d, want %d (state should be single-use)", rec2.Code, http.StatusBadRequest)
	}
}

func TestCatchAllHandlerGet404(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	rec := httptest.NewRecorder()

	catchAllHandler(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestCatchAllHandlerPost404(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/missing", nil)
	rec := httptest.NewRecorder()

	catchAllHandler(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusNotFound)
	}
}
