package chatbot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/adanalife/tripbot/pkg/database"
	"github.com/adanalife/tripbot/pkg/feature"
	"github.com/adanalife/tripbot/pkg/helpers"
	"github.com/adanalife/tripbot/pkg/natsclient"
	"github.com/adanalife/tripbot/pkg/users"
	"gorm.io/gorm"
)

// Tunables for !find. SigLIP2 cosine similarities run low (~0–0.3), so cosine
// *distances* run high — the ceiling is calibrated against real corpus scores,
// not the intuitive "close to 0" range.
const (
	// findEmbedTimeout bounds the NATS request to the embed responder. A cold
	// responder embeds a query in well under a second; the headroom is slack.
	findEmbedTimeout = 10 * time.Second
	// findCandidatePool is how many nearest frames the pgvector search returns.
	// !find jumps to a random distinct moment among them (below the ceiling)
	// rather than always the single closest, so repeated !find <thing> tours
	// different matches. Wider than findRandomizeTopN because the nearest frames
	// cluster into a handful of moments once deduped.
	findCandidatePool = 40
	// findMomentBucketSec groups frames from the same clip within this many
	// seconds into one "moment", so near-identical adjacent frames don't crowd
	// the randomization down to a single spot on stream.
	findMomentBucketSec = 60.0
	// findRandomizeTopN caps how many distinct moments the pick randomizes over,
	// keeping the jump reasonably close while still varied.
	findRandomizeTopN = 10
	// findMaxDistance is the cosine-distance ceiling for "close enough to jump".
	// pgvector's <=> is (1 - cosine_similarity). Above this we treat the query
	// as a miss rather than yanking the stream to a bad match. Calibrated on the
	// stage corpus: a real match ("rain") tops out around 0.887, a nonsense
	// query around 0.977, so 0.93 splits the two.
	findMaxDistance = 0.93
	// findJumpLeadInSec lands the playhead this many seconds BEFORE the matched
	// frame, so the moment is still upcoming when playback resumes (we don't
	// want to land right on it and have it slip past on stream) and so it plays
	// out in view after the warp overlay clears rather than being hidden by it.
	// The vlc client clamps a resulting negative timestamp to start-of-clip.
	findJumpLeadInSec = 12.0
)

// findFlagKey gates !find. Off until the flag exists + is enabled in the
// backing store (unknown keys evaluate false), so the command stays dormant
// until the embed responder is deployed and we flip it on.
const findFlagKey = "chatbot.find"

// findEmbedRequest / findEmbedResponse are the NATS request/reply wire format
// on tripbot.<env>.find.embed. The video-pipeline embed responder (deployment
// deferred) parses the natural-language query, embeds the visual residual with
// SigLIP2, and replies with the query vector plus the structured place/time
// facets it stripped out; tripbot applies those as SQL filters against
// frame_embeddings here in Go. Duplicated by hand in the two repos — keep in
// sync (same convention as the eventbus envelopes across tripbot/console).
type findEmbedRequest struct {
	Query string `json:"query"`
}

type findEmbedResponse struct {
	// Vector is the query embedding, same dimensionality + model as the rows in
	// frame_embeddings (SigLIP2 so400m NaFlex, 1152-dim).
	Vector []float32 `json:"vector"`
	// Model is the checkpoint the responder embedded with; the search filters
	// frame_embeddings.model to it so vectors from different checkpoints are
	// never compared in one ranking.
	Model string `json:"model"`
	// States / Months are the structured facets parsed out of the query
	// ("...in nevada", "...in May") — applied as filters on the videos join.
	States []string `json:"states,omitempty"`
	Months []int    `json:"months,omitempty"`
	// Error, when non-empty, is a responder-side failure (e.g. model not loaded).
	Error string `json:"error,omitempty"`
}

// findEmbedSubject is the request/reply subject the embed responder serves.
func findEmbedSubject(env string) string {
	return fmt.Sprintf("tripbot.%s.find.embed", env)
}

// SearchHit is one ranked frame from the visual search: which clip, how far in,
// the state it was filmed in, and the cosine distance (smaller = closer match).
type SearchHit struct {
	Slug     string
	TsSec    float64
	State    string
	Distance float64
}

// Search runs visual search over the dashcam corpus for !find. Tests inject a
// fake; production uses realSearch, which requests a query embedding from the
// video-pipeline responder over NATS and runs the pgvector cosine search.
type Search interface {
	// Find returns the closest frames to query (nearest first), or an empty
	// slice when nothing matches / the corpus isn't embedded. A non-nil error
	// means the search itself couldn't run (responder down, NATS off, DB error).
	Find(ctx context.Context, query string) ([]SearchHit, error)
}

// errSearchUnavailable is returned when NATS isn't connected, so the embed
// request can't be made — the expected state until the responder is deployed.
var errSearchUnavailable = errors.New("search unavailable: NATS not connected")

// realSearch is the production Search adapter. Beyond the env its NATS
// subjects are namespaced by, it holds no state: the embed request goes
// through the pkg/natsclient singleton (like realNATS) and the pgvector
// query through the process-wide DB (like realSessions).
type realSearch struct {
	env string
}

func (s realSearch) Find(ctx context.Context, query string) ([]SearchHit, error) {
	resp, err := requestEmbedding(ctx, s.env, query)
	if err != nil {
		return nil, err
	}
	return searchFrameEmbeddings(ctx, database.GormDB(), resp.Vector, resp.Model, resp.States, resp.Months, findCandidatePool)
}

// requestEmbedding asks the video-pipeline responder to embed query, returning
// the vector + parsed facets. The model lives there (it's heavy + Python), so
// this is a NATS request/reply hop rather than an in-process embed.
func requestEmbedding(_ context.Context, env, query string) (findEmbedResponse, error) {
	conn := natsclient.Conn()
	if conn == nil {
		return findEmbedResponse{}, errSearchUnavailable
	}
	payload, err := json.Marshal(findEmbedRequest{Query: query})
	if err != nil {
		return findEmbedResponse{}, err
	}
	msg, err := conn.Request(findEmbedSubject(env), payload, findEmbedTimeout)
	if err != nil {
		return findEmbedResponse{}, fmt.Errorf("embed responder request: %w", err)
	}
	var resp findEmbedResponse
	if err := json.Unmarshal(msg.Data, &resp); err != nil {
		return findEmbedResponse{}, fmt.Errorf("decode embed response: %w", err)
	}
	if resp.Error != "" {
		return findEmbedResponse{}, fmt.Errorf("embed responder: %s", resp.Error)
	}
	if len(resp.Vector) == 0 {
		return findEmbedResponse{}, errors.New("embed responder returned an empty vector")
	}
	return resp, nil
}

// searchFrameEmbeddings runs the pgvector cosine search: the nearest frames to
// vec, filtered to the responder's model and any parsed state/month facets.
// Ports video-pipeline's search.py SQL to Go (tripbot owns the DB). Exported-ish
// as a standalone func so it's sqlmock-testable without NATS or a real model.
func searchFrameEmbeddings(ctx context.Context, db *gorm.DB, vec []float32, model string, states []string, months []int, limit int) ([]SearchHit, error) {
	if len(vec) == 0 {
		return nil, errors.New("empty query vector")
	}
	lit := vectorLiteral(vec)

	// Explicit SQL (not the builder) so the two ?::vector binds — distance in
	// SELECT, ordering in ORDER BY — land in a predictable placeholder order,
	// and ORDER BY references the expression directly so the HNSW index applies.
	sql := `SELECT v.slug AS slug, fe.ts_sec AS ts_sec, v.state AS state,
       fe.embedding <=> ?::vector AS distance
FROM frame_embeddings fe
JOIN videos v ON v.id = fe.video_id
WHERE fe.model = ?`
	args := []any{lit, model}

	if len(states) > 0 {
		lower := make([]string, len(states))
		for i, s := range states {
			lower[i] = strings.ToLower(s)
		}
		sql += "\n  AND lower(v.state) IN ?"
		args = append(args, lower)
	}
	if len(months) > 0 {
		sql += "\n  AND extract(month FROM v.date_filmed)::int IN ?"
		args = append(args, months)
	}
	sql += "\nORDER BY fe.embedding <=> ?::vector\nLIMIT ?"
	args = append(args, lit, limit)

	var hits []SearchHit
	if err := db.WithContext(ctx).Raw(sql, args...).Scan(&hits).Error; err != nil {
		return nil, err
	}
	return hits, nil
}

// vectorLiteral renders a float slice as pgvector's text input form,
// "[0.1,0.2,...]". 32-bit precision matches the stored vector(1152) column.
func vectorLiteral(v []float32) string {
	var b strings.Builder
	b.WriteByte('[')
	for i, f := range v {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(float64(f), 'f', -1, 32))
	}
	b.WriteByte(']')
	return b.String()
}

// findRandIntn is the randomness seam for moment selection; tests override it
// to force a deterministic pick.
var findRandIntn = rand.Intn

// pickFindHit chooses which matching frame to jump to. Rather than always the
// single closest frame (which makes !find deterministic — !find bridge would
// land the same bridge every time), it collapses the nearest frames into
// distinct moments — adjacent frames of one clip embed near-identically — and
// picks one at random from the closest few, so repeated !find <thing> tours
// different matches with no stored state. hits must be ordered nearest-first;
// the caller guarantees hits[0] is under findMaxDistance, so at least one
// moment always qualifies.
func pickFindHit(hits []SearchHit) SearchHit {
	seen := make(map[string]bool)
	moments := make([]SearchHit, 0, findRandomizeTopN)
	for _, h := range hits {
		if h.Distance > findMaxDistance {
			continue
		}
		key := fmt.Sprintf("%s:%d", h.Slug, int(h.TsSec/findMomentBucketSec))
		if seen[key] {
			continue
		}
		seen[key] = true
		moments = append(moments, h)
		if len(moments) == findRandomizeTopN {
			break
		}
	}
	if len(moments) == 0 {
		return hits[0]
	}
	return moments[findRandIntn(len(moments))]
}

// findCmd implements !find: visual search over the dashcam corpus, jumping the
// stream to the closest matching moment. Shares the playback-jump rate-limiter
// with !timewarp / !goto so the playhead can't be yanked too often.
func (a *App) findCmd(ctx context.Context, user *users.User, params []string) {
	slog.InfoContext(ctx, "ran !find", "username", user.Username)

	// Feature-flagged: stays fully silent (no usage hint, no jump) until enabled.
	if !a.Flags.Bool(ctx, findFlagKey, feature.EvalContext{
		Username: user.Username,
		Channel:  a.Cfg.ChannelName,
		Env:      a.Cfg.Environment,
	}) {
		slog.InfoContext(ctx, "!find disabled by feature flag", "flag", findFlagKey, "username", user.Username)
		return
	}

	// VLC playback isn't wired up on the dev Mac (same guard as !goto).
	if helpers.RunningOnDarwin() {
		a.Chat.Say("Sorry, find isn't available right now")
		return
	}

	// rate-limit non-admins, shared with the other playback jumps.
	if !a.Cfg.UserIsAdmin(user.Username) {
		if time.Since(lastTimewarpTime) < 20*time.Second {
			a.Chat.Say("Not yet; enjoy the moment!")
			return
		}
	}

	if len(params) == 0 {
		a.Chat.Say("Usage: !find <something you want to see, e.g. a tunnel at sunset>")
		return
	}
	query := strings.Join(params, " ")

	// Acknowledge immediately — the embed + pgvector search can take several
	// seconds, and without a reply the command looks dead. Remove once the
	// search is consistently fast.
	a.Chat.Say(fmt.Sprintf("@%s 👀", user.Username))

	hits, err := a.Search.Find(ctx, query)
	if err != nil {
		slog.ErrorContext(ctx, "find search failed", "err", err, "query", query)
		a.Chat.Say("Search isn't available right now, sorry!")
		return
	}
	if len(hits) == 0 || hits[0].Distance > findMaxDistance {
		a.Chat.Say(fmt.Sprintf("Couldn't find anything like %q 😔", query))
		return
	}

	hit := pickFindHit(hits)

	// Don't name the state — a viewer can still guess where this is.
	a.Chat.Say(fmt.Sprintf("Found %q! Jumping there...", query))

	// Cover the jump with the full-screen warp overlay, same as !timewarp/!guess.
	a.showTimewarpOverlay(ctx, user.Username)

	// Land ahead of the matched frame so the moment doesn't slip past on stream.
	if err := a.VLC.PlayFileAtTimestamp(ctx, hit.Slug+".MP4", hit.TsSec-findJumpLeadInSec); err != nil {
		slog.ErrorContext(ctx, "find jump failed", "err", err, "slug", hit.Slug)
		a.Chat.Say("Found it, but couldn't jump there — sorry!")
		return
	}

	a.Video.GetCurrentlyPlaying(ctx)
	lastTimewarpTime = time.Now()
}
