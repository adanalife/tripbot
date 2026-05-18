package users

import (
	"context"

	terrors "github.com/adanalife/tripbot/pkg/errors"
	"github.com/adanalife/tripbot/pkg/scoreboards"
)

func (u User) GetScore(ctx context.Context, scoreboardName string) float32 {
	value, err := scoreboards.GetScoreByName(ctx, u.Username, scoreboardName)
	if err != nil {
		terrors.LogContext(ctx, err, "error getting score for user")
		return -1.0
	}
	return value
}

func (u User) AddToScore(ctx context.Context, scoreboardName string, valueToAdd float32) {
	err := scoreboards.AddToScoreByName(ctx, u.Username, scoreboardName, valueToAdd)
	if err != nil {
		terrors.LogContext(ctx, err, "error setting score for user")
	}
}
