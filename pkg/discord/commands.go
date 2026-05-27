package discord

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/bwmarrin/discordgo"

	"github.com/adanalife/tripbot/pkg/scoreboards"
	"github.com/adanalife/tripbot/pkg/users"
)

const leaderboardSize = 10

// commandSummary backs the /commands enumeration. New leaderboard
// commands added here also flow into /commands automatically.
type commandSummary struct {
	name        string
	description string
}

// catalogue is the source of truth for both ApplicationCommand
// registration and the /commands ephemeral reply. /commands itself is
// intentionally omitted from the catalogue — listing it inside its own
// reply is just noise.
var catalogue = []commandSummary{
	{name: "leaderboard", description: "Top miles this month"},
	{name: "totalleaderboard", description: "Top miles of all time"},
	{name: "guessleaderboard", description: "Top correct guesses this month"},
}

// commandDefinitions returns the discordgo.ApplicationCommand list to
// register against the guild. Re-registration is idempotent — Discord
// overwrites by name on each ApplicationCommandBulkOverwrite call.
func commandDefinitions() []*discordgo.ApplicationCommand {
	defs := make([]*discordgo.ApplicationCommand, 0, len(catalogue)+1)
	for _, c := range catalogue {
		defs = append(defs, &discordgo.ApplicationCommand{
			Name:        c.name,
			Description: c.description,
		})
	}
	defs = append(defs, &discordgo.ApplicationCommand{
		Name:        "commands",
		Description: "List the available tripbot Discord commands",
	})
	return defs
}

// handleInteraction is the single dispatch point for slash command
// invocations. Unknown command names get an ephemeral reply rather
// than silently no-op'ing.
func (s *Session) handleInteraction(sess *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand {
		return
	}
	ctx := context.Background()
	name := i.ApplicationCommandData().Name

	switch name {
	case "leaderboard":
		s.runLeaderboard(ctx, i, "Monthly Miles", scoreboards.CurrentMilesScoreboard, false)
	case "totalleaderboard":
		s.runLifetimeLeaderboard(ctx, i)
	case "guessleaderboard":
		s.runLeaderboard(ctx, i, "Correct Guesses This Month", scoreboards.CurrentGuessScoreboard, true)
	case "commands":
		s.runCommands(ctx, i)
	default:
		slog.WarnContext(ctx, "unknown discord command", "command", name)
		s.replyEphemeral(ctx, i, "Unknown command.")
	}
}

// runLeaderboard handles the two scoreboard-backed leaderboards
// (monthly miles, monthly guesses). filterZeros mirrors the
// guess-leaderboard transform in pkg/chatbot/commands.go:307 — only
// users with non-zero scores are listed.
func (s *Session) runLeaderboard(
	ctx context.Context,
	i *discordgo.InteractionCreate,
	title string,
	scoreboardFn func() string,
	filterZeros bool,
) {
	if err := s.deferReply(i); err != nil {
		slog.ErrorContext(ctx, "discord defer reply failed", "err", err, "command", title)
		return
	}
	entries := scoreboards.TopUsers(ctx, scoreboardFn(), leaderboardSize)
	if filterZeros {
		entries = filterNonZeroInts(entries)
	}
	if len(entries) == 0 {
		s.editReplyText(ctx, i, "No one is on that leaderboard yet!")
		return
	}
	s.editReplyEmbed(ctx, i, leaderboardEmbed(title, entries))
}

// runLifetimeLeaderboard reads from the in-memory
// users.LifetimeMilesLeaderboard slice that's hydrated at startup
// (cmd/tripbot/tripbot.go calls users.InitLeaderboard) and refreshed
// by the users.UpdateLeaderboard cron. No DB query needed at command
// time.
func (s *Session) runLifetimeLeaderboard(ctx context.Context, i *discordgo.InteractionCreate) {
	if err := s.deferReply(i); err != nil {
		slog.ErrorContext(ctx, "discord defer reply failed", "err", err, "command", "totalleaderboard")
		return
	}
	entries := users.LifetimeMilesLeaderboard
	if len(entries) > leaderboardSize {
		entries = entries[:leaderboardSize]
	}
	if len(entries) == 0 {
		s.editReplyText(ctx, i, "No one is on that leaderboard yet!")
		return
	}
	s.editReplyEmbed(ctx, i, leaderboardEmbed("Total Miles", entries))
}

// runCommands renders the static catalogue as an ephemeral embed.
// Pure-function reply — no DB, no async, no risk of timing out the
// 3-second initial response window. Discord renders it only to the
// invoker via MessageFlagsEphemeral.
func (s *Session) runCommands(ctx context.Context, i *discordgo.InteractionCreate) {
	var body string
	for _, c := range catalogue {
		body += fmt.Sprintf("**/%s** — %s\n", c.name, c.description)
	}
	embed := &discordgo.MessageEmbed{
		Title:       "TripBot commands",
		Description: body,
		Color:       0xff7a00,
	}
	err := s.s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
			Flags:  discordgo.MessageFlagsEphemeral,
		},
	})
	if err != nil {
		slog.ErrorContext(ctx, "discord /commands reply failed", "err", err)
	}
}

// deferReply tells Discord "I'll respond shortly" — gives us the full
// 15-minute interaction-token window instead of the 3-second initial
// window. Use for any handler that hits the DB or otherwise might
// exceed 3 seconds.
func (s *Session) deferReply(i *discordgo.InteractionCreate) error {
	return s.s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
}

func (s *Session) editReplyEmbed(ctx context.Context, i *discordgo.InteractionCreate, embed *discordgo.MessageEmbed) {
	embeds := []*discordgo.MessageEmbed{embed}
	if _, err := s.s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Embeds: &embeds}); err != nil {
		slog.ErrorContext(ctx, "discord edit embed reply failed", "err", err)
	}
}

func (s *Session) editReplyText(ctx context.Context, i *discordgo.InteractionCreate, text string) {
	if _, err := s.s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &text}); err != nil {
		slog.ErrorContext(ctx, "discord edit text reply failed", "err", err)
	}
}

func (s *Session) replyEphemeral(ctx context.Context, i *discordgo.InteractionCreate, text string) {
	err := s.s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: text,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
	if err != nil {
		slog.ErrorContext(ctx, "discord ephemeral reply failed", "err", err)
	}
}
