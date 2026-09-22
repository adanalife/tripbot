package discord

import (
	"fmt"
	"strings"

	"github.com/adanalife/tripbot/pkg/scoreboards"
	"github.com/bwmarrin/discordgo"
)

// leaderboardEmbed builds a Discord embed for a [username, value] pair
// list. The shape mirrors the IRC text format used in
// pkg/chatbot/commands.go for the three leaderboard commands. Returns
// nil when entries is empty — caller decides whether to send a fallback
// message instead.
func leaderboardEmbed(title string, entries [][]string) *discordgo.MessageEmbed {
	if len(entries) == 0 {
		return nil
	}
	// Drop the malformed rows before ranking, not while rendering: a skipped
	// row must not consume a place, or the survivors start at second.
	rows := make([][]string, 0, len(entries))
	for _, pair := range entries {
		if len(pair) < 2 {
			continue
		}
		rows = append(rows, pair)
	}
	if len(rows) == 0 {
		return nil
	}
	// Ties share the better place, matching what the chat commands say, so the
	// same board doesn't rank two level viewers differently in the two places.
	ranks := scoreboards.Ranks(rows)
	var b strings.Builder
	for i, pair := range rows {
		fmt.Fprintf(&b, "**%d.** %s — %s\n", ranks[i], pair[0], pair[1])
	}
	return &discordgo.MessageEmbed{
		Title:       title,
		Description: b.String(),
		Color:       0xff7a00,
	}
}
