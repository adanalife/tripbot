package playoutClient

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/adanalife/tripbot/pkg/natsclient"
	ve "github.com/adanalife/tripbot/pkg/playout-events"
	"github.com/nats-io/nats.go/jetstream"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// Client issues playout playback commands over NATS and reads the
// currently-playing file over HTTP. Construct via New(host, nats, env, platform).
//
// The four fire-and-forget commands (PlayRandom, PlayFileInPlaylist, Skip,
// Back) publish to tripbot.<env>.playout.<verb>.<platform>; playout's subscriber
// acts on them. The HTTP command path (the mirror that preceded the peel) is
// gone. nats may be nil (tests that don't exercise pubsub) — publishes no-op
// then. CurrentlyPlaying is a read and stays on HTTP, so host is still required.
//
// platform is the streaming platform this tripbot instance serves ("twitch" /
// "youtube"); it's the trailing leaf on every command subject so only the
// matching playout-<platform> server acts on it — a Twitch-triggered !find never
// seeks the YouTube stream.
type Client struct {
	serverURL  string
	httpClient *http.Client
	nats       natsclient.Publisher
	env        string
	platform   string
}

// New returns a Client pointed at the given playout host. The HTTP
// transport is OTel-instrumented so outbound calls produce spans and
// propagate W3C tracecontext headers. Callers must pass ctx so the
// propagation has an active span to attach to — passing context.Background()
// will still send the request, just without a parent span linking it to the
// caller's trace. nats + env drive command publishing; pass
// natsclient.DefaultPublisher() in production, or a nil publisher to disable
// publishing (tests).
func New(host string, nats natsclient.Publisher, env, platform string) *Client {
	return &Client{
		serverURL:  "http://" + host,
		httpClient: &http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)},
		nats:       nats,
		env:        env,
		platform:   platform,
	}
}

// publish marshals ev and fires it on subject. Fire-and-forget: marshal
// errors are logged, and a nil publisher (or a nil underlying conn) no-ops.
func (c *Client) publish(ctx context.Context, subject string, ev any) {
	if c.nats == nil {
		return
	}
	payload, err := json.Marshal(ev)
	if err != nil {
		slog.ErrorContext(ctx, "marshal playout event", "err", err, "subject", subject)
		return
	}
	c.nats.Publish(ctx, subject, payload)
}

// CurrentlyPlaying finds the currently-playing video path
func (c *Client) CurrentlyPlaying(ctx context.Context) string {
	response, err := c.get(ctx, c.serverURL+"/playout/current")
	if err != nil {
		slog.ErrorContext(ctx, "unable to determine current video", "err", err)
		return ""
	}
	return response
}

// lastPlayedStream is the JetStream last-value cache playout publishes its
// now-playing state to. playout declares it; this side only reads, and a
// reader that finds it missing degrades to its own fallback.
const lastPlayedStream = "TRIPBOT_PLAYOUT_LASTPLAYED"

// lastPlayedStaleAfter is how old a position report may be and still be worth
// advancing by wall clock. playout ticks every 5 s, so past this it has stopped
// reporting and extrapolating would be inventing a position for a stream that
// may not be running.
const lastPlayedStaleAfter = 30 * time.Second

// Playhead reads playout's own report of what it is playing and how far into
// it. pos is the reported position advanced by however long ago the report was
// stamped, so it tracks the stream between the five-second ticks. ok is false
// when there is nothing to read (NATS off, JetStream unavailable, the stream or
// subject empty) or the report is too stale to extrapolate from.
func (c *Client) Playhead(ctx context.Context) (file string, pos time.Duration, ok bool) {
	js := natsclient.JetStream()
	if js == nil {
		return "", 0, false
	}
	stream, err := js.Stream(ctx, lastPlayedStream)
	if err != nil {
		slog.DebugContext(ctx, "lastplayed stream lookup failed", "err", err, "stream", lastPlayedStream)
		return "", 0, false
	}
	subj := ve.LastPlayedSubject(c.env, c.platform)
	raw, err := stream.GetLastMsgForSubject(ctx, subj)
	if err != nil {
		if !errors.Is(err, jetstream.ErrMsgNotFound) {
			slog.DebugContext(ctx, "lastplayed read failed", "err", err, "subject", subj)
		}
		return "", 0, false
	}
	var ev ve.LastPlayed
	if err := json.Unmarshal(raw.Data, &ev); err != nil {
		slog.DebugContext(ctx, "lastplayed decode failed", "err", err, "subject", subj)
		return "", 0, false
	}
	return playheadFrom(ev, time.Now())
}

// playheadFrom advances a position report to now. Split out from Playhead so
// the arithmetic — which is the part that can be wrong — is testable without a
// JetStream server. A report stamped in the future, or older than
// lastPlayedStaleAfter, is refused rather than trusted.
func playheadFrom(ev ve.LastPlayed, now time.Time) (file string, pos time.Duration, ok bool) {
	emittedAt, err := time.Parse(time.RFC3339Nano, ev.EmittedAt)
	if err != nil {
		return "", 0, false
	}
	age := now.Sub(emittedAt)
	if age < 0 || age > lastPlayedStaleAfter {
		return "", 0, false
	}
	if ev.File == "" {
		return "", 0, false
	}
	return ev.File, time.Duration(ev.PositionMs)*time.Millisecond + age, true
}

// PlayRandom plays a random file from the playlist
func (c *Client) PlayRandom(ctx context.Context) error {
	c.publish(ctx, ve.PlayRandomSubject(c.env, c.platform), ve.Command{Envelope: ve.NewEnvelope()})
	return nil
}

// PlayFileInPlaylist plays a given file
func (c *Client) PlayFileInPlaylist(ctx context.Context, filename string) error {
	c.publish(ctx, ve.PlayFileSubject(c.env, c.platform), ve.PlayFile{Envelope: ve.NewEnvelope(), File: filename})
	return nil
}

// PlayFileAtTimestamp plays a file and seeks to tsSec seconds into it — the
// jump-to-moment path behind !find. A non-positive tsSec just plays from the
// top (playout's seek guard no-ops there). Fire-and-forget like the other
// commands.
func (c *Client) PlayFileAtTimestamp(ctx context.Context, filename string, tsSec float64) error {
	posMs := int64(tsSec * 1000)
	if posMs < 0 {
		posMs = 0
	}
	c.publish(ctx, ve.PlayFileAtSubject(c.env, c.platform), ve.PlayFileAt{Envelope: ve.NewEnvelope(), File: filename, PositionMs: posMs})
	return nil
}

func (c *Client) Skip(ctx context.Context, n int) error {
	c.publish(ctx, ve.SkipSubject(c.env, c.platform), ve.Skip{Envelope: ve.NewEnvelope(), N: n})
	return nil
}

func (c *Client) Back(ctx context.Context, n int) error {
	c.publish(ctx, ve.BackSubject(c.env, c.platform), ve.Back{Envelope: ve.NewEnvelope(), N: n})
	return nil
}

// Seek moves the playhead by delta relative to the current playback position,
// crossing clip boundaries as needed; negative rewinds. The server walks real
// clip durations, so the move covers that much footage — the duration-based
// !skip/!back path. Fire-and-forget like the other commands.
func (c *Client) Seek(ctx context.Context, delta time.Duration) error {
	c.publish(ctx, ve.SeekSubject(c.env, c.platform), ve.Seek{Envelope: ve.NewEnvelope(), DeltaMs: delta.Milliseconds()})
	return nil
}

// Transport-layer errors log at Debug, not Error: CurrentlyPlaying (its
// only caller) logs its own operation-specific failure at Error with the
// same underlying err. Logging here too would double-count every playout outage in
// Loki and Sentry.
func (c *Client) get(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		slog.DebugContext(ctx, "error building request to playout server", "err", err)
		return "", err
	}
	response, err := c.httpClient.Do(req)
	if err != nil {
		slog.DebugContext(ctx, "error connecting to playout server", "err", err)
		return "", err
	}
	defer response.Body.Close()
	contents, err := io.ReadAll(response.Body)
	if err != nil {
		slog.DebugContext(ctx, "error reading response from playout server", "err", err)
		return "", err
	}
	return string(contents), nil
}
