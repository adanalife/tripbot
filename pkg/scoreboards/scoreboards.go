package scoreboards

import (
	"context"
	"fmt"
	"log/slog"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/database"
)

type topUserResult struct {
	Username string
	Value    float32
}

func TopUsers(ctx context.Context, cfg *c.TripbotConfig, scoreboardName string, size int) [][]string {
	var results []topUserResult
	result := database.GormDB().WithContext(ctx).
		Table("scores").
		Select("users.username, scores.value").
		Joins("JOIN scoreboards ON scores.scoreboard_id = scoreboards.id").
		Joins("JOIN users ON scores.user_id = users.id").
		Where("scoreboards.name = ? AND scoreboards.platform = ?", scoreboardName, cfg.Platform).
		// users.platform too: scores written before boards were per-platform
		// may hang off the other platform's same-named board.
		Where("users.is_bot = false AND users.exclude_from_leaderboard = false AND users.platform = ? AND users.username != ?", cfg.Platform, cfg.ChannelName).
		// username breaks the tie: without it Postgres is free to return equal
		// scores in any order, so a board that re-renders every rotation tick
		// shuffles its tied rows on screen for no reason a viewer can see.
		Order("scores.value DESC, users.username ASC").
		Limit(size).
		Scan(&results)
	if result.Error != nil {
		slog.ErrorContext(ctx, "error fetching top users", "err", result.Error)
	}

	return leaderboardRows(results)
}

// SnapshotTopUsers reads a frozen monthly board out of scoreboard_snapshots,
// which the rollup tick writes once per finished month (top 50 per platform,
// already ranked, bots and opted-out accounts already excluded). The channel
// owner is filtered here, as TopUsers does, because the snapshot keeps them.
func SnapshotTopUsers(ctx context.Context, cfg *c.TripbotConfig, scoreboardName string, size int) [][]string {
	var results []topUserResult
	result := database.GormDB().WithContext(ctx).
		Table("scoreboard_snapshots").
		Select("username, value").
		Where("scoreboard_name = ? AND platform = ? AND username != ?", scoreboardName, cfg.Platform, cfg.ChannelName).
		Order("rank ASC").
		Limit(size).
		Scan(&results)
	if result.Error != nil {
		slog.ErrorContext(ctx, "error fetching snapshot top users", "err", result.Error, "scoreboard", scoreboardName)
	}
	return leaderboardRows(results)
}

// leaderboardRows renders query rows as the [username, value] pairs every
// leaderboard surface consumes, with the value at one decimal.
func leaderboardRows(results []topUserResult) [][]string {
	var leaderboard [][]string
	for _, r := range results {
		leaderboard = append(leaderboard, []string{r.Username, fmt.Sprintf("%.1f", r.Value)})
	}
	return leaderboard
}
