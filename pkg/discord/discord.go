// Package discord runs the tripbot Discord bot session — a small set of
// slash commands that mirror tripbot's read-only Twitch leaderboard
// commands. It is intentionally additive: every failure path logs and
// returns so tripbot's core IRC / EventSub paths are never blocked or
// crashed by Discord being misconfigured or unreachable.
package discord

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/bwmarrin/discordgo"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
)

// FlagKey is the feature flag that gates Discord bot startup. cmd/tripbot
// evaluates it after the config-shaped ShouldStart check; when false (the
// default until a row is inserted), startup is skipped and the bot stays
// idle even in an env whose config is fully wired.
const FlagKey = "discord.bot_enabled"

// ShouldStart inspects the loaded config and returns whether to bring up
// the Discord session. Returns (false, reason) for the three
// intentionally-disabled cases (missing token, missing guild id,
// unfilled SM placeholder) so the caller can log a single INFO line
// rather than the gateway thrashing on auth errors.
func ShouldStart(cfg *c.TripbotConfig) (bool, string) {
	if cfg.DiscordBotToken == "" {
		return false, "token unset"
	}
	if cfg.DiscordGuildID == "" {
		return false, "guild_id unset"
	}
	// The SM container created by terraform writes this literal string
	// until aws secretsmanager put-secret-value is run. ESO syncs it
	// faithfully, so we'd otherwise try to auth with garbage.
	if strings.HasPrefix(cfg.DiscordBotToken, "placeholder") {
		return false, "token is SM placeholder"
	}
	return true, ""
}

// Session wraps a discordgo session plus the guild we register
// commands against. Construct with New, then call Start; call Stop on
// process shutdown.
type Session struct {
	s              *discordgo.Session
	guildID        string
	registeredCmds []*discordgo.ApplicationCommand
}

// New constructs the underlying discordgo session. Does not open the
// gateway — call Start for that.
func New(token, guildID string) (*Session, error) {
	s, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, fmt.Errorf("discordgo.New: %w", err)
	}
	// Slash commands don't need any privileged intents, and we don't
	// consume normal message events.
	s.Identify.Intents = discordgo.IntentsNone
	// Diagnostic: log gateway-level events so we can see whether
	// INTERACTION_CREATE is reaching us. Bump to LogDebug if even
	// informational chatter doesn't surface the interaction dispatch.
	s.LogLevel = discordgo.LogInformational
	return &Session{s: s, guildID: guildID}, nil
}

// Start opens the gateway, registers the slash commands against the
// configured guild, and attaches the interaction handler. Returns once
// initial setup is complete; the session goroutine continues running
// inside discordgo until Stop is called or ctx is canceled.
func (s *Session) Start(ctx context.Context) error {
	s.s.AddHandler(s.handleInteraction)

	if err := s.s.Open(); err != nil {
		return fmt.Errorf("discord open: %w", err)
	}
	slog.InfoContext(ctx, "discord session opened",
		"bot", s.s.State.User.Username,
		"guild_id", s.guildID,
	)

	registered, err := s.s.ApplicationCommandBulkOverwrite(s.s.State.User.ID, s.guildID, commandDefinitions())
	if err != nil {
		// Don't tear the session down — viewers can still see the bot
		// online; we just won't have any commands to invoke. Surface
		// loudly so the failure is obvious in logs.
		slog.ErrorContext(ctx, "discord command registration failed",
			"err", err,
			"guild_id", s.guildID,
		)
		return nil
	}
	s.registeredCmds = registered
	slog.InfoContext(ctx, "discord commands registered",
		"count", len(registered),
		"guild_id", s.guildID,
	)

	// ctx cancellation closes the session cleanly even if Stop isn't
	// reached (e.g. main returns from a panic path).
	go func() {
		<-ctx.Done()
		if err := s.s.Close(); err != nil {
			slog.WarnContext(ctx, "discord close on ctx cancel", "err", err)
		}
	}()

	return nil
}

// Stop closes the gateway connection. Safe to call multiple times.
//
// Deliberately does NOT deregister commands: ApplicationCommandBulkOverwrite
// matches commands by name and updates them in-place, so the new pod's
// registered command IDs are identical to the old pod's. Deleting on shutdown
// would wipe out the just-registered commands of the incoming pod on every
// rollout restart — leaving zero commands in Discord even though both pods
// successfully called register. Commands persisting across pod cycles is the
// correct shape; the next Start() reconciles them via BulkOverwrite.
func (s *Session) Stop() error {
	if s == nil || s.s == nil {
		return nil
	}
	if err := s.s.Close(); err != nil {
		return fmt.Errorf("discord close: %w", err)
	}
	slog.Info("discord session closed")
	return nil
}
