package social

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// pulseFriendsSQL is one query: accepted friends who have not opted out of
// the feed, joined to finished matches in the week. Live matches and social
// nights are not read. One row per friend; the latest played_at orders them.
const pulseFriendsSQL = `
	WITH friends AS (` + friendIDsSQL + `)
	SELECT u.id::text, u.nickname, u.name, u.surname, MAX(m.played_at)
	FROM friends f
	JOIN users u ON u.id = f.fid AND NOT u.feed_opt_out
	JOIN matches m ON m.played_at >= $2 AND m.played_at < $3
	  AND (
	    m.player_a_user_id = u.id OR m.player_b_user_id = u.id
	    OR m.player_a2_user_id = u.id OR m.player_b2_user_id = u.id
	  )
	GROUP BY u.id, u.nickname, u.name, u.surname`

func (r *repo) loadPulseGroups(ctx context.Context, userID string) ([]pulseGroup, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT g.id::text, g.name, g.sport
		FROM groups g
		JOIN group_members me ON me.group_id = g.id AND me.user_id = $1`, userID)
	if err != nil {
		return nil, fmt.Errorf("pulse groups: %w", err)
	}
	defer rows.Close()
	var groups []pulseGroup
	var ids []string
	for rows.Next() {
		var g pulseGroup
		if err := rows.Scan(&g.ID, &g.Name, &g.Sport); err != nil {
			return nil, fmt.Errorf("pulse groups scan: %w", err)
		}
		groups = append(groups, g)
		ids = append(ids, g.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}
	mrows, err := r.pool.Query(ctx, `
		SELECT m.group_id::text, m.user_id::text, u.nickname, u.name, u.surname, m.joined_at
		FROM group_members m
		JOIN users u ON u.id = m.user_id
		WHERE m.group_id = ANY($1::uuid[])`, ids)
	if err != nil {
		return nil, fmt.Errorf("pulse members: %w", err)
	}
	defer mrows.Close()
	byGroup := map[string][]GroupMember{}
	for mrows.Next() {
		var groupID, user string
		var nickname, name, surname *string
		var joined time.Time
		if err := mrows.Scan(&groupID, &user, &nickname, &name, &surname, &joined); err != nil {
			return nil, fmt.Errorf("pulse members scan: %w", err)
		}
		byGroup[groupID] = append(byGroup[groupID], GroupMember{
			UserID:   user,
			Name:     memberDisplayName(nickname, name, surname),
			Nickname: nickname,
			JoinedAt: joined,
		})
	}
	if err := mrows.Err(); err != nil {
		return nil, err
	}
	for i := range groups {
		groups[i].Members = byGroup[groups[i].ID]
	}
	return groups, nil
}

func (r *repo) loadPulseMatches(ctx context.Context, memberIDs, sports []string, start, end time.Time) ([]ladderMatch, error) {
	if len(memberIDs) == 0 || len(sports) == 0 {
		return nil, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT sport, played_at,
		       COALESCE(player_a_user_id::text, ''),
		       COALESCE(player_a2_user_id::text, ''),
		       COALESCE(player_b_user_id::text, ''),
		       COALESCE(player_b2_user_id::text, ''),
		       sets_a, sets_b, winner_side::text
		FROM matches
		WHERE played_at >= $1 AND played_at < $2
		  AND sport = ANY($3::text[])
		  AND (
		    player_a_user_id = ANY($4::uuid[])
		    OR player_b_user_id = ANY($4::uuid[])
		    OR player_a2_user_id = ANY($4::uuid[])
		    OR player_b2_user_id = ANY($4::uuid[])
		  )`, start, end, sports, memberIDs)
	if err != nil {
		return nil, fmt.Errorf("pulse matches: %w", err)
	}
	defer rows.Close()
	out := []ladderMatch{}
	for rows.Next() {
		var m ladderMatch
		if err := rows.Scan(&m.Sport, &m.PlayedAt, &m.PlayerA, &m.PlayerA2, &m.PlayerB, &m.PlayerB2, &m.SetsA, &m.SetsB, &m.WinnerSide); err != nil {
			return nil, fmt.Errorf("pulse matches scan: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *repo) loadPulseNights(ctx context.Context, groupIDs []string, start, end time.Time) ([]socialNight, error) {
	if len(groupIDs) == 0 {
		return nil, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT t.id::text, t.format, t.status, COALESCE(t.group_id::text, ''), t.updated_at
		FROM tournaments t
		WHERE t.status = 'COMPLETED'
		  AND t.format IN ('AMERICANO', 'MEXICANO', 'KING_OF_THE_COURT')
		  AND t.updated_at >= $1 AND t.updated_at < $2
		  AND (t.group_id = ANY($3::uuid[]) OR t.group_id IS NULL)`, start, end, groupIDs)
	if err != nil {
		return nil, fmt.Errorf("pulse nights: %w", err)
	}
	defer rows.Close()
	var nights []socialNight
	var ids []string
	for rows.Next() {
		var n socialNight
		if err := rows.Scan(&n.TournamentID, &n.Format, &n.Status, &n.GroupID, &n.FinishedAt); err != nil {
			return nil, fmt.Errorf("pulse nights scan: %w", err)
		}
		nights = append(nights, n)
		ids = append(ids, n.TournamentID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nights, nil
	}
	parts, err := r.loadPulseNightParts(ctx, ids)
	if err != nil {
		return nil, err
	}
	fixtures, err := r.loadPulseNightFixtures(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range nights {
		players := scoreNightPlayers(parts[nights[i].TournamentID], fixtures[nights[i].TournamentID])
		nights[i].UserIDs = playerUserIDs(players)
		nights[i].Placements = rankNightPlayers(players)
	}
	return nights, nil
}

type pulseNightPart struct {
	ParticipantID string
	UserID        string
}

type pulseNightFixture struct {
	A, A2, B, B2 string
	Score        string
	Status       string
}

func (r *repo) loadPulseNightParts(ctx context.Context, tournamentIDs []string) (map[string][]pulseNightPart, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT tournament_id::text, id::text, user_id::text
		FROM tournament_participants
		WHERE status = 'CONFIRMED' AND tournament_id = ANY($1::uuid[])
		ORDER BY tournament_id, created_at`, tournamentIDs)
	if err != nil {
		return nil, fmt.Errorf("pulse night participants: %w", err)
	}
	defer rows.Close()
	out := map[string][]pulseNightPart{}
	for rows.Next() {
		var tid string
		var p pulseNightPart
		if err := rows.Scan(&tid, &p.ParticipantID, &p.UserID); err != nil {
			return nil, fmt.Errorf("pulse night participants scan: %w", err)
		}
		out[tid] = append(out[tid], p)
	}
	return out, rows.Err()
}

func (r *repo) loadPulseNightFixtures(ctx context.Context, tournamentIDs []string) (map[string][]pulseNightFixture, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT tournament_id::text,
		       COALESCE(participant_a_id::text, ''), COALESCE(participant_a2_id::text, ''),
		       COALESCE(participant_b_id::text, ''), COALESCE(participant_b2_id::text, ''),
		       COALESCE(score_summary, ''), status
		FROM tournament_matches
		WHERE tournament_id = ANY($1::uuid[])`, tournamentIDs)
	if err != nil {
		return nil, fmt.Errorf("pulse night matches: %w", err)
	}
	defer rows.Close()
	out := map[string][]pulseNightFixture{}
	for rows.Next() {
		var tid string
		var f pulseNightFixture
		if err := rows.Scan(&tid, &f.A, &f.A2, &f.B, &f.B2, &f.Score, &f.Status); err != nil {
			return nil, fmt.Errorf("pulse night matches scan: %w", err)
		}
		out[tid] = append(out[tid], f)
	}
	return out, rows.Err()
}

// scoreNightPlayers applies the same credit as loadNightPlayers, in one pass
// over the batch instead of one query per night.
func scoreNightPlayers(parts []pulseNightPart, fixtures []pulseNightFixture) []nightPlayerScore {
	partToUser := map[string]string{}
	byUser := map[string]*nightPlayerScore{}
	order := []string{}
	for _, p := range parts {
		partToUser[p.ParticipantID] = p.UserID
		if _, ok := byUser[p.UserID]; ok {
			continue
		}
		byUser[p.UserID] = &nightPlayerScore{UserID: p.UserID}
		order = append(order, p.UserID)
	}
	for _, f := range fixtures {
		if f.Status != "COMPLETED" {
			continue
		}
		aPts, bPts, ok := parseNightScore(f.Score)
		if !ok {
			continue
		}
		creditNight(byUser, partToUser, []string{f.A, f.A2}, aPts, aPts > bPts)
		creditNight(byUser, partToUser, []string{f.B, f.B2}, bPts, bPts > aPts)
	}
	out := make([]nightPlayerScore, 0, len(order))
	for _, id := range order {
		out = append(out, *byUser[id])
	}
	return out
}

const pulseMatchAudienceSQL = `
	WITH seeds AS (
		SELECT unnest($1::uuid[]) AS user_id
	),
	friend_aud AS (
		SELECT f.fid AS user_id
		FROM seeds s
		JOIN LATERAL (
			SELECT addressee_id AS fid FROM friendships
			WHERE requester_id = s.user_id AND status = 'accepted'
			UNION
			SELECT requester_id AS fid FROM friendships
			WHERE addressee_id = s.user_id AND status = 'accepted'
		) f ON TRUE
		JOIN users u ON u.id = f.fid AND NOT u.feed_opt_out
	),
	group_aud AS (
		SELECT gm.user_id
		FROM group_members gm
		WHERE gm.group_id IN (SELECT group_id FROM group_members WHERE user_id IN (SELECT user_id FROM seeds))
	)
	SELECT user_id::text FROM (
		SELECT user_id FROM seeds
		UNION
		SELECT user_id FROM friend_aud
		UNION
		SELECT user_id FROM group_aud
	) q`

const pulseNightGroupAudienceSQL = `
	SELECT user_id::text FROM (
		SELECT unnest($1::uuid[]) AS user_id
		UNION
		SELECT gm.user_id FROM group_members gm WHERE gm.group_id = $2::uuid
	) q`

const pulseNightOpenAudienceSQL = `
	SELECT user_id::text FROM (
		SELECT unnest($1::uuid[]) AS user_id
		UNION
		SELECT gm.user_id FROM group_members gm
		WHERE gm.group_id IN (SELECT group_id FROM group_members WHERE user_id = ANY($1::uuid[]))
	) q`

func cleanIDs(ids []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func (r *repo) matchAudience(ctx context.Context, participantIDs []string) ([]string, error) {
	ids := cleanIDs(participantIDs)
	if len(ids) == 0 {
		return nil, nil
	}
	return r.queryAudience(ctx, pulseMatchAudienceSQL, ids)
}

func (r *repo) nightAudience(ctx context.Context, playerIDs []string, groupID string) ([]string, error) {
	ids := cleanIDs(playerIDs)
	groupID = strings.TrimSpace(groupID)
	if groupID != "" {
		if len(ids) == 0 {
			ids = []string{}
		}
		rows, err := r.pool.Query(ctx, pulseNightGroupAudienceSQL, ids, groupID)
		if err != nil {
			return nil, fmt.Errorf("pulse night audience: %w", err)
		}
		return scanIDs(rows)
	}
	if len(ids) == 0 {
		return nil, nil
	}
	return r.queryAudience(ctx, pulseNightOpenAudienceSQL, ids)
}

func (r *repo) queryAudience(ctx context.Context, q string, ids []string) ([]string, error) {
	rows, err := r.pool.Query(ctx, q, ids)
	if err != nil {
		return nil, fmt.Errorf("pulse audience: %w", err)
	}
	return scanIDs(rows)
}

func scanIDs(rows pgx.Rows) ([]string, error) {
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		if id != "" {
			out = append(out, id)
		}
	}
	return out, rows.Err()
}

func (r *repo) loadPulseFriends(ctx context.Context, userID string, start, end time.Time) ([]pulseFriend, error) {
	rows, err := r.pool.Query(ctx, pulseFriendsSQL, userID, start, end)
	if err != nil {
		return nil, fmt.Errorf("pulse friends: %w", err)
	}
	defer rows.Close()
	var out []pulseFriend
	for rows.Next() {
		var id string
		var nickname, name, surname *string
		var played time.Time
		if err := rows.Scan(&id, &nickname, &name, &surname, &played); err != nil {
			return nil, fmt.Errorf("pulse friends scan: %w", err)
		}
		out = append(out, pulseFriend{
			UserID:   id,
			Name:     memberDisplayName(nickname, name, surname),
			PlayedAt: played,
		})
	}
	return out, rows.Err()
}
