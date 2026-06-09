package chatbot

import (
	"context"

	vlcClient "github.com/adanalife/tripbot/pkg/vlc-client"
)

// VLC is the subset of the vlc-client surface that chatbot commands depend
// on (timewarp, jump, skip, back). Tests inject a fake; production uses the
// realVLC adapter wired in New(). Mirrors the Onscreens injection
// pattern.
type VLC interface {
	PlayRandom(ctx context.Context) error
	PlayFileInPlaylist(ctx context.Context, filename string) error
	Skip(ctx context.Context, n int) error
	Back(ctx context.Context, n int) error
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
func (r realVLC) Skip(ctx context.Context, n int) error { return r.c.Skip(ctx, n) }
func (r realVLC) Back(ctx context.Context, n int) error { return r.c.Back(ctx, n) }
