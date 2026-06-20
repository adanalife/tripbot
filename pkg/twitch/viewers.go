package twitch

import (
	"context"
	"log/slog"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/nicklaw5/helix/v2"
)

// ChannelID returns the cached twitch-internal user ID for the channel. It is
// populated lazily by UpdateChatters / GetSubscribers; "" until then. Exposed
// for cmd/tripbot's EventSub subscription setup.
func (cl *API) ChannelID() string {
	return cl.channelID
}

// SetChannelID seeds the cached channel ID from out-of-band (the
// platform-gateway's /v1/users/{login}) instead of an in-process getChannelID
// lookup. Needed when Helix calls route through the gateway — the in-process
// audience polls that used to populate channelID as a side effect no longer
// run, so EventSub setup would otherwise see "".
func (cl *API) SetChannelID(id string) {
	cl.channelID = id
}

// ChatterCount returns the number of chatters as reported by Twitch.
func (cl *API) ChatterCount() int {
	return cl.chatterCount
}

// Chatters returns a set of current chatter logins.
func (cl *API) Chatters() map[string]struct{} {
	chatters := make(map[string]struct{})
	for _, chatter := range cl.currentChatters {
		chatters[chatter.UserLogin] = struct{}{}
	}
	return chatters
}

// UpdateChatters fetches the current chatter list via the Helix chat/chatters
// endpoint and updates the in-memory state. Requires the bot account to be a
// moderator of the channel (moderator:read:chatters scope).
func (cl *API) UpdateChatters() {
	client, err := cl.Client()
	if err != nil {
		slog.Error("twitch API client unavailable", "err", err)
		return
	}
	if cl.channelID == "" {
		cl.channelID = cl.getChannelID(c.Conf.ChannelName)
	}
	if cl.botID == "" {
		cl.botID = cl.getChannelID(c.Conf.BotUsername)
	}

	resp, err := client.GetChannelChatChatters(&helix.GetChatChattersParams{
		BroadcasterID: cl.channelID,
		ModeratorID:   cl.botID,
	})
	if err != nil {
		slog.Error("error getting chatters from twitch", "err", err)
		return
	}
	if cl.checkHelixResp(context.Background(), "GetChannelChatChatters", "bot", &resp.ResponseCommon) {
		// don't overwrite cached chatter state with an empty response —
		// 4xx here means the bot lost a scope or moderator role and the
		// next call probably succeeds once that's fixed.
		return
	}

	cl.currentChatters = resp.Data.Chatters
	cl.chatterCount = resp.Data.Total
}
