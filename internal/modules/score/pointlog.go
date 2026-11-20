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
	Server      string `json:"sv,omitempty"`
	TieBreak    bool   `json:"tb,omitempty"`
	Advantage   bool   `json:"adv"`
	PointsToWin int    `json:"ptw,omitempty"`
	Rally       bool   `json:"rl,omitempty"`
	// Outcome is optional point-end tag: "ace" | "df" | "winner" | "ue".
	Outcome string `json:"o,omitempty"`
	// OutcomeSide is who the outcome is attributed to: "A" | "B".
	OutcomeSide string `json:"op,omitempty"`
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
	AcesA           int  `json:"aces_a"`
	AcesB           int  `json:"aces_b"`
	DoubleFaultsA   int  `json:"double_faults_a"`
	DoubleFaultsB   int  `json:"double_faults_b"`
	WinnersA        int  `json:"winners_a"`
	WinnersB        int  `json:"winners_b"`
	UnforcedErrorsA int  `json:"unforced_errors_a"`
	UnforcedErrorsB int  `json:"unforced_errors_b"`
	TaggedPoints    int  `json:"tagged_points"`
}

func sanitizeOutcome(p *PointEvent) {
	switch p.Outcome {
	case "ace", "df", "winner", "ue":
	default:
		p.Outcome = ""
	}
	if p.OutcomeSide != "A" && p.OutcomeSide != "B" {
		p.OutcomeSide = ""
	}
	if p.Outcome == "" {
		p.OutcomeSide = ""
	}
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
		if p.PointsToWin < 0 || p.PointsToWin > 99 {
			p.PointsToWin = 0
		}
		sanitizeOutcome(&p)
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
		tallyOutcome(&stats, p)
		if !isTennisServeSlot(p.Server) || p.TieBreak {
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

func tallyOutcome(stats *MatchPointStats, p PointEvent) {
	if p.Outcome == "" {
		return
	}
	side := p.OutcomeSide
	if side != "A" && side != "B" {
		return
	}
	stats.TaggedPoints++
	switch p.Outcome {
	case "ace":
		if side == "A" {
			stats.AcesA++
		} else {
			stats.AcesB++
		}
	case "df":
		if side == "A" {
			stats.DoubleFaultsA++
		} else {
			stats.DoubleFaultsB++
		}
	case "winner":
		if side == "A" {
			stats.WinnersA++
		} else {
			stats.WinnersB++
		}
	case "ue":
		if side == "A" {
			stats.UnforcedErrorsA++
		} else {
			stats.UnforcedErrorsB++
		}
	default:
		stats.TaggedPoints--
	}
}

func isTennisServeSlot(sv string) bool {
	return sv == "A1" || sv == "A2" || sv == "B1" || sv == "B2"
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
