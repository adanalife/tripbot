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
)

// insightsDays parses ?days, falling back to def when absent or unparseable
// and clamping to [insightsMinDays, insightsMaxDays].
func insightsDays(r *http.Request, def int) int {
	raw := r.URL.Query().Get("days")
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	if n < insightsMinDays {
		return insightsMinDays
	}
	if n > insightsMaxDays {
		return insightsMaxDays
	}
	return n
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
