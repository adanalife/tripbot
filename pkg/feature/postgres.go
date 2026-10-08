package feature

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/lib/pq"
	"gorm.io/gorm"
)

// flagRow mirrors the feature_flags table for GORM. Mapped into the public
// Flag type by toFlag; the columns we don't evaluate against (description,
// target_removal_date, timestamps) are loaded for the admin-panel surface
// even though Bool() only reads the targeting fields.
type flagRow struct {
	Key                 string         `gorm:"primaryKey;column:key"`
	Platform            string         `gorm:"primaryKey;column:platform"`
	Description         string         `gorm:"column:description"`
	Enabled             bool           `gorm:"column:enabled"`
	EnabledForUsernames pq.StringArray `gorm:"type:text[];column:enabled_for_usernames"`
	EnabledForRoles     pq.StringArray `gorm:"type:text[];column:enabled_for_roles"`
	TargetRemovalDate   time.Time      `gorm:"column:target_removal_date"`
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

func (flagRow) TableName() string { return "feature_flags" }

func (r flagRow) toFlag() Flag {
	return Flag{
		Key:                 r.Key,
		Description:         r.Description,
		Enabled:             r.Enabled,
		EnabledForUsernames: []string(r.EnabledForUsernames),
		EnabledForRoles:     []string(r.EnabledForRoles),
		TargetRemovalDate:   r.TargetRemovalDate,
	}
}

// repository is the DB-access seam for the Postgres client. Split from the
// client itself so the cache + refresh logic can be tested independently
// from the SQL. Scoped to one platform: every query filters on it, so a
// client only ever sees (and toggles) its own platform's rows.
type repository struct {
	db       *gorm.DB
	platform string
}

func newRepository(db *gorm.DB, platform string) *repository {
	return &repository{db: db, platform: platform}
}

// LoadAll fetches every flag row for the repository's platform. The table is
// small (bounded by the number of named flags in the codebase × platforms)
// so a platform-filtered SELECT is fine.
func (r *repository) LoadAll(ctx context.Context) (map[string]Flag, error) {
	var rows []flagRow
	if err := r.db.WithContext(ctx).Where("platform = ?", r.platform).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[string]Flag, len(rows))
	for _, row := range rows {
		out[row.Key] = row.toFlag()
	}
	return out, nil
}

// SetEnabled flips the enabled column for one flag on the repository's
// platform. RowsAffected == 0 means the key doesn't exist — surfaced as an
// error so an admin toggle of an unknown key fails loudly rather than
// silently no-op'ing.
func (r *repository) SetEnabled(ctx context.Context, key string, enabled bool) error {
	res := r.db.WithContext(ctx).
		Model(&flagRow{}).
		Where("key = ? AND platform = ?", key, r.platform).
		Updates(map[string]any{"enabled": enabled, "updated_at": time.Now()})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("feature flag %q not found", key)
	}
	return nil
}

// PostgresClient is a FlagClient backed by a Postgres-loaded snapshot
// refreshed in a background goroutine. The hot path (Bool) is a map read
// under an RWMutex — no DB hit per evaluation.
//
// On a refresh failure the previous snapshot is retained, so a transient DB
// outage cannot silently flip features. Recovery happens automatically at
// the next successful refresh tick.
type PostgresClient struct {
	repo     *repository
	interval time.Duration

	mu    sync.RWMutex
	flags map[string]Flag
}

// NewPostgresClient builds a client scoped to one platform's flag rows and
// performs the initial load. The platform comes from the caller (the binary's
// config) so this package stays free of binary-specific config imports. The
// initial load is synchronous — a startup failure surfaces here rather
// than hiding behind a background goroutine and serving false-by-default
// for every key.
func NewPostgresClient(ctx context.Context, db *gorm.DB, interval time.Duration, platform string) (*PostgresClient, error) {
	c := &PostgresClient{
		repo:     newRepository(db, platform),
		interval: interval,
		flags:    map[string]Flag{},
	}
	if err := c.refresh(ctx); err != nil {
		return nil, err
	}
	return c, nil
}

// Start runs the refresh loop until ctx is done. Intended to be invoked
// in a goroutine by the App.
func (c *PostgresClient) Start(ctx context.Context) {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := c.refresh(ctx); err != nil {
				slog.WarnContext(ctx, "feature flag refresh failed; retaining last-known-good",
					"err", err)
			}
		}
	}
}

func (c *PostgresClient) refresh(ctx context.Context) error {
	next, err := c.repo.LoadAll(ctx)
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.flags = next
	c.mu.Unlock()
	return nil
}

// SetEnabled persists the new global-default state for key, then force-loads
// the snapshot so the change is live immediately — both this client's Bool
// evaluations and its Snapshot reflect it without waiting for the next poll.
// Implements FlagToggler.
func (c *PostgresClient) SetEnabled(ctx context.Context, key string, enabled bool) error {
	if err := c.repo.SetEnabled(ctx, key, enabled); err != nil {
		return err
	}
	// Pull the fresh state in immediately. A refresh error here is unexpected
	// (the write just succeeded), but if it happens the next poll reconciles;
	// the DB is already authoritative, so we don't fail the toggle.
	if err := c.refresh(ctx); err != nil {
		slog.WarnContext(ctx, "flag toggle persisted but immediate refresh failed; next poll will reconcile",
			"err", err, "key", key)
	}
	return nil
}

// Bool evaluates the named flag against the cached snapshot. Returns false
// for unknown keys.
func (c *PostgresClient) Bool(_ context.Context, key string, evalCtx EvalContext) bool {
	c.mu.RLock()
	f, ok := c.flags[key]
	c.mu.RUnlock()
	if !ok {
		return false
	}
	return evaluate(f, evalCtx)
}

// Snapshot returns every cached flag, sorted by key. Reads the in-memory
// map under RLock — no DB hit. Reflects the most recent successful refresh;
// during a transient DB outage this is still the last-known-good set.
func (c *PostgresClient) Snapshot(_ context.Context) []Flag {
	c.mu.RLock()
	out := make([]Flag, 0, len(c.flags))
	for _, f := range c.flags {
		out = append(out, f)
	}
	c.mu.RUnlock()
	sortFlags(out)
	return out
}
