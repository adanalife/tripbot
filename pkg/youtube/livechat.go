package youtube

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"google.golang.org/api/googleapi"
	ytapi "google.golang.org/api/youtube/v3"
)

// ErrNoActiveBroadcast is returned by ActiveLiveChatID when the channel has
// no active broadcast (or it has chat disabled). Callers treat it as "not
// live right now" and retry on their own cadence rather than failing hard.
var ErrNoActiveBroadcast = errors.New("youtube: no active broadcast with live chat")

// ErrChatGone is returned by ListChatMessages when the bound live chat no
// longer exists — the broadcast ended or chat was disabled. Callers unbind
// and go back to broadcast discovery.
var ErrChatGone = errors.New("youtube: live chat ended or not found")

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

// LiveChatMessage is one inbound chat message in provider-neutral shape, so
// pkg/chatbot's poller never sees Google API types.
type LiveChatMessage struct {
	AuthorChannelID string // for filtering the bot's own echoed sends
	Author          string // display name
	Text            string // raw message text
}

// LiveChatPage is one liveChatMessages.list response: the text messages, the
// cursor for the next page, and how long YouTube asks us to wait before
// fetching it.
type LiveChatPage struct {
	Messages      []LiveChatMessage
	NextPageToken string
	PollAfter     time.Duration
}

// ListChatMessages fetches one page of the live chat. Only textMessageEvent
// items are returned — Super Chats, membership milestones etc. are not
// command input. A 403/404 meaning "this chat is over" maps to ErrChatGone;
// quota errors pass through unmapped (check with IsQuotaError) so callers
// back off instead of spinning rediscovery.
func (cl *Client) ListChatMessages(ctx context.Context, chatID, pageToken string) (*LiveChatPage, error) {
	svc, err := cl.Service(ctx)
	if err != nil {
		return nil, err
	}
	call := svc.LiveChatMessages.List(chatID, []string{"snippet", "authorDetails"})
	if pageToken != "" {
		call = call.PageToken(pageToken)
	}
	resp, err := call.Context(ctx).Do()
	if err != nil {
		if isChatGone(err) {
			return nil, ErrChatGone
		}
		return nil, err
	}

	page := &LiveChatPage{
		NextPageToken: resp.NextPageToken,
		PollAfter:     time.Duration(resp.PollingIntervalMillis) * time.Millisecond,
	}
	for _, item := range resp.Items {
		if item.Snippet == nil || item.Snippet.Type != "textMessageEvent" || item.Snippet.TextMessageDetails == nil {
			continue
		}
		var author, authorID string
		if item.AuthorDetails != nil {
			author = item.AuthorDetails.DisplayName
			authorID = item.AuthorDetails.ChannelId
		}
		page.Messages = append(page.Messages, LiveChatMessage{
			AuthorChannelID: authorID,
			Author:          author,
			Text:            item.Snippet.TextMessageDetails.MessageText,
		})
	}
	return page, nil
}

// isChatGone reports whether err means the live chat is over (broadcast
// ended, chat disabled, or chat ID no longer resolves) — distinct from a 403
// that means quota exhaustion.
func isChatGone(err error) bool {
	var gerr *googleapi.Error
	if !errors.As(err, &gerr) {
		return false
	}
	for _, e := range gerr.Errors {
		switch e.Reason {
		case "liveChatEnded", "liveChatDisabled", "liveChatNotFound", "notFound":
			return true
		}
	}
	return gerr.Code == 404
}

// IsQuotaError reports whether err is a YouTube Data API quota / rate-limit
// rejection. Callers back off rather than retrying at the normal cadence.
func IsQuotaError(err error) bool {
	var gerr *googleapi.Error
	if !errors.As(err, &gerr) {
		return false
	}
	for _, e := range gerr.Errors {
		switch e.Reason {
		case "quotaExceeded", "rateLimitExceeded", "userRateLimitExceeded":
			return true
		}
	}
	return false
}
