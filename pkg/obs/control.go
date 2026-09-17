package obs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/adanalife/tripbot/pkg/contract"
	goobs "github.com/andreykaipov/goobs"
)

// defaultOBSWebsocketAddr is the fallback dialed when OBS_WEBSOCKET_ADDR is
// unset. Derived from the shared contract so it tracks the canonical
// obs-twitch service name + websocket port instead of drifting when OBS is
// renamed (it was left as the pre-per-platform "obs:4455" once and broke the
// watchdog on stage). In-cluster, cdk8s stamps OBS_WEBSOCKET_ADDR per platform
// so the YouTube stack dials obs-youtube; this default only covers the
// env-unset case.
var defaultOBSWebsocketAddr = fmt.Sprintf("%s:%d", contract.ServiceOBSTwitch, contract.PortOBSWebsocket)

// Dial opens an OBS WebSocket connection using the same env vars
// PollStreamingActive reads (OBS_WEBSOCKET_ADDR / OBS_WEBSOCKET_PASSWD),
// applying any extra goobs options on top of the password. Callers are
// responsible for client.Disconnect(). Returns an error if either OBS is
// unreachable or the password is rejected; caller logs at the appropriate
// level for its context.
//
// Exported so the audio-fallback watchdog's long-lived volume-meter
// connection can add goobs.WithEventSubscriptions without duplicating the
// addr/password resolution.
func Dial(_ context.Context, opts ...goobs.Option) (*goobs.Client, error) {
	addr := os.Getenv("OBS_WEBSOCKET_ADDR")
	if addr == "" {
		addr = defaultOBSWebsocketAddr
	}
	passwd := os.Getenv("OBS_WEBSOCKET_PASSWD")
	if passwd == "" {
		passwd = "adanalife"
	}
	return goobs.New(addr, append([]goobs.Option{goobs.WithPassword(passwd)}, opts...)...)
}

// dial opens a fresh OBS WebSocket connection with no extra options — the
// per-call form used by the Start/Stop/Get helpers below.
func dial(ctx context.Context) (*goobs.Client, error) {
	return Dial(ctx)
}

// StartStream tells OBS to begin streaming. Opens a fresh connection per
// call — toggle clicks are rare, so a long-lived shared client isn't worth
// the coordination cost.
func StartStream(ctx context.Context) error {
	client, err := dial(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if err := client.Disconnect(); err != nil {
			slog.WarnContext(ctx, "obs disconnect", "err", err)
		}
	}()
	_, err = client.Stream.StartStream()
	return err
}

// StopStream tells OBS to stop streaming. Symmetric to StartStream.
func StopStream(ctx context.Context) error {
	client, err := dial(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if err := client.Disconnect(); err != nil {
			slog.WarnContext(ctx, "obs disconnect", "err", err)
		}
	}()
	_, err = client.Stream.StopStream()
	return err
}

// ErrUnreachable is returned by GetStreamStatus when OBS itself can't be
// reached — distinguishes "OBS is down" from "OBS replied 'not streaming'".
// Callers can render a different UX for the unreachable case.
var ErrUnreachable = errors.New("obs websocket unreachable")

// GetStreamStatus reports whether OBS is currently streaming. Returns
// ErrUnreachable wrapped with the underlying dial error when OBS itself
// can't be reached, so the admin panel can render "OBS unreachable"
// rather than a misleading "not streaming."
func GetStreamStatus(ctx context.Context) (bool, error) {
	client, err := dial(ctx)
	if err != nil {
		return false, errors.Join(ErrUnreachable, err)
	}
	defer func() {
		if err := client.Disconnect(); err != nil {
			slog.WarnContext(ctx, "obs disconnect", "err", err)
		}
	}()
	resp, err := client.Stream.GetStreamStatus()
	if err != nil {
		return false, err
	}
	return resp.OutputActive, nil
}

// StreamState is OBS's streaming output as three distinct states rather than a
// boolean. Reconnecting is deliberately not folded into either neighbour: it is
// neither healthy nor idle, and a caller that collapses it into "not streaming"
// stands down on the one state where OBS has already admitted something is
// wrong.
type StreamState int

const (
	// StreamInactive: the output is stopped. Nothing is wrong and nothing is
	// streaming — a parked or scaled-down instance lives here.
	StreamInactive StreamState = iota
	// StreamReconnecting: OBS knows the RTMP session failed and is retrying.
	// Recovery may be underway, or OBS may have given up after one attempt;
	// only elapsed time tells those apart from outside.
	StreamReconnecting
	// StreamSteady: outputActive with no reconnect in progress. The silent
	// half-open lives here — OBS believes it is streaming, which is why this
	// state still has to be cross-checked against the platform.
	StreamSteady
)

func (s StreamState) String() string {
	switch s {
	case StreamReconnecting:
		return "reconnecting"
	case StreamSteady:
		return "steady"
	default:
		return "inactive"
	}
}

// GetStreamState reports which of the three streaming states OBS is in.
func GetStreamState(ctx context.Context) (StreamState, error) {
	client, err := dial(ctx)
	if err != nil {
		return StreamInactive, errors.Join(ErrUnreachable, err)
	}
	defer func() {
		if err := client.Disconnect(); err != nil {
			slog.WarnContext(ctx, "obs disconnect", "err", err)
		}
	}()
	resp, err := client.Stream.GetStreamStatus()
	if err != nil {
		return StreamInactive, err
	}
	switch {
	case !resp.OutputActive:
		return StreamInactive, nil
	case resp.OutputReconnecting:
		return StreamReconnecting, nil
	default:
		return StreamSteady, nil
	}
}
