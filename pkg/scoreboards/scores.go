package scoreboards

import (
	"context"
	"log/slog"

	"github.com/adanalife/tripbot/pkg/database"
	"github.com/adanalife/tripbot/pkg/instrumentation"
)

// GetScoreByName returns the score value for a given username and scoreboard
// name. One SELECT joins scores to its user and its scoreboard, both scoped to
// the platform; an unknown user, an unknown board and a user who has never
// scored on a known board all read as 0.
func GetScoreByName(ctx context.Context, platform, username, scoreboardName string) (float32, error) {
	var result struct{ Value float32 }
	err := database.GormDB().WithContext(ctx).Raw(`
		SELECT scores.value
		FROM scores
		JOIN users ON users.id = scores.user_id
		JOIN scoreboards ON scoreboards.id = scores.scoreboard_id
		WHERE users.platform = ? AND users.username = ?
		  AND scoreboards.platform = ? AND scoreboards.name = ?`,
		platform, username, platform, scoreboardName,
	).Scan(&result).Error
	if err != nil {
		slog.ErrorContext(ctx, "error getting score", "err", err)
		return -1.0, err
	}
	return result.Value, nil
}

// AddToScoreByName increases the score value for a given username and
// scoreboard name, in one statement: the CTE find-or-creates the board for this
// platform (the no-op DO UPDATE is what makes RETURNING yield the existing
// row's id), then the score row is upserted onto (user_id, scoreboard_id) so
// concurrent adds accumulate instead of racing into duplicate rows. An unknown
// username selects no row, so nothing is scored.
func AddToScoreByName(ctx context.Context, platform, username, scoreboardName string, scoreToAdd float32) error {
	err := database.GormDB().WithContext(ctx).Exec(`
		WITH scoreboard AS (
			INSERT INTO scoreboards (name, platform) VALUES (?, ?)
			ON CONFLICT (name, platform) DO UPDATE SET name = EXCLUDED.name
			RETURNING id
		)
		INSERT INTO scores (user_id, scoreboard_id, value)
		SELECT users.id, scoreboard.id, ?
		FROM users, scoreboard
		WHERE users.platform = ? AND users.username = ?
		ON CONFLICT (user_id, scoreboard_id)
		DO UPDATE SET value = scores.value + EXCLUDED.value`,
		scoreboardName, platform, scoreToAdd, platform, username,
	).Error
	if err != nil {
		slog.ErrorContext(ctx, "error adding to score", "err", err)
		return err
	}
	instrumentation.ScoreboardWrites.Inc(scoreboardName)
	return nil
}
