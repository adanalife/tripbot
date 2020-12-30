package users

import (
	terrors "github.com/adanalife/tripbot/pkg/errors"
	"github.com/adanalife/tripbot/pkg/scoreboards"
)

func (u User) GetScore(scoreboardName string) float32 {
	value, err := scoreboards.GetScoreByName(u.Username, scoreboardName)
	if err != nil {
		terrors.Log(err, "error getting score for user")
		return -1.0
	}
	return value
}

func (u User) AddToScore(scoreboardName string, valueToAdd float32) {
	err := scoreboards.AddToScoreByName(u.Username, scoreboardName, valueToAdd)
	if err != nil {
		terrors.Log(err, "error setting score for user")
	}
}
