package vlcServer

import (
	c "github.com/adanalife/tripbot/pkg/config/vlc-server"
	terrors "github.com/adanalife/tripbot/pkg/errors"
)

// init runs before any test. terrors.Log dereferences a package-level
// config.Config interface that's nil until Initialize is called — without
// this, any handler test that walks an error path NPEs in the logger.
func init() {
	terrors.Initialize(*c.Conf, "test")
}
