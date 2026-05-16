package scoreboards

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/database"
	terrors "github.com/adanalife/tripbot/pkg/errors"
)

// Scoreboard represents a bucket of scores, and has a name to identify it
type Scoreboard struct {
	ID          uint16 `gorm:"primaryKey"`
	Name        string
	DateCreated time.Time
}

type topUserResult struct {
	Username string
	Value    float32
}

func TopUsers(ctx context.Context, scoreboardName string, size int) [][]string {
	var leaderboard [][]string

	ignoredUsers := append(c.IgnoredUsers, strings.ToLower(c.Conf.ChannelName))

	var results []topUserResult
	result := database.GormDB().WithContext(ctx).
		Table("scores").
		Select("users.username, scores.value").
		Joins("JOIN scoreboards ON scores.scoreboard_id = scoreboards.id").
		Joins("JOIN users ON scores.user_id = users.id").
		Where("scoreboards.name = ?", scoreboardName).
		Where("users.username NOT IN ?", ignoredUsers).
		Order("scores.value DESC").
		Limit(size).
		Scan(&results)
	if result.Error != nil {
		terrors.Log(result.Error, "error fetching top users")
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
	result := database.GormDB().WithContext(ctx).Where(Scoreboard{Name: name}).FirstOrCreate(&scoreboard)
	return scoreboard, result.Error
}

// createScoreboard() will actually create the DB record
func createScoreboard(ctx context.Context, name string) (Scoreboard, error) {
	if c.Conf.Verbose {
		log.Println("creating scoreboard", name)
	}
	scoreboard := Scoreboard{Name: name}
	result := database.GormDB().WithContext(ctx).Create(&scoreboard)
	return scoreboard, result.Error
}
