package main

import (
	"go.uber.org/zap"
)

var ChatLog *SugaredLogger

func init() {
	logger, _ := zap.NewProduction()
	defer logger.Sync() // flushes buffer, if any
	ChatLog := logger.Sugar()
}
