package twitch

import (
	"slices"
	"strings"

	"github.com/adanalife/tripbot/pkg/instrumentation"
	"github.com/nicklaw5/helix/v2"
)

// SetSubscribers replaces the cached subscriber list with logins sourced from
// the platform-gateway (which polls Helix). Logins are lowercased so
// UserIsSubscriber compares consistently. The audience gauge is set here too.
func (cl *API) SetSubscribers(logins []string) {
	subs := make([]string, len(logins))
	for i, login := range logins {
		subs[i] = strings.ToLower(login)
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
	return slices.Contains(cl.subscribers, username)
}
