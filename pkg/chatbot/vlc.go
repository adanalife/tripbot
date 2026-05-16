package chatbot

import (
	vlcClient "github.com/adanalife/tripbot/pkg/vlc-client"
)

// VLC is the subset of the vlc-client surface that chatbot commands depend
// on (timewarp, jump, skip, back). Tests inject a fake; production uses the
// package-backed realVLC adapter wired in defaultApp. Mirrors the Onscreens
// injection pattern.
type VLC interface {
	PlayRandom() error
	PlayFileInPlaylist(filename string) error
	Skip(n int) error
	Back(n int) error
}

// realVLC delegates to pkg/vlc-client.
type realVLC struct{}

func (realVLC) PlayRandom() error                     { return vlcClient.PlayRandom() }
func (realVLC) PlayFileInPlaylist(filename string) error { return vlcClient.PlayFileInPlaylist(filename) }
func (realVLC) Skip(n int) error                      { return vlcClient.Skip(n) }
func (realVLC) Back(n int) error                      { return vlcClient.Back(n) }
