package scoreboards

import (
	"errors"
	"log"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/database"
	terrors "github.com/adanalife/tripbot/pkg/errors"
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
func GetScoreByName(username, scoreboardName string) (float32, error) {
	userID, err := getUserIDByName(username)
	if err != nil {
		terrors.Log(err, "error getting user ID")
		return -1.0, err
	}
	scoreboard, err := findOrCreateScoreboard(scoreboardName)
	if err != nil {
		terrors.Log(err, "error finding or creating scoreboard")
		return -1.0, err
	}
	score, err := findOrCreateScore(userID, scoreboard.ID)
	if err != nil {
		terrors.Log(err, "error finding score")
		return -1.0, err
	}
	return score.Value, err
}

// AddToScoreByName increases the score value for a given username and scoreboard name
//TODO: this could be achieved with less queries
func AddToScoreByName(username, scoreboardName string, scoreToAdd float32) error {
	userID, err := getUserIDByName(username)
	if err != nil {
		terrors.Log(err, "error getting userID for user")
		return err
	}
	scoreboard, err := findOrCreateScoreboard(scoreboardName)
	if err != nil {
		terrors.Log(err, "error finding or creating scoreboard")
		return err
	}
	score, err := findOrCreateScore(userID, scoreboard.ID)
	if err != nil {
		terrors.Log(err, "error finding score")
		return err
	}
	score.Value += scoreToAdd
	if err := score.save(); err != nil {
		return err
	}
	instrumentation.ScoreboardWrites.Inc(scoreboardName)
	return nil
}

// findOrCreateScore will look up the username in the DB, and return a Score if possible
func findOrCreateScore(userID, scoreboardID uint16) (Score, error) {
	var score Score
	result := database.GormDB().
		Where(Score{UserID: userID, ScoreboardID: scoreboardID}).
		FirstOrCreate(&score)
	return score, result.Error
}

// findScore will look up the score in the DB
//TODO: this shouldn't be necessary, join the tables instead
func findScore(userID, scoreboardID uint16) (Score, error) {
	var score Score
	result := database.GormDB().
		Where("user_id = ? AND scoreboard_id = ?", userID, scoreboardID).
		First(&score)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return Score{}, gorm.ErrRecordNotFound
	}
	return score, result.Error
}

// createScore() will actually create the DB record
func createScore(userID, scoreboardID uint16) (Score, error) {
	log.Printf("creating score user_id:%d, scoreboard_id:%d", userID, scoreboardID)
	score := Score{UserID: userID, ScoreboardID: scoreboardID}
	result := database.GormDB().Create(&score)
	return score, result.Error
}

// save() will take the given score and store it in the DB
func (s Score) save() error {
	if c.Conf.Verbose {
		log.Println("saving score", s)
	}
	err := database.GormDB().Model(&s).Update("value", s.Value).Error
	if err != nil {
		terrors.Log(err, "error saving score")
	}
	return err
}

// getUserIDByName fetches the user ID for a given username
//TODO: this shouldn't be necessary, join the tables instead
func getUserIDByName(username string) (uint16, error) {
	var result struct{ ID uint16 }
	err := database.GormDB().Raw("SELECT id FROM users WHERE username = ?", username).Scan(&result).Error
	if err != nil {
		terrors.Log(err, "error fetching user ID")
	}
	return result.ID, err
}
