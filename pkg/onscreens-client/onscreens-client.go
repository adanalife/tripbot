package onscreensClient

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/adanalife/tripbot/pkg/natsclient"
	oe "github.com/adanalife/tripbot/pkg/onscreens-events"
)

// Client publishes onscreens overlay commands onto NATS. Construct via
// New(nats, env, platform).
//
// NATS is the sole command transport: onscreens-server subscribes to these
// subjects and drives the overlays. The HTTP command path (the mirror that
// preceded the peel) is gone. nats may still be nil in tests that don't
// exercise pubsub — publishes no-op then.
//
// platform is the streaming platform this tripbot instance serves ("twitch" /
// "youtube"); it's the trailing leaf on every subject so only the matching
// onscreens-<platform> server receives these overlays — a Twitch-triggered
// leaderboard never renders on the YouTube stream.
type Client struct {
	nats     natsclient.Publisher
	env      string
	platform string
}

// New returns a Client that publishes commands for the given environment and
// streaming platform. Pass natsclient.DefaultPublisher() in production, or a
// nil publisher to disable publishing (tests).
func New(nats natsclient.Publisher, env, platform string) *Client {
	return &Client{nats: nats, env: env, platform: platform}
}

// publish marshals ev and fires it on subject. Fire-and-forget: marshal
// errors are logged, and a nil publisher (or a nil underlying conn) no-ops.
func (c *Client) publish(ctx context.Context, subject string, ev any) {
	if c.nats == nil {
		return
	}
	payload, err := json.Marshal(ev)
	if err != nil {
		slog.ErrorContext(ctx, "marshal onscreens event", "err", err, "subject", subject)
		return
	}
	c.nats.Publish(ctx, subject, payload)
}

func (c *Client) HideMiddleText(ctx context.Context) error {
	c.publish(ctx, oe.MiddleHideSubject(c.env, c.platform), oe.Command{Envelope: oe.NewEnvelope()})
	return nil
}

func (c *Client) ShowMiddleText(ctx context.Context, msg string) error {
	c.publish(ctx, oe.MiddleShowSubject(c.env, c.platform), oe.MiddleShow{
		Envelope: oe.NewEnvelope(),
		Msg:      msg,
	})
	return nil
}

func (c *Client) ShowLeaderboard(ctx context.Context, title string, leaderboard [][]string) error {
	// onscreens-server renders the HTML from the structured {title, rows}
	// payload it receives on this subject.
	c.publish(ctx, oe.LeaderboardShowSubject(c.env, c.platform), oe.LeaderboardShow{
		Envelope: oe.NewEnvelope(),
		Title:    title,
		Rows:     leaderboard,
	})
	return nil
}

func (c *Client) ShowTimewarp(ctx context.Context, username string) error {
	c.publish(ctx, oe.TimewarpShowSubject(c.env, c.platform), oe.TimewarpShow{
		Envelope: oe.NewEnvelope(),
		Username: username,
	})
	return nil
}

// UpdateLocation publishes the currently-playing clip's location + date for the
// rotators to surface on a bot-less YouTube stream (see oe.LocationData).
// Fire-and-forget; tripbot republishes on a timer.
func (c *Client) UpdateLocation(ctx context.Context, location, date string) error {
	c.publish(ctx, oe.LocationUpdateSubject(c.env, c.platform), oe.LocationData{
		Envelope: oe.NewEnvelope(),
		Location: location,
		Date:     date,
	})
	return nil
}

func (c *Client) ShowGPSImage(ctx context.Context, dur time.Duration) error {
	// dur isn't transported — the server owns the GPS overlay's duration
	// (gpsDuration).
	c.publish(ctx, oe.GPSShowSubject(c.env, c.platform), oe.Command{Envelope: oe.NewEnvelope()})
	return nil
}

func (c *Client) HideGPSImage(ctx context.Context) error {
	c.publish(ctx, oe.GPSHideSubject(c.env, c.platform), oe.Command{Envelope: oe.NewEnvelope()})
	return nil
}
