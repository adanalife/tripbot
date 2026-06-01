package chatbot

import (
	"fmt"
)

// AnnounceNewFollower says a thank-you to a new follower in chat. Wired
// from pkg/eventsub on channel.follow v2 events.
func AnnounceNewFollower(username string) {
	sayFn(fmt.Sprintf("Thank you for the follow, @%s", username))
}

// AnnounceSubscriber says a thank-you to a new subscriber, gives every
// currently-logged-in viewer +1 mile, and announces the bonus. Wired
// from pkg/eventsub on channel.subscribe events.
//
// isGift / tier round-trip from the EventSub payload so callers /
// future enhancements can branch on them; today only the username drives
// the chat shout. Gift-sub thanks (channel.subscription.gift) and
// resub-with-message handling (channel.subscription.message) are
// separate event types — wire them through pkg/eventsub.Handlers as
// they're added.
func AnnounceSubscriber(username string, isGift bool, tier string) {
	_ = isGift
	_ = tier
	sayFn(fmt.Sprintf("Thank you for the sub, @%s; enjoy your !bonusmiles bleedPurple", username))
	currentSessions().GiveEveryoneMiles(1.0)
	sayFn(fmt.Sprintf("The %d current viewers have been given a bonus mile, too HolidayPresent", currentSessions().LoggedInCount()))
}
