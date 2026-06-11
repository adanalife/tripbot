package youtube

import (
	"context"
	"errors"
	"log/slog"

	ytapi "google.golang.org/api/youtube/v3"
)

// ErrNoActiveBroadcast is returned by ActiveLiveChatID when the channel has
// no active broadcast (or it has chat disabled). Callers treat it as "not
// live right now" and retry on their own cadence rather than failing hard.
var ErrNoActiveBroadcast = errors.New("youtube: no active broadcast with live chat")

// maxChatMessageLen is YouTube's per-message limit for
// liveChatMessages.insert (200 characters; the API 400s above it). Twitch
// allows 500, so command output written for Twitch can exceed this —
// InsertChatMessage truncates rather than letting the send fail.
const maxChatMessageLen = 200

// ActiveLiveChatID finds the channel's active broadcast and returns its live
// chat ID. liveBroadcasts.list's filters (mine, broadcastStatus, id) are
// mutually exclusive — broadcastStatus=active is already scoped to the
// authorized channel, so it's the only filter set. Works for unlisted
// broadcasts (the quiet-testing surface) the same as public ones.
func (cl *Client) ActiveLiveChatID(ctx context.Context) (string, error) {
	svc, err := cl.Service(ctx)
	if err != nil {
		return "", err
	}
	resp, err := svc.LiveBroadcasts.List([]string{"snippet"}).
		BroadcastStatus("active").
		BroadcastType("all").
		Context(ctx).
		Do()
	if err != nil {
		return "", err
	}
	if len(resp.Items) == 0 || resp.Items[0].Snippet.LiveChatId == "" {
		return "", ErrNoActiveBroadcast
	}
	return resp.Items[0].Snippet.LiveChatId, nil
}

// InsertChatMessage posts one message to the given live chat as the channel
// owner. Messages over YouTube's 200-character limit are truncated (with a
// log) instead of failing the send.
func (cl *Client) InsertChatMessage(ctx context.Context, chatID, text string) error {
	svc, err := cl.Service(ctx)
	if err != nil {
		return err
	}
	if r := []rune(text); len(r) > maxChatMessageLen {
		slog.WarnContext(ctx, "youtube chat message truncated to 200 chars", "text", text)
		text = string(r[:maxChatMessageLen-1]) + "…"
	}
	_, err = svc.LiveChatMessages.Insert([]string{"snippet"}, &ytapi.LiveChatMessage{
		Snippet: &ytapi.LiveChatMessageSnippet{
			LiveChatId: chatID,
			Type:       "textMessageEvent",
			TextMessageDetails: &ytapi.LiveChatTextMessageDetails{
				MessageText: text,
			},
		},
	}).Context(ctx).Do()
	return err
}
