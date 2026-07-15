package scoreboards

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/database"
)

// Scoreboard represents a bucket of scores, and has a name to identify it.
// Names are per-platform (miles_2026_07 exists once per platform): uniqueness
// is (name, platform), and every lookup scopes by this instance's platform.
type Scoreboard struct {
	ID       uint16 `gorm:"primaryKey"`
	Name     string
	Platform string
	// autoCreateTime stamps date_created on insert; createScoreboard() doesn't
	// set it, so without the tag GORM writes the 0001-01-01 zero value over the
	// column's DEFAULT CURRENT_TIMESTAMP. See pkg/events for the full story.
	DateCreated time.Time `gorm:"autoCreateTime"`
}

type topUserResult struct {
	Username string
	Value    float32
}

func TopUsers(ctx context.Context, scoreboardName string, size int) [][]string {
	var leaderboard [][]string

	var results []topUserResult
	result := database.GormDB().WithContext(ctx).
		Table("scores").
		Select("users.username, scores.value").
		Joins("JOIN scoreboards ON scores.scoreboard_id = scoreboards.id").
		Joins("JOIN users ON scores.user_id = users.id").
		Where("scoreboards.name = ? AND scoreboards.platform = ?", scoreboardName, c.Conf.Platform).
		// users.platform too: scores written before boards were per-platform
		// may hang off the other platform's same-named board.
		Where("users.is_bot = false AND users.platform = ? AND users.username != ?", c.Conf.Platform, strings.ToLower(c.Conf.ChannelName)).
		Order("scores.value DESC").
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

// findOrCreateScoreboard will find a Scoreboard in the DB or create one
func findOrCreateScoreboard(ctx context.Context, name string) (Scoreboard, error) {
	var scoreboard Scoreboard
	result := database.GormDB().WithContext(ctx).Where(Scoreboard{Name: name, Platform: c.Conf.Platform}).FirstOrCreate(&scoreboard)
	return scoreboard, result.Error
}

// createScoreboard() will actually create the DB record
func createScoreboard(ctx context.Context, name string) (Scoreboard, error) {
	scoreboard := Scoreboard{Name: name, Platform: c.Conf.Platform}
	result := database.GormDB().WithContext(ctx).Create(&scoreboard)
	return scoreboard, result.Error
}
