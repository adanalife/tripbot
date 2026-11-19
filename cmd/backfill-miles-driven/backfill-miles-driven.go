// cmd/backfill-miles-driven computes videos.miles_driven: the real road
// distance the van covered during each clip.
//
// A clip whose own GPS track is trustworthy (videos.coord_confidence at or
// above --min-confidence) is measured along that track: the sum of the steps
// between its video_coords fixes inside the aired clip, which follows the
// road's curves. A fix the van could not have reached from the last believed
// one at --max-speed-mph is a misread and is stepped over; a track that loses
// more than half its fixes that way is not used.
//
// Every other clip falls back to the chord: the great-circle distance from
// its representative fix to the next clip's in its corpus, in film order.
// Clips are contiguous ~3-minute dashcam segments while driving, so the chord
// runs close to the road but cuts its corners.
//
// videos.miles_driven_source records which of the two a value came from
// ('track' or 'chord').
//
// A clip gets NULL (unknown) instead of a distance when it has no usable
// track and:
//
//   - it has no usable fix, or no later clip does (coord_source rejected /
//     missing, or 0/0 coords)
//   - the film-time gap to the next usable fix exceeds --max-gap: the dashcam
//     was off, so whatever distance was covered wasn't filmed and shouldn't be
//     credited to this clip
//   - the implied speed exceeds --max-speed-mph: a residual bad coordinate,
//     not a real drive
//
// Like cmd/backfill-coords it is a dry run by default. Three modes:
//
//	(default)     dry-run report: totals and what would change
//	--apply       write values directly to the connected database
//	--output-sql  print idempotent UPDATEs keyed by slug, suitable for piping
//	              into psql against a different database (e.g. prod)
//
// Usage:
//
//	DATABASE_USER=tripbot DATABASE_PASS=hunter2 \
//	  DATABASE_HOST=localhost DATABASE_DB=tripbot \
//	  go run ./cmd/backfill-miles-driven
//
// The pass is idempotent: recomputing writes only rows whose value changed,
// so it can re-run after any coordinate correction pass.
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"math"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/adanalife/tripbot/pkg/helpers"
	_ "github.com/lib/pq"
)

// The coord_source values that carry no coordinate. Kept in sync with
// pkg/video's CoordSource* constants; duplicated so this command stays
// standalone (raw database/sql, no pkg/video import and its config init side
// effects) like its siblings.
const (
	sourceRejected = "rejected"
	sourceMissing  = "missing"
)

// The miles_driven_source values.
const (
	viaTrack = "track"
	viaChord = "chord"
)

// fix is one moment of a clip's track: seconds into the aired clip, and where
// the van was.
type fix struct {
	ts, lat, lng float64
}

// clip is one row of the videos table, in corpus then film order.
type clip struct {
	id     int
	slug   string
	lat    float64
	lng    float64
	source string
	filmed time.Time
	miles  sql.NullFloat64 // current miles_driven
	via    sql.NullString  // current miles_driven_source
	corpus string
	speed  int // real seconds per clip second
	// confidence is how far the clip's track can be believed, 0..1; NULL
	// when the coords stage has not looked at it.
	confidence sql.NullFloat64
}

// hasFix reports whether the clip carries a usable coordinate.
func (c clip) hasFix() bool {
	return (c.lat != 0 || c.lng != 0) &&
		c.source != sourceRejected && c.source != sourceMissing
}

// decision is the computed miles_driven for one clip.
type decision struct {
	clip
	newMiles sql.NullFloat64
	// newVia is viaTrack or viaChord; empty when newMiles is NULL.
	newVia string
	// reason explains a NULL for the report: "no-fix", "gap", "speed",
	// "last-fix". Empty when a distance was computed.
	reason string
}

// changed reports whether this decision would alter the stored row. Values
// are compared rounded to 3 decimals (~5 ft), which also absorbs the
// float64→REAL round-trip.
func (d decision) changed() bool {
	if d.miles.Valid != d.newMiles.Valid || d.via.String != d.newVia {
		return true
	}
	return d.miles.Valid && round3(d.miles.Float64) != round3(d.newMiles.Float64)
}

func round3(f float64) float64 {
	return math.Round(f*1000) / 1000
}

func main() {
	apply := flag.Bool("apply", false, "write values directly to the connected database")
	outputSQL := flag.Bool("output-sql", false, "print idempotent UPDATE statements instead of a report")
	maxGap := flag.Duration("max-gap", 5*time.Minute, "largest film-time gap to the next fix that still counts as continuous recording")
	maxSpeedMph := flag.Float64("max-speed-mph", 100, "implied speeds above this are bad data: a track fix is stepped over, a chord pair is NULL")
	minConfidence := flag.Float64("min-confidence", 0.8, "lowest videos.coord_confidence whose track is measured; below it the clip falls back to the chord")
	flag.Parse()

	if *apply && *outputSQL {
		log.Fatal("choose at most one of --apply / --output-sql")
	}

	db, err := openDB()
	if err != nil {
		log.Fatalf("connecting to database: %s", err)
	}
	defer db.Close()

	clips, err := loadClips(db)
	if err != nil {
		log.Fatalf("loading clips: %s", err)
	}

	tracks, err := loadTracks(db)
	if err != nil {
		log.Fatalf("loading tracks: %s", err)
	}

	// A fast corpus's pieces start speed× farther apart in film time, so the
	// continuous-recording gap stretches with it.
	var decisions []decision
	for _, run := range byCorpus(clips) {
		decisions = append(decisions, analyze(run, tracks, *maxGap*time.Duration(run[0].speed), *maxSpeedMph, *minConfidence)...)
	}

	switch {
	case *outputSQL:
		writeSQL(os.Stdout, decisions)
	case *apply:
		if err := applyDecisions(db, decisions); err != nil {
			log.Fatalf("applying values: %s", err)
		}
		writeReport(os.Stderr, decisions)
	default:
		writeReport(os.Stdout, decisions)
	}
}

// openDB builds a URL DSN so a password holding a space or quote survives,
// which a key=value DSN does not.
func openDB() (*sql.DB, error) {
	u := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(os.Getenv("DATABASE_USER"), os.Getenv("DATABASE_PASS")),
		Host:     os.Getenv("DATABASE_HOST"),
		Path:     os.Getenv("DATABASE_DB"),
		RawQuery: "sslmode=disable",
	}
	return sql.Open("postgres", u.String())
}

func loadClips(db *sql.DB) ([]clip, error) {
	rows, err := db.Query(`
		SELECT id, slug, lat, lng, COALESCE(coord_source, 'ocr'), date_filmed, miles_driven, corpus, speed,
		       coord_confidence, miles_driven_source
		FROM videos
		ORDER BY corpus, date_filmed, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var clips []clip
	for rows.Next() {
		var c clip
		if err := rows.Scan(&c.id, &c.slug, &c.lat, &c.lng, &c.source, &c.filmed, &c.miles, &c.corpus, &c.speed, &c.confidence, &c.via); err != nil {
			return nil, err
		}
		clips = append(clips, c)
	}
	return clips, rows.Err()
}

// loadTracks reads every clip's fixes inside its aired cut, in clip-time order,
// keyed by video id.
func loadTracks(db *sql.DB) (map[int][]fix, error) {
	rows, err := db.Query(`
		SELECT video_id, ts_sec, lat, lng
		FROM video_coords
		WHERE ts_sec IS NOT NULL
		ORDER BY video_id, ts_sec`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tracks := map[int][]fix{}
	for rows.Next() {
		var id int
		var f fix
		if err := rows.Scan(&id, &f.ts, &f.lat, &f.lng); err != nil {
			return nil, err
		}
		tracks[id] = append(tracks[id], f)
	}
	return tracks, rows.Err()
}

// trackMiles sums a track's steps. A fix farther from the last believed one
// than maxSpeedMph could have carried the van is stepped over, so a misread
// costs nothing rather than a round trip to wherever it pointed. ok is false
// when the track is too short, or lost more than half its fixes that way.
// speed is the clip's real seconds per clip second.
func trackMiles(track []fix, speed int, maxSpeedMph float64) (miles float64, ok bool) {
	if len(track) < 2 {
		return 0, false
	}
	last, kept := track[0], 1
	for _, f := range track[1:] {
		hours := (f.ts - last.ts) * float64(speed) / 3600
		step := helpers.MilesBetween(last.lat, last.lng, f.lat, f.lng)
		if hours <= 0 || step/hours > maxSpeedMph {
			continue
		}
		miles += step
		last = f
		kept++
	}
	return miles, kept*2 >= len(track)
}

// byCorpus splits clips, sorted by corpus first, into one run per corpus, so
// no walk pairs a fix from one trip with a fix from another.
func byCorpus(clips []clip) [][]clip {
	var runs [][]clip
	for i, c := range clips {
		if i == 0 || c.corpus != clips[i-1].corpus {
			runs = append(runs, nil)
		}
		runs[len(runs)-1] = append(runs[len(runs)-1], c)
	}
	return runs
}

// analyze computes one decision per clip, in the same film order as the
// input. It is pure (no DB) so it can be unit-tested.
func analyze(clips []clip, tracks map[int][]fix, maxGap time.Duration, maxSpeedMph, minConfidence float64) []decision {
	out := make([]decision, len(clips))
	for i, c := range clips {
		out[i] = decision{clip: c}
		if c.confidence.Valid && c.confidence.Float64 >= minConfidence {
			if miles, ok := trackMiles(tracks[c.id], c.speed, maxSpeedMph); ok {
				out[i].newMiles = sql.NullFloat64{Float64: round3(miles), Valid: true}
				out[i].newVia = viaTrack
				continue
			}
		}
		if !c.hasFix() {
			out[i].reason = "no-fix"
			continue
		}
		next := -1
		for j := i + 1; j < len(clips); j++ {
			if clips[j].hasFix() {
				next = j
				break
			}
		}
		if next < 0 {
			out[i].reason = "last-fix"
			continue
		}
		gap := clips[next].filmed.Sub(c.filmed)
		if gap <= 0 || gap > maxGap {
			out[i].reason = "gap"
			continue
		}
		miles := helpers.MilesBetween(c.lat, c.lng, clips[next].lat, clips[next].lng)
		if miles/gap.Hours() > maxSpeedMph {
			out[i].reason = "speed"
			continue
		}
		out[i].newMiles = sql.NullFloat64{Float64: round3(miles), Valid: true}
		out[i].newVia = viaChord
	}
	return out
}

func applyDecisions(db *sql.DB, decisions []decision) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, d := range decisions {
		if !d.changed() {
			continue
		}
		if _, err := tx.Exec(`UPDATE videos SET miles_driven=$1, miles_driven_source=$2 WHERE id=$3`,
			d.newMiles, sql.NullString{String: d.newVia, Valid: d.newVia != ""}, d.id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// writeSQL emits idempotent UPDATEs keyed by slug (stable across databases).
func writeSQL(w *os.File, decisions []decision) {
	fmt.Fprintln(w, "-- Generated by cmd/backfill-miles-driven --output-sql")
	fmt.Fprintln(w, "-- Keyed by slug; safe to run against any target DB. Apply with:")
	fmt.Fprintln(w, "--   psql \"$DSN\" < this-file.sql")
	fmt.Fprintln(w)
	for _, d := range decisions {
		if !d.changed() {
			continue
		}
		if d.newMiles.Valid {
			fmt.Fprintf(w, "UPDATE videos SET miles_driven=%.3f, miles_driven_source='%s' WHERE slug='%s';\n", d.newMiles.Float64, d.newVia, strings.ReplaceAll(d.slug, "'", "''"))
		} else {
			fmt.Fprintf(w, "UPDATE videos SET miles_driven=NULL, miles_driven_source=NULL WHERE slug='%s';\n", strings.ReplaceAll(d.slug, "'", "''"))
		}
	}
}

func writeReport(w *os.File, decisions []decision) {
	var computed, changed int
	var total float64
	nulls := map[string]int{}
	// by is clips and miles per "corpus via", so a corpus leaning on the
	// chord shows up next to one measured along its tracks.
	type tally struct {
		clips int
		miles float64
	}
	by := map[string]*tally{}
	var keys []string
	for _, d := range decisions {
		if d.changed() {
			changed++
		}
		if !d.newMiles.Valid {
			nulls[d.reason]++
			continue
		}
		computed++
		total += d.newMiles.Float64
		key := fmt.Sprintf("%-6s %s", d.corpus, d.newVia)
		if by[key] == nil {
			by[key] = &tally{}
			keys = append(keys, key)
		}
		by[key].clips++
		by[key].miles += d.newMiles.Float64
	}
	fmt.Fprintf(w, "clips:            %d\n", len(decisions))
	fmt.Fprintf(w, "with distance:    %d\n", computed)
	fmt.Fprintf(w, "total miles:      %.1f\n", total)
	fmt.Fprintf(w, "rows to update:   %d\n", changed)
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(w, "  %-13s %5d clips  %8.1f mi\n", k+":", by[k].clips, by[k].miles)
	}
	for _, r := range []string{"no-fix", "gap", "speed", "last-fix"} {
		if nulls[r] > 0 {
			fmt.Fprintf(w, "null (%s): %d\n", r, nulls[r])
		}
	}
}
