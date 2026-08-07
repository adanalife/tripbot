package chatbot

import (
	"context"
	"log/slog"
	"time"

	"github.com/adanalife/tripbot/pkg/feature"
	"github.com/adanalife/tripbot/pkg/helpers"
)

// IncomingGift is one viewer gift from a gateway-wired platform, in the shape
// the effect path needs: who sent it and what it's worth.
//
// Value is the gift's total worth in the platform's creator-side unit (TikTok
// diamonds × count). It is the routing key rather than Name because platforms
// rename and retire gifts, and a value ladder keeps working when they do; Name
// is carried for logs and on-screen copy.
type IncomingGift struct {
	User   string
	UserID string
	Name   string
	Count  int
	Value  int
}

// giftFlagKey gates gift-triggered effects. Off until the flag exists and is
// enabled in the backing store (unknown keys evaluate false), so gifts are
// recorded but move nothing on stream until we turn it on.
const giftFlagKey = "chatbot.gifts"

// giftEffectFloor is the shortest gap between two gift-driven effects. Gifts
// deliberately bypass the 20s chat playback rate-limit — a viewer paid for the
// jump — but the warp overlay runs ~4.6s over an 800ms cover delay, so two
// gifts inside that window would stack covers and jump the playhead mid-warp.
// The floor is just long enough for one warp to finish.
var giftEffectFloor = 6 * time.Second

// lastGiftEffectTime is when a gift last moved the stream, for giftEffectFloor.
var lastGiftEffectTime time.Time

// giftEffect is what a gift makes happen on stream.
type giftEffect string

const (
	// effectTimewarp jumps the playhead to a random clip behind the full-screen
	// warp overlay — what !timewarp does.
	effectTimewarp giftEffect = "timewarp"
)

// giftTier is one rung of the gift ladder: a gift worth at least MinValue (and
// less than the next rung's) fires Effect.
type giftTier struct {
	MinValue int
	Effect   giftEffect
}

// giftTiers maps a gift's value onto the effect it fires. Sorted ascending; a
// gift takes the highest rung it reaches, and one below the first rung still
// fires it (a gift is a gift).
//
// Every rung is a timewarp today. It is the only effect that reads as a payoff
// on a stream with no chat reply: the other argument-free effects are !daytime
// (a no-op in daylight footage) and !skip / !back (too small to notice), and
// the argument-taking ones (!goto, !find) have nothing to take an argument
// from — a TikTok gift carries no message. The ladder is here so that changing
// what a rung does is an edit to this table rather than a refactor of the
// dispatch below.
var giftTiers = []giftTier{
	{MinValue: 0, Effect: effectTimewarp},
	{MinValue: 10, Effect: effectTimewarp},
	{MinValue: 100, Effect: effectTimewarp},
}

// effectFor returns the effect a gift of this value fires. giftTiers is never
// empty and its first rung is MinValue 0, so every gift resolves to something.
func effectFor(value int) giftEffect {
	effect := giftTiers[0].Effect
	for _, t := range giftTiers {
		if value < t.MinValue {
			break
		}
		effect = t.Effect
	}
	return effect
}

// HandleGatewayGift turns one viewer gift into its on-stream effect.
//
// This is the interaction that works on a read-only platform: TikTok's webcast
// is observe-only, so a gift can't be thanked in chat, but the effect itself is
// the acknowledgement — the gifter's name rides the warp overlay where the
// whole audience sees it.
func (a *App) HandleGatewayGift(ctx context.Context, gift IncomingGift) {
	slog.InfoContext(ctx, "received gift", "username", gift.User, "gift", gift.Name,
		"count", gift.Count, "value", gift.Value, "platform", a.platform())

	if !a.Flags.Bool(ctx, giftFlagKey, feature.EvalContext{
		Username: gift.User,
		Channel:  a.Cfg.ChannelName,
		Env:      a.Cfg.Environment,
	}) {
		slog.InfoContext(ctx, "gift effects disabled by feature flag", "flag", giftFlagKey)
		return
	}

	// Playout playback isn't wired up on the dev Mac (same guard as !timewarp).
	if helpers.RunningOnDarwin() {
		return
	}

	if since := time.Since(lastGiftEffectTime); since < giftEffectFloor {
		slog.InfoContext(ctx, "gift effect skipped; one is still playing",
			"username", gift.User, "since", since)
		return
	}

	effect := effectFor(gift.Value)
	slog.InfoContext(ctx, "running gift effect", "effect", string(effect),
		"username", gift.User, "value", gift.Value)

	switch effect {
	case effectTimewarp:
		a.timewarp(ctx, gift.User)
	}

	lastGiftEffectTime = time.Now()
}
