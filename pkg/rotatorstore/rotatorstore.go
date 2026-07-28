// Package rotatorstore persists console-edited corner-rotator copy in Postgres,
// the record of truth behind the /api/rotators surface.
//
// It lives apart from pkg/rotator on purpose. pkg/rotator is imported by
// onscreens-server and must stay dependency-free; this package imports GORM and
// is imported only by cmd/tripbot and pkg/server, so the DB never reaches the
// overlay renderer's binary (the package-boundary-init-discipline ADR).
//
// Postgres owns the copy rather than the JetStream cache that pushes it to
// onscreens-server: that cache sits on a local-path PVC which `talosctl upgrade`
// wipes even with --preserve, and hand-authored copy isn't something a wipe
// should be able to discard. This table lands in the ordinary pg_dump instead.
package rotatorstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	rot "github.com/adanalife/tripbot/pkg/rotator"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// rotatorRow mirrors the onscreens_rotators table for GORM. The config travels
// as a JSONB document keyed by platform — see the migration for why it isn't a
// row per message.
//
// Config is a string, not []byte / json.RawMessage: lib/pq encodes a byte slice
// as bytea, which Postgres then refuses to coerce into jsonb ("invalid input
// syntax for type json"). A string is sent as text and casts cleanly.
type rotatorRow struct {
	Platform  string    `gorm:"primaryKey;column:platform"`
	Config    string    `gorm:"type:jsonb;column:config"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

func (rotatorRow) TableName() string { return "onscreens_rotators" }

// Store reads and writes per-platform rotator copy.
type Store struct {
	db *gorm.DB
}

func New(db *gorm.DB) *Store { return &Store{db: db} }

// Get returns the stored copy for platform. found=false means the platform has
// never been edited, and the caller should fall back to rot.DefaultConfigFor —
// distinguishing "no edits yet" from "edited to empty", which is a legitimate
// state the console can save.
func (s *Store) Get(ctx context.Context, platform string) (cfg rot.Config, found bool, err error) {
	var row rotatorRow
	tx := s.db.WithContext(ctx).Where("platform = ?", platform).Take(&row)
	if tx.Error != nil {
		if errors.Is(tx.Error, gorm.ErrRecordNotFound) {
			return rot.Config{}, false, nil
		}
		return rot.Config{}, false, fmt.Errorf("load rotator config for %s: %w", platform, tx.Error)
	}
	if err := json.Unmarshal([]byte(row.Config), &cfg); err != nil {
		// A row we can't decode is worse than no row: fall back to defaults
		// rather than serving nothing, but surface it as an error so it's visible.
		return rot.Config{}, false, fmt.Errorf("decode rotator config for %s: %w", platform, err)
	}
	return cfg, true, nil
}

// GetOrDefault returns the stored copy for platform, or the copy compiled into
// the binary (filtered to that platform) when nothing is stored yet. The
// second return reports whether the result came from the database, so callers
// can tell the console whether it's looking at saved copy or a prefill.
func (s *Store) GetOrDefault(ctx context.Context, platform string) (rot.Config, bool, error) {
	cfg, found, err := s.Get(ctx, platform)
	if err != nil || !found {
		return rot.DefaultConfigFor(platform), false, err
	}
	return cfg, true, nil
}

// Put replaces the stored copy for platform, upserting so the console's save is
// idempotent whether or not the platform has been edited before.
func (s *Store) Put(ctx context.Context, platform string, cfg rot.Config) error {
	raw, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("encode rotator config for %s: %w", platform, err)
	}
	row := rotatorRow{Platform: platform, Config: string(raw), UpdatedAt: time.Now().UTC()}
	tx := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "platform"}},
		DoUpdates: clause.AssignmentColumns([]string{"config", "updated_at"}),
	}).Create(&row)
	if tx.Error != nil {
		return fmt.Errorf("save rotator config for %s: %w", platform, tx.Error)
	}
	return nil
}

// Delete drops a platform's stored copy, reverting it to the defaults compiled
// into onscreens-server. Deleting a platform that was never edited is not an
// error — the end state is what was asked for either way.
func (s *Store) Delete(ctx context.Context, platform string) error {
	tx := s.db.WithContext(ctx).Where("platform = ?", platform).Delete(&rotatorRow{})
	if tx.Error != nil {
		return fmt.Errorf("delete rotator config for %s: %w", platform, tx.Error)
	}
	return nil
}

// Platforms lists the platforms with stored copy, so tripbot can republish all
// of them on startup and refill a wiped JetStream cache.
func (s *Store) Platforms(ctx context.Context) ([]string, error) {
	var out []string
	tx := s.db.WithContext(ctx).Model(&rotatorRow{}).Pluck("platform", &out)
	if tx.Error != nil {
		return nil, fmt.Errorf("list rotator platforms: %w", tx.Error)
	}
	return out, nil
}
