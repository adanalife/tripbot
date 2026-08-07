package users

import (
	"context"
	"log/slog"

	"github.com/adanalife/tripbot/pkg/scoreboards"
)

func (u User) GetScore(ctx context.Context, scoreboardName string) float32 {
	value, err := scoreboards.GetScoreByName(ctx, u.Platform, u.Username, scoreboardName)
	if err != nil {
		slog.ErrorContext(ctx, "error getting score for user", "err", err)
		return -1.0
	}
	return value
}

func (u User) AddToScore(ctx context.Context, scoreboardName string, valueToAdd float32) {
	err := scoreboards.AddToScoreByName(ctx, u.Platform, u.Username, scoreboardName, valueToAdd)
	if err != nil {
		slog.ErrorContext(ctx, "error setting score for user", "err", err)
	}
}
