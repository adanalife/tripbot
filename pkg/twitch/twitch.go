package twitch

import (
	"strings"

	"github.com/adanalife/tripbot/pkg/instrumentation"
	"github.com/nicklaw5/helix/v2"
)

// SetSubscribers replaces the cached login → tier map with values sourced from
// the platform-gateway (which polls Helix). Logins are lowercased so
// UserIsSubscriber compares consistently; tiers below 1 are clamped to 1 (a
// listed subscriber is at least tier 1). The audience gauge is set here too.
func (cl *API) SetSubscribers(tiers map[string]int) {
	subs := make(map[string]int, len(tiers))
	for login, tier := range tiers {
		if tier < 1 {
			tier = 1
		}
		subs[strings.ToLower(login)] = tier
	}
	cl.audienceMu.Lock()
	cl.subscribers = subs
	cl.audienceMu.Unlock()
	instrumentation.TwitchAudience.SetSubscribers(int64(len(subs)))
}

// SetChatters replaces the cached chatter set and total with values sourced
// from the platform-gateway. count is the authoritative total (it may exceed
// len(logins) when the channel has more chatters than one page); Chatters()
// reads the logins, ChatterCount() reads the total.
func (cl *API) SetChatters(logins []string, count int) {
	chatters := make([]helix.ChatChatter, len(logins))
	for i, login := range logins {
		chatters[i] = helix.ChatChatter{UserLogin: login}
	}
	cl.audienceMu.Lock()
	cl.currentChatters = chatters
	cl.chatterCount = count
	cl.audienceMu.Unlock()
}

// UserIsSubscriber returns true if the user subscribes to the channel.
func (cl *API) UserIsSubscriber(username string) bool {
	cl.audienceMu.RLock()
	defer cl.audienceMu.RUnlock()
	_, ok := cl.subscribers[username]
	return ok
}

// UserSubscriberTier returns the user's subscription tier (1–3), or 0 if they
// don't subscribe to the channel.
func (cl *API) UserSubscriberTier(username string) int {
	cl.audienceMu.RLock()
	defer cl.audienceMu.RUnlock()
	return cl.subscribers[username]
}
