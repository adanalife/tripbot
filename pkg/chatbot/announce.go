package chatbot

import (
	"context"
	"fmt"
	"log/slog"
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
// A console subscriber event is published only for organic subs (isGift
// false). Gift recipients also fire channel.subscribe with isGift true, but
// the console celebrates the gifter's channel.subscription.gift instead (see
// AnnounceGiftSub) — otherwise a single mass-gift would storm the panel with
// one banner per recipient. tier rides along for the console's badge.
func (a *App) AnnounceSubscriber(username string, isGift bool, tier string) {
	a.Chat.Say(fmt.Sprintf("Thank you for the sub, @%s; enjoy your !bonusmiles bleedPurple", username))
	a.Sessions.GiveEveryoneMiles(1.0)
	a.Chat.Say(fmt.Sprintf("The %d current viewers have been given a bonus mile, too HolidayPresent", a.Sessions.LoggedInCount()))
	// Counted for gift recipients too, matching channel.subscribe, which fires
	// for them as well — the shout goes out either way, so skipping them would
	// show a shortfall against tripbot_events_total that isn't one.
	instrumentation.Announcements.Inc(a.Platform, "sub")
	if err := a.Events.Subscribe(context.Background(), username); err != nil {
		slog.ErrorContext(context.Background(), "error creating subscribe event", "err", err)
	}
	if !isGift {
		eventbus.EmitSubscriberEvent(context.Background(), a.Cfg.Environment, eventbus.SubscriberEvent{
			Platform: a.Platform,
			Kind:     "sub",
			Username: username,
			Tier:     tier,
		})
	}
}

// AnnounceGiftSub thanks a gifter in chat and publishes a gift event to the
// console. count is how many subs were gifted in this event; gifter is empty
// (and isAnonymous true) for an anonymous gift. Wired from pkg/eventsub on
// channel.subscription.gift events. The per-recipient subscribe events land
// separately (see AnnounceSubscriber) — this is the gifter's shout-out.
func (a *App) AnnounceGiftSub(gifter string, count int, tier string, isAnonymous bool) {
	if isAnonymous {
		a.Chat.Say(fmt.Sprintf("Thank you to an anonymous gifter for %d sub(s)! bleedPurple", count))
	} else {
		a.Chat.Say(fmt.Sprintf("Thank you @%s for gifting %d sub(s)! bleedPurple", gifter, count))
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

// raidWelcome is how long the onscreen welcome stays up: long enough for
// someone who just landed to read it, gone before the clip changes. It is
// hidden with the same command !middle hide sends, so a message put up by
// hand inside that window goes with it — the welcome is the one that should
// win in that rare overlap.
const raidWelcome = 20 * time.Second

// welcomeRaid greets the raid's audience on the stream itself, where they are
// looking, not only in a chat they may not have open yet.
func (a *App) welcomeRaid(ctx context.Context, from string) {
	if a.Onscreens == nil {
		return
	}
	a.Onscreens.ShowMiddleText(ctx, fmt.Sprintf("Welcome, raiders from @%s! 🚐", from))
	time.AfterFunc(raidWelcome, func() { a.Onscreens.HideMiddleText(context.Background()) })
}

// RecordRaid thanks the raiding channel in chat, publishes a raid event to the
// console (the chat line, the TV banner, the push), and logs a raid event
// stamped with the airing clip and playhead, so audience rollups can separate
// a raid's viewer spike from footage that genuinely draws viewers. Wired from
// pkg/eventsub on channel.raid events, and puts a welcome on the overlay. A
// repeat raid from the same channel inside raidFoldWindow is still logged
// but neither thanked, published nor welcomed again.
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
		a.welcomeRaid(ctx, from)
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
