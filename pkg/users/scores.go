package users

import "github.com/adanalife/tripbot/pkg/scoreboards"

func (u User) GetScore(scoreboardName string) float32 {
	value, err := scoreboards.GetScoreByName(u.Username, scoreboardName)
	if err != nil {
		//TODO: do something here
	}
	return value
}

func (u User) AddToScore(scoreboardName string, valueToAdd float32) {
	err := scoreboards.AddToScoreByName(u.Username, scoreboardName, valueToAdd)
	if err != nil {
		//TODO: do something here
	}
}
