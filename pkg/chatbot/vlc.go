package chatbot

import (
	"context"
	"time"

	vlcClient "github.com/adanalife/tripbot/pkg/vlc-client"
)

// VLC is the subset of the vlc-client surface that chatbot commands depend
// on (timewarp, jump, skip, back). Tests inject a fake; production uses the
// realVLC adapter wired in New(). Mirrors the Onscreens injection
// pattern.
type VLC interface {
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

// realVLC delegates to a constructed *vlcClient.Client. The concrete Client
// instance is owned by the App (wired up in New()), not read off a
// package-level global in pkg/vlc-client.
type realVLC struct {
	c *vlcClient.Client
}

func (r realVLC) PlayRandom(ctx context.Context) error {
	return r.c.PlayRandom(ctx)
}
func (r realVLC) PlayFileInPlaylist(ctx context.Context, filename string) error {
	return r.c.PlayFileInPlaylist(ctx, filename)
}
func (r realVLC) PlayFileAtTimestamp(ctx context.Context, filename string, tsSec float64) error {
	return r.c.PlayFileAtTimestamp(ctx, filename, tsSec)
}
func (r realVLC) Skip(ctx context.Context, n int) error { return r.c.Skip(ctx, n) }
func (r realVLC) Back(ctx context.Context, n int) error { return r.c.Back(ctx, n) }
func (r realVLC) Seek(ctx context.Context, delta time.Duration) error {
	return r.c.Seek(ctx, delta)
}
