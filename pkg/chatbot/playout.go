package chatbot

import (
	"context"
	"time"

	playoutClient "github.com/adanalife/tripbot/pkg/playout-client"
	"github.com/adanalife/tripbot/pkg/viewstats"
)

// Playout is the subset of the playout-client surface that chatbot commands depend
// on (timewarp, jump, skip, back). Tests inject a fake; production uses the
// realPlayout adapter wired in New(). Mirrors the Onscreens injection
// pattern.
type Playout interface {
	// PlayRandom plays a random clip: from playout's current mode when corpus
	// is empty, else from that corpus.
	PlayRandom(ctx context.Context, corpus string) error
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
//
// Every command also notes its cause with viewstats before it goes out, so the
// clip switch the Player observes next is recorded as this command rather than
// as a jump nobody sent. Noted here, at the one funnel every command passes
// through, rather than at each handler.
type realPlayout struct {
	c        *playoutClient.Client
	platform string
}

func (r realPlayout) PlayRandom(ctx context.Context, corpus string) error {
	viewstats.NoteCause(r.platform, viewstats.CauseTimewarp)
	return r.c.PlayRandom(ctx, corpus)
}
func (r realPlayout) PlayFileInPlaylist(ctx context.Context, filename string) error {
	viewstats.NoteCause(r.platform, viewstats.CauseJump)
	return r.c.PlayFileInPlaylist(ctx, filename)
}
func (r realPlayout) PlayFileAtTimestamp(ctx context.Context, filename string, tsSec float64) error {
	viewstats.NoteCause(r.platform, viewstats.CauseFind)
	return r.c.PlayFileAtTimestamp(ctx, filename, tsSec)
}
func (r realPlayout) Skip(ctx context.Context, n int) error {
	viewstats.NoteCause(r.platform, viewstats.CauseSkip)
	return r.c.Skip(ctx, n)
}
func (r realPlayout) Back(ctx context.Context, n int) error {
	viewstats.NoteCause(r.platform, viewstats.CauseSkip)
	return r.c.Back(ctx, n)
}
func (r realPlayout) Seek(ctx context.Context, delta time.Duration) error {
	viewstats.NoteCause(r.platform, viewstats.CauseSeek)
	return r.c.Seek(ctx, delta)
}
