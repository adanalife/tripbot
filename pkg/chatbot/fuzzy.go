package chatbot

import (
	"strings"
)

// fuzzyMaxDistance returns the maximum edit distance allowed when fuzzy-
// matching an input of the given rune length (including the "!"). Inputs
// shorter than 4 runes never fuzzy-match: at that length a single edit
// reaches too many unrelated triggers.
func fuzzyMaxDistance(length int) int {
	switch {
	case length < 4:
		return 0
	case length <= 6:
		return 1
	default:
		return 2
	}
}

// fuzzyLookup returns the registered command whose trigger or alias is
// closest to command by edit distance, or nil when there is no unambiguous
// match within fuzzyMaxDistance. Only !-prefixed triggers are considered, so
// bare-word triggers ("hello") can't be reached by typo. A tie between two
// *different* commands at the best distance returns nil rather than guessing
// (e.g. "!tate" is one edit from both !date and !state).
func (a *App) fuzzyLookup(command string) *Command {
	maxDist := fuzzyMaxDistance(len([]rune(command)))
	if maxDist == 0 {
		return nil
	}

	var best *Command
	bestDist := maxDist + 1
	ambiguous := false
	for trigger, cmd := range a.singleWordLookup {
		if !strings.HasPrefix(trigger, "!") {
			continue
		}
		dist := levenshtein(command, trigger)
		if dist > maxDist {
			continue
		}
		switch {
		case dist < bestDist:
			best, bestDist, ambiguous = cmd, dist, false
		case dist == bestDist && cmd != best:
			ambiguous = true
		}
	}
	if ambiguous {
		return nil
	}
	return best
}

// levenshtein returns the edit distance (insertions, deletions,
// substitutions) between two strings, comparing rune-by-rune.
func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	curr := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		curr[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			curr[j] = min(prev[j]+1, curr[j-1]+1, prev[j-1]+cost)
		}
		prev, curr = curr, prev
	}
	return prev[len(rb)]
}
