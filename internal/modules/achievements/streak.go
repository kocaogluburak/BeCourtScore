package achievements

import "time"

// ISOWeek is the ISO week containing t, Monday 00:00 UTC inclusive
// through the next Monday exclusive. Matches social.isoWeek.
func ISOWeek(t time.Time) (start, end time.Time) {
	u := t.UTC()
	wd := int(u.Weekday())
	if wd == 0 {
		wd = 7
	}
	start = time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -(wd - 1))
	return start, start.AddDate(0, 0, 7)
}

// ISOWeekStart is the Monday 00:00 UTC of the ISO week containing t.
func ISOWeekStart(t time.Time) time.Time {
	start, _ := ISOWeek(t)
	return start
}

// ApplyStreak updates a snapshot after a counted match played at playedAt.
// Same-week activity is a no-op. A match in the immediately following week
// increments current; a gap >1 week resets current to 1.
func ApplyStreak(s Streak, playedAt, now time.Time) Streak {
	week := ISOWeekStart(playedAt)
	if s.LastActiveWeek == nil {
		s.CurrentWeeks = 1
		if s.LongestWeeks < 1 {
			s.LongestWeeks = 1
		}
		s.LastActiveWeek = &week
		return s
	}
	last := dateOnly(*s.LastActiveWeek)
	if !week.After(last) {
		return s
	}
	if week.Equal(last.AddDate(0, 0, 7)) {
		s.CurrentWeeks++
	} else {
		s.CurrentWeeks = 1
	}
	if s.CurrentWeeks > s.LongestWeeks {
		s.LongestWeeks = s.CurrentWeeks
	}
	s.LastActiveWeek = &week
	_ = now
	return s
}

// ReconcileStreak resets current_weeks to 0 once a full ISO week has passed
// with no counted match (last_active_week is older than last Monday).
func ReconcileStreak(s Streak, now time.Time) Streak {
	if s.LastActiveWeek == nil || s.CurrentWeeks == 0 {
		if s.CurrentWeeks != 0 && s.LastActiveWeek == nil {
			s.CurrentWeeks = 0
		}
		return s
	}
	thisWeek, _ := ISOWeek(now)
	last := dateOnly(*s.LastActiveWeek)
	// Still alive if last active is this week or the immediately previous week.
	if !last.Before(thisWeek.AddDate(0, 0, -7)) {
		return s
	}
	s.CurrentWeeks = 0
	return s
}

// PulseDaysLeft is the number of UTC days left in the ISO week excluding today
// (Monday 6 … Sunday 0). The goal card uses RemainingWeekDays, which counts today.
func PulseDaysLeft(now time.Time) int {
	u := now.UTC()
	wd := int(u.Weekday())
	if wd == 0 {
		return 0
	}
	return 7 - wd
}

// RemainingWeekDays is the number of UTC calendar days left in the ISO week
// including today (Mon=7 … Sun=1).
func RemainingWeekDays(now time.Time) int {
	_, end := ISOWeek(now)
	u := now.UTC()
	today := time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
	days := int(end.Sub(today).Hours() / 24)
	if days < 0 {
		return 0
	}
	return days
}

func dateOnly(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
}
