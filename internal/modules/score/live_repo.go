package score

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// LiveMatch is an in-progress shared scoreboard session.
type LiveMatch struct {
	ID               string     `json:"id"`
	Sport            string     `json:"sport"`
	Status           string     `json:"status"`
	PlayerAName      string     `json:"player_a_name"`
	PlayerBName      string     `json:"player_b_name"`
	PlayerAUserID    *string    `json:"player_a_user_id"`
	PlayerBUserID    *string    `json:"player_b_user_id"`
	PlayerA2Name     *string    `json:"player_a2_name,omitempty"`
	PlayerB2Name     *string    `json:"player_b2_name,omitempty"`
	PlayerA2UserID   *string    `json:"player_a2_user_id,omitempty"`
	PlayerB2UserID   *string    `json:"player_b2_user_id,omitempty"`
	DeclinedUserIDs  []string   `json:"declined_user_ids,omitempty"`
	SetsA            int        `json:"sets_a"`
	SetsB            int        `json:"sets_b"`
	GamesA           int        `json:"games_a"`
	GamesB           int        `json:"games_b"`
	ScoreA           int        `json:"score_a"`
	ScoreB           int        `json:"score_b"`
	IsTieBreak       bool       `json:"is_tie_break"`
	ServerIsA        bool       `json:"server_is_a"`
	SetsToWin        int        `json:"sets_to_win"`
	SetRule          string     `json:"set_rule"`
	FinalSet         string     `json:"final_set"`
	AdvantageEnabled bool       `json:"advantage_enabled"`
	PointsToWin      int        `json:"points_to_win"`
	WinnerSide       *string    `json:"winner_side"`
	CreatedBy        string     `json:"created_by"`
	HistoryMatchID   *string    `json:"history_match_id"`
	ShareToken       *string    `json:"share_token,omitempty"`
	Archived         bool       `json:"archived"`
	StartedAt        time.Time  `json:"started_at"`
	EndedAt          *time.Time `json:"ended_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

type LiveStartInput struct {
	Sport            string
	PlayerAName      string
	PlayerBName      string
	PlayerAUserID    *string
	PlayerBUserID    *string
	PlayerA2Name     *string
	PlayerB2Name     *string
	PlayerA2UserID   *string
	PlayerB2UserID   *string
	SetsToWin        int
	SetRule          string
	FinalSet         string
	AdvantageEnabled bool
	PointsToWin      int
	Status           string // PENDING | IN_PROGRESS; set by Service before insert
}

type LiveScoreUpdate struct {
	SetsA      *int
	SetsB      *int
	GamesA     *int
	GamesB     *int
	ScoreA     *int
	ScoreB     *int
	IsTieBreak *bool
	ServerIsA  *bool
	WinnerSide *string
}

type LiveEndInput struct {
	SetsA      int
	SetsB      int
	GamesA     int
	GamesB     int
	ScoreA     int
	ScoreB     int
	IsTieBreak bool
	WinnerSide      string
	SetScores       []SetScore
	PointLog        []PointEvent
	DurationSeconds *int
}

const liveCols = `id, sport, status, player_a_name, player_b_name, player_a_user_id, player_b_user_id,
	player_a2_name, player_b2_name, player_a2_user_id, player_b2_user_id, declined_user_ids,
	sets_a, sets_b, games_a, games_b, score_a, score_b, is_tie_break, sets_to_win, advantage_enabled,
	points_to_win, set_rule, final_set, winner_side, created_by, history_match_id, share_token, server_is_a, started_at, ended_at, updated_at`

const openLiveStatuses = `status IN ('PENDING', 'IN_PROGRESS')`

func scanLive(row pgx.Row) (LiveMatch, error) {
	var m LiveMatch
	var declinedRaw []byte
	err := row.Scan(
		&m.ID, &m.Sport, &m.Status, &m.PlayerAName, &m.PlayerBName, &m.PlayerAUserID, &m.PlayerBUserID,
		&m.PlayerA2Name, &m.PlayerB2Name, &m.PlayerA2UserID, &m.PlayerB2UserID, &declinedRaw,
		&m.SetsA, &m.SetsB, &m.GamesA, &m.GamesB, &m.ScoreA, &m.ScoreB, &m.IsTieBreak, &m.SetsToWin,
		&m.AdvantageEnabled, &m.PointsToWin, &m.SetRule, &m.FinalSet, &m.WinnerSide, &m.CreatedBy, &m.HistoryMatchID,
		&m.ShareToken, &m.ServerIsA, &m.StartedAt, &m.EndedAt, &m.UpdatedAt,
	)
	if len(declinedRaw) > 0 {
		_ = json.Unmarshal(declinedRaw, &m.DeclinedUserIDs)
	}
	if m.DeclinedUserIDs == nil {
		m.DeclinedUserIDs = []string{}
	}
	m.Archived = m.HistoryMatchID != nil
	return m, err
}

func (r *repo) insertLive(ctx context.Context, createdBy string, in LiveStartInput) (LiveMatch, error) {
	status := in.Status
	if status == "" {
		status = "IN_PROGRESS"
	}
	const q = `
		INSERT INTO live_matches (
			sport, status, player_a_name, player_b_name, player_a_user_id, player_b_user_id,
			player_a2_name, player_b2_name, player_a2_user_id, player_b2_user_id,
			sets_to_win, advantage_enabled, points_to_win, set_rule, final_set, created_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
		RETURNING ` + liveCols
	return scanLive(r.pool.QueryRow(ctx, q,
		in.Sport, status, in.PlayerAName, in.PlayerBName, in.PlayerAUserID, in.PlayerBUserID,
		in.PlayerA2Name, in.PlayerB2Name, in.PlayerA2UserID, in.PlayerB2UserID,
		in.SetsToWin, in.AdvantageEnabled, in.PointsToWin, in.SetRule, in.FinalSet, createdBy,
	))
}

func (r *repo) getLiveByID(ctx context.Context, id string) (LiveMatch, error) {
	m, err := scanLive(r.pool.QueryRow(ctx,
		`SELECT `+liveCols+` FROM live_matches WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return LiveMatch{}, ErrNotFound
	}
	return m, err
}

func (r *repo) hasOpenLiveBetween(ctx context.Context, creatorID, opponentID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM live_matches
			WHERE created_by = $1
			  AND `+openLiveStatuses+`
			  AND (player_a_user_id = $2 OR player_b_user_id = $2
			    OR player_a2_user_id = $2 OR player_b2_user_id = $2)
		)`, creatorID, opponentID).Scan(&exists)
	return exists, err
}

func (r *repo) updateLiveScore(ctx context.Context, id string, u LiveScoreUpdate) (LiveMatch, error) {
	m, err := r.getLiveByID(ctx, id)
	if err != nil {
		return LiveMatch{}, err
	}
	if m.Status != "IN_PROGRESS" && m.Status != "PENDING" {
		return LiveMatch{}, ErrWrongState
	}
	if u.SetsA != nil {
		m.SetsA = *u.SetsA
	}
	if u.SetsB != nil {
		m.SetsB = *u.SetsB
	}
	if u.GamesA != nil {
		m.GamesA = *u.GamesA
	}
	if u.GamesB != nil {
		m.GamesB = *u.GamesB
	}
	if u.ScoreA != nil {
		m.ScoreA = *u.ScoreA
	}
	if u.ScoreB != nil {
		m.ScoreB = *u.ScoreB
	}
	if u.IsTieBreak != nil {
		m.IsTieBreak = *u.IsTieBreak
	}
	if u.ServerIsA != nil {
		m.ServerIsA = *u.ServerIsA
	}
	if u.WinnerSide != nil {
		m.WinnerSide = u.WinnerSide
	}
	const q = `
		UPDATE live_matches SET
			sets_a = $2, sets_b = $3, games_a = $4, games_b = $5,
			score_a = $6, score_b = $7, is_tie_break = $8, winner_side = $9, server_is_a = $10,
			updated_at = NOW()
		WHERE id = $1 AND ` + openLiveStatuses + `
		RETURNING ` + liveCols
	return scanLive(r.pool.QueryRow(ctx, q, id,
		m.SetsA, m.SetsB, m.GamesA, m.GamesB, m.ScoreA, m.ScoreB, m.IsTieBreak, m.WinnerSide, m.ServerIsA,
	))
}

func (r *repo) endLive(ctx context.Context, id string, in LiveEndInput, historyID *string) (LiveMatch, error) {
	const q = `
		UPDATE live_matches SET
			status = 'ENDED',
			sets_a = $2, sets_b = $3, games_a = $4, games_b = $5,
			score_a = $6, score_b = $7, is_tie_break = $8, winner_side = $9,
			history_match_id = $10, ended_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND ` + openLiveStatuses + `
		RETURNING ` + liveCols
	m, err := scanLive(r.pool.QueryRow(ctx, q, id,
		in.SetsA, in.SetsB, in.GamesA, in.GamesB, in.ScoreA, in.ScoreB, in.IsTieBreak, in.WinnerSide, historyID,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		existing, gerr := r.getLiveByID(ctx, id)
		if gerr != nil {
			return LiveMatch{}, gerr
		}
		if existing.Status == "ENDED" {
			return LiveMatch{}, ErrWrongState
		}
		return LiveMatch{}, ErrNotFound
	}
	return m, err
}

func (r *repo) cancelLive(ctx context.Context, id string) (LiveMatch, error) {
	const q = `
		UPDATE live_matches SET
			status = 'ENDED', ended_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND ` + openLiveStatuses + `
		RETURNING ` + liveCols
	m, err := scanLive(r.pool.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) {
		existing, gerr := r.getLiveByID(ctx, id)
		if gerr != nil {
			return LiveMatch{}, gerr
		}
		if existing.Status == "ENDED" {
			return LiveMatch{}, ErrWrongState
		}
		return LiveMatch{}, ErrNotFound
	}
	return m, err
}

func (r *repo) acceptLive(ctx context.Context, id string) (LiveMatch, error) {
	const q = `
		UPDATE live_matches SET
			status = 'IN_PROGRESS', updated_at = NOW()
		WHERE id = $1 AND status = 'PENDING'
		RETURNING ` + liveCols
	m, err := scanLive(r.pool.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) {
		existing, gerr := r.getLiveByID(ctx, id)
		if gerr != nil {
			return LiveMatch{}, gerr
		}
		if existing.Status != "PENDING" {
			return LiveMatch{}, ErrWrongState
		}
		return LiveMatch{}, ErrNotFound
	}
	return m, err
}

const liveParticipant = `(created_by = $1 OR (
		(player_a_user_id = $1 OR player_b_user_id = $1 OR player_a2_user_id = $1 OR player_b2_user_id = $1)
		AND NOT (COALESCE(declined_user_ids, '[]'::jsonb) ? $1)
	))`

func (r *repo) listOpenLiveForParticipant(ctx context.Context, userID string, limit, offset int) ([]LiveMatch, int64, error) {
	var total int64
	if err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM live_matches
		WHERE `+openLiveStatuses+`
		  AND `+liveParticipant,
		userID,
	).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.pool.Query(ctx, `
		SELECT `+liveCols+` FROM live_matches
		WHERE `+openLiveStatuses+`
		  AND `+liveParticipant+`
		ORDER BY started_at DESC
		LIMIT $2 OFFSET $3`,
		userID, limit, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []LiveMatch
	for rows.Next() {
		m, err := scanLive(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, m)
	}
	return out, total, rows.Err()
}

// expireStaleLive ends open sessions whose updated_at is strictly before olderThan.
func (r *repo) expireStaleLive(ctx context.Context, olderThan time.Time) ([]LiveMatch, error) {
	rows, err := r.pool.Query(ctx, `
		UPDATE live_matches SET
			status = 'ENDED', ended_at = NOW(), updated_at = NOW()
		WHERE `+openLiveStatuses+` AND updated_at < $1
		RETURNING `+liveCols, olderThan)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LiveMatch
	for rows.Next() {
		m, err := scanLive(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *repo) setShareToken(ctx context.Context, id, token string) (LiveMatch, error) {
	m, err := scanLive(r.pool.QueryRow(ctx, `
		UPDATE live_matches SET share_token = $2, updated_at = NOW()
		WHERE id = $1
		RETURNING `+liveCols, id, token))
	if errors.Is(err, pgx.ErrNoRows) {
		return LiveMatch{}, ErrNotFound
	}
	return m, err
}

func (r *repo) getLiveByShareToken(ctx context.Context, token string) (LiveMatch, error) {
	m, err := scanLive(r.pool.QueryRow(ctx,
		`SELECT `+liveCols+` FROM live_matches WHERE share_token = $1`, token))
	if errors.Is(err, pgx.ErrNoRows) {
		return LiveMatch{}, ErrNotFound
	}
	return m, err
}

func (r *repo) setDeclined(ctx context.Context, id string, userIDs []string) (LiveMatch, error) {
	if userIDs == nil {
		userIDs = []string{}
	}
	raw, err := json.Marshal(userIDs)
	if err != nil {
		return LiveMatch{}, err
	}
	m, err := scanLive(r.pool.QueryRow(ctx, `
		UPDATE live_matches SET declined_user_ids = $2::jsonb, updated_at = NOW()
		WHERE id = $1 AND status = 'PENDING'
		RETURNING `+liveCols, id, raw))
	if errors.Is(err, pgx.ErrNoRows) {
		return LiveMatch{}, ErrWrongState
	}
	return m, err
}

// ErrWrongState is returned when a live match is not in the expected status.
var ErrWrongState = errors.New("wrong state")
