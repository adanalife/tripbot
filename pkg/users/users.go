package users

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/adanalife/tripbot/pkg/helpers"
	"github.com/adanalife/tripbot/pkg/scoreboards"
	"github.com/google/uuid"
	"github.com/logrusorgru/aurora/v3"
	"gorm.io/gorm"

	"github.com/adanalife/tripbot/pkg/database"
)

type User struct {
	ID         uint16 `gorm:"primaryKey"`
	Username   string
	Platform   string
	Miles      float32
	NumVisits  uint16
	HasDonated bool
	IsBot      bool
	// ExcludeFromLeaderboard hides the account from every leaderboard read
	// while leaving it a normal chatter everywhere else. Independent of
	// IsBot, which carries behavioral meaning beyond ranking.
	ExcludeFromLeaderboard bool
	// autoCreateTime stamps these with the current time on insert. create()
	// builds a User without setting them, so without the tag GORM writes the
	// zero value (0001-01-01) into columns whose DEFAULT is CURRENT_TIMESTAMP —
	// which is why first_seen/date_created read back as "unknown" for every
	// account created after the GORM migration (#499). LastSeen self-heals on
	// the first save(), but is tagged for the same correctness on insert.
	FirstSeen   time.Time `gorm:"autoCreateTime"`
	LastSeen    time.Time `gorm:"autoCreateTime"`
	DateCreated time.Time `gorm:"autoCreateTime"`
	// in-memory session fields, not stored in DB
	LoggedIn     time.Time `gorm:"-"`
	sessionID    uuid.UUID `gorm:"-"`
	lastCmd      time.Time `gorm:"-"`
	lastLocation time.Time `gorm:"-"`
	// sessionExtraMiles accumulates community sub-grants received during this
	// session (GiveEveryoneMiles), so logout can record the full unreconstructable
	// bonus. Resets each login (fresh User from FindOrCreate).
	sessionExtraMiles float32 `gorm:"-"`
}

// this is how long they have before they can guess again
var guessCooldown = 3 * time.Minute

// The miles + follower/subscriber computations below are *Sessions methods
// (not User methods) because they read the session's live login map + chatter
// source. They take the User as a parameter so the session state and the
// per-user data stay explicitly separate.

func (s *Sessions) loggedInDur(u User) time.Duration {
	// lookup the user in the session so the LoggedIn value is current
	live, ok := s.get(u.Username)
	if !ok {
		return 0 * time.Second
	}
	return time.Since(live.LoggedIn)
}

func (s *Sessions) sessionMiles(ctx context.Context, u User) float32 {
	// exit early if they're not logged in
	if !s.isLoggedIn(u.Username) {
		return 0.0
	}
	loggedInDur := s.loggedInDur(u)
	sessionMiles := helpers.DurationToMiles(loggedInDur)
	// give subscribers a miles bonus
	if s.IsSubscriber(u) {
		sessionMiles += s.BonusMiles(u)
	}
	return sessionMiles
}

func (s *Sessions) CurrentMiles(ctx context.Context, u User) float32 {
	return u.Miles + s.sessionMiles(ctx, u)
}

// BonusMiles is the subscriber miles bonus accrued this session: 5% of session
// miles per subscription tier (tier 1 = 5%, tier 2 = 10%, tier 3 = 15%).
// Non-subscribers get the tier-1 rate — !bonusmiles reports the would-be bonus
// for anyone, and only sessionMiles' IsSubscriber gate decides who is awarded.
func (s *Sessions) BonusMiles(u User) float32 {
	if s.isLoggedIn(u.Username) {
		loggedInDur := s.loggedInDur(u)
		sessionMiles := helpers.DurationToMiles(loggedInDur)
		tier := s.source.SubscriberTier(u.Username)
		if tier < 1 {
			tier = 1
		}
		return sessionMiles * 0.05 * float32(tier)
	}
	return 0.0
}

func (s *Sessions) CurrentMonthlyMiles(ctx context.Context, u User) float32 {
	return u.GetScore(ctx, scoreboards.CurrentMilesScoreboard()) + s.sessionMiles(ctx, u)
}

// User.save() will take the given user and store it in the DB
func (u User) save(ctx context.Context) {
	// A zero ID means we never found or created a DB row for this user (e.g. a
	// transient DB error in Find). Model(&u).Updates() would build an UPDATE
	// with no WHERE clause, which GORM refuses ("WHERE conditions required").
	// Skip rather than emit that misleading error every tick.
	if u.ID == 0 {
		slog.WarnContext(ctx, "refusing to save user with no ID", "username", u.Username)
		return
	}
	// exclude_from_leaderboard is deliberately absent: nothing in the app sets
	// it, so writing the session's copy back would let a stale in-memory false
	// revert a flag set directly against the DB while the user was logged in.
	err := database.GormDB().WithContext(ctx).Model(&u).Updates(map[string]any{
		"last_seen":  u.LastSeen,
		"num_visits": u.NumVisits,
		"miles":      u.Miles,
		"is_bot":     u.IsBot,
	}).Error
	if err != nil {
		slog.ErrorContext(ctx, "error saving user", "err", err)
	}
}

// SetBot flips users.is_bot for a username. Returns gorm.ErrRecordNotFound
// if the user doesn't exist in the DB.
func (s *Sessions) SetBot(ctx context.Context, username string, isBot bool) error {
	user, err := Find(ctx, s.cfg.Platform, username)
	if err != nil {
		return err
	}
	user.IsBot = isBot
	user.save(ctx)
	s.mu.Lock()
	if loggedIn, ok := s.loggedIn[username]; ok {
		loggedIn.IsBot = isBot
	}
	s.mu.Unlock()
	return nil
}

// IsFollower returns true if the user is a follower
func (s *Sessions) IsFollower(u User) bool {
	return s.source.IsFollower(u.Username)
}

// IsSubscriber returns true if the user is a subscriber
func (s *Sessions) IsSubscriber(u User) bool {
	return s.source.IsSubscriber(u.Username)
}

// SubscriberTier returns the user's subscription tier (1–3), or 0 for a
// non-subscriber.
func (s *Sessions) SubscriberTier(u User) int {
	return s.source.SubscriberTier(u.Username)
}

// User.String prints a colored version of the user
func (u User) String() string {
	if u.IsBot {
		return aurora.Gray(15, u.Username).String()
	}
	return aurora.Magenta(u.Username).String()
}

// ErrLookupFailed and ErrCreateFailed mark which half of FindOrCreate gave up:
// an existing row that couldn't be read, versus a new row that couldn't be
// written. Both wrap the underlying DB error, so errors.Is still reaches it.
var (
	ErrLookupFailed = errors.New("users: lookup failed")
	ErrCreateFailed = errors.New("users: create failed")
)

// FindOrCreate will try to find the user in the DB, otherwise it will create a
// new user. Either failure comes back as a zero User alongside an error tagged
// with ErrLookupFailed or ErrCreateFailed; callers key off the zero ID to treat
// it as "no DB row" and retry on a later tick.
func FindOrCreate(ctx context.Context, platform, username string) (User, error) {
	user, err := Find(ctx, platform, username)
	if err == nil {
		return user, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		// A real DB error must not look like a new user — creating here could
		// duplicate an existing row.
		return User{}, fmt.Errorf("%w for %s: %w", ErrLookupFailed, username, err)
	}
	// create the user in the DB
	user, err = create(ctx, platform, username)
	if err != nil {
		return User{}, fmt.Errorf("%w: %w", ErrCreateFailed, err)
	}
	return user, nil
}

// Find looks up the username in the DB. A missing user surfaces as
// gorm.ErrRecordNotFound; any other error is a real DB failure.
func Find(ctx context.Context, platform, username string) (User, error) {
	var user User
	result := database.GormDB().WithContext(ctx).Where("platform = ? AND username = ?", platform, username).First(&user)
	if result.Error != nil {
		return User{}, result.Error
	}
	return user, nil
}

// HasCommandAvailable lets users run a command once a day,
// unless they are a follower in which case they can run
// as many as they like
func (s *Sessions) HasCommandAvailable(ctx context.Context, u *User) bool {
	// followers get unlimited commands
	if s.IsFollower(*u) {
		return true
	}
	// check if they ran a command in the last 24 hrs
	now := time.Now()
	if now.Sub(u.lastCmd) > 24*time.Hour {
		slog.InfoContext(ctx, "letting user run a command", "username", u.Username)
		// update their lastCmd time
		u.lastCmd = now
		return true
	}
	return false
}

// GuessCooldownRemaining returns the amount of time a user needs to
// wait before they can guess again
func (u User) GuessCooldownRemaining() time.Duration {
	now := time.Now()
	cooldownExpiry := u.lastLocation.Add(guessCooldown)

	if u.lastLocation.Add(guessCooldown).After(now) {
		return cooldownExpiry.Sub(now)
	}
	return 0 * time.Minute
}

// HasGuessCommandAvailable returns true if the user is allowed to use the guess command
func (u *User) HasGuessCommandAvailable(ctx context.Context, lastTimewarpTime time.Time) bool {
	// let the user run if there has been a timewarp recently
	if u.lastLocation.Before(lastTimewarpTime) {
		return true
	}

	// check if they ran a location command recently
	if u.GuessCooldownRemaining() <= 0 {
		slog.InfoContext(ctx, "letting user run guess command", "username", u.Username)
		return true
	}
	return false
}

func (u *User) SetLastLocationTime() {
	u.lastLocation = time.Now()
}

// create() inserts the DB record and reads it back, so the returned User
// carries the assigned ID and the DB-defaulted columns. Any failure is
// returned rather than folded into a zero User.
func create(ctx context.Context, platform, username string) (User, error) {
	slog.InfoContext(ctx, "creating user", "username", username)
	// create a new row, using default vals and creating a single visit
	newUser := User{Username: username, Platform: platform, NumVisits: 1}
	if err := database.GormDB().WithContext(ctx).Create(&newUser).Error; err != nil {
		return User{}, fmt.Errorf("create user %s: %w", username, err)
	}
	user, err := Find(ctx, platform, username)
	if err != nil {
		return User{}, fmt.Errorf("find user %s after create: %w", username, err)
	}
	return user, nil
}
