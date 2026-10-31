package eventsub

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestRun_RejectsEmptyConfig(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
	}{
		{"empty everything", Config{}},
		{"missing ClientID", Config{BroadcasterToken: "t", BroadcasterUserID: "u"}},
		{"missing BroadcasterToken", Config{ClientID: "c", BroadcasterUserID: "u"}},
		{"missing BroadcasterUserID", Config{ClientID: "c", BroadcasterToken: "t"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := Run(context.Background(), tc.cfg, Handlers{})
			if err == nil {
				t.Fatal("Run with incomplete Config should return error; got nil")
			}
			if !strings.Contains(err.Error(), "Config") {
				t.Errorf("err message %q should mention Config", err.Error())
			}
		})
	}
}

// A rejected broadcaster token must be distinguishable from a dropped socket:
// the caller redials the second and gives up on the first. Getting this wrong
// costs roughly three Sentry events a minute forever, since Twitch closes a
// subscription-less session after ~10s and the retry loop re-subscribes.
func TestIsUnauthorized(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{
			// Verbatim from the TRIPBOT-9Y events (2026-07-29 prod).
			name: "twitch rejects the token",
			err:  errors.New(`could not subscribe to event: 401 Unauthorized: {"error":"Unauthorized","status":401,"message":"Invalid OAuth token"}`),
			want: true,
		},
		{
			// A malformed request is our bug, not an expired token — retrying is
			// equally useless but it must not be reported as an auth failure.
			name: "bad request is not an auth failure",
			err:  errors.New(`could not subscribe to event: 400 Bad Request: {"error":"Bad Request","status":400,"message":"invalid transport and auth combination"}`),
			want: false,
		},
		{
			name: "missing scope is a 403, not a 401",
			err:  errors.New(`could not subscribe to event: 403 Forbidden: {"error":"Forbidden","status":403}`),
			want: false,
		},
		{
			name: "transient network failure",
			err:  errors.New("could not subscribe to event: dial tcp: i/o timeout"),
			want: false,
		},
		{name: "no error", err: nil, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isUnauthorized(tc.err); got != tc.want {
				t.Errorf("isUnauthorized(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

// Run must surface ErrUnauthorized through errors.Is so the caller can branch on
// it; a bare string match at the call site would rot the moment this is wrapped.
func TestErrUnauthorizedIsMatchable(t *testing.T) {
	wrapped := fmt.Errorf("%w (connection ended: %v)", ErrUnauthorized, errors.New("close 4003"))
	if !errors.Is(wrapped, ErrUnauthorized) {
		t.Error("wrapped ErrUnauthorized should satisfy errors.Is")
	}
	if errors.Is(errors.New("some other failure"), ErrUnauthorized) {
		t.Error("unrelated errors must not satisfy errors.Is")
	}
}

// Giving up is only correct when the token bought nothing. A token missing one
// event type's scope must keep the subscriptions it did get, and must keep
// redialing so a later socket drop still recovers — the partial-subscription
// behavior this package documents.
func TestTokenRejected(t *testing.T) {
	cases := []struct {
		name              string
		attempted, denied int32
		want              bool
	}{
		{"every subscription denied", 5, 5, true},
		{"one scope missing, rest fine", 5, 1, false},
		{"all succeeded", 5, 0, false},
		{"no handlers registered", 0, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tokenRejected(tc.attempted, tc.denied); got != tc.want {
				t.Errorf("tokenRejected(%d, %d) = %v, want %v", tc.attempted, tc.denied, got, tc.want)
			}
		})
	}
}
