package chatbot

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/adanalife/tripbot/pkg/events"
)

// AnnounceNewFollower says a thank-you to a new follower in chat. Wired
// from pkg/eventsub on channel.follow v2 events.
func (a *App) AnnounceNewFollower(username string) {
	a.Chat.Say(fmt.Sprintf("Thank you for the follow, @%s", username))
}

// AnnounceSubscriber says a thank-you to a new subscriber, gives every
// currently-logged-in viewer +1 mile, announces the bonus, and logs a
// subscribe event (opening the viewer's subscribed interval). Wired from
// pkg/eventsub on channel.subscribe events.
//
// isGift / tier round-trip from the EventSub payload so callers /
// future enhancements can branch on them; today only the username drives
// the chat shout. Gift-sub thanks (channel.subscription.gift) and
// resub-with-message handling (channel.subscription.message) are
// separate event types — wire them through pkg/eventsub.Handlers as
// they're added.
func (a *App) AnnounceSubscriber(username string, isGift bool, tier string) {
	_ = isGift
	_ = tier
	a.Chat.Say(fmt.Sprintf("Thank you for the sub, @%s; enjoy your !bonusmiles bleedPurple", username))
	a.UserSessions.GiveEveryoneMiles(1.0)
	a.Chat.Say(fmt.Sprintf("The %d current viewers have been given a bonus mile, too HolidayPresent", a.UserSessions.LoggedInCount()))
	if err := events.Subscribe(context.Background(), a.Cfg, username); err != nil {
		slog.ErrorContext(context.Background(), "error creating subscribe event", "err", err)
	}
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
