package users

import "github.com/adanalife/tripbot/pkg/scoreboards"

func (u User) GetScore(scoreboardName string) float32 {
	score := scoreboards.FindScoreByName(u.Username, scoreboardName)
	return score.Score
}

func (u User) SetScore(scoreboardName string, value float32) {
	score := scoreboards.SetScoreByName(u.Username, scoreboardName, value)
	return score.Score
}
