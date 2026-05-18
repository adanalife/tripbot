package scoreboards

import (
	"context"
	"errors"
	"log/slog"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/database"
	"github.com/adanalife/tripbot/pkg/instrumentation"
	"gorm.io/gorm"
)

// Score represents a user's score on a scoreboard
type Score struct {
	ID           uint16 `gorm:"primaryKey"`
	UserID       uint16
	ScoreboardID uint16
	Value        float32
	DateCreated  time.Time
}

// GetScoreByName returns the score value for a given username and scoreboard name
//TODO: this could be achieved with a single query
func GetScoreByName(ctx context.Context, username, scoreboardName string) (float32, error) {
	userID, err := getUserIDByName(ctx, username)
	if err != nil {
		slog.ErrorContext(ctx, "error getting user ID", "err", err)
		return -1.0, err
	}
	scoreboard, err := findOrCreateScoreboard(ctx, scoreboardName)
	if err != nil {
		slog.ErrorContext(ctx, "error finding or creating scoreboard", "err", err)
		return -1.0, err
	}
	score, err := findOrCreateScore(ctx, userID, scoreboard.ID)
	if err != nil {
		slog.ErrorContext(ctx, "error finding score", "err", err)
		return -1.0, err
	}
	return score.Value, err
}

// AddToScoreByName increases the score value for a given username and scoreboard name
//TODO: this could be achieved with less queries
func AddToScoreByName(ctx context.Context, username, scoreboardName string, scoreToAdd float32) error {
	userID, err := getUserIDByName(ctx, username)
	if err != nil {
		slog.ErrorContext(ctx, "error getting userID for user", "err", err)
		return err
	}
	scoreboard, err := findOrCreateScoreboard(ctx, scoreboardName)
	if err != nil {
		slog.ErrorContext(ctx, "error finding or creating scoreboard", "err", err)
		return err
	}
	score, err := findOrCreateScore(ctx, userID, scoreboard.ID)
	if err != nil {
		slog.ErrorContext(ctx, "error finding score", "err", err)
		return err
	}
	score.Value += scoreToAdd
	if err := score.save(ctx); err != nil {
		return err
	}
	instrumentation.ScoreboardWrites.Inc(scoreboardName)
	return nil
}

// findOrCreateScore will look up the username in the DB, and return a Score if possible
func findOrCreateScore(ctx context.Context, userID, scoreboardID uint16) (Score, error) {
	var score Score
	result := database.GormDB().WithContext(ctx).
		Where(Score{UserID: userID, ScoreboardID: scoreboardID}).
		FirstOrCreate(&score)
	return score, result.Error
}

// findScore will look up the score in the DB
//TODO: this shouldn't be necessary, join the tables instead
func findScore(ctx context.Context, userID, scoreboardID uint16) (Score, error) {
	var score Score
	result := database.GormDB().WithContext(ctx).
		Where("user_id = ? AND scoreboard_id = ?", userID, scoreboardID).
		First(&score)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return Score{}, gorm.ErrRecordNotFound
	}
	return score, result.Error
}

// createScore() will actually create the DB record
func createScore(ctx context.Context, userID, scoreboardID uint16) (Score, error) {
	slog.InfoContext(ctx, "creating score", "user_id", userID, "scoreboard_id", scoreboardID)
	score := Score{UserID: userID, ScoreboardID: scoreboardID}
	result := database.GormDB().WithContext(ctx).Create(&score)
	return score, result.Error
}

// save() will take the given score and store it in the DB
func (s Score) save(ctx context.Context) error {
	if c.Conf.Verbose {
		slog.InfoContext(ctx, "saving score", "user_id", s.UserID, "scoreboard_id", s.ScoreboardID, "value", s.Value)
	}
	err := database.GormDB().WithContext(ctx).Model(&s).Update("value", s.Value).Error
	if err != nil {
		slog.ErrorContext(ctx, "error saving score", "err", err)
	}
	return err
}

// getUserIDByName fetches the user ID for a given username
//TODO: this shouldn't be necessary, join the tables instead
func getUserIDByName(ctx context.Context, username string) (uint16, error) {
	var result struct{ ID uint16 }
	err := database.GormDB().WithContext(ctx).Raw("SELECT id FROM users WHERE username = ?", username).Scan(&result).Error
	if err != nil {
		slog.ErrorContext(ctx, "error fetching user ID", "err", err)
	}
	return result.ID, err
}
