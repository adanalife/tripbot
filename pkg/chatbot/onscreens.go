package chatbot

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	onscreensClient "github.com/adanalife/tripbot/pkg/onscreens-client"
)

// Onscreens is the subset of the onscreens-client surface that chatbot
// commands depend on. Tests inject a fake; production uses the
// realOnscreens adapter wired in defaultApp.
type Onscreens interface {
	ShowFlag(ctx context.Context, dur time.Duration) error
	ShowLeaderboard(ctx context.Context, title string, leaderboard [][]string) error
	HideMiddleText(ctx context.Context) error
	ShowMiddleText(ctx context.Context, msg string) error
	ShowTimewarp(ctx context.Context) error
}

// realOnscreens delegates to a constructed *onscreensClient.Client. The
// concrete Client instance is owned by the App (wired up in defaultApp),
// not read off a package-level global in pkg/onscreens-client.
//
// nats + env are used by ShowMiddleText to mirror the call to NATS
// alongside the HTTP request — phase 1 of the pubsub migration. Other
// methods stay HTTP-only until phase 2 peels them off.
type realOnscreens struct {
	c    *onscreensClient.Client
	nats NATS
	env  string
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

// middleTextEvent is the wire format published to
// tripbot.<env>.onscreens.middle.show. Snake_case so a future protobuf
// schema maps 1-1.
type middleTextEvent struct {
	Msg       string `json:"msg"`
	EmittedAt string `json:"emitted_at"`
}

func (r realOnscreens) ShowMiddleText(ctx context.Context, msg string) error {
	// Parallel publish: NATS first (cheap, fire-and-forget) so a slow
	// HTTP call doesn't delay it. HTTP remains the source of truth for
	// phase 1; the NATS subscriber on onscreens-server acts on the same
	// payload but its actions are presently redundant with the HTTP path.
	payload, err := json.Marshal(middleTextEvent{
		Msg:       msg,
		EmittedAt: time.Now().UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		slog.ErrorContext(ctx, "marshal middle-text event", "err", err)
	} else {
		r.nats.Publish(ctx, fmt.Sprintf("tripbot.%s.onscreens.middle.show", r.env), payload)
	}
	return r.c.ShowMiddleText(ctx, msg)
}

func (r realOnscreens) ShowTimewarp(ctx context.Context) error {
	return r.c.ShowTimewarp(ctx)
}
