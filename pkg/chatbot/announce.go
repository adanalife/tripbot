package chatbot

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/adanalife/tripbot/pkg/eventbus"
	"github.com/adanalife/tripbot/pkg/events"
	"github.com/adanalife/tripbot/pkg/instrumentation"
)

// AnnounceNewFollower says a thank-you to a new follower in chat, logs a
// follow event (the durable rows behind "followers gained"), and publishes a
// follow event to the console. Wired from pkg/eventsub on channel.follow v2
// events.
func (a *App) AnnounceNewFollower(username string) {
	a.Chat.Say(fmt.Sprintf("Thank you for the follow, @%s; type !commands in chat to see what you can do", username))
	instrumentation.Announcements.Inc(a.Platform, "follow")
	if err := a.Events.Follow(context.Background(), username); err != nil {
		slog.ErrorContext(context.Background(), "error creating follow event", "err", err)
	}
	eventbus.EmitSubscriberEvent(context.Background(), a.Cfg.Environment, eventbus.SubscriberEvent{
		Platform: a.Platform,
		Kind:     "follow",
		Username: username,
	})
}

// AnnounceSubscriber says a thank-you to a new subscriber, gives every
// currently-logged-in viewer +1 mile, announces the bonus, and logs a
// subscribe event (opening the viewer's subscribed interval). Wired from
// pkg/eventsub on channel.subscribe events.
//
// Gift recipients (isGift true) only get the subscribe event. Twitch fires
// channel.subscribe once per recipient, so a 50-gift bomb shouted per
// recipient would be 100 bot lines, past the 20-per-30s limit a non-mod bot
// gets, and 50 separate mile grants. The gifter's channel.subscription.gift
// carries the count, and AnnounceGiftSub celebrates the whole bomb once, both
// in chat and on the console. tier rides along for the console's badge.
func (a *App) AnnounceSubscriber(username string, isGift bool, tier string) {
	if err := a.Events.Subscribe(context.Background(), username); err != nil {
		slog.ErrorContext(context.Background(), "error creating subscribe event", "err", err)
	}
	if isGift {
		return
	}
	a.Chat.Say(fmt.Sprintf("Thank you for the sub, @%s; enjoy your !bonusmiles bleedPurple", username))
	a.Sessions.GiveEveryoneMiles(1.0)
	a.Chat.Say(fmt.Sprintf("The %d current viewers have been given a bonus mile, too HolidayPresent", a.Sessions.LoggedInCount()))
	instrumentation.Announcements.Inc(a.Platform, "sub")
	eventbus.EmitSubscriberEvent(context.Background(), a.Cfg.Environment, eventbus.SubscriberEvent{
		Platform: a.Platform,
		Kind:     "sub",
		Username: username,
		Tier:     tier,
	})
}

// giftWallMax caps the emote wall on a gift-bomb thank-you, keeping the line
// well inside Twitch's 500-character message limit.
const giftWallMax = 25

// giftWall is the emote run that sizes a gift thank-you to the gift: one
// HolidayPresent per sub gifted, up to giftWallMax.
func giftWall(count int) string {
	return strings.TrimSpace(strings.Repeat("HolidayPresent ", min(max(count, 1), giftWallMax)))
}

// AnnounceGiftSub celebrates a gift once for the whole batch: a thank-you
// sized to the gift with an emote wall, one mile per sub gifted to every
// logged-in viewer (the total the per-recipient bonus used to add up to, in one
// grant and one line), and a gift event to the console. count is how many subs
// were gifted in this event; gifter is empty (and isAnonymous true) for an
// anonymous gift. Wired from pkg/eventsub on channel.subscription.gift events.
// The recipients' subscribe events land separately and quietly (see
// AnnounceSubscriber).
func (a *App) AnnounceGiftSub(gifter string, count int, tier string, isAnonymous bool) {
	who := "an anonymous gifter"
	if !isAnonymous {
		who = "@" + gifter
	}
	subs := "a sub"
	if count != 1 {
		subs = fmt.Sprintf("%d subs", count)
	}
	a.Chat.Say(fmt.Sprintf("Thank you to %s for gifting %s! bleedPurple %s", who, subs, giftWall(count)))
	if count > 0 {
		a.Sessions.GiveEveryoneMiles(float32(count))
		miles := "a bonus mile"
		if count != 1 {
			miles = fmt.Sprintf("%d bonus miles", count)
		}
		a.Chat.Say(fmt.Sprintf("The %d current viewers have been given %s, too HolidayPresent", a.Sessions.LoggedInCount(), miles))
	}
	// One per gift event, not per sub gifted: this counts shouts, and a
	// mass-gift gets a single one.
	instrumentation.Announcements.Inc(a.Platform, "gift")
	eventbus.EmitSubscriberEvent(context.Background(), a.Cfg.Environment, eventbus.SubscriberEvent{
		Platform:    a.Platform,
		Kind:        "gift",
		Username:    gifter,
		Tier:        tier,
		GiftCount:   count,
		IsAnonymous: isAnonymous,
	})
}

// AnnounceResub thanks a returning subscriber who shared their resub in chat
// and publishes a resub event to the console. cumulativeMonths is their total
// months subscribed, streakMonths their consecutive run (0 when the viewer
// hides it), and message the note they typed. Wired from pkg/eventsub on
// channel.subscription.message events — channel.subscribe does not fire for
// resubs, so this is the only signal. No subscribed-interval event is logged:
// the viewer was already subscribed, so no interval opens.
func (a *App) AnnounceResub(username string, cumulativeMonths, streakMonths int, tier, message string) {
	if cumulativeMonths > 0 {
		a.Chat.Say(fmt.Sprintf("Thank you for the %d-month resub, @%s bleedPurple", cumulativeMonths, username))
	} else {
		a.Chat.Say(fmt.Sprintf("Thank you for the resub, @%s bleedPurple", username))
	}
	instrumentation.Announcements.Inc(a.Platform, "resub")
	eventbus.EmitSubscriberEvent(context.Background(), a.Cfg.Environment, eventbus.SubscriberEvent{
		Platform: a.Platform,
		Kind:     "resub",
		Username: username,
		Tier:     tier,
		Months:   cumulativeMonths,
		Streak:   streakMonths,
		Message:  message,
	})
}

// RecordUnsubscribe logs a subscription-end event so a viewer's subscribed
// interval has a close (the real lapse/cancel from Twitch, not a guessed
// expiry). No chat shout — unsubs aren't announced. Wired from pkg/eventsub
// on channel.subscription.end events.
func (a *App) RecordUnsubscribe(username string, isGift bool, tier string) {
	_ = isGift
	_ = tier
	if err := a.Events.Unsubscribe(context.Background(), username); err != nil {
		slog.ErrorContext(context.Background(), "error creating unsubscribe event", "err", err)
	}
}

// raidFoldWindow is how soon a second raid from the same channel counts as
// the same raid: a raider whose first raid looked like it failed on their end
// sends it again within a minute or two, and one thank-you covers both.
const raidFoldWindow = 5 * time.Minute

// RecordRaid thanks the raiding channel in chat, publishes a raid event to the
// console (the chat line, the TV banner, the push), and logs a raid event
// stamped with the airing clip and playhead, so audience rollups can separate
// a raid's viewer spike from footage that genuinely draws viewers. Wired from
// pkg/eventsub on channel.raid events. A repeat raid from the same channel
// inside raidFoldWindow is still logged but neither thanked nor published
// again.
func (a *App) RecordRaid(from string, viewers int) {
	ctx := context.Background()
	now := time.Now()
	repeat := from == a.lastRaidFrom && now.Sub(a.lastRaidAt) < raidFoldWindow
	a.lastRaidFrom, a.lastRaidAt = from, now
	if !repeat {
		a.Chat.Say(fmt.Sprintf("Thank you for the raid, @%s; welcome to all %d raider(s) bleedPurple", from, viewers))
		instrumentation.Announcements.Inc(a.Platform, "raid")
		eventbus.EmitSubscriberEvent(ctx, a.Cfg.Environment, eventbus.SubscriberEvent{
			Platform: a.Platform,
			Kind:     "raid",
			Username: from,
		})
	}
	r := events.Raid{From: from, Viewers: viewers}
	// Current() is the cached notion of what's playing — no I/O, because
	// recording a raid must not cost a round-trip to playout.
	if a.Video != nil {
		r.VideoID = a.Video.Current().ID
		secs := a.Video.CurrentProgress().Seconds()
		r.TsSec = &secs
	}
	if err := a.Events.Raided(ctx, r); err != nil {
		slog.ErrorContext(ctx, "error creating raid event", "err", err, "from", from)
	}
}
