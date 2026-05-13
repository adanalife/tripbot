package twitch

import (
	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	terrors "github.com/adanalife/tripbot/pkg/errors"
	"github.com/nicklaw5/helix/v2"
)

// BotID is the Twitch user ID for the bot account (moderator identity for
// API calls that require moderator:read:chatters).
var BotID string

// currentChatters holds the most recent chatter list from the Helix API.
var currentChatters []helix.ChatChatter

// chatterCount is the total reported by the API (may exceed len(currentChatters)
// if the channel has more than the default page size of chatters).
var chatterCount int

// ChatterCount returns the number of chatters as reported by Twitch.
func ChatterCount() int {
	return chatterCount
}

// Chatters returns a set of current chatter logins.
func Chatters() map[string]struct{} {
	chatters := make(map[string]struct{})
	for _, chatter := range currentChatters {
		chatters[chatter.UserLogin] = struct{}{}
	}
	return chatters
}

// UpdateChatters fetches the current chatter list via the Helix chat/chatters
// endpoint and updates the in-memory state. Requires the bot account to be a
// moderator of the channel (moderator:read:chatters scope).
func UpdateChatters() {
	if ChannelID == "" {
		ChannelID = getChannelID(c.Conf.ChannelName)
	}
	if BotID == "" {
		BotID = getChannelID(c.Conf.BotUsername)
	}

	resp, err := currentTwitchClient.GetChannelChatChatters(&helix.GetChatChattersParams{
		BroadcasterID: ChannelID,
		ModeratorID:   BotID,
	})
	if err != nil {
		terrors.Log(err, "error getting chatters from twitch")
		return
	}

	currentChatters = resp.Data.Chatters
	chatterCount = resp.Data.Total
}
