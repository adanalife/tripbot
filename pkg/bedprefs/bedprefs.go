// Package bedprefs persists the album bed's selection and shuffle per platform
// in Postgres, so a tripbot restart resumes the choice instead of narrowing it
// to whichever album OBS happens to be playing.
//
// It lives apart from pkg/obs/beds so the bed store and its tests stay off the
// database; cmd/tripbot wires the two together.
package bedprefs

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type prefsRow struct {
	Platform  string    `gorm:"primaryKey;column:platform"`
	Album     string    `gorm:"column:album"`
	Shuffle   bool      `gorm:"column:shuffle"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

func (prefsRow) TableName() string { return "background_audio_prefs" }

// Store reads and writes per-platform album bed preferences.
type Store struct {
	db *gorm.DB
}

func New(db *gorm.DB) *Store { return &Store{db: db} }

// Get returns the saved selection for platform. found=false means nothing was
// ever saved, and the bed store keeps its defaults.
func (s *Store) Get(ctx context.Context, platform string) (album string, shuffle, found bool, err error) {
	var row prefsRow
	tx := s.db.WithContext(ctx).Where("platform = ?", platform).Take(&row)
	if tx.Error != nil {
		if errors.Is(tx.Error, gorm.ErrRecordNotFound) {
			return "", false, false, nil
		}
		return "", false, false, fmt.Errorf("load background audio prefs for %s: %w", platform, tx.Error)
	}
	return row.Album, row.Shuffle, true, nil
}

// Save upserts platform's selection.
func (s *Store) Save(ctx context.Context, platform, album string, shuffle bool) error {
	row := prefsRow{Platform: platform, Album: album, Shuffle: shuffle, UpdatedAt: time.Now().UTC()}
	tx := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "platform"}},
		DoUpdates: clause.AssignmentColumns([]string{"album", "shuffle", "updated_at"}),
	}).Create(&row)
	if tx.Error != nil {
		return fmt.Errorf("save background audio prefs for %s: %w", platform, tx.Error)
	}
	return nil
}
