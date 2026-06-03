package twitch

import (
	"context"
	"errors"
	"fmt"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/nicklaw5/helix/v2"
)

// SendChatMessageAsBroadcaster posts text to the channel's chat as the
// broadcaster identity, via Helix "Send Chat Message" (requires user:write:chat
// on the broadcaster token — see BroadcasterScopes). The broadcaster is both
// the channel and the sender, so BroadcasterID and SenderID are the same id.
//
// Unlike the bot's IRC Say, a Helix-sent message is read back by the bot's own
// chat connection like any other viewer line, so it surfaces in the admin live
// console through the normal chat.message path — no separate console mirror is
// needed here. Errors (missing token, missing scope, rate limit, a dropped
// message) are returned so the caller can log them; the admin console's
// optimistic line will time out and redden when a send never lands.
func (cl *API) SendChatMessageAsBroadcaster(ctx context.Context, text string) error {
	if !cl.broadcasterTokenLoaded() {
		return errors.New("no broadcaster oauth_tokens row")
	}
	bclient, err := cl.BroadcasterClient()
	if err != nil {
		return fmt.Errorf("broadcaster helix client unavailable: %w", err)
	}
	if cl.channelID == "" {
		cl.channelID = cl.getChannelID(c.Conf.ChannelName)
	}
	resp, err := bclient.SendChatMessage(&helix.SendChatMessageParams{
		BroadcasterID: cl.channelID,
		SenderID:      cl.channelID,
		Message:       text,
	})
	if err != nil {
		return fmt.Errorf("send chat message: %w", err)
	}
	if cl.checkHelixResp(ctx, "SendChatMessage", "broadcaster", &resp.ResponseCommon) {
		return fmt.Errorf("send chat message: helix returned %d: %s", resp.StatusCode, resp.ErrorMessage)
	}
	// Twitch can accept the request (HTTP 200) but still drop the message
	// (e.g. a follower-only/slow-mode rejection); IsSent=false carries the why.
	if len(resp.Data.Messages) > 0 && !resp.Data.Messages[0].IsSent {
		dr := resp.Data.Messages[0].DropReasons.Data
		return fmt.Errorf("message dropped: %s %s", dr.Code, dr.Message)
	}
	return nil
}
