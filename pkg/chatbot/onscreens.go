package chatbot

import (
	"context"
	"time"

	onscreensClient "github.com/adanalife/tripbot/pkg/onscreens-client"
)

// Onscreens is the subset of the onscreens-client surface that chatbot
// commands depend on. Tests inject a fake; production uses the
// realOnscreens adapter wired in New().
type Onscreens interface {
	ShowFlag(ctx context.Context, dur time.Duration) error
	ShowLeaderboard(ctx context.Context, title string, leaderboard [][]string) error
	HideMiddleText(ctx context.Context) error
	ShowMiddleText(ctx context.Context, msg string) error
	ShowTimewarp(ctx context.Context) error
}

// realOnscreens delegates to a constructed *onscreensClient.Client. The
// concrete Client instance is owned by the App (wired up in New()),
// not read off a package-level global in pkg/onscreens-client.
//
// The NATS mirror lives in the client itself now (it talks to NATS + HTTP
// for every command), so this adapter is a thin pass-through.
type realOnscreens struct {
	c *onscreensClient.Client
}

func (r realOnscreens) ShowFlag(ctx context.Context, dur time.Duration) error {
	return r.c.ShowFlag(ctx, dur)
}
func (r realOnscreens) ShowLeaderboard(ctx context.Context, title string, lb [][]string) error {
	return r.c.ShowLeaderboard(ctx, title, lb)
}
func (r realOnscreens) HideMiddleText(ctx context.Context) error {
	return r.c.HideMiddleText(ctx)
}
func (r realOnscreens) ShowMiddleText(ctx context.Context, msg string) error {
	return r.c.ShowMiddleText(ctx, msg)
}
func (r realOnscreens) ShowTimewarp(ctx context.Context) error {
	return r.c.ShowTimewarp(ctx)
}
