package chatbot

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/geo"
	myyoutube "github.com/adanalife/tripbot/pkg/youtube"
)

// liveChatBinding holds the currently-bound live chat ID, shared between the
// outbound youtubeChat (Say targets it) and the inbound poller (which
// discovers and re-binds it across broadcast lifecycles — Phase B3). Empty
// means "not live right now": sends drop, the poller keeps re-discovering.
type liveChatBinding struct {
	mu sync.RWMutex
	id string
}

func (b *liveChatBinding) ID() string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.id
}

func (b *liveChatBinding) Bind(id string) {
	b.mu.Lock()
	b.id = id
	b.mu.Unlock()
}

// youtubeChat sends to YouTube live chat through pkg/youtube, implementing
// the provider-neutral ChatClient seam — the second provider the seam was
// built for. The insert func defaults to myyoutube.InsertChatMessage;
// tests inject a recorder.
type youtubeChat struct {
	binding *liveChatBinding
	insert  func(ctx context.Context, chatID, text string) error
}

func (yc youtubeChat) Say(msg string) {
	// Twitch-only IRC emote command (the Chatter cron prefixes help messages
	// with it); on YouTube it would render as literal text.
	msg = strings.TrimPrefix(msg, "/me ")

	chatID := yc.binding.ID()
	if chatID == "" {
		// Not live (or the poller hasn't bound the broadcast yet). Same
		// drop-quietly contract as disconnectedChat, but logged: a dropped
		// command response is worth seeing in Loki.
		slog.Warn("youtube chat send dropped; no live chat bound", "text", msg)
		return
	}
	if err := yc.insert(context.Background(), chatID, msg); err != nil {
		slog.Error("youtube chat send failed", "err", err, "text", msg)
	}
}

// Whisper is a no-op: YouTube live chat has no direct-message equivalent.
// The only production caller is HandleWhisper's admin remote-say, which is
// Twitch-inbound anyway.
func (yc youtubeChat) Whisper(username, msg string) {
	slog.Debug("youtube has no whispers; dropped", "to", username, "text", msg)
}

// ConnectYouTube wires this App's outbound chat to YouTube — the
// ConnectIRC analog for a PLATFORM=youtube instance. It loads the
// channel-owner token from the DB, binds the active broadcast's live chat
// (non-fatal when nothing is live: sends drop until the inbound poller —
// Phase B3 — binds it), and points a.Chat at the youtubeChat client wrapped
// in the provider-neutral console mirror.
//
// Returns the binding for the inbound poller to share. Errors only when the
// OAuth token is missing/unusable — the operator-facing signal to visit
// /auth/init?account=youtube.
func (a *App) ConnectYouTube(ctx context.Context) (*liveChatBinding, error) {
	Uptime = time.Now()

	// process-wide geocoder warmup, same as ConnectIRC (pkg/video routes
	// through the default for coords -> places).
	geo.SetDefault(geo.New(c.Conf.GoogleMapsAPIKey))

	if err := myyoutube.LoadFromDB(); err != nil {
		return nil, err
	}

	binding := &liveChatBinding{}
	chatID, err := myyoutube.ActiveLiveChatID(ctx)
	switch {
	case err == nil:
		binding.Bind(chatID)
		slog.InfoContext(ctx, "bound youtube live chat", "live_chat_id", chatID)
	case errors.Is(err, myyoutube.ErrNoActiveBroadcast):
		slog.WarnContext(ctx, "no active youtube broadcast at startup; chat sends drop until one is bound")
	default:
		// Discovery failed for a non-"not live" reason (network, auth).
		// Still come up — the poller retries — but say why loudly.
		slog.ErrorContext(ctx, "youtube live-chat discovery failed at startup", "err", err)
	}

	a.Chat = consoleMirror{
		inner: youtubeChat{
			binding: binding,
			insert:  myyoutube.InsertChatMessage,
		},
		env:         c.Conf.Environment,
		botUsername: c.Conf.BotUsername,
	}
	return binding, nil
}
