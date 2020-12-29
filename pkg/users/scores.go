package users

import "github.com/adanalife/tripbot/pkg/scoreboards"

func (u User) GetScore(scoreboardName string) float32 {
	score := scoreboards.FindScoreByName(u.Username, scoreboardName)
	return score.Score
}

func (u User) SetScore(scoreboardName string, valueToAdd float32) {
	err := scoreboards.AddToScoreByName(u.Username, scoreboardName, valueToAdd)
	if err != nil {
		//TODO: do something here
	}
}
