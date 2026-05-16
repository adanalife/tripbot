package chatbot

import (
	"time"

	onscreensClient "github.com/adanalife/tripbot/pkg/onscreens-client"
)

// Onscreens is the subset of the onscreens-client surface that chatbot
// commands depend on. Tests inject a fake; production uses the package-backed
// realOnscreens adapter wired in defaultApp.
type Onscreens interface {
	ShowFlag(dur time.Duration) error
	ShowLeaderboard(title string, leaderboard [][]string) error
	HideMiddleText() error
	ShowMiddleText(msg string) error
	ShowTimewarp() error
}

// realOnscreens delegates to pkg/onscreens-client.
type realOnscreens struct{}

func (realOnscreens) ShowFlag(dur time.Duration) error { return onscreensClient.ShowFlag(dur) }
func (realOnscreens) ShowLeaderboard(title string, lb [][]string) error {
	return onscreensClient.ShowLeaderboard(title, lb)
}
func (realOnscreens) HideMiddleText() error          { return onscreensClient.HideMiddleText() }
func (realOnscreens) ShowMiddleText(msg string) error { return onscreensClient.ShowMiddleText(msg) }
func (realOnscreens) ShowTimewarp() error            { return onscreensClient.ShowTimewarp() }
