package onscreensClient

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"github.com/adanalife/tripbot/pkg/natsclient"
	oe "github.com/adanalife/tripbot/pkg/onscreens-events"
	"github.com/adanalife/tripbot/pkg/scoreboards"
)

// Client publishes onscreens overlay commands onto NATS. Construct via
// New(nats, env).
//
// NATS is the sole command transport: onscreens-server subscribes to these
// subjects and drives the overlays. The HTTP command path (the mirror that
// preceded the peel) is gone. nats may still be nil in tests that don't
// exercise pubsub — publishes no-op then.
type Client struct {
	nats natsclient.Publisher
	env  string
}

// New returns a Client that publishes commands for the given environment.
// Pass natsclient.DefaultPublisher() in production, or a nil publisher to
// disable publishing (tests).
func New(nats natsclient.Publisher, env string) *Client {
	return &Client{nats: nats, env: env}
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
	c.publish(ctx, oe.MiddleHideSubject(c.env), oe.Command{Envelope: oe.NewEnvelope()})
	return nil
}

func (c *Client) ShowMiddleText(ctx context.Context, msg string) error {
	c.publish(ctx, oe.MiddleShowSubject(c.env), oe.MiddleShow{
		Envelope: oe.NewEnvelope(),
		Msg:      msg,
	})
	return nil
}

func (c *Client) ShowLeaderboard(ctx context.Context, title string, leaderboard [][]string) error {
	// onscreens-server renders the HTML from the structured {title, rows}
	// payload it receives on this subject.
	c.publish(ctx, oe.LeaderboardShowSubject(c.env), oe.LeaderboardShow{
		Envelope: oe.NewEnvelope(),
		Title:    title,
		Rows:     leaderboard,
	})
	return nil
}

// TODO: this is taken right from the !guessleaderboard command, DRY it?
func (c *Client) ShowGuessLeaderboard(ctx context.Context) {
	// select users to show in leaderboard
	size := 10
	leaderboard := scoreboards.TopUsers(ctx, scoreboards.CurrentGuessScoreboard(), size)
	if size > len(leaderboard) {
		size = len(leaderboard)
	}
	leaderboard = leaderboard[:size]

	// Filter zero-scorers (AddToScoreByName uses FirstOrCreate, so every
	// user who's ever guessed has a row — many at 0 early in the month).
	// If the filtered list is empty, skip the overlay entirely.
	var intLeaderboard [][]string
	for _, leaderPair := range leaderboard {
		// guesses are ints not floats, so remove the decimal place
		intVersion := strings.Split(leaderPair[1], ".")[0]
		if intVersion == "0" || intVersion == "" {
			continue
		}
		intLeaderboard = append(intLeaderboard, []string{leaderPair[0], intVersion})
	}
	if len(intLeaderboard) == 0 {
		return
	}

	// display leaderboard on screen
	c.ShowLeaderboard(ctx, "Correct Guesses This Month", intLeaderboard)
}

func (c *Client) ShowTimewarp(ctx context.Context) error {
	c.publish(ctx, oe.TimewarpShowSubject(c.env), oe.Command{Envelope: oe.NewEnvelope()})
	return nil
}

func (c *Client) ShowFlag(ctx context.Context, dur time.Duration) error {
	// flag.show is disabled — there's no subject in the taxonomy, so nothing
	// is published.
	//TODO: bring this back
	return nil
}

func (c *Client) ShowGPSImage(ctx context.Context, dur time.Duration) error {
	// dur isn't transported — the server owns the GPS overlay's duration
	// (gpsDuration).
	c.publish(ctx, oe.GPSShowSubject(c.env), oe.Command{Envelope: oe.NewEnvelope()})
	return nil
}

func (c *Client) HideGPSImage(ctx context.Context) error {
	c.publish(ctx, oe.GPSHideSubject(c.env), oe.Command{Envelope: oe.NewEnvelope()})
	return nil
}
