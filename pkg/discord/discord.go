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

// Stop deregisters the per-guild slash commands and closes the gateway
// connection. Safe to call multiple times.
func (s *Session) Stop() error {
	if s == nil || s.s == nil {
		return nil
	}
	for _, cmd := range s.registeredCmds {
		if err := s.s.ApplicationCommandDelete(s.s.State.User.ID, s.guildID, cmd.ID); err != nil {
			slog.Warn("discord command deregister failed", "command", cmd.Name, "err", err)
		}
	}
	s.registeredCmds = nil
	if err := s.s.Close(); err != nil {
		return fmt.Errorf("discord close: %w", err)
	}
	slog.Info("discord session closed")
	return nil
}
