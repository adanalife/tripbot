package chatbot

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/adanalife/tripbot/pkg/eventbus"
	"github.com/adanalife/tripbot/pkg/events"
)

// AnnounceNewFollower says a thank-you to a new follower in chat and publishes
// a follow event to the console. Wired from pkg/eventsub on channel.follow v2
// events.
func (a *App) AnnounceNewFollower(username string) {
	a.Chat.Say(fmt.Sprintf("Thank you for the follow, @%s", username))
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
	a.UserSessions.GiveEveryoneMiles(1.0)
	a.Chat.Say(fmt.Sprintf("The %d current viewers have been given a bonus mile, too HolidayPresent", a.UserSessions.LoggedInCount()))
	if err := events.Subscribe(context.Background(), a.Cfg, username); err != nil {
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
	if err := events.Unsubscribe(context.Background(), a.Cfg, username); err != nil {
		slog.ErrorContext(context.Background(), "error creating unsubscribe event", "err", err)
	}
}
