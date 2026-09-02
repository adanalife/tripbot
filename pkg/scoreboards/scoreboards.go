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
	var leaderboard [][]string

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

	for _, r := range results {
		valueAsString := fmt.Sprintf("%.1f", r.Value)
		leaderboard = append(leaderboard, []string{r.Username, valueAsString})
	}
	return leaderboard
}
