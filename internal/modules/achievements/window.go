package achievements

import "time"

// PulseWindow is the local 18:00 hour of the last evening that still falls
// inside the open ISO week. A nil location never sends. Hour 18 is inclusive
// through 18:59; 19:00 is the next hour and is closed.
func PulseWindow(now time.Time, loc *time.Location) (weekStart time.Time, ok bool) {
	if loc == nil {
		return time.Time{}, false
	}
	local := now.In(loc)
	if local.Hour() != 18 {
		return time.Time{}, false
	}
	weekStart, weekEnd := ISOWeek(now)
	nextEvening := time.Date(local.Year(), local.Month(), local.Day(), 18, 0, 0, 0, loc).AddDate(0, 0, 1)
	if nextEvening.Before(weekEnd) {
		return time.Time{}, false
	}
	return weekStart, true
}
