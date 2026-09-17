package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/adanalife/tripbot/pkg/database"
)

// The insights endpoints serve read-only aggregates over the append-only
// analytics tables (events, video_plays, viewer_samples) for the standalone
// tripbot-console's insights panels. Like the rest of /api they're internal-only
// (in-namespace Service), and the console's client gives them a 2-second
// budget — every query here is bounded by a date window the events_event_date /
// viewstats date indexes (migrations 048/049) can scan.
//
// All numbers are fleet-wide: events carry their platform, and "which commands
// perform well" is a question about the project, not one instance. Bots are
// excluded from every event-derived number, per the events-table design; the
// users join below is the events-shaped analog of the is_bot filter
// pkg/rollups' snapshot read applies.

// Insights windows are clamped so an arbitrary ?days can't turn a window scan
// into a table scan.
const (
	insightsMinDays = 1
	insightsMaxDays = 90

	commandInsightsDefaultDays = 7
	guessInsightsDefaultDays   = 30
	footageInsightsDefaultDays = 7
	// Regions default to a wider window than clips: the rollup exists because a
	// state needs many plays before its churn settles, and a week of airtime
	// spread over ~50 states rarely gets there.
	regionInsightsDefaultDays = 30
)

// insightsDays parses ?days, falling back to def when absent or unparseable
// and clamping to [insightsMinDays, insightsMaxDays].
func insightsDays(r *http.Request, def int) int {
	return queryInt(r, "days", def, insightsMinDays, insightsMaxDays)
}

// queryInt parses the named integer query parameter, falling back to def when
// absent or unparseable and clamping to [lo, hi].
func queryInt(r *http.Request, key string, def, lo, hi int) int {
	raw := r.URL.Query().Get(key)
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	return min(max(n, lo), hi)
}

// commandUsage is one command's in-window activity: how often it ran, how many
// distinct non-bot chatters ran it, and how often it was refused (gates,
// cooldowns, typos that resolved to it). Command strings pass through verbatim
// from the stored meta.
type commandUsage struct {
	Command  string `json:"command"`
	Runs     int64  `json:"runs"`
	Users    int64  `json:"users"`
	Refusals int64  `json:"refusals"`
}

// refusalReason is one command_refused reason's in-window count.
type refusalReason struct {
	Reason string `json:"reason"`
	Count  int64  `json:"count"`
}

// unknownCommand is one token viewers reach for that no command answers —
// the signal that seeds new commands and aliases.
type unknownCommand struct {
	Typed string `json:"typed"`
	Count int64  `json:"count"`
}

// commandInsightsResponse is the wire shape of GET /api/insights/commands.
// The console renders these fields by name; empty result sets are empty
// arrays, never null.
type commandInsightsResponse struct {
	Days            int              `json:"days"`
	Commands        []commandUsage   `json:"commands"`
	RefusalReasons  []refusalReason  `json:"refusal_reasons"`
	UnknownCommands []unknownCommand `json:"unknown_commands"`
}

// commandUsageSQL joins command_run and command_refused counts per command.
// The FULL OUTER JOIN keeps commands that only have refusals (runs 0), so a
// gate-heavy command still shows up.
const commandUsageSQL = `
WITH runs AS (
    SELECT COALESCE(e.meta->>'command', '') AS command,
           COUNT(*)                   AS runs,
           COUNT(DISTINCT e.username) AS users
    FROM events e
    JOIN users u ON u.platform = e.platform AND u.username = e.username
    WHERE e.event = 'command_run'
      AND e.date_created >= now() - make_interval(days => @days)
      AND u.is_bot = false
    GROUP BY 1
),
refused AS (
    SELECT COALESCE(e.meta->>'command', '') AS command,
           COUNT(*) AS refusals
    FROM events e
    JOIN users u ON u.platform = e.platform AND u.username = e.username
    WHERE e.event = 'command_refused'
      AND e.date_created >= now() - make_interval(days => @days)
      AND u.is_bot = false
    GROUP BY 1
)
SELECT COALESCE(r.command, f.command) AS command,
       COALESCE(r.runs, 0)            AS runs,
       COALESCE(r.users, 0)           AS users,
       COALESCE(f.refusals, 0)        AS refusals
FROM runs r
FULL OUTER JOIN refused f ON f.command = r.command
ORDER BY runs DESC, refusals DESC, command
LIMIT 20`

const refusalReasonsSQL = `
SELECT COALESCE(e.meta->>'reason', '') AS reason,
       COUNT(*) AS count
FROM events e
JOIN users u ON u.platform = e.platform AND u.username = e.username
WHERE e.event = 'command_refused'
  AND e.date_created >= now() - make_interval(days => @days)
  AND u.is_bot = false
GROUP BY 1
ORDER BY count DESC, reason`

// unknownCommandsSQL surfaces what viewers type that nothing answers. An
// unknown refusal stores the raw token in meta's command field (there is no
// canonical form to report) and omits typed, which only carries an alias when
// it differs — so the token viewers actually typed is typed when present,
// command otherwise.
const unknownCommandsSQL = `
SELECT COALESCE(e.meta->>'typed', e.meta->>'command', '') AS typed,
       COUNT(*) AS count
FROM events e
JOIN users u ON u.platform = e.platform AND u.username = e.username
WHERE e.event = 'command_refused'
  AND e.meta->>'reason' = 'unknown'
  AND e.date_created >= now() - make_interval(days => @days)
  AND u.is_bot = false
GROUP BY 1
ORDER BY count DESC, typed
LIMIT 10`

func gatherCommandInsights(ctx context.Context, days int) (commandInsightsResponse, error) {
	out := commandInsightsResponse{
		Days:            days,
		Commands:        []commandUsage{},
		RefusalReasons:  []refusalReason{},
		UnknownCommands: []unknownCommand{},
	}
	db := database.GormDB().WithContext(ctx)
	if err := db.Raw(commandUsageSQL, sql.Named("days", days)).Scan(&out.Commands).Error; err != nil {
		return out, fmt.Errorf("command usage: %w", err)
	}
	if err := db.Raw(refusalReasonsSQL, sql.Named("days", days)).Scan(&out.RefusalReasons).Error; err != nil {
		return out, fmt.Errorf("refusal reasons: %w", err)
	}
	if err := db.Raw(unknownCommandsSQL, sql.Named("days", days)).Scan(&out.UnknownCommands).Error; err != nil {
		return out, fmt.Errorf("unknown commands: %w", err)
	}
	return out, nil
}

// stateGuesses is one state's in-window !guess activity, grouped by the state
// that was actually on screen — "which states are hard to guess".
type stateGuesses struct {
	State   string `json:"state"`
	Guesses int64  `json:"guesses"`
	Correct int64  `json:"correct"`
}

// guessInsightsResponse is the wire shape of GET /api/insights/guesses.
type guessInsightsResponse struct {
	Days    int            `json:"days"`
	Total   int64          `json:"total"`
	Correct int64          `json:"correct"`
	Players int64          `json:"players"`
	States  []stateGuesses `json:"states"`
}

// meta->>'correct' reads 'true' whether the writer stored a JSON boolean or
// the string "true", so the predicate is stable across either encoding.
const guessTotalsSQL = `
SELECT COUNT(*) AS total,
       COUNT(*) FILTER (WHERE e.meta->>'correct' = 'true') AS correct,
       COUNT(DISTINCT e.username) AS players
FROM events e
JOIN users u ON u.platform = e.platform AND u.username = e.username
WHERE e.event = 'guess_submitted'
  AND e.date_created >= now() - make_interval(days => @days)
  AND u.is_bot = false`

const guessStatesSQL = `
SELECT COALESCE(e.meta->>'actual', '') AS state,
       COUNT(*) AS guesses,
       COUNT(*) FILTER (WHERE e.meta->>'correct' = 'true') AS correct
FROM events e
JOIN users u ON u.platform = e.platform AND u.username = e.username
WHERE e.event = 'guess_submitted'
  AND e.date_created >= now() - make_interval(days => @days)
  AND u.is_bot = false
GROUP BY 1
ORDER BY guesses DESC, state
LIMIT 15`

func gatherGuessInsights(ctx context.Context, days int) (guessInsightsResponse, error) {
	out := guessInsightsResponse{Days: days, States: []stateGuesses{}}
	db := database.GormDB().WithContext(ctx)
	var totals struct {
		Total   int64
		Correct int64
		Players int64
	}
	if err := db.Raw(guessTotalsSQL, sql.Named("days", days)).Scan(&totals).Error; err != nil {
		return out, fmt.Errorf("guess totals: %w", err)
	}
	out.Total, out.Correct, out.Players = totals.Total, totals.Correct, totals.Players
	if err := db.Raw(guessStatesSQL, sql.Named("days", days)).Scan(&out.States).Error; err != nil {
		return out, fmt.Errorf("guess states: %w", err)
	}
	return out, nil
}

// clipInsight is one clip's in-window footage performance. AvgChatters /
// MaxChatters aggregate viewer_samples.count — the number of people who've
// spoken in chat while the clip aired, sampled every ~61s — which is an
// engagement signal, not a concurrent-viewer count.
type clipInsight struct {
	VideoID     int     `json:"video_id"`
	Label       string  `json:"label"`
	Plays       int64   `json:"plays"`
	AvgChatters float64 `json:"avg_chatters"`
	MaxChatters int64   `json:"max_chatters"`
	Samples     int64   `json:"samples"`
}

// footageInsightsResponse is the wire shape of GET /api/insights/footage.
type footageInsightsResponse struct {
	Days  int           `json:"days"`
	Clips []clipInsight `json:"clips"`
	// Platforms are the platforms whose chat actually fed the window's samples,
	// in name order. It exists because the chatter figures read as fleet-wide
	// and are not: a platform running bot-less has no chatters to sample, so it
	// contributes nothing and says nothing about it. Reported rather than
	// asserted so a platform that starts sampling joins the list on its own.
	Platforms []string `json:"platforms"`
}

// footageMinSamples drops clips seen fewer than this many sample ticks in the
// window: a single ~61s sample says nothing about how a clip performs.
const footageMinSamples = 3

// footageSQL aggregates chatter samples per clip, joined with the window's
// play counts and the clip's place columns for a label. samples is the
// driving side so a clip whose plays all predate the window (it was already
// airing when the window opened) still reports.
const footageSQL = `
WITH samples AS (
    SELECT video_id,
           AVG(count) AS avg_chatters,
           MAX(count) AS max_chatters,
           COUNT(*)   AS samples
    FROM viewer_samples
    WHERE video_id IS NOT NULL
      AND sampled_at >= now() - make_interval(days => @days)
    GROUP BY video_id
    HAVING COUNT(*) >= @min_samples
),
plays AS (
    SELECT video_id, COUNT(*) AS plays
    FROM video_plays
    WHERE video_id IS NOT NULL
      AND started_at >= now() - make_interval(days => @days)
    GROUP BY video_id
)
SELECT s.video_id,
       COALESCE(p.plays, 0)                       AS plays,
       ROUND(s.avg_chatters::numeric, 1)::float8  AS avg_chatters,
       s.max_chatters,
       s.samples,
       COALESCE(v.state, '') AS state,
       COALESCE(v.city, '')  AS city,
       v.city_m
FROM samples s
LEFT JOIN plays p ON p.video_id = s.video_id
LEFT JOIN videos v ON v.id = s.video_id
ORDER BY s.avg_chatters DESC, s.video_id
LIMIT 15`

// footagePlatformsSQL names the platforms that contributed samples to the
// window. Deliberately not filtered by min_samples or joined to the clip list:
// the question is which chats were being sampled at all, and a platform whose
// every clip fell under the threshold was still being sampled.
const footagePlatformsSQL = `
SELECT DISTINCT platform
FROM viewer_samples
WHERE video_id IS NOT NULL
  AND sampled_at >= now() - make_interval(days => @days)
ORDER BY platform`

// clipLabelNearLimitM matches pkg/video's nearPlaceLimit: beyond 10 km the
// nearest town isn't an honest name for where the clip is.
const clipLabelNearLimitM = 10000

// clipLabel renders a clip's place columns as a short table label: the city
// when the geocode pass named one close enough to claim ("Moab, Utah"),
// otherwise the state alone, otherwise empty. Same nearness rule as
// video.Moment.Place, without the chat phrasing — a table cell wants the
// place, not a sentence. A city without a distance degrades to the state,
// for the same honesty reason Video.Place does.
func clipLabel(state, city string, cityM *float64) string {
	if city != "" && cityM != nil && *cityM <= clipLabelNearLimitM {
		if state != "" {
			return city + ", " + state
		}
		return city
	}
	return state
}

func gatherFootageInsights(ctx context.Context, days int) (footageInsightsResponse, error) {
	out := footageInsightsResponse{Days: days, Clips: []clipInsight{}, Platforms: []string{}}
	var rows []struct {
		VideoID     int
		Plays       int64
		AvgChatters float64
		MaxChatters int64
		Samples     int64
		State       string
		City        string
		CityM       *float64
	}
	err := database.GormDB().WithContext(ctx).
		Raw(footageSQL, sql.Named("days", days), sql.Named("min_samples", footageMinSamples)).
		Scan(&rows).Error
	if err != nil {
		return out, fmt.Errorf("footage: %w", err)
	}
	for _, r := range rows {
		out.Clips = append(out.Clips, clipInsight{
			VideoID:     r.VideoID,
			Label:       clipLabel(r.State, r.City, r.CityM),
			Plays:       r.Plays,
			AvgChatters: r.AvgChatters,
			MaxChatters: r.MaxChatters,
			Samples:     r.Samples,
		})
	}
	if err := database.GormDB().WithContext(ctx).
		Raw(footagePlatformsSQL, sql.Named("days", days)).
		Scan(&out.Platforms).Error; err != nil {
		return out, fmt.Errorf("footage platforms: %w", err)
	}
	return out, nil
}

// writeInsights writes an insights payload the way the other /api endpoints
// do: JSON, uncached.
func writeInsights(w http.ResponseWriter, r *http.Request, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		slog.ErrorContext(r.Context(), "couldn't encode insights payload", "err", err)
	}
}

// insightsError reports a failed insights query as a JSON 500. The message
// stays generic — the detail goes to the log, not to the wire.
func insightsError(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusInternalServerError)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// commandInsightsHandler serves GET /api/insights/commands: per-command run /
// distinct-user / refusal counts, refusal reasons, and the top unknown tokens
// over the ?days window.
func commandInsightsHandler(w http.ResponseWriter, r *http.Request) {
	days := insightsDays(r, commandInsightsDefaultDays)
	payload, err := gatherCommandInsights(r.Context(), days)
	if err != nil {
		slog.ErrorContext(r.Context(), "command insights query failed", "err", err, "days", days)
		insightsError(w, "couldn't gather command insights")
		return
	}
	writeInsights(w, r, payload)
}

// guessInsightsHandler serves GET /api/insights/guesses: !guess volume,
// accuracy, distinct players, and the per-state breakdown over the ?days
// window.
func guessInsightsHandler(w http.ResponseWriter, r *http.Request) {
	days := insightsDays(r, guessInsightsDefaultDays)
	payload, err := gatherGuessInsights(r.Context(), days)
	if err != nil {
		slog.ErrorContext(r.Context(), "guess insights query failed", "err", err, "days", days)
		insightsError(w, "couldn't gather guess insights")
		return
	}
	writeInsights(w, r, payload)
}

// footageInsightsHandler serves GET /api/insights/footage: per-clip play
// counts and chatter-sample aggregates over the ?days window.
func footageInsightsHandler(w http.ResponseWriter, r *http.Request) {
	days := insightsDays(r, footageInsightsDefaultDays)
	payload, err := gatherFootageInsights(r.Context(), days)
	if err != nil {
		slog.ErrorContext(r.Context(), "footage insights query failed", "err", err, "days", days)
		insightsError(w, "couldn't gather footage insights")
		return
	}
	writeInsights(w, r, payload)
}

// regionInsight is one state's in-window footage performance. It answers the
// question the per-clip report above structurally can't: any single clip plays
// too rarely for its average to settle, while a state accumulates enough
// airtime to be acted on ("Utah desert holds viewers; Nebraska highway sheds
// them").
//
// State comes from the clip's current videos.state rather than the state
// denormalized onto video_plays at play time. The play row is the better
// record of one play, but a rollup needs all three of its sub-aggregates
// keyed the same way, and viewer_samples and events carry only video_id — so
// resolving every side through videos keeps a clip whose state was backfilled
// after it aired from splitting across an empty group and a named one.
type regionInsight struct {
	State string `json:"state"`
	// Plays is every play that started in the window; ClosedPlays is the
	// subset whose end was observed, which is the part AirtimeHours can be
	// summed from. They diverge when tripbot restarted mid-clip.
	Plays        int64   `json:"plays"`
	ClosedPlays  int64   `json:"closed_plays"`
	AirtimeHours float64 `json:"airtime_hours"`
	Samples      int64   `json:"samples"`
	// LiveSamples counts ticks that positively reported something was
	// broadcasting. It exists to price the other figures rather than to
	// filter them: viewer_samples.live only started being written at
	// migration 047, so a window reaching back before that has samples it
	// cannot vouch for, and a caller comparing the two numbers can see how
	// much of the average rests on ticks that predate the column.
	LiveSamples int64 `json:"live_samples"`
	// AvgViewers and MaxViewers aggregate viewer_samples.viewers — the
	// platform's own concurrent-viewer number. Null when no in-window tick
	// carried one: a platform with no such endpoint, a gateway too old to
	// serve it, or rows written before migration 047.
	AvgViewers *float64 `json:"avg_viewers"`
	MaxViewers *int64   `json:"max_viewers"`
	// AvgChatters aggregates viewer_samples.count, which is the *chatter*
	// total — an engagement signal, and a self-selecting slice biased toward
	// whatever provokes typing. Kept beside the viewer figures because it is
	// the only series with history: it has been collected since 2026-07-06.
	AvgChatters float64 `json:"avg_chatters"`
	// Joins and Leaves are non-bot session starts and ends stamped with a
	// clip in this state, so Net is the audience the state gained while it
	// held the screen. This is the "best performing" figure the design
	// settled on: raw viewer count is dominated by time-of-day, raids and
	// day-of-week, which churn differences out.
	Joins  int64 `json:"joins"`
	Leaves int64 `json:"leaves"`
	Net    int64 `json:"net"`
	// NetPerHour normalizes Net by airtime, because a state that aired ten
	// times longer collects more joins without performing any better. Null
	// when no play in the window closed, leaving no airtime to divide by.
	NetPerHour *float64 `json:"net_per_hour"`
}

// regionInsightsResponse is the wire shape of GET /api/insights/regions.
type regionInsightsResponse struct {
	Days    int             `json:"days"`
	Regions []regionInsight `json:"regions"`
	// Platforms are the platforms whose samples fed the window, in name
	// order — same honesty check as the per-clip report's list.
	Platforms []string `json:"platforms"`
}

// regionMinSamples drops states seen fewer than this many sample ticks in the
// window. Higher than footageMinSamples because the whole point of rolling up
// to a state is that it clears a bar a clip can't: ten ticks is ~10 minutes of
// having been on screen.
const regionMinSamples = 10

// regionsSQL rolls the three raw signals up per state: airtime from
// video_plays, audience from viewer_samples, churn from the airing-stamped
// login/logout events. Samples drive the join because they are what measures
// performance — a state that aired but was never sampled has nothing to say.
//
// live IS NOT FALSE rather than live: the column only started being written at
// migration 047, so filtering on it strictly would throw away every earlier
// tick and quietly return nothing on a window that reaches back past it. NULL
// means "nobody reported", which is not the same claim as "offline" — the
// rows that positively say offline are the ones worth excluding, and
// live_samples reports how much of the window could be vouched for.
const regionsSQL = `
WITH samples AS (
    SELECT v.state                                            AS state,
           COUNT(*)                                           AS samples,
           COUNT(*) FILTER (WHERE s.live)                     AS live_samples,
           AVG(s.viewers) FILTER (WHERE s.live IS NOT FALSE)   AS avg_viewers,
           MAX(s.viewers) FILTER (WHERE s.live IS NOT FALSE)   AS max_viewers,
           AVG(s.count)   FILTER (WHERE s.live IS NOT FALSE)   AS avg_chatters
    FROM viewer_samples s
    JOIN videos v ON v.id = s.video_id
    WHERE s.video_id IS NOT NULL
      AND v.state <> ''
      AND s.sampled_at >= now() - make_interval(days => @days)
    GROUP BY v.state
    HAVING COUNT(*) >= @min_samples
),
airtime AS (
    SELECT v.state   AS state,
           COUNT(*)  AS plays,
           COUNT(p.ended_at) AS closed_plays,
           COALESCE(SUM(EXTRACT(EPOCH FROM (p.ended_at - p.started_at))), 0) / 3600.0 AS airtime_hours
    FROM video_plays p
    JOIN videos v ON v.id = p.video_id
    WHERE p.video_id IS NOT NULL
      AND v.state <> ''
      AND p.started_at >= now() - make_interval(days => @days)
    GROUP BY v.state
),
churn AS (
    SELECT v.state AS state,
           COUNT(*) FILTER (WHERE e.event = 'login')  AS joins,
           COUNT(*) FILTER (WHERE e.event = 'logout') AS leaves
    FROM events e
    JOIN users u  ON u.platform = e.platform AND u.username = e.username
    JOIN videos v ON v.id = e.video_id
    WHERE e.event IN ('login', 'logout')
      AND e.video_id IS NOT NULL
      AND v.state <> ''
      AND u.is_bot = false
      AND e.date_created >= now() - make_interval(days => @days)
    GROUP BY v.state
)
SELECT s.state,
       COALESCE(a.plays, 0)                              AS plays,
       COALESCE(a.closed_plays, 0)                       AS closed_plays,
       ROUND(COALESCE(a.airtime_hours, 0)::numeric, 2)::float8 AS airtime_hours,
       s.samples,
       s.live_samples,
       ROUND(s.avg_viewers::numeric, 1)::float8          AS avg_viewers,
       s.max_viewers,
       ROUND(COALESCE(s.avg_chatters, 0)::numeric, 1)::float8 AS avg_chatters,
       COALESCE(c.joins, 0)                              AS joins,
       COALESCE(c.leaves, 0)                             AS leaves,
       COALESCE(c.joins, 0) - COALESCE(c.leaves, 0)      AS net,
       ROUND(((COALESCE(c.joins, 0) - COALESCE(c.leaves, 0))
              / NULLIF(a.airtime_hours, 0))::numeric, 2)::float8 AS net_per_hour
FROM samples s
LEFT JOIN airtime a ON a.state = s.state
LEFT JOIN churn   c ON c.state = s.state
ORDER BY net_per_hour DESC NULLS LAST, avg_viewers DESC NULLS LAST, s.state
LIMIT 20`

// regionPlatformsSQL names the platforms that contributed samples to the
// window. Unfiltered by min_samples for the same reason as the per-clip
// list: the question is which platforms were being sampled at all.
const regionPlatformsSQL = `
SELECT DISTINCT s.platform
FROM viewer_samples s
JOIN videos v ON v.id = s.video_id
WHERE s.video_id IS NOT NULL
  AND v.state <> ''
  AND s.sampled_at >= now() - make_interval(days => @days)
ORDER BY s.platform`

func gatherRegionInsights(ctx context.Context, days int) (regionInsightsResponse, error) {
	out := regionInsightsResponse{Days: days, Regions: []regionInsight{}, Platforms: []string{}}
	err := database.GormDB().WithContext(ctx).
		Raw(regionsSQL, sql.Named("days", days), sql.Named("min_samples", regionMinSamples)).
		Scan(&out.Regions).Error
	if err != nil {
		return out, fmt.Errorf("regions: %w", err)
	}
	if err := database.GormDB().WithContext(ctx).
		Raw(regionPlatformsSQL, sql.Named("days", days)).
		Scan(&out.Platforms).Error; err != nil {
		return out, fmt.Errorf("region platforms: %w", err)
	}
	return out, nil
}

// regionInsightsHandler serves GET /api/insights/regions: per-state airtime,
// audience and join/leave churn over the ?days window.
func regionInsightsHandler(w http.ResponseWriter, r *http.Request) {
	days := insightsDays(r, regionInsightsDefaultDays)
	payload, err := gatherRegionInsights(r.Context(), days)
	if err != nil {
		slog.ErrorContext(r.Context(), "region insights query failed", "err", err, "days", days)
		insightsError(w, "couldn't gather region insights")
		return
	}
	writeInsights(w, r, payload)
}

// The viewer series is the one insights endpoint that answers with a time
// series rather than a ranking: per-platform concurrent viewers and chat
// volume over a recent window, bucketed for a chart. Hours rather than days,
// because the question it answers is "how did today go".
const (
	viewerSeriesMinHours     = 1
	viewerSeriesMaxHours     = 24 * 7
	viewerSeriesDefaultHours = 24
	// How many buckets a window is cut into, whatever its length — enough to
	// draw a line on a phone, few enough that a week stays one small payload.
	viewerSeriesBuckets = 240
)

// viewerPoint is one bucket of one platform's audience.
type viewerPoint struct {
	// The bucket's start, in unix seconds.
	T int64 `json:"t"`
	// Mean concurrent viewers across the bucket's live ticks. Null when no
	// tick in the bucket carried a viewer count (a platform without the
	// endpoint, or offline the whole bucket).
	Viewers *float64 `json:"viewers"`
	// Chat messages that arrived during the bucket. Null when no tick in the
	// bucket had the counter wired.
	Chat *int64 `json:"chat"`
}

// viewerSeries is one platform's points, oldest first.
type viewerSeries struct {
	Platform string        `json:"platform"`
	Points   []viewerPoint `json:"points"`
}

type viewerSeriesResponse struct {
	Hours int `json:"hours"`
	// The bucket width in seconds, so a chart can draw gaps where a bucket
	// is missing rather than joining across them.
	Step   int            `json:"step"`
	Series []viewerSeries `json:"series"`
}

// viewerSeriesStep is the bucket width for a window: the window cut into
// viewerSeriesBuckets, rounded down to whole minutes and never under one,
// since the samples themselves land about a minute apart.
func viewerSeriesStep(hours int) int {
	return max(60, hours*3600/viewerSeriesBuckets/60*60)
}

// Offline ticks (live = false) are kept out of the viewer mean the way the
// region rollup keeps them out: an empty room and a dark channel both read
// zero and mean opposite things. Chat counts every tick — a message is a
// message whether or not the channel was live.
const viewerSeriesSQL = `
SELECT platform,
       (floor(extract(epoch FROM sampled_at) / @step) * @step)::bigint AS t,
       ROUND(AVG(viewers) FILTER (WHERE live IS NOT FALSE)::numeric, 1)::float8 AS viewers,
       SUM(chat_messages)                                                        AS chat
FROM viewer_samples
WHERE sampled_at >= now() - make_interval(hours => @hours)
GROUP BY platform, t
ORDER BY platform, t`

func gatherViewerSeries(ctx context.Context, hours int) (viewerSeriesResponse, error) {
	step := viewerSeriesStep(hours)
	out := viewerSeriesResponse{Hours: hours, Step: step, Series: []viewerSeries{}}
	var rows []struct {
		Platform string
		T        int64
		Viewers  *float64
		Chat     *int64
	}
	err := database.GormDB().WithContext(ctx).
		Raw(viewerSeriesSQL, sql.Named("hours", hours), sql.Named("step", step)).
		Scan(&rows).Error
	if err != nil {
		return out, fmt.Errorf("viewer series: %w", err)
	}
	// The rows arrive grouped by platform, so a change of platform starts a
	// new series.
	for _, row := range rows {
		if n := len(out.Series); n == 0 || out.Series[n-1].Platform != row.Platform {
			out.Series = append(out.Series, viewerSeries{Platform: row.Platform, Points: []viewerPoint{}})
		}
		last := &out.Series[len(out.Series)-1]
		last.Points = append(last.Points, viewerPoint{T: row.T, Viewers: row.Viewers, Chat: row.Chat})
	}
	return out, nil
}

// viewerSeriesHandler serves GET /api/insights/viewers: per-platform viewer
// and chat-volume buckets over the ?hours window.
func viewerSeriesHandler(w http.ResponseWriter, r *http.Request) {
	hours := queryInt(r, "hours", viewerSeriesDefaultHours, viewerSeriesMinHours, viewerSeriesMaxHours)
	payload, err := gatherViewerSeries(r.Context(), hours)
	if err != nil {
		slog.ErrorContext(r.Context(), "viewer series query failed", "err", err, "hours", hours)
		insightsError(w, "couldn't gather the viewer series")
		return
	}
	writeInsights(w, r, payload)
}
