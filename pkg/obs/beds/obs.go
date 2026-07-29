package beds

import (
	"context"

	"github.com/adanalife/tripbot/pkg/obs"
)

// RealOBS drives the actual OBS WebSocket. Each call dials fresh, same as the
// rest of pkg/obs — bed switches are rare (a console click or a track ending).
type RealOBS struct{}

func (RealOBS) SetNetwork(ctx context.Context, inputName string) error {
	return obs.SetInputNetworkMode(ctx, inputName)
}

func (RealOBS) SetLocalFile(ctx context.Context, inputName, file string, loop bool) error {
	return obs.SetInputLocalFileMode(ctx, inputName, file, loop)
}

func (RealOBS) Settings(ctx context.Context, inputName string) (map[string]any, error) {
	return obs.GetInputSettings(ctx, inputName)
}
