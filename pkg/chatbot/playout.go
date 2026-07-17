package chatbot

import (
	"context"
	"time"

	playoutClient "github.com/adanalife/tripbot/pkg/playout-client"
)

// Playout is the subset of the playout-client surface that chatbot commands depend
// on (timewarp, jump, skip, back). Tests inject a fake; production uses the
// realPlayout adapter wired in New(). Mirrors the Onscreens injection
// pattern.
type Playout interface {
	PlayRandom(ctx context.Context) error
	PlayFileInPlaylist(ctx context.Context, filename string) error
	// PlayFileAtTimestamp plays filename and seeks to tsSec seconds in — the
	// jump-to-moment path behind !find.
	PlayFileAtTimestamp(ctx context.Context, filename string, tsSec float64) error
	Skip(ctx context.Context, n int) error
	Back(ctx context.Context, n int) error
	// Seek moves the playhead by delta of footage, crossing clip boundaries;
	// negative rewinds. The duration form of !skip/!back.
	Seek(ctx context.Context, delta time.Duration) error
}

// realPlayout delegates to a constructed *playoutClient.Client. The concrete Client
// instance is owned by the App (wired up in New()), not read off a
// package-level global in pkg/playout-client.
type realPlayout struct {
	c *playoutClient.Client
}

func (r realPlayout) PlayRandom(ctx context.Context) error {
	return r.c.PlayRandom(ctx)
}
func (r realPlayout) PlayFileInPlaylist(ctx context.Context, filename string) error {
	return r.c.PlayFileInPlaylist(ctx, filename)
}
func (r realPlayout) PlayFileAtTimestamp(ctx context.Context, filename string, tsSec float64) error {
	return r.c.PlayFileAtTimestamp(ctx, filename, tsSec)
}
func (r realPlayout) Skip(ctx context.Context, n int) error { return r.c.Skip(ctx, n) }
func (r realPlayout) Back(ctx context.Context, n int) error { return r.c.Back(ctx, n) }
func (r realPlayout) Seek(ctx context.Context, delta time.Duration) error {
	return r.c.Seek(ctx, delta)
}
