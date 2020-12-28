package users

import "github.com/adanalife/tripbot/pkg/scoreboards"

func (u User) Score(scoreboardName string) float32 {
	scoreboard := scoreboards.FindScoreboard(scoreboardName)
	score := scoreboards.FindScore(u.ID, scoreboard.ID)
	return score.Score
}
