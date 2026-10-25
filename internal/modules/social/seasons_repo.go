package social

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type seasonStore interface {
	createSeason(ctx context.Context, s Season, members []seasonMember) (Season, error)
	getActiveSeason(ctx context.Context, groupID string) (Season, error)
	getSeason(ctx context.Context, id string) (Season, error)
	listSeasons(ctx context.Context, groupID string, limit, offset int) ([]Season, int64, error)
	listSeasonMembers(ctx context.Context, seasonID string) ([]seasonMember, error)
	listFrozenStandings(ctx context.Context, seasonID string) ([]SeasonStanding, error)
	freezeAndComplete(ctx context.Context, seasonID, championUserID string, rows []SeasonStanding) error
	listSeasonMatches(ctx context.Context, groupID, sport string, start, end time.Time) ([]ladderMatch, error)
	listSeasonSocialNights(ctx context.Context, groupID string, start, end time.Time) ([]socialNight, error)
	listDueActiveSeasons(ctx context.Context, now time.Time) ([]Season, error)
	listActiveSeasonsForUsers(ctx context.Context, userIDs []string) ([]Season, error)
	maxSeasonNumber(ctx context.Context, groupID string) (int, error)
}

func scanSeason(row pgx.Row) (Season, error) {
	var s Season
	var champion *string
	err := row.Scan(
		&s.ID, &s.GroupID, &s.SeasonNumber, &s.Name, &s.Status,
		&s.StartsAt, &s.EndsAt, &s.DivisionCount, &s.CountSocialNights,
		&champion, &s.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Season{}, ErrNotFound
	}
	if err != nil {
		return Season{}, err
	}
	s.ChampionUserID = champion
	return s, nil
}

func (r *repo) createSeason(ctx context.Context, s Season, members []seasonMember) (Season, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Season{}, fmt.Errorf("createSeason begin: %w", err)
	}
	defer tx.Rollback(ctx)

	var champion any
	if s.ChampionUserID != nil && *s.ChampionUserID != "" {
		champion = *s.ChampionUserID
	}
	created, err := scanSeason(tx.QueryRow(ctx, `
		INSERT INTO seasons (
			group_id, season_number, name, status, starts_at, ends_at,
			division_count, count_social_nights, champion_user_id
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		RETURNING id, group_id, season_number, name, status, starts_at, ends_at,
		          division_count, count_social_nights, champion_user_id::text, created_at`,
		s.GroupID, s.SeasonNumber, s.Name, s.Status, s.StartsAt, s.EndsAt,
		s.DivisionCount, s.CountSocialNights, champion))
	if isUniqueViolation(err) {
		return Season{}, ErrConflict
	}
	if err != nil {
		return Season{}, fmt.Errorf("createSeason: %w", err)
	}
	for _, m := range members {
		if _, err := tx.Exec(ctx, `
			INSERT INTO season_members (season_id, user_id, division_tier)
			VALUES ($1,$2,$3)
			ON CONFLICT DO NOTHING`, created.ID, m.UserID, m.DivisionTier); err != nil {
			return Season{}, fmt.Errorf("createSeason member: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Season{}, fmt.Errorf("createSeason commit: %w", err)
	}
	return created, nil
}

func (r *repo) getActiveSeason(ctx context.Context, groupID string) (Season, error) {
	s, err := scanSeason(r.pool.QueryRow(ctx, `
		SELECT id, group_id, season_number, name, status, starts_at, ends_at,
		       division_count, count_social_nights, champion_user_id::text, created_at
		FROM seasons WHERE group_id = $1 AND status = 'ACTIVE'`, groupID))
	if err != nil && !errors.Is(err, ErrNotFound) {
		return Season{}, fmt.Errorf("getActiveSeason: %w", err)
	}
	if errors.Is(err, ErrNotFound) {
		return Season{}, ErrNoActiveSeason
	}
	return s, nil
}

func (r *repo) getSeason(ctx context.Context, id string) (Season, error) {
	s, err := scanSeason(r.pool.QueryRow(ctx, `
		SELECT id, group_id, season_number, name, status, starts_at, ends_at,
		       division_count, count_social_nights, champion_user_id::text, created_at
		FROM seasons WHERE id = $1`, id))
	if err != nil && !errors.Is(err, ErrNotFound) {
		return Season{}, fmt.Errorf("getSeason: %w", err)
	}
	return s, err
}

func (r *repo) listSeasons(ctx context.Context, groupID string, limit, offset int) ([]Season, int64, error) {
	var total int64
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM seasons WHERE group_id = $1`, groupID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("listSeasons count: %w", err)
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, group_id, season_number, name, status, starts_at, ends_at,
		       division_count, count_social_nights, champion_user_id::text, created_at
		FROM seasons WHERE group_id = $1
		ORDER BY season_number DESC
		LIMIT $2 OFFSET $3`, groupID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("listSeasons: %w", err)
	}
	defer rows.Close()
	out := []Season{}
	for rows.Next() {
		s, err := scanSeason(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("listSeasons scan: %w", err)
		}
		out = append(out, s)
	}
	return out, total, rows.Err()
}

func (r *repo) listSeasonMembers(ctx context.Context, seasonID string) ([]seasonMember, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT sm.user_id, u.nickname, u.name, u.surname, sm.division_tier, sm.joined_at
		FROM season_members sm
		JOIN users u ON u.id = sm.user_id
		WHERE sm.season_id = $1
		ORDER BY sm.division_tier ASC, sm.joined_at ASC, sm.user_id`, seasonID)
	if err != nil {
		return nil, fmt.Errorf("listSeasonMembers: %w", err)
	}
	defer rows.Close()
	out := []seasonMember{}
	for rows.Next() {
		var nickname, name, surname *string
		var m seasonMember
		if err := rows.Scan(&m.UserID, &nickname, &name, &surname, &m.DivisionTier, &m.JoinedAt); err != nil {
			return nil, fmt.Errorf("listSeasonMembers scan: %w", err)
		}
		m.Name = memberDisplayName(nickname, name, surname)
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *repo) listFrozenStandings(ctx context.Context, seasonID string) ([]SeasonStanding, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT user_id, division_tier, rank, points, wins, losses, matches, set_diff, outcome, display_name
		FROM season_standings
		WHERE season_id = $1
		ORDER BY division_tier ASC, rank ASC, user_id`, seasonID)
	if err != nil {
		return nil, fmt.Errorf("listFrozenStandings: %w", err)
	}
	defer rows.Close()
	out := []SeasonStanding{}
	for rows.Next() {
		var s SeasonStanding
		if err := rows.Scan(&s.UserID, &s.DivisionTier, &s.Rank, &s.Points, &s.Wins, &s.Losses, &s.Matches, &s.SetDiff, &s.Outcome, &s.Name); err != nil {
			return nil, fmt.Errorf("listFrozenStandings scan: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *repo) freezeAndComplete(ctx context.Context, seasonID, championUserID string, rows []SeasonStanding) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("freeze begin: %w", err)
	}
	defer tx.Rollback(ctx)

	var status string
	err = tx.QueryRow(ctx, `SELECT status FROM seasons WHERE id = $1 FOR UPDATE`, seasonID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("freeze lock: %w", err)
	}
	if status != seasonStatusActive {
		return nil
	}
	if _, err := tx.Exec(ctx, `DELETE FROM season_standings WHERE season_id = $1`, seasonID); err != nil {
		return fmt.Errorf("freeze clear: %w", err)
	}
	for _, row := range rows {
		if row.Outcome == "" {
			row.Outcome = outcomeStayed
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO season_standings (
				season_id, user_id, division_tier, rank, points, wins, losses, matches, set_diff, outcome, display_name
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
			seasonID, row.UserID, row.DivisionTier, row.Rank, row.Points, row.Wins, row.Losses, row.Matches, row.SetDiff, row.Outcome, row.Name,
		); err != nil {
			return fmt.Errorf("freeze insert: %w", err)
		}
	}
	var champ any
	if championUserID != "" {
		champ = championUserID
	}
	tag, err := tx.Exec(ctx, `
		UPDATE seasons SET status = 'COMPLETED', champion_user_id = $2
		WHERE id = $1 AND status = 'ACTIVE'`, seasonID, champ)
	if err != nil {
		return fmt.Errorf("completeSeason: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("freeze commit: %w", err)
	}
	return nil
}

func (r *repo) listSeasonMatches(ctx context.Context, groupID, sport string, start, end time.Time) ([]ladderMatch, error) {
	return r.listWeekMatches(ctx, groupID, sport, start, end)
}

func (r *repo) listSeasonSocialNights(ctx context.Context, groupID string, start, end time.Time) ([]socialNight, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT t.id, t.format, t.status, COALESCE(t.group_id::text, ''), t.updated_at
		FROM tournaments t
		WHERE t.status = 'COMPLETED'
		  AND t.format IN ('AMERICANO','MEXICANO','KING_OF_THE_COURT')
		  AND t.updated_at >= $1 AND t.updated_at < $2
		  AND (t.group_id = $3 OR t.group_id IS NULL)`, start, end, groupID)
	if err != nil {
		return nil, fmt.Errorf("listSeasonSocialNights: %w", err)
	}
	defer rows.Close()
	var nights []socialNight
	for rows.Next() {
		var n socialNight
		if err := rows.Scan(&n.TournamentID, &n.Format, &n.Status, &n.GroupID, &n.FinishedAt); err != nil {
			return nil, fmt.Errorf("listSeasonSocialNights scan: %w", err)
		}
		nights = append(nights, n)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range nights {
		players, err := r.loadNightPlayers(ctx, nights[i].TournamentID)
		if err != nil {
			return nil, err
		}
		nights[i].UserIDs = playerUserIDs(players)
		nights[i].Placements = rankNightPlayers(players)
	}
	return nights, nil
}

type nightPlayerScore struct {
	UserID string
	Points int
	Won    int
	Played int
}

func (r *repo) loadNightPlayers(ctx context.Context, tournamentID string) ([]nightPlayerScore, error) {
	prows, err := r.pool.Query(ctx, `
		SELECT id::text, user_id::text FROM tournament_participants
		WHERE tournament_id = $1 AND status = 'CONFIRMED'`, tournamentID)
	if err != nil {
		return nil, fmt.Errorf("night participants: %w", err)
	}
	defer prows.Close()
	partToUser := map[string]string{}
	byUser := map[string]*nightPlayerScore{}
	order := []string{}
	for prows.Next() {
		var pid, uid string
		if err := prows.Scan(&pid, &uid); err != nil {
			return nil, fmt.Errorf("night participants scan: %w", err)
		}
		partToUser[pid] = uid
		if _, ok := byUser[uid]; !ok {
			byUser[uid] = &nightPlayerScore{UserID: uid}
			order = append(order, uid)
		}
	}
	if err := prows.Err(); err != nil {
		return nil, err
	}
	mrows, err := r.pool.Query(ctx, `
		SELECT COALESCE(participant_a_id::text, ''), COALESCE(participant_a2_id::text, ''),
		       COALESCE(participant_b_id::text, ''), COALESCE(participant_b2_id::text, ''),
		       COALESCE(score_summary, ''), status
		FROM tournament_matches WHERE tournament_id = $1`, tournamentID)
	if err != nil {
		return nil, fmt.Errorf("night matches: %w", err)
	}
	defer mrows.Close()
	for mrows.Next() {
		var a, a2, b, b2, score, status string
		if err := mrows.Scan(&a, &a2, &b, &b2, &score, &status); err != nil {
			return nil, fmt.Errorf("night matches scan: %w", err)
		}
		if status != "COMPLETED" {
			continue
		}
		aPts, bPts, ok := parseNightScore(score)
		if !ok {
			continue
		}
		creditNight(byUser, partToUser, []string{a, a2}, aPts, aPts > bPts)
		creditNight(byUser, partToUser, []string{b, b2}, bPts, bPts > aPts)
	}
	if err := mrows.Err(); err != nil {
		return nil, err
	}
	out := make([]nightPlayerScore, 0, len(order))
	for _, id := range order {
		out = append(out, *byUser[id])
	}
	return out, nil
}

func creditNight(byUser map[string]*nightPlayerScore, partToUser map[string]string, partIDs []string, points int, won bool) {
	seen := map[string]bool{}
	for _, pid := range partIDs {
		pid = strings.TrimSpace(pid)
		uid := partToUser[pid]
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

func parseNightScore(score string) (int, int, bool) {
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

func playerUserIDs(players []nightPlayerScore) []string {
	out := make([]string, 0, len(players))
	for _, p := range players {
		out = append(out, p.UserID)
	}
	return out
}

func rankNightPlayers(players []nightPlayerScore) []NightPlacement {
	sorted := append([]nightPlayerScore(nil), players...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Points != sorted[j].Points {
			return sorted[i].Points > sorted[j].Points
		}
		if sorted[i].Won != sorted[j].Won {
			return sorted[i].Won > sorted[j].Won
		}
		return sorted[i].UserID < sorted[j].UserID
	})
	out := make([]NightPlacement, 0, len(sorted))
	rank := 0
	for _, p := range sorted {
		played := p.Played > 0
		if played {
			rank++
		}
		out = append(out, NightPlacement{UserID: p.UserID, Rank: rank, Played: played})
	}
	return out
}

func (r *repo) listDueActiveSeasons(ctx context.Context, now time.Time) ([]Season, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, group_id, season_number, name, status, starts_at, ends_at,
		       division_count, count_social_nights, champion_user_id::text, created_at
		FROM seasons WHERE status = 'ACTIVE' AND ends_at <= $1
		ORDER BY ends_at ASC`, now)
	if err != nil {
		return nil, fmt.Errorf("listDueActiveSeasons: %w", err)
	}
	defer rows.Close()
	out := []Season{}
	for rows.Next() {
		s, err := scanSeason(rows)
		if err != nil {
			return nil, fmt.Errorf("listDueActiveSeasons scan: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *repo) listActiveSeasonsForUsers(ctx context.Context, userIDs []string) ([]Season, error) {
	if len(userIDs) == 0 {
		return nil, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT DISTINCT s.id, s.group_id, s.season_number, s.name, s.status, s.starts_at, s.ends_at,
		       s.division_count, s.count_social_nights, s.champion_user_id::text, s.created_at
		FROM seasons s
		JOIN season_members sm ON sm.season_id = s.id
		WHERE s.status = 'ACTIVE' AND sm.user_id = ANY($1::uuid[])`, userIDs)
	if err != nil {
		return nil, fmt.Errorf("listActiveSeasonsForUsers: %w", err)
	}
	defer rows.Close()
	out := []Season{}
	for rows.Next() {
		s, err := scanSeason(rows)
		if err != nil {
			return nil, fmt.Errorf("listActiveSeasonsForUsers scan: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *repo) maxSeasonNumber(ctx context.Context, groupID string) (int, error) {
	var n *int
	if err := r.pool.QueryRow(ctx, `SELECT max(season_number) FROM seasons WHERE group_id = $1`, groupID).Scan(&n); err != nil {
		return 0, fmt.Errorf("maxSeasonNumber: %w", err)
	}
	if n == nil {
		return 0, nil
	}
	return *n, nil
}
