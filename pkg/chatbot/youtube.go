package chatbot

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	mylog "github.com/adanalife/tripbot/pkg/chatbot/log"
	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/eventbus"
	"github.com/adanalife/tripbot/pkg/gateway"
	"github.com/adanalife/tripbot/pkg/geo"
	"github.com/adanalife/tripbot/pkg/instrumentation"
	"github.com/adanalife/tripbot/pkg/users"
	myyoutube "github.com/adanalife/tripbot/pkg/youtube"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// liveChatBinding holds the currently-bound live chat ID, shared between the
// outbound youtubeChat (Say targets it) and the inbound poller (which
// discovers and re-binds it across broadcast lifecycles). Empty
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
// (non-fatal when nothing is live: sends drop until the inbound poller
// binds it), and points a.Chat at the youtubeChat client wrapped
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
			insert:  newYouTubeSend().send,
		},
		env:         c.Conf.Environment,
		platform:    c.Conf.Platform,
		botUsername: c.Conf.BotUsername,
	}
	return binding, nil
}

// youtubeSend is the outbound YouTube chat-send seam. The in-process path
// inserts into the tripbot-bound live chat (chatID); the gateway path posts via
// gateway-youtube's SendChat, which resolves the active live chat itself (so it
// ignores chatID). Same signature as youtubeChat.insert so youtubeChat is
// untouched.
type youtubeSend interface {
	send(ctx context.Context, chatID, text string) error
}

// newYouTubeSend wires the production send path. A youtube instance wired with
// YOUTUBE_API_URL routes sends through the platform-gateway (gateway-youtube)
// unconditionally — no runtime flag. Unlike the Twitch cutover (which keeps a
// flag as a de-risking tool for the live-prod swap), YouTube cuts straight over;
// a revert is a git revert + redeploy. With no YOUTUBE_API_URL there's no
// gateway to reach, so it falls back to the in-process insert — the zero-config
// default for an un-wired instance, which goes away once pkg/youtube is deleted
// (after the gateway owns the inbound poll too).
func newYouTubeSend() youtubeSend {
	if c.Conf.YouTubeAPIURL == "" {
		return realYouTubeSend{}
	}
	return gatewayYouTubeSend{client: gateway.New(c.Conf.YouTubeAPIURL)}
}

// realYouTubeSend is the in-process adapter — inserts into the bound live chat
// via pkg/youtube. Used when no YOUTUBE_API_URL is configured.
type realYouTubeSend struct{}

func (realYouTubeSend) send(ctx context.Context, chatID, text string) error {
	return myyoutube.InsertChatMessage(ctx, chatID, text)
}

// gatewayYouTubeSend posts through the platform-gateway gateway-youtube instance
// via the shared pkg/gateway client. chatID is ignored — the gateway-youtube
// adapter resolves the channel's active live chat itself. identity is "" so the
// gateway uses its default (YouTube has a single channel-owner token).
type gatewayYouTubeSend struct {
	client *gateway.Client
}

func (g gatewayYouTubeSend) send(ctx context.Context, _, text string) error {
	return g.client.SendChat(ctx, "", text)
}

// HandleYouTubeMessage processes one inbound YouTube chat message. Identical
// to HandleMessage except the login step: YouTube viewers are NOT logged in
// or persisted — v1 punts identity, presence, and miles entirely (see the
// platform allowlist), so the command path gets a transient User carrying
// just the display name. The Loki chat line, the admin-console event-bus
// mirror, and the metrics all stay.
func (a *App) HandleYouTubeMessage(ctx context.Context, msg IncomingMessage) {
	// span attribute key shared with the Twitch path for observability
	// continuity; renaming both to a platform-tagged key is the B4 pass.
	ctx, span := tracer.Start(ctx, "chatbot.handle_message",
		trace.WithAttributes(attribute.String("twitch.user", msg.User)))
	defer span.End()

	instrumentation.ChatMessages.Inc()
	mylog.ChatMsg(msg.User, msg.Text)
	eventbus.EmitChatMessage(ctx, c.Conf.Environment, a.Platform, msg.User, msg.Text)

	// transient, never written to the users table — the allowlisted command
	// subset reads nothing user-specific beyond the name.
	user := &users.User{Username: strings.ToLower(msg.User)}
	a.runCommand(ctx, user, strings.ToLower(msg.Text))
}

// youtubeChatPoller is the inbound transport for a PLATFORM=youtube
// instance: it discovers the active broadcast, binds its live chat (the same
// binding youtubeChat sends to), and pages through liveChatMessages.list at
// the server-suggested cadence, feeding each viewer message into the shared
// command path. The seam fields default to pkg/youtube in
// NewYouTubeChatPoller; tests inject fakes.
type youtubeChatPoller struct {
	app     *App
	binding *liveChatBinding

	discover     func(ctx context.Context) (string, error)
	list         func(ctx context.Context, chatID, pageToken string) (*myyoutube.LiveChatPage, error)
	ownChannelID func() string

	pollFloor      time.Duration // minimum wait between list calls
	rediscoverWait time.Duration // wait between discovery attempts while not live
	quotaWait      time.Duration // backoff after a quota rejection
}

// NewYouTubeChatPoller builds the production poller sharing this App and the
// binding returned by ConnectYouTube. Run it in a goroutine (cmd/tripbot's
// platform branch does).
func (a *App) NewYouTubeChatPoller(binding *liveChatBinding) *youtubeChatPoller {
	return &youtubeChatPoller{
		app:          a,
		binding:      binding,
		discover:     myyoutube.ActiveLiveChatID,
		list:         myyoutube.ListChatMessages,
		ownChannelID: myyoutube.ChannelID,
		// floor under the server-suggested interval; YouTube usually asks
		// for ~3-10s, this only guards against pathological responses.
		pollFloor:      2 * time.Second,
		rediscoverWait: time.Minute,
		quotaWait:      5 * time.Minute,
	}
}

// Run polls until ctx is done. Lifecycle: unbound → discover (quietly idle
// while the channel isn't live) → bind → page through chat → on ErrChatGone
// (broadcast ended) unbind and rediscover. The first page after every bind
// is discarded: liveChatMessages.list opens with recent history, and
// replaying an hour-old !skip against the live pipeline would be wrong.
func (p *youtubeChatPoller) Run(ctx context.Context) {
	pageToken := ""
	fresh := true // next page is the post-bind backlog page

	for ctx.Err() == nil {
		chatID := p.binding.ID()
		if chatID == "" {
			id, err := p.discover(ctx)
			switch {
			case err == nil:
				p.binding.Bind(id)
				pageToken = ""
				fresh = true
				slog.InfoContext(ctx, "youtube poller bound live chat", "live_chat_id", id)
				continue
			case errors.Is(err, myyoutube.ErrNoActiveBroadcast):
				// not live is the normal idle state; stay quiet
			default:
				slog.ErrorContext(ctx, "youtube broadcast discovery failed", "err", err)
			}
			if !sleepCtx(ctx, p.rediscoverWait) {
				return
			}
			continue
		}

		page, err := p.list(ctx, chatID, pageToken)
		if err != nil {
			switch {
			case errors.Is(err, myyoutube.ErrChatGone):
				slog.InfoContext(ctx, "youtube live chat ended; rediscovering")
				p.binding.Bind("")
				pageToken = ""
			case myyoutube.IsQuotaError(err):
				slog.ErrorContext(ctx, "youtube quota rejection; backing off", "err", err)
				if !sleepCtx(ctx, p.quotaWait) {
					return
				}
			case errors.Is(err, context.Canceled):
				return
			default:
				slog.ErrorContext(ctx, "youtube chat poll failed", "err", err)
				if !sleepCtx(ctx, p.rediscoverWait) {
					return
				}
			}
			continue
		}
		pageToken = page.NextPageToken

		if fresh {
			slog.InfoContext(ctx, "youtube poller skipped backlog page", "count", len(page.Messages))
			fresh = false
		} else {
			own := p.ownChannelID()
			for _, m := range page.Messages {
				// the bot posts as the channel owner, so its own sends echo
				// back in the list — already mirrored by consoleMirror.
				if own != "" && m.AuthorChannelID == own {
					continue
				}
				p.app.HandleYouTubeMessage(ctx, IncomingMessage{User: m.Author, Text: m.Text})
			}
		}

		wait := page.PollAfter
		if wait < p.pollFloor {
			wait = p.pollFloor
		}
		if !sleepCtx(ctx, wait) {
			return
		}
	}
}

// sleepCtx waits d or until ctx is done; false means ctx ended first.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
