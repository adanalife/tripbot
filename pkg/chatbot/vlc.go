package chatbot

import (
	"context"

	vlcClient "github.com/adanalife/tripbot/pkg/vlc-client"
)

// VLC is the subset of the vlc-client surface that chatbot commands depend
// on (timewarp, jump, skip, back). Tests inject a fake; production uses the
// package-backed realVLC adapter wired in defaultApp. Mirrors the Onscreens
// injection pattern.
type VLC interface {
	PlayRandom(ctx context.Context) error
	PlayFileInPlaylist(ctx context.Context, filename string) error
	Skip(ctx context.Context, n int) error
	Back(ctx context.Context, n int) error
}

// realVLC delegates to pkg/vlc-client.
type realVLC struct{}

func (realVLC) PlayRandom(ctx context.Context) error {
	return vlcClient.PlayRandom(ctx)
}
func (realVLC) PlayFileInPlaylist(ctx context.Context, filename string) error {
	return vlcClient.PlayFileInPlaylist(ctx, filename)
}
func (realVLC) Skip(ctx context.Context, n int) error { return vlcClient.Skip(ctx, n) }
func (realVLC) Back(ctx context.Context, n int) error { return vlcClient.Back(ctx, n) }
