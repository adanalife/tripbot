package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/nicklaw5/helix/v2"
)

func TestAuthInit_RedirectsToTwitch(t *testing.T) {
	// Build a real *helix.Client with non-empty options. GetAuthorizationURL
	// is pure URL construction (no network), so this works without app tokens.
	stub, err := helix.NewClient(&helix.Options{
		ClientID:    "test-client-id",
		RedirectURI: "http://localhost:8080/auth/callback",
	})
	if err != nil {
		t.Fatalf("helix.NewClient: %v", err)
	}
	saved := helixClient
	helixClient = func() (*helix.Client, error) { return stub, nil }
	t.Cleanup(func() { helixClient = saved })

	req := httptest.NewRequest(http.MethodGet, "/auth/init", nil)
	rec := httptest.NewRecorder()
	authInitHandler(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("got status %d, want %d (302 redirect)", rec.Code, http.StatusFound)
	}
	loc := rec.Header().Get("Location")
	if loc == "" {
		t.Fatal("Location header empty")
	}
	u, err := url.Parse(loc)
	if err != nil {
		t.Fatalf("Location is not a valid URL: %v", err)
	}
	if !strings.Contains(u.Host, "id.twitch.tv") {
		t.Errorf("redirect host %q does not contain id.twitch.tv", u.Host)
	}
	if !strings.HasSuffix(u.Path, "/oauth2/authorize") {
		t.Errorf("redirect path %q does not end with /oauth2/authorize", u.Path)
	}
	q := u.Query()
	if q.Get("state") == "" {
		t.Error("redirect URL missing state param")
	}
	if q.Get("response_type") != "code" {
		t.Errorf("response_type = %q, want %q", q.Get("response_type"), "code")
	}
	scopes := q.Get("scope")
	for _, required := range []string{"chat:read", "chat:edit"} {
		if !strings.Contains(scopes, required) {
			t.Errorf("scope param %q missing required scope %q", scopes, required)
		}
	}
}
