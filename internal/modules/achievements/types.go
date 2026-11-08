package achievements

import (
	"encoding/json"
	"errors"
	"time"
)

var (
	ErrInvalid   = errors.New("invalid input")
	ErrForbidden = errors.New("forbidden")
	ErrNotFound  = errors.New("not found")
)

const maxWeeklyTarget = 14

// UserBadge is a persisted unlock.
type UserBadge struct {
	BadgeCode  string          `json:"badge_code"`
	UnlockedAt time.Time       `json:"unlocked_at"`
	Meta       json.RawMessage `json:"meta,omitempty"`
}

// Streak is the cached consecutive-week snapshot.
type Streak struct {
	CurrentWeeks   int        `json:"current_weeks"`
	LongestWeeks   int        `json:"longest_weeks"`
	LastActiveWeek *time.Time `json:"last_active_week,omitempty"`
}

// GoalConfig is the stored weekly target.
type GoalConfig struct {
	WeeklyTarget  int  `json:"weekly_target"`
	ReminderOptIn bool `json:"reminder_opt_in"`
}

// WeekProgress is the current ISO week's match count vs target.
type WeekProgress struct {
	Start   time.Time `json:"start"`
	End     time.Time `json:"end"`
	Current int       `json:"current"`
	Target  int       `json:"target"`
	Reached bool      `json:"reached"`
}

// GoalState is GET/PUT /v1/me/goal.
type GoalState struct {
	WeeklyTarget  int          `json:"weekly_target"`
	ReminderOptIn bool         `json:"reminder_opt_in"`
	Week          WeekProgress `json:"week"`
	Streak        Streak       `json:"streak"`
}

// UnlockedEvent is SSE achievement.unlocked (one badge; batch via items).
type UnlockedEvent struct {
	BadgeCode  string          `json:"badge_code"`
	UnlockedAt time.Time       `json:"unlocked_at"`
	Meta       json.RawMessage `json:"meta,omitempty"`
}

// GoalProgressEvent is SSE goal.progress.
type GoalProgressEvent struct {
	WeekStart time.Time `json:"week_start"`
	Target    int       `json:"target"`
	Current   int       `json:"current"`
	Reached   bool      `json:"reached"`
}

// ReminderCandidate is a user who may receive a weekly reminder.
type ReminderCandidate struct {
	UserID        string
	WeeklyTarget  int
	ReminderOptIn bool
	Current       int
}

// PulseCandidate is a user with a stored IANA time zone.
type PulseCandidate struct {
	UserID   string
	TimeZone string
}

// PulseGoal is the weekly match count. Nil on the card when the target is 0.
type PulseGoal struct {
	Current int  `json:"current"`
	Target  int  `json:"target"`
	Reached bool `json:"reached"`
}

// PulseRank is the one ladder row. Nil when the player scored no points.
type PulseRank struct {
	GroupID   string `json:"group_id"`
	GroupName string `json:"group_name"`
	Rank      int    `json:"rank"`
	Points    int    `json:"points"`
}

// PulseFriends lists names of friends who finished a match this week.
type PulseFriends struct {
	Names  []string `json:"names"`
	Others int      `json:"others"`
}

// PulseCard is GET /v1/me/pulse. Empty parts are null. days_left is always 0..6.
type PulseCard struct {
	WeekStart time.Time     `json:"week_start"`
	WeekEnd   time.Time     `json:"week_end"`
	DaysLeft  int           `json:"days_left"`
	Goal      *PulseGoal    `json:"goal"`
	Rank      *PulseRank    `json:"rank"`
	Friends   *PulseFriends `json:"friends"`
}
