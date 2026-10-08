package main

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/adanalife/tripbot/pkg/eventsub"
)

// The gateway rotates the broadcaster grant while the pod runs, so a redial has
// to present whatever the last token reload wrote. A loop that read the token
// once would resubscribe with a credential Twitch has already retired — which is
// indistinguishable, from tripbot's side, from a revoked grant.
func TestRunEventSubLoop_ReadsTokenPerAttempt(t *testing.T) {
	tokens := []string{"rotated-1", "rotated-2", "rotated-3"}
	var seen []string

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// run bounds the loop, so a loop that reads the token once still terminates
	// and fails on the assertion instead of spinning.
	var attempts int
	run := func(_ context.Context, tok string) error {
		seen = append(seen, tok)
		attempts++
		if attempts == len(tokens) {
			cancel()
		}
		return errors.New("socket closed")
	}
	token := func() string {
		if attempts >= len(tokens) {
			return tokens[len(tokens)-1]
		}
		return tokens[attempts]
	}

	runEventSubLoop(ctx, token, run, 0, 0)

	if len(seen) != len(tokens) {
		t.Fatalf("tokens presented = %v, want %v", seen, tokens)
	}
	for i := range tokens {
		if seen[i] != tokens[i] {
			t.Errorf("attempt %d presented token %q, want %q — the token is being captured instead of re-read", i+1, seen[i], tokens[i])
		}
	}
}

// A wholly refused subscribe round is what a rotation looks like from here, so
// the loop must keep going rather than shut EventSub down until the pod
// restarts — but it waits a full token-reload interval, since retrying before
// there is a new row to read just repeats the rejection.
func TestRunEventSubLoop_RetriesAfterTokenRejection(t *testing.T) {
	const rejectedDelay = 30 * time.Millisecond
	var attempts int
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	run := func(context.Context, string) error {
		attempts++
		if attempts == 2 {
			cancel()
		}
		return fmt.Errorf("%w (connection ended: close 4003)", eventsub.ErrUnauthorized)
	}

	start := time.Now()
	runEventSubLoop(ctx, func() string { return "tok" }, run, 0, rejectedDelay)

	if attempts < 2 {
		t.Fatalf("attempts = %d, want at least 2 — a rejected token must not disable eventsub for the life of the pod", attempts)
	}
	if elapsed := time.Since(start); elapsed < rejectedDelay {
		t.Errorf("loop redialed after %v, want at least the %v reject backoff — retrying sooner than a token reload repeats the same rejection", elapsed, rejectedDelay)
	}
}

// A cancelled context is a shutdown, not a failure: the loop returns without
// scheduling another dial.
func TestRunEventSubLoop_StopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var attempts int
	runEventSubLoop(ctx, func() string { return "tok" }, func(context.Context, string) error {
		attempts++
		return context.Canceled
	}, time.Hour, time.Hour)

	if attempts != 0 {
		t.Errorf("attempts = %d, want 0 — an already-cancelled context must not dial", attempts)
	}
}

// An unseeded install has no row to present and no redial can conjure one, so
// that case alone waits out a token reload rather than spinning at the redial
// delay.
func TestRunEventSubLoop_UnloadedTokenWaitsForReload(t *testing.T) {
	const rejectedDelay = 30 * time.Millisecond
	var attempts int
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	run := func(context.Context, string) error {
		attempts++
		if attempts == 2 {
			cancel()
		}
		return errBroadcasterTokenUnloaded
	}

	start := time.Now()
	runEventSubLoop(ctx, func() string { return "" }, run, 0, rejectedDelay)

	if attempts < 2 {
		t.Fatalf("attempts = %d, want at least 2 — a missing broadcaster row must not disable eventsub permanently", attempts)
	}
	if elapsed := time.Since(start); elapsed < rejectedDelay {
		t.Errorf("loop redialed after %v, want at least the %v reload backoff — retrying sooner just repeats the same empty read", elapsed, rejectedDelay)
	}
}
