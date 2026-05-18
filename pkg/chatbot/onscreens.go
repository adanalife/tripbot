package chatbot

import (
	"context"
	"time"

	onscreensClient "github.com/adanalife/tripbot/pkg/onscreens-client"
)

// Onscreens is the subset of the onscreens-client surface that chatbot
// commands depend on. Tests inject a fake; production uses the package-backed
// realOnscreens adapter wired in defaultApp.
type Onscreens interface {
	ShowFlag(ctx context.Context, dur time.Duration) error
	ShowLeaderboard(ctx context.Context, title string, leaderboard [][]string) error
	HideMiddleText(ctx context.Context) error
	ShowMiddleText(ctx context.Context, msg string) error
	ShowTimewarp(ctx context.Context) error
}

// realOnscreens delegates to pkg/onscreens-client.
type realOnscreens struct{}

func (realOnscreens) ShowFlag(ctx context.Context, dur time.Duration) error {
	return onscreensClient.ShowFlag(ctx, dur)
}
func (realOnscreens) ShowLeaderboard(ctx context.Context, title string, lb [][]string) error {
	return onscreensClient.ShowLeaderboard(ctx, title, lb)
}
func (realOnscreens) HideMiddleText(ctx context.Context) error {
	return onscreensClient.HideMiddleText(ctx)
}
func (realOnscreens) ShowMiddleText(ctx context.Context, msg string) error {
	return onscreensClient.ShowMiddleText(ctx, msg)
}
func (realOnscreens) ShowTimewarp(ctx context.Context) error {
	return onscreensClient.ShowTimewarp(ctx)
}
