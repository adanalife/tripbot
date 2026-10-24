// cmd/backfill-miles computes historically correct per-user lifetime miles from
// the events table and optionally writes them back to users.miles.
//
// The events table is the ground truth: miles are derived by pairing each
// user's login events with their logout events and summing the session
// durations, the same pairing pkg/rollups uses. A login pairs with the logout
// carrying its session_id; legacy rows from before session_id existed pair by
// row number. Sessions longer than 24 hours are dropped to exclude cases where
// a logout was missed across a server restart.
//
// Bots (users.is_bot = true) are always excluded — they accumulate events
// continuously but should never appear on the leaderboard.
//
// By default this command is a dry run that prints a report.  Three modes:
//
//	(default)     dry-run report: show what would change and why
//	--apply       write corrections directly to the connected database
//	--output-sql  print idempotent UPDATE statements suitable for piping
//	              into psql against a different database (e.g. prod)
//
// Usage:
//
//	# dry run against the historical dump
//	DATABASE_USER=tripbot_docker DATABASE_PASS=hunter2 \
//	  DATABASE_HOST=localhost DATABASE_DB=tripbot_historical \
//	  go run ./cmd/backfill-miles > miles-backfill-report.txt
//
//	# generate SQL to apply the dump's corrections to a different (prod) DB
//	... go run ./cmd/backfill-miles --output-sql > miles-corrections.sql
//	psql "$PROD_DSN" < miles-corrections.sql
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"math"
	"net/url"
	"os"
	"strings"

	_ "github.com/lib/pq"
)

// Only considers non-bot users. Bots are in channel 24/7 and accumulate
// enormous event counts that would dominate the leaderboard.
//
// The pairing CTEs mirror pkg/rollups' reconcileSQL without its dirty-user
// filter; keep the two in step.
const computeMilesSQL = `
WITH login_rn AS (
    SELECT platform, username, date_created AS login_time,
           ROW_NUMBER() OVER (PARTITION BY platform, username ORDER BY date_created) AS rn
    FROM events
    WHERE event = 'login'
      AND COALESCE(session_id, '00000000-0000-0000-0000-000000000000') = '00000000-0000-0000-0000-000000000000'
),
logout_rn AS (
    SELECT platform, username, date_created AS logout_time,
           ROW_NUMBER() OVER (PARTITION BY platform, username ORDER BY date_created) AS rn
    FROM events
    WHERE event = 'logout'
      AND COALESCE(session_id, '00000000-0000-0000-0000-000000000000') = '00000000-0000-0000-0000-000000000000'
),
-- MIN collapses a session_id that was logged out twice into one session.
keyed AS (
    SELECT platform, username, session_id,
           MIN(date_created) FILTER (WHERE event = 'login') AS login_time,
           MIN(date_created) FILTER (WHERE event = 'logout') AS logout_time
    FROM events
    WHERE event IN ('login', 'logout')
      AND session_id <> '00000000-0000-0000-0000-000000000000'
    GROUP BY platform, username, session_id
),
pairs AS (
    SELECT l.platform, l.username, l.login_time, lo.logout_time
    FROM login_rn l
    JOIN logout_rn lo ON lo.platform = l.platform AND lo.username = l.username AND lo.rn = l.rn
    UNION ALL
    SELECT platform, username, login_time, logout_time FROM keyed
),
sessions AS (
    SELECT platform, username,
           EXTRACT(EPOCH FROM (logout_time - login_time)) / 60.0 AS minutes
    FROM pairs
    WHERE logout_time > login_time
      AND EXTRACT(EPOCH FROM (logout_time - login_time)) / 3600.0 < 24
),
computed AS (
    SELECT platform, username, SUM(0.1 * minutes / 3.0)::real AS miles
    FROM sessions GROUP BY platform, username
)
SELECT u.id, u.platform, u.username, u.miles AS stored, COALESCE(c.miles, 0) AS computed,
       COALESCE(c.miles, 0) - u.miles AS delta
FROM users u LEFT JOIN computed c ON c.platform = u.platform AND c.username = u.username
WHERE u.is_bot = false
ORDER BY ABS(COALESCE(c.miles, 0) - u.miles) DESC
`

const updateMilesSQL = `UPDATE users SET miles = $1 WHERE id = $2`

// Used with --output-sql: safe to run against a target DB that may not have
// the same user IDs as the source. Matches by platform + username and only
// raises miles.
const upsertMilesSQLFmt = "UPDATE users SET miles = GREATEST(miles, %.4f) WHERE platform = '%s' AND username = '%s';\n"

// sqlQuote escapes single quotes for inlined SQL string literals.
func sqlQuote(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

type row struct {
	id       int
	platform string
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
	u := url.URL{Scheme: "postgres", User: url.UserPassword(user, pass), Host: host, Path: dbName, RawQuery: "sslmode=disable"}
	return u.String()
}

func main() {
	apply := flag.Bool("apply", false, "write corrections to the connected database")
	outputSQL := flag.Bool("output-sql", false, "print UPDATE statements for piping into a different (e.g. prod) database")
	minDelta := flag.Float64("min-delta", 0.5, "only report/fix users where |computed-stored| exceeds this value")
	flag.Parse()

	if *apply && *outputSQL {
		log.Fatal("--apply and --output-sql are mutually exclusive")
	}

	db, err := sql.Open("postgres", connStr())
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			slog.Warn("close db", "err", err)
		}
	}()
	if err := db.Ping(); err != nil {
		log.Fatalf("ping db: %v", err)
	}

	rows, err := db.Query(computeMilesSQL)
	if err != nil {
		log.Fatalf("query: %v", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("close rows", "err", err)
		}
	}()

	var results []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.platform, &r.username, &r.stored, &r.computed, &r.delta); err != nil {
			log.Fatalf("scan: %v", err)
		}
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		log.Fatalf("rows: %v", err)
	}

	if *outputSQL {
		fmt.Println("-- Generated by cmd/backfill-miles --output-sql")
		fmt.Println("-- Safe to run against any target DB: matches by platform + username, only raises miles.")
		fmt.Println("-- Bots excluded. Apply with: psql \"$PROD_DSN\" < this-file.sql")
		fmt.Println()
		for _, r := range results {
			if r.computed <= r.stored || r.delta < *minDelta {
				continue
			}
			fmt.Printf(upsertMilesSQLFmt, r.computed, sqlQuote(r.platform), sqlQuote(r.username))
		}
		return
	}

	// human-readable report
	fmt.Printf("%-10s  %-30s  %10s  %10s  %10s  %s\n", "platform", "username", "stored", "computed", "delta", "action")
	fmt.Printf("%-10s  %-30s  %10s  %10s  %10s  %s\n",
		"----------", "------------------------------", "----------", "----------", "----------", "------")

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

		fmt.Printf("%-10s  %-30s  %10.2f  %10.2f  %+10.2f  %s\n",
			r.platform, r.username, r.stored, r.computed, r.delta, action)

		if *apply && willUpdate {
			if _, err := db.Exec(updateMilesSQL, r.computed, r.id); err != nil {
				slog.Error("updating user miles failed", "username", r.username, "id", r.id, "err", err)
			} else {
				updateCount++
			}
		} else if willUpdate {
			updateCount++
		} else {
			skipCount++
		}
	}

	fmt.Println()
	if *apply {
		fmt.Printf("applied: %d users updated, %d skipped (stored >= computed)\n", updateCount, skipCount)
	} else {
		fmt.Printf("dry run: %d users would be updated, %d skipped (stored >= computed)\n", updateCount, skipCount)
		fmt.Println("use --apply to write to this DB, or --output-sql to generate SQL for a different DB")
	}
}
