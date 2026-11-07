package score

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type nightPlayer struct {
	UserID string
	Points int
	Won    int
	Played int
}

type socialFixture struct {
	SideA [2]string
	SideB [2]string
	APts  int
	BPts  int
}

// nightRanks assigns 1-based places to players who appeared in a completed round.
// Sit-outs are omitted. Order is points, rounds won, then user id.
func nightRanks(players []nightPlayer) map[string]int {
	sorted := append([]nightPlayer(nil), players...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Points != sorted[j].Points {
			return sorted[i].Points > sorted[j].Points
		}
		if sorted[i].Won != sorted[j].Won {
			return sorted[i].Won > sorted[j].Won
		}
		return sorted[i].UserID < sorted[j].UserID
	})
	ranks := map[string]int{}
	rank := 0
	for _, p := range sorted {
		if p.Played == 0 || p.UserID == "" {
			continue
		}
		rank++
		ranks[p.UserID] = rank
	}
	return ranks
}

func summarizeUserNights(userID string, nights [][]nightPlayer) SocialNightStats {
	var stats SocialNightStats
	for _, players := range nights {
		rank, ok := nightRanks(players)[userID]
		if !ok {
			continue
		}
		stats.Nights++
		if stats.BestRank == 0 || rank < stats.BestRank {
			stats.BestRank = rank
		}
	}
	return stats
}

// socialFixtureOutcome reports whether the viewer won a 2v2 point round.
// Ties and a viewer on both sides are ignored so they never enter set totals.
func socialFixtureOutcome(viewer string, row socialFixture) (won bool, ok bool) {
	onA := row.SideA[0] == viewer || row.SideA[1] == viewer
	onB := row.SideB[0] == viewer || row.SideB[1] == viewer
	if onA == onB || row.APts == row.BPts {
		return false, false
	}
	if onA {
		return row.APts > row.BPts, true
	}
	return row.BPts > row.APts, true
}

func countSocialH2H(viewer string, rows []socialFixture) SocialH2H {
	var out SocialH2H
	for _, row := range rows {
		won, ok := socialFixtureOutcome(viewer, row)
		if !ok {
			continue
		}
		out.Rounds++
		if won {
			out.Wins++
		} else {
			out.Losses++
		}
	}
	return out
}

func parseTeamPointScore(score string) (int, int, bool) {
	score = strings.TrimSpace(score)
	parts := strings.Split(score, "-")
	if len(parts) != 2 {
		return 0, 0, false
	}
	a, errA := strconv.Atoi(strings.TrimSpace(parts[0]))
	b, errB := strconv.Atoi(strings.TrimSpace(parts[1]))
	if errA != nil || errB != nil || a < 0 || b < 0 {
		return 0, 0, false
	}
	return a, b, true
}

func (r *repo) listUserSocialNights(ctx context.Context, userID, sport string) (SocialNightStats, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT t.id::text
		FROM tournaments t
		JOIN tournament_participants me ON me.tournament_id = t.id
		WHERE me.user_id = $1
		  AND me.status = 'CONFIRMED'
		  AND t.status = 'COMPLETED'
		  AND t.format IN ('AMERICANO','MEXICANO','KING_OF_THE_COURT')
		  AND ($2 = '' OR t.sport = $2)`, userID, sport)
	if err != nil {
		return SocialNightStats{}, fmt.Errorf("list user nights: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return SocialNightStats{}, fmt.Errorf("scan user night: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return SocialNightStats{}, err
	}
	if len(ids) == 0 {
		return SocialNightStats{}, nil
	}
	nights, err := r.loadNightPlayers(ctx, ids)
	if err != nil {
		return SocialNightStats{}, err
	}
	return summarizeUserNights(userID, nights), nil
}

func (r *repo) loadNightPlayers(ctx context.Context, tournamentIDs []string) ([][]nightPlayer, error) {
	prows, err := r.pool.Query(ctx, `
		SELECT tournament_id::text, id::text, user_id::text
		FROM tournament_participants
		WHERE status = 'CONFIRMED' AND tournament_id = ANY($1::uuid[])`, tournamentIDs)
	if err != nil {
		return nil, fmt.Errorf("night participants: %w", err)
	}
	defer prows.Close()
	partToUser := map[string]string{}
	byNight := map[string]map[string]*nightPlayer{}
	order := map[string][]string{}
	for prows.Next() {
		var tid, pid, uid string
		if err := prows.Scan(&tid, &pid, &uid); err != nil {
			return nil, fmt.Errorf("night participant scan: %w", err)
		}
		partToUser[pid] = uid
		if byNight[tid] == nil {
			byNight[tid] = map[string]*nightPlayer{}
		}
		if _, ok := byNight[tid][uid]; !ok {
			byNight[tid][uid] = &nightPlayer{UserID: uid}
			order[tid] = append(order[tid], uid)
		}
	}
	if err := prows.Err(); err != nil {
		return nil, err
	}

	mrows, err := r.pool.Query(ctx, `
		SELECT tournament_id::text,
		       COALESCE(participant_a_id::text, ''), COALESCE(participant_a2_id::text, ''),
		       COALESCE(participant_b_id::text, ''), COALESCE(participant_b2_id::text, ''),
		       COALESCE(score_summary, ''), status
		FROM tournament_matches
		WHERE tournament_id = ANY($1::uuid[])`, tournamentIDs)
	if err != nil {
		return nil, fmt.Errorf("night matches: %w", err)
	}
	defer mrows.Close()
	for mrows.Next() {
		var tid, a, a2, b, b2, score, status string
		if err := mrows.Scan(&tid, &a, &a2, &b, &b2, &score, &status); err != nil {
			return nil, fmt.Errorf("night match scan: %w", err)
		}
		if status != "COMPLETED" {
			continue
		}
		aPts, bPts, ok := parseTeamPointScore(score)
		if !ok {
			continue
		}
		creditNightSide(byNight[tid], partToUser, []string{a, a2}, aPts, aPts > bPts)
		creditNightSide(byNight[tid], partToUser, []string{b, b2}, bPts, bPts > aPts)
	}
	if err := mrows.Err(); err != nil {
		return nil, err
	}

	out := make([][]nightPlayer, 0, len(tournamentIDs))
	for _, tid := range tournamentIDs {
		players := make([]nightPlayer, 0, len(order[tid]))
		for _, uid := range order[tid] {
			if p := byNight[tid][uid]; p != nil {
				players = append(players, *p)
			}
		}
		out = append(out, players)
	}
	return out, nil
}

func creditNightSide(byUser map[string]*nightPlayer, partToUser map[string]string, partIDs []string, points int, won bool) {
	if byUser == nil {
		return
	}
	seen := map[string]bool{}
	for _, pid := range partIDs {
		uid := partToUser[strings.TrimSpace(pid)]
		if uid == "" || seen[uid] {
			continue
		}
		seen[uid] = true
		p := byUser[uid]
		if p == nil {
			continue
		}
		p.Points += points
		p.Played++
		if won {
			p.Won++
		}
	}
}

func (r *repo) listSocialH2H(ctx context.Context, viewerID, opponentID, sport string) (SocialH2H, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT COALESCE(pa.user_id::text, ''), COALESCE(pa2.user_id::text, ''),
		       COALESCE(pb.user_id::text, ''), COALESCE(pb2.user_id::text, ''),
		       COALESCE(tm.score_summary, '')
		FROM tournament_matches tm
		JOIN tournaments t ON t.id = tm.tournament_id
		JOIN tournament_participants pa ON pa.id = tm.participant_a_id
		LEFT JOIN tournament_participants pa2 ON pa2.id = tm.participant_a2_id
		JOIN tournament_participants pb ON pb.id = tm.participant_b_id
		LEFT JOIN tournament_participants pb2 ON pb2.id = tm.participant_b2_id
		WHERE t.status = 'COMPLETED'
		  AND t.format IN ('AMERICANO','MEXICANO','KING_OF_THE_COURT')
		  AND tm.status = 'COMPLETED'
		  AND ($3 = '' OR t.sport = $3)
		  AND (
		    ((pa.user_id = $1 OR pa2.user_id = $1) AND (pb.user_id = $2 OR pb2.user_id = $2))
		    OR
		    ((pa.user_id = $2 OR pa2.user_id = $2) AND (pb.user_id = $1 OR pb2.user_id = $1))
		  )`, viewerID, opponentID, sport)
	if err != nil {
		return SocialH2H{}, fmt.Errorf("list social h2h: %w", err)
	}
	defer rows.Close()
	var fixtures []socialFixture
	for rows.Next() {
		var a, a2, b, b2, score string
		if err := rows.Scan(&a, &a2, &b, &b2, &score); err != nil {
			return SocialH2H{}, fmt.Errorf("scan social h2h: %w", err)
		}
		aPts, bPts, ok := parseTeamPointScore(score)
		if !ok {
			continue
		}
		fixtures = append(fixtures, socialFixture{
			SideA: [2]string{a, a2},
			SideB: [2]string{b, b2},
			APts:  aPts,
			BPts:  bPts,
		})
	}
	if err := rows.Err(); err != nil {
		return SocialH2H{}, err
	}
	return countSocialH2H(viewerID, fixtures), nil
}
