// cmd/backfill-miles computes historically correct per-user lifetime miles from
// the events table and optionally writes them back to users.miles.
//
// The events table is the ground truth: miles are derived by pairing each
// user's login events with their corresponding logout events (by row number)
// and summing the session durations. Sessions longer than 24 hours are capped
// to exclude cases where a logout was missed across a server restart.
//
// By default this command is a dry run — it prints a report and exits.
// Pass --apply to write the corrections to the database.
//
// Usage:
//
//	DATABASE_USER=tripbot DATABASE_DB=tripbot DATABASE_HOST=localhost \
//	  go run ./cmd/backfill-miles [--apply] [--min-delta 0.5]
//
// To save the report:
//
//	... go run ./cmd/backfill-miles > miles-backfill-report.txt
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"math"
	"os"

	_ "github.com/lib/pq"
)

const computeMilesSQL = `
WITH login_rn AS (
    SELECT username, date_created AS login_time,
           ROW_NUMBER() OVER (PARTITION BY username ORDER BY date_created) AS rn
    FROM events WHERE event = 'login'
),
logout_rn AS (
    SELECT username, date_created AS logout_time,
           ROW_NUMBER() OVER (PARTITION BY username ORDER BY date_created) AS rn
    FROM events WHERE event = 'logout'
),
sessions AS (
    SELECT l.username,
           EXTRACT(EPOCH FROM (lo.logout_time - l.login_time)) / 60.0 AS minutes
    FROM login_rn l
    JOIN logout_rn lo ON lo.username = l.username AND lo.rn = l.rn
    WHERE lo.logout_time > l.login_time
      AND EXTRACT(EPOCH FROM (lo.logout_time - l.login_time)) / 3600.0 < 24
),
computed AS (
    SELECT username, SUM(0.1 * minutes / 3.0)::real AS miles
    FROM sessions GROUP BY username
)
SELECT u.id, u.username, u.miles AS stored, COALESCE(c.miles, 0) AS computed,
       COALESCE(c.miles, 0) - u.miles AS delta
FROM users u LEFT JOIN computed c ON c.username = u.username
ORDER BY ABS(COALESCE(c.miles, 0) - u.miles) DESC
`

const updateMilesSQL = `UPDATE users SET miles = $1 WHERE id = $2`

type row struct {
	id       int
	username string
	stored   float64
	computed float64
	delta    float64
}

func connStr() string {
	user := os.Getenv("DATABASE_USER")
	pass := os.Getenv("DATABASE_PASS")
	host := os.Getenv("DATABASE_HOST")
	dbName := os.Getenv("DATABASE_DB")
	if user == "" || host == "" || dbName == "" {
		log.Fatal("DATABASE_USER, DATABASE_HOST, and DATABASE_DB must be set")
	}
	return fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable", user, pass, host, dbName)
}

func main() {
	apply := flag.Bool("apply", false, "write corrections to the database (default: dry run)")
	minDelta := flag.Float64("min-delta", 0.5, "only report/fix users where |computed-stored| exceeds this value")
	flag.Parse()

	db, err := sql.Open("postgres", connStr())
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		log.Fatalf("ping db: %v", err)
	}

	rows, err := db.Query(computeMilesSQL)
	if err != nil {
		log.Fatalf("query: %v", err)
	}
	defer rows.Close()

	var results []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.username, &r.stored, &r.computed, &r.delta); err != nil {
			log.Fatalf("scan: %v", err)
		}
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		log.Fatalf("rows: %v", err)
	}

	// print header
	fmt.Printf("%-30s  %10s  %10s  %10s  %s\n", "username", "stored", "computed", "delta", "action")
	fmt.Printf("%-30s  %10s  %10s  %10s  %s\n", "-------------------------------", "----------", "----------", "----------", "------")

	var updateCount, skipCount int
	for _, r := range results {
		if math.Abs(r.delta) < *minDelta {
			continue
		}

		action := "skip (stored >= computed)"
		willUpdate := r.computed > r.stored
		if willUpdate {
			action = "update"
		}

		fmt.Printf("%-30s  %10.2f  %10.2f  %+10.2f  %s\n",
			r.username, r.stored, r.computed, r.delta, action)

		if *apply && willUpdate {
			if _, err := db.Exec(updateMilesSQL, r.computed, r.id); err != nil {
				log.Printf("ERROR updating %s (id=%d): %v", r.username, r.id, err)
			} else {
				updateCount++
			}
		} else if willUpdate {
			updateCount++ // count what would be updated in dry-run
		} else {
			skipCount++
		}
	}

	fmt.Println()
	if *apply {
		fmt.Printf("applied: %d users updated, %d skipped (stored >= computed)\n", updateCount, skipCount)
	} else {
		fmt.Printf("dry run: %d users would be updated, %d skipped (stored >= computed)\n", updateCount, skipCount)
		fmt.Println("re-run with --apply to write corrections")
	}
}
