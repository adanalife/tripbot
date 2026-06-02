package users

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/adanalife/tripbot/pkg/helpers"
	"github.com/adanalife/tripbot/pkg/scoreboards"
	"github.com/adanalife/tripbot/pkg/twitch"
	"github.com/google/uuid"
	"github.com/logrusorgru/aurora/v3"
	"gorm.io/gorm"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/database"
)

type User struct {
	ID          uint16 `gorm:"primaryKey"`
	Username    string
	Miles       float32
	NumVisits   uint16
	HasDonated  bool
	IsBot       bool
	FirstSeen   time.Time
	LastSeen    time.Time
	DateCreated time.Time
	// in-memory session fields, not stored in DB
	LoggedIn     time.Time `gorm:"-"`
	sessionID    uuid.UUID `gorm:"-"`
	lastCmd      time.Time `gorm:"-"`
	lastLocation time.Time `gorm:"-"`
}

// this is how long they have before they can guess again
var guessCooldown = 3 * time.Minute

func (u User) loggedInDur() time.Duration {
	// exit early if they're not logged in
	if !isLoggedIn(u.Username) {
		return 0 * time.Second
	}
	// lookup the user in the session so the LoggedIn value is current
	return time.Now().Sub(LoggedIn[u.Username].LoggedIn)
}

func (u User) sessionMiles(ctx context.Context) float32 {
	// exit early if they're not logged in
	if !isLoggedIn(u.Username) {
		return 0.0
	}
	loggedInDur := u.loggedInDur()
	sessionMiles := helpers.DurationToMiles(loggedInDur)
	// give subscribers a miles bonus
	if u.IsSubscriber() {
		bonusMiles := u.BonusMiles()
		if c.Conf.Verbose {
			slog.InfoContext(ctx, "subscriber will get bonus miles", "username", u.Username, "bonus_miles", bonusMiles)
		}
		sessionMiles += bonusMiles
	}
	return sessionMiles
}

func (u User) CurrentMiles(ctx context.Context) float32 {
	return u.Miles + u.sessionMiles(ctx)
}

func (u User) BonusMiles() float32 {
	if isLoggedIn(u.Username) {
		loggedInDur := u.loggedInDur()
		sessionMiles := helpers.DurationToMiles(loggedInDur)
		return sessionMiles * 0.05
	}
	return 0.0
}

func (u User) CurrentMonthlyMiles(ctx context.Context) float32 {
	return u.GetScore(ctx, scoreboards.CurrentMilesScoreboard()) + u.sessionMiles(ctx)
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
	if c.Conf.Verbose {
		slog.InfoContext(ctx, "saving user", "username", u.Username)
	}
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
func SetBot(ctx context.Context, username string, isBot bool) error {
	user := Find(ctx, username)
	if user.ID == 0 {
		return gorm.ErrRecordNotFound
	}
	user.IsBot = isBot
	user.save(ctx)
	if loggedIn, ok := LoggedIn[username]; ok {
		loggedIn.IsBot = isBot
	}
	return nil
}

// IsFollower returns true if the user is a follower
func (u User) IsFollower() bool {
	return twitch.UserIsFollower(u.Username)
}

// IsSubscriber returns true if the user is a subscriber
func (u User) IsSubscriber() bool {
	return twitch.UserIsSubscriber(u.Username)
}

// User.String prints a colored version of the user
func (u User) String() string {
	if u.IsBot {
		return aurora.Gray(15, u.Username).String()
	}
	if c.UserIsAdmin(u.Username) {
		return aurora.Gray(11, u.Username).String()
	}
	return aurora.Magenta(u.Username).String()
}

// FindOrCreate will try to find the user in the DB, otherwise it will create a new user
func FindOrCreate(ctx context.Context, username string) User {
	if c.Conf.Verbose {
		slog.InfoContext(ctx, "FindOrCreate", "username", username)
	}
	user := Find(ctx, username)
	if user.ID != 0 {
		return user
	}
	// create the user in the DB
	return create(ctx, username)
}

// Find will look up the username in the DB, and return a User if possible
func Find(ctx context.Context, username string) User {
	var user User
	result := database.GormDB().WithContext(ctx).Where("username = ?", username).First(&user)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		//TODO: is there a better way to do this?
		return User{ID: 0}
	}
	if result.Error != nil {
		slog.ErrorContext(ctx, "error finding user", "err", result.Error)
		return User{ID: 0}
	}
	return user
}

// HasCommandAvailable lets users run a command once a day,
// unless they are a follower in which case they can run
// as many as they like
func (u *User) HasCommandAvailable(ctx context.Context) bool {
	// followers get unlimited commands
	if u.IsFollower() {
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

// TODO: maybe return an err here?
// create() will actually create the DB record
func create(ctx context.Context, username string) User {
	slog.InfoContext(ctx, "creating user", "username", username)
	// create a new row, using default vals and creating a single visit
	newUser := User{Username: username, NumVisits: 1}
	if err := database.GormDB().WithContext(ctx).Create(&newUser).Error; err != nil {
		slog.ErrorContext(ctx, "error creating user", "err", err)
	}
	return Find(ctx, username)
}
