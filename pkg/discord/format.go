package discord

import (
	"fmt"
	"strings"

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
	var b strings.Builder
	rank := 0
	for _, pair := range entries {
		if len(pair) < 2 {
			continue
		}
		rank++
		fmt.Fprintf(&b, "**%d.** %s — %s\n", rank, pair[0], pair[1])
	}
	if rank == 0 {
		return nil
	}
	return &discordgo.MessageEmbed{
		Title:       title,
		Description: b.String(),
		Color:       0xff7a00,
	}
}

// filterNonZeroInts strips entries whose value is "0" or empty and
// drops the decimal portion. Mirrors the guess-leaderboard transform
// in pkg/chatbot/commands.go:307-315 (guesses are stored as floats
// but always whole numbers, and AddToScoreByName creates 0-rows for
// everyone who's ever guessed).
func filterNonZeroInts(entries [][]string) [][]string {
	var out [][]string
	for _, pair := range entries {
		if len(pair) < 2 {
			continue
		}
		intVersion := strings.Split(pair[1], ".")[0]
		if intVersion == "0" || intVersion == "" {
			continue
		}
		out = append(out, []string{pair[0], intVersion})
	}
	return out
}
