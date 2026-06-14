// cmd/backfill-coords corrects the per-clip GPS coordinates in the videos
// table.
//
// Each clip carries a single (lat, lng) recovered by a long-removed OCR pass
// over the dashcam's burned-in GPS overlay. Two failure modes leak into the
// data: clips with no GPS lock (stored 0/0, flagged) and clips whose OCR was
// off by a digit. The latter render as impossible jumps on the console map and
// throw off the !guess / !location games.
//
// This pass walks every clip in film order and:
//
//   - rejects "there-and-back" outliers: a fix whose in- AND out-edges both
//     imply a speed above --max-speed-mph, where dropping it makes the edge
//     that skips over it plausible again — the signature of a single bad OCR
//     digit (one wrong point inflates both the edge into it and the edge back
//     out). The legitimate recovery edge after a long parked gap has only one
//     fast edge, so it is left alone.
//   - interpolates missing and rejected points linearly by film time between
//     the surrounding good fixes, but only across gaps <= --max-interp-gap.
//     Beyond that the van was parked/off and a synthetic midpoint would lie, so
//     those are left flagged (genuinely unknown).
//   - records provenance in videos.coord_source (ocr / interpolated / rejected
//     / missing) so the map can style synthesized points and a future re-OCR
//     pass won't mistake them for real fixes.
//
// Like cmd/backfill-miles it is a dry run by default. Three modes:
//
//	(default)     dry-run report: show what would change and why
//	--apply       write corrections directly to the connected database
//	--output-sql  print idempotent UPDATEs keyed by slug, suitable for piping
//	              into psql against a different database (e.g. prod)
//
// Usage:
//
//	# dry run against the connected DB
//	DATABASE_USER=tripbot DATABASE_PASS=hunter2 \
//	  DATABASE_HOST=localhost DATABASE_DB=tripbot \
//	  go run ./cmd/backfill-coords > coords-report.txt
//
//	# generate SQL to apply the corrections to a different (prod) DB
//	... go run ./cmd/backfill-coords --output-sql > coords-corrections.sql
//	psql "$PROD_DSN" < coords-corrections.sql
//
// The pass is convergent: re-running reads the corrected data, treats
// interpolated points as fixed anchors, and leaves rejects rejected.
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"time"

	_ "github.com/lib/pq"
)

// Provenance values. Kept in sync with pkg/video's CoordSource* constants;
// duplicated here so this command stays standalone (raw database/sql, no
// pkg/video import and its config init side effects) like cmd/backfill-miles.
const (
	sourceInterpolated = "interpolated" // synthesized from neighbouring clips
	sourceMissing      = "missing"      // no GPS fix, none recoverable
	// "ocr" (original fix) and "rejected" (manually discarded) are the other
	// videos.coord_source values (see migration 020); this tool emits only the
	// two above.
)

// action labels for the report and decision routing.
const (
	actionKeep        = "keep"
	actionInterpolate = "interpolate"
	actionMissing     = "leave-missing"
)

// clip is one row of the videos table, in film order.
type clip struct {
	id     int
	slug   string
	lat    float64
	lng    float64
	state  string
	source string
	filmed time.Time
}

// hasFix reports whether the clip carries a usable original coordinate.
func (c clip) hasFix() bool {
	return c.lat != 0 || c.lng != 0
}

// decision is the outcome of analyzing one clip.
type decision struct {
	clip
	action    string
	newLat    float64
	newLng    float64
	newState  string
	newSource string
	// wasOutlier records that the clip's original OCR fix was rejected as an
	// impossible jump. Such a clip is then interpolated (action interpolate) if
	// it has good neighbours, or cleared (action reject) if it doesn't — either
	// way the bad coordinate is gone, but the flag keeps it visible in the
	// report.
	wasOutlier bool
	// implied speeds (mph) into and out of this clip, for the report. NaN when
	// there is no neighbouring fix to measure against.
	speedIn  float64
	speedOut float64
}

// changed reports whether this decision would alter the stored row.
func (d decision) changed() bool {
	return d.action != actionKeep &&
		!(d.action == actionMissing && d.source == sourceMissing && !d.hasFix())
}

func main() {
	apply := flag.Bool("apply", false, "write corrections directly to the connected database")
	outputSQL := flag.Bool("output-sql", false, "print idempotent UPDATE statements instead of a report")
	maxSpeedMph := flag.Float64("max-speed-mph", 100, "implied speed (mph) a fix must exceed to be eligible for rejection")
	minSpikeMiles := flag.Float64("min-spike-miles", 2, "a rejected outlier must jut at least this far from its previous fix")
	detourRatio := flag.Float64("detour-ratio", 2, "reject when prev→point→next is this many times longer than prev→next")
	maxInterpGap := flag.Duration("max-interp-gap", 30*time.Minute, "largest film-time gap across which missing points are interpolated")
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

	decisions := analyze(clips, *maxSpeedMph, *minSpikeMiles, *detourRatio, *maxInterpGap)

	switch {
	case *outputSQL:
		writeSQL(os.Stdout, decisions)
	case *apply:
		if err := applyDecisions(db, decisions); err != nil {
			log.Fatalf("applying corrections: %s", err)
		}
		writeReport(os.Stderr, decisions)
	default:
		writeReport(os.Stdout, decisions)
	}
}

func openDB() (*sql.DB, error) {
	dsn := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s sslmode=disable",
		os.Getenv("DATABASE_HOST"),
		os.Getenv("DATABASE_USER"),
		os.Getenv("DATABASE_PASS"),
		os.Getenv("DATABASE_DB"),
	)
	return sql.Open("postgres", dsn)
}

func loadClips(db *sql.DB) ([]clip, error) {
	rows, err := db.Query(`
		SELECT id, slug, lat, lng, COALESCE(state, ''), COALESCE(coord_source, 'ocr'), date_filmed
		FROM videos
		ORDER BY date_filmed, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var clips []clip
	for rows.Next() {
		var c clip
		if err := rows.Scan(&c.id, &c.slug, &c.lat, &c.lng, &c.state, &c.source, &c.filmed); err != nil {
			return nil, err
		}
		clips = append(clips, c)
	}
	return clips, rows.Err()
}

// analyze corrects clips already sorted by film time and returns one decision
// per clip, in the same order. It is pure (no DB) so it can be unit-tested.
//
// It iterates a detect→interpolate cycle to a fixed point: rejecting a bad fix
// and replacing it with an interpolated one can unmask a second bad fix that
// was previously hidden between two outliers (its detour looked normal only
// because a neighbour was also wrong). Looping until a pass finds no new
// outliers means one run fully converges, rather than needing repeated --apply.
func analyze(clips []clip, maxSpeedMph, minSpikeMiles, detourRatio float64, maxInterpGap time.Duration) []decision {
	n := len(clips)
	// Working state, mutated across iterations. cur* are the current best
	// coordinates (originals, then interpolated as holes get filled); fix marks
	// points that currently carry a usable coordinate (anchor for neighbours).
	curLat := make([]float64, n)
	curLng := make([]float64, n)
	curState := make([]string, n)
	fix := make([]bool, n)
	wasOutlier := make([]bool, n)
	interp := make([]bool, n)
	out := make([]decision, n)
	for i, c := range clips {
		curLat[i], curLng[i], curState[i] = c.lat, c.lng, c.state
		fix[i] = c.hasFix()
		out[i] = decision{clip: c, action: actionKeep, speedIn: math.NaN(), speedOut: math.NaN()}
	}

	for {
		// Detection: reject digit-flip outliers by their geometric signature. A
		// bad OCR reading juts far from BOTH neighbouring fixes and snaps back,
		// so the detour through it (prev→point→next) is much longer than the
		// direct path (prev→next). A genuine fast stretch or one-sided position
		// change is roughly collinear (detour ≈ direct) and is left alone. Three
		// conditions must all hold, to stay conservative:
		//   1. the point sits far from its previous anchor (excursion > minSpike),
		//   2. the detour ratio exceeds detourRatio (it snaps back), and
		//   3. the implied speed to reach it is physically impossible (> maxSpeed).
		newOutliers := 0
		for i := 0; i < n; i++ {
			if !fix[i] || interp[i] {
				continue // gaps and already-synthesized points aren't judged
			}
			prev := prevTrue(fix, i)
			next := nextTrue(fix, i)
			if prev < 0 || next < 0 {
				continue // an endpoint fix has nothing to bracket it
			}
			// Only judge a clip against neighbours in the same recording
			// session. If the nearest fix on either side is more than the interp
			// window away, this is a trip/session boundary, not a within-drive
			// jump — a real position change we can't (and shouldn't) second-guess.
			if clips[i].filmed.Sub(clips[prev].filmed) > maxInterpGap ||
				clips[next].filmed.Sub(clips[i].filmed) > maxInterpGap {
				continue
			}
			out[i].speedIn = speedMph(clips[prev].filmed, clips[i].filmed, curLat[prev], curLng[prev], curLat[i], curLng[i])
			out[i].speedOut = speedMph(clips[i].filmed, clips[next].filmed, curLat[i], curLng[i], curLat[next], curLng[next])

			excursion := haversineMiles(curLat[prev], curLng[prev], curLat[i], curLng[i])
			direct := haversineMiles(curLat[prev], curLng[prev], curLat[next], curLng[next])
			detour := excursion + haversineMiles(curLat[i], curLng[i], curLat[next], curLng[next])
			if excursion > minSpikeMiles && detour > direct*detourRatio && out[i].speedIn > maxSpeedMph {
				fix[i] = false
				wasOutlier[i] = true
				newOutliers++
			}
		}

		// Interpolation: fill runs of holes (originally missing, or just-rejected
		// outliers) that are bracketed by anchors within the gap budget. Filled
		// points become anchors for the next iteration.
		for i := 0; i < n; i++ {
			if fix[i] || (i > 0 && !fix[i-1]) {
				continue // not a hole, or not the first element of its run
			}
			j := i
			for j < n && !fix[j] {
				j++
			}
			before, after := i-1, j
			if before < 0 || after >= n ||
				clips[after].filmed.Sub(clips[before].filmed) > maxInterpGap {
				continue // unbracketed or gap too wide — leave for now
			}
			for k := i; k < j; k++ {
				curLat[k], curLng[k] = interpAt(clips[before].filmed, clips[after].filmed,
					curLat[before], curLng[before], curLat[after], curLng[after], clips[k].filmed)
				curState[k] = curState[before]
				fix[k] = true
				interp[k] = true
			}
		}

		if newOutliers == 0 {
			break
		}
	}

	// Finalize one decision per clip from the converged working state.
	for i, c := range clips {
		out[i].wasOutlier = wasOutlier[i]
		switch {
		case interp[i]:
			out[i].action = actionInterpolate
			out[i].newLat, out[i].newLng = curLat[i], curLng[i]
			out[i].newState = curState[i]
			out[i].newSource = sourceInterpolated
		case wasOutlier[i]:
			// A suspected bad fix we couldn't replace from good in-session
			// neighbours. Never destroy a coordinate we're unsure about — keep
			// the original and leave it flagged for manual review.
			out[i].action = actionKeep
		case !c.hasFix():
			out[i].action = actionMissing
			out[i].newSource = sourceMissing
		}
	}
	return out
}

// prevTrue returns the index of the nearest set element before i, or -1.
func prevTrue(b []bool, i int) int {
	for k := i - 1; k >= 0; k-- {
		if b[k] {
			return k
		}
	}
	return -1
}

// nextTrue returns the index of the nearest set element after i, or -1.
func nextTrue(b []bool, i int) int {
	for k := i + 1; k < len(b); k++ {
		if b[k] {
			return k
		}
	}
	return -1
}

// speedMph returns the implied speed in mph between two timestamped points.
// Returns +Inf when the timestamps are equal (any movement in zero time is
// impossible).
func speedMph(ta, tb time.Time, latA, lngA, latB, lngB float64) float64 {
	hours := math.Abs(tb.Sub(ta).Hours())
	miles := haversineMiles(latA, lngA, latB, lngB)
	if hours == 0 {
		if miles == 0 {
			return 0
		}
		return math.Inf(1)
	}
	return miles / hours
}

// interpAt returns the lat/lng at time t, linearly between the anchors at times
// ta/tb by film-time fraction.
func interpAt(ta, tb time.Time, latA, lngA, latB, lngB float64, t time.Time) (lat, lng float64) {
	span := tb.Sub(ta).Seconds()
	if span == 0 {
		return latA, lngA
	}
	f := t.Sub(ta).Seconds() / span
	return latA + f*(latB-latA), lngA + f*(lngB-lngA)
}

// haversineMiles returns the great-circle distance between two lat/lng points.
func haversineMiles(lat1, lng1, lat2, lng2 float64) float64 {
	const earthRadiusMiles = 3958.7613
	rad := math.Pi / 180
	dLat := (lat2 - lat1) * rad
	dLng := (lng2 - lng1) * rad
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLng/2)*math.Sin(dLng/2)
	return earthRadiusMiles * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
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
		switch d.action {
		case actionInterpolate:
			_, err = tx.Exec(
				`UPDATE videos SET lat=$1, lng=$2, state=$3, flagged=false, coord_source=$4 WHERE id=$5`,
				d.newLat, d.newLng, d.newState, sourceInterpolated, d.id)
		case actionMissing:
			_, err = tx.Exec(
				`UPDATE videos SET coord_source=$1 WHERE id=$2`, sourceMissing, d.id)
		}
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

// writeSQL emits idempotent UPDATEs keyed by slug (stable across databases).
func writeSQL(w *os.File, decisions []decision) {
	fmt.Fprintln(w, "-- Generated by cmd/backfill-coords --output-sql")
	fmt.Fprintln(w, "-- Keyed by slug; safe to run against any target DB. Apply with:")
	fmt.Fprintln(w, "--   psql \"$DSN\" < this-file.sql")
	fmt.Fprintln(w)
	for _, d := range decisions {
		if !d.changed() {
			continue
		}
		switch d.action {
		case actionInterpolate:
			fmt.Fprintf(w,
				"UPDATE videos SET lat=%.6f, lng=%.6f, state='%s', flagged=false, coord_source='%s' WHERE slug='%s';\n",
				d.newLat, d.newLng, sqlQuote(d.newState), sourceInterpolated, sqlQuote(d.slug))
		case actionMissing:
			fmt.Fprintf(w,
				"UPDATE videos SET coord_source='%s' WHERE slug='%s';\n",
				sourceMissing, sqlQuote(d.slug))
		}
	}
}

// sqlQuote escapes single quotes for inlined SQL string literals.
func sqlQuote(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if r == '\'' {
			out = append(out, '\'')
		}
		out = append(out, r)
	}
	return string(out)
}

func writeReport(w *os.File, decisions []decision) {
	var interps, missing, kept, suspects, replaced int
	fmt.Fprintf(w, "%-32s  %-19s  %-10s  %-10s  %9s  %9s  %s\n",
		"slug", "date_filmed", "lat", "lng", "speed_in", "speed_out", "action")
	fmt.Fprintf(w, "%s\n", dashes(32, 19, 10, 10, 9, 9, 22))
	for _, d := range decisions {
		switch d.action {
		case actionInterpolate:
			interps++
			if d.wasOutlier {
				replaced++
			}
		case actionMissing:
			missing++
		default:
			kept++
			if d.wasOutlier {
				suspects++
			}
		}
		action := reportAction(d)
		if action == "" {
			continue // unremarkable kept clip; keep the report focused
		}
		fmt.Fprintf(w, "%-32s  %-19s  %10.6f  %10.6f  %9s  %9s  %s\n",
			d.slug, d.filmed.Format("2006-01-02 15:04:05"),
			d.lat, d.lng, fmtSpeed(d.speedIn), fmtSpeed(d.speedOut), action)
	}
	fmt.Fprintf(w, "\n%d clips: %d kept, %d interpolated (%d of them replacing an outlier), %d left missing\n",
		len(decisions), kept, interps, replaced, missing)
	fmt.Fprintf(w, "%d suspected outliers could not be auto-corrected and were left unchanged for review (action SUSPECT)\n", suspects)
}

// reportAction returns the action label to print for a decision, or "" to omit
// it from the report (an unremarkable kept clip).
func reportAction(d decision) string {
	switch {
	case d.wasOutlier && d.action == actionInterpolate:
		return "interpolate (outlier)"
	case d.wasOutlier:
		return "SUSPECT (kept, review)"
	case d.action == actionInterpolate:
		return "interpolate"
	case d.action == actionMissing && d.changed():
		return "leave-missing"
	case d.action == actionKeep && overSpeed(d):
		return "keep (fast edge)"
	default:
		return ""
	}
}

// overSpeed reports whether either implied edge is unusually fast, so kept-but-
// fast clips still surface in the report for eyeballing.
func overSpeed(d decision) bool {
	return (!math.IsNaN(d.speedIn) && d.speedIn > 80) ||
		(!math.IsNaN(d.speedOut) && d.speedOut > 80)
}

func fmtSpeed(v float64) string {
	if math.IsNaN(v) {
		return "-"
	}
	if math.IsInf(v, 1) {
		return "inf"
	}
	return fmt.Sprintf("%.1f", v)
}

func dashes(widths ...int) string {
	out := ""
	for i, w := range widths {
		if i > 0 {
			out += "  "
		}
		for j := 0; j < w; j++ {
			out += "-"
		}
	}
	return out
}
