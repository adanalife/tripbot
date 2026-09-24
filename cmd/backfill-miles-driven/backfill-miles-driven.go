// cmd/backfill-miles-driven computes videos.miles_driven: the real road
// distance the van covered during each clip.
//
// Each clip carries a single GPS fix (see cmd/backfill-coords), so the
// distance driven during clip A is approximated as the great-circle distance
// from A's fix to the next clip's fix in film order. Clips are contiguous
// ~3-minute dashcam segments while driving, so the chord between consecutive
// fixes tracks the road closely; a finer per-clip GPS track (the video-pipeline
// coords stage) can later overwrite these values through the same column.
//
// A clip gets NULL (unknown) instead of a distance when:
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
	"os"
	"time"

	_ "github.com/lib/pq"
)

// Usable coord_source values. Kept in sync with pkg/video's CoordSource*
// constants; duplicated so this command stays standalone (raw database/sql,
// no pkg/video import and its config init side effects) like its siblings.
const (
	sourceOCR          = "ocr"
	sourceInterpolated = "interpolated"
)

// clip is one row of the videos table, in film order.
type clip struct {
	id     int
	slug   string
	lat    float64
	lng    float64
	source string
	filmed time.Time
	miles  sql.NullFloat64 // current miles_driven
}

// hasFix reports whether the clip carries a usable coordinate.
func (c clip) hasFix() bool {
	return (c.lat != 0 || c.lng != 0) &&
		(c.source == sourceOCR || c.source == sourceInterpolated)
}

// decision is the computed miles_driven for one clip.
type decision struct {
	clip
	newMiles sql.NullFloat64
	// reason explains a NULL for the report: "no-fix", "gap", "speed",
	// "last-fix". Empty when a distance was computed.
	reason string
}

// changed reports whether this decision would alter the stored row. Values
// are compared rounded to 3 decimals (~5 ft), which also absorbs the
// float64→REAL round-trip.
func (d decision) changed() bool {
	if d.miles.Valid != d.newMiles.Valid {
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
	maxSpeedMph := flag.Float64("max-speed-mph", 100, "implied speeds above this mark the pair as bad data (NULL)")
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

	decisions := analyze(clips, *maxGap, *maxSpeedMph)

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
		SELECT id, slug, lat, lng, COALESCE(coord_source, 'ocr'), date_filmed, miles_driven
		FROM videos
		ORDER BY date_filmed, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var clips []clip
	for rows.Next() {
		var c clip
		if err := rows.Scan(&c.id, &c.slug, &c.lat, &c.lng, &c.source, &c.filmed, &c.miles); err != nil {
			return nil, err
		}
		clips = append(clips, c)
	}
	return clips, rows.Err()
}

// analyze computes one decision per clip, in the same film order as the
// input. It is pure (no DB) so it can be unit-tested.
func analyze(clips []clip, maxGap time.Duration, maxSpeedMph float64) []decision {
	out := make([]decision, len(clips))
	for i, c := range clips {
		out[i] = decision{clip: c}
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
		miles := haversineMiles(c.lat, c.lng, clips[next].lat, clips[next].lng)
		if miles/gap.Hours() > maxSpeedMph {
			out[i].reason = "speed"
			continue
		}
		out[i].newMiles = sql.NullFloat64{Float64: round3(miles), Valid: true}
	}
	return out
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
		if _, err := tx.Exec(`UPDATE videos SET miles_driven=$1 WHERE id=$2`, d.newMiles, d.id); err != nil {
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
			fmt.Fprintf(w, "UPDATE videos SET miles_driven=%.3f WHERE slug='%s';\n", d.newMiles.Float64, sqlQuote(d.slug))
		} else {
			fmt.Fprintf(w, "UPDATE videos SET miles_driven=NULL WHERE slug='%s';\n", sqlQuote(d.slug))
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
	var computed, changed int
	var total float64
	nulls := map[string]int{}
	for _, d := range decisions {
		if d.changed() {
			changed++
		}
		if d.newMiles.Valid {
			computed++
			total += d.newMiles.Float64
		} else {
			nulls[d.reason]++
		}
	}
	fmt.Fprintf(w, "clips:            %d\n", len(decisions))
	fmt.Fprintf(w, "with distance:    %d\n", computed)
	fmt.Fprintf(w, "total miles:      %.1f\n", total)
	fmt.Fprintf(w, "rows to update:   %d\n", changed)
	for _, r := range []string{"no-fix", "gap", "speed", "last-fix"} {
		if nulls[r] > 0 {
			fmt.Fprintf(w, "null (%s): %d\n", r, nulls[r])
		}
	}
}
