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
