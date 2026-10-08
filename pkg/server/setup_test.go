package server

import (
	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	terrors "github.com/adanalife/tripbot/pkg/errors"
)

// testConf is the config test Servers carry; only Start reads it, so the
// handler tests need nothing beyond a stable Environment.
var testConf = &c.TripbotConfig{Environment: "testing"}

// init runs before any test. terrors.Log dereferences a nil
// config.Config interface until Initialize is called — without this,
// any handler test that walks an error path NPEs in the logger.
func init() {
	terrors.Initialize(*testConf, "test")
}
