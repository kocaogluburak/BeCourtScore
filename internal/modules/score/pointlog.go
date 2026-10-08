package score

import "time"

const maxPointLog = 500

// PointEvent is one scored point, captured before the score changes.
// Short JSON keys keep a long match small. Server is empty when the sport
// has no serve marker (squash, ping pong, or a match scored before serve tracking).
type PointEvent struct {
	Side      string `json:"s"`
	AtMs      int64  `json:"t"`
	SetsA     int    `json:"sa"`
	SetsB     int    `json:"sb"`
	GamesA    int    `json:"ga"`
	GamesB    int    `json:"gb"`
	ScoreA    int    `json:"pa"`
	ScoreB    int    `json:"pb"`
	Server    string `json:"sv,omitempty"`
	TieBreak  bool   `json:"tb,omitempty"`
	Advantage bool   `json:"adv"`
}

// MatchPointStats is derived for the match detail screen. It does not feed career W/L.
type MatchPointStats struct {
	DurationSeconds int  `json:"duration_seconds"`
	TotalPoints     int  `json:"total_points"`
	PointsA         int  `json:"points_a"`
	PointsB         int  `json:"points_b"`
	HasServer       bool `json:"has_server"`
	ServeWonA       int  `json:"serve_won_a"`
	ServePlayedA    int  `json:"serve_played_a"`
	ServeWonB       int  `json:"serve_won_b"`
	ServePlayedB    int  `json:"serve_played_b"`
	BreakWonA       int  `json:"break_won_a"`
	BreakFacedA     int  `json:"break_faced_a"`
	BreakWonB       int  `json:"break_won_b"`
	BreakFacedB     int  `json:"break_faced_b"`
}

func capPointLog(in []PointEvent) []PointEvent {
	if in == nil {
		return nil
	}
	out := make([]PointEvent, 0, len(in))
	for _, p := range in {
		if p.Side != "A" && p.Side != "B" {
			continue
		}
		if len(out) == maxPointLog {
			break
		}
		out = append(out, p)
	}
	return out
}

func DurationSeconds(started, ended time.Time) int {
	if ended.Before(started) {
		return 0
	}
	return int(ended.Sub(started) / time.Second)
}

func Summarize(points []PointEvent, durationSeconds *int) MatchPointStats {
	stats := MatchPointStats{}
	if durationSeconds != nil && *durationSeconds > 0 {
		stats.DurationSeconds = *durationSeconds
	}
	for _, p := range points {
		if p.Side == "A" {
			stats.PointsA++
		} else if p.Side == "B" {
			stats.PointsB++
		} else {
			continue
		}
		stats.TotalPoints++
		if p.Server == "" || p.TieBreak {
			continue
		}
		stats.HasServer = true
		serverA := p.Server == "A1" || p.Server == "A2"
		serverScore, returnScore := p.ScoreA, p.ScoreB
		if !serverA {
			serverScore, returnScore = p.ScoreB, p.ScoreA
		}
		returnerWon := (p.Side == "B" && serverA) || (p.Side == "A" && !serverA)
		if isBreakPoint(serverScore, returnScore, p.Advantage) {
			if serverA {
				stats.BreakFacedB++
				if returnerWon {
					stats.BreakWonB++
				}
			} else {
				stats.BreakFacedA++
				if returnerWon {
					stats.BreakWonA++
				}
			}
		}
		if !pointWinsGame(serverScore, returnScore, !returnerWon, p.Advantage) {
			continue
		}
		if serverA {
			stats.ServePlayedA++
			if !returnerWon {
				stats.ServeWonA++
			}
		} else {
			stats.ServePlayedB++
			if !returnerWon {
				stats.ServeWonB++
			}
		}
	}
	return stats
}

func isBreakPoint(serverScore, returnScore int, advantage bool) bool {
	if advantage {
		return returnScore >= 3 && returnScore > serverScore
	}
	if serverScore >= 3 && returnScore >= 3 {
		return true
	}
	return returnScore >= 3 && returnScore > serverScore
}

func pointWinsGame(serverScore, returnScore int, serverWon, advantage bool) bool {
	winner, loser := returnScore, serverScore
	if serverWon {
		winner, loser = serverScore, returnScore
	}
	if advantage {
		if winner == 4 {
			return true
		}
		return winner == 3 && loser < 3
	}
	if winner < 3 {
		return false
	}
	return loser < 3 || loser == winner
}
