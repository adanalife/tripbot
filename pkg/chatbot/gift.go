package chatbot

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/adanalife/tripbot/pkg/events"
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
	// effectFind jumps to the moment that best matches what the gifter last
	// said in chat — what !find does, with their comment as the query. A gifter
	// who said nothing recently, or whose comment matches nothing, gets the
	// timewarp instead: a gift always moves the stream.
	effectFind giftEffect = "find"
)

// giftFindWindow is how far back a gifter's comment still counts as the query
// for effectFind. Viewers comment and then gift, so the comment precedes the
// gift by seconds; past a minute it is about something that has left the
// screen.
const giftFindWindow = 60 * time.Second

// recentCommentsMax is the size at which noteComment sweeps out comments older
// than giftFindWindow, so the map holds about a minute of distinct chatters.
const recentCommentsMax = 256

// recentComment is the last thing a chatter said, kept for effectFind.
type recentComment struct {
	text string
	at   time.Time
}

// commentKey identifies a chatter across the comment and gift paths: the
// platform's id where there is one, else the name with its casing folded.
func commentKey(user, userID string) string {
	if userID != "" {
		return userID
	}
	return strings.ToLower(user)
}

// noteComment remembers a chatter's latest comment as a possible gift query.
// Commands are skipped: "!skip" describes nothing to look for.
func (a *App) noteComment(user, userID, text string) {
	text = strings.TrimSpace(text)
	if text == "" || strings.HasPrefix(text, "!") {
		return
	}
	now := time.Now()
	a.recentCommentsMu.Lock()
	defer a.recentCommentsMu.Unlock()
	if a.recentComments == nil {
		a.recentComments = make(map[string]recentComment)
	}
	if len(a.recentComments) >= recentCommentsMax {
		for k, c := range a.recentComments {
			if now.Sub(c.at) > giftFindWindow {
				delete(a.recentComments, k)
			}
		}
	}
	a.recentComments[commentKey(user, userID)] = recentComment{text: text, at: now}
}

// takeComment returns the chatter's comment from the last giftFindWindow and
// forgets it, so one comment drives one find.
func (a *App) takeComment(user, userID string) (string, bool) {
	key := commentKey(user, userID)
	a.recentCommentsMu.Lock()
	defer a.recentCommentsMu.Unlock()
	c, ok := a.recentComments[key]
	delete(a.recentComments, key)
	if !ok || time.Since(c.at) > giftFindWindow {
		return "", false
	}
	return c.text, true
}

// giftFind runs effectFind and reports whether it moved the stream. False —
// no recent comment, no close match, a search that couldn't run — leaves the
// caller to fall back.
func (a *App) giftFind(ctx context.Context, gift IncomingGift) bool {
	query, ok := a.takeComment(gift.User, gift.UserID)
	if !ok {
		return false
	}
	// A state's full name means the state, as it does for !find.
	if helpers.StateToStateAbbrev(query) != "" {
		moved, _ := a.jumpToState(ctx, query)
		return moved
	}
	hit, err := a.findHit(ctx, query)
	if err != nil {
		return false
	}
	// A failed jump has already put the warp overlay up; a second warp on top
	// of it would be worse than the miss.
	_ = a.jumpToHit(ctx, gift.User, hit)
	return true
}

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
// The lower rungs are a timewarp. It is the argument-free effect that reads as
// a payoff on a stream with no chat reply: !daytime is a no-op in daylight
// footage and !skip / !back are too small to notice. The top rung is a find: a
// TikTok gift carries no message to take a query from, so it borrows the
// gifter's last comment (see effectFind). The ladder is here so that changing
// what a rung does is an edit to this table rather than a refactor of the
// dispatch below.
var giftTiers = []giftTier{
	{MinValue: 0, Effect: effectTimewarp},
	{MinValue: 10, Effect: effectTimewarp},
	{MinValue: 100, Effect: effectFind},
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
	if runningOnDarwin() {
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
	case effectFind:
		if !a.giftFind(ctx, gift) {
			a.timewarp(ctx, gift.User, events.WarpSourceGift)
		}
	case effectTimewarp:
		a.timewarp(ctx, gift.User, events.WarpSourceGift)
	}

	lastGiftEffectTime = time.Now()
}
