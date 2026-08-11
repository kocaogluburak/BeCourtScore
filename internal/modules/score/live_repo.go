package score

import (
	"context"
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
	SetsA            int        `json:"sets_a"`
	SetsB            int        `json:"sets_b"`
	GamesA           int        `json:"games_a"`
	GamesB           int        `json:"games_b"`
	ScoreA           int        `json:"score_a"`
	ScoreB           int        `json:"score_b"`
	IsTieBreak       bool       `json:"is_tie_break"`
	SetsToWin        int        `json:"sets_to_win"`
	AdvantageEnabled bool       `json:"advantage_enabled"`
	PointsToWin      int        `json:"points_to_win"`
	WinnerSide       *string    `json:"winner_side"`
	CreatedBy        string     `json:"created_by"`
	HistoryMatchID   *string    `json:"history_match_id"`
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
	SetsToWin        int
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
	WinnerSide string
}

const liveCols = `id, sport, status, player_a_name, player_b_name, player_a_user_id, player_b_user_id,
	sets_a, sets_b, games_a, games_b, score_a, score_b, is_tie_break, sets_to_win, advantage_enabled,
	points_to_win, winner_side, created_by, history_match_id, started_at, ended_at, updated_at`

const openLiveStatuses = `status IN ('PENDING', 'IN_PROGRESS')`

func scanLive(row pgx.Row) (LiveMatch, error) {
	var m LiveMatch
	err := row.Scan(
		&m.ID, &m.Sport, &m.Status, &m.PlayerAName, &m.PlayerBName, &m.PlayerAUserID, &m.PlayerBUserID,
		&m.SetsA, &m.SetsB, &m.GamesA, &m.GamesB, &m.ScoreA, &m.ScoreB, &m.IsTieBreak, &m.SetsToWin,
		&m.AdvantageEnabled, &m.PointsToWin, &m.WinnerSide, &m.CreatedBy, &m.HistoryMatchID,
		&m.StartedAt, &m.EndedAt, &m.UpdatedAt,
	)
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
			sets_to_win, advantage_enabled, points_to_win, created_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		RETURNING ` + liveCols
	return scanLive(r.pool.QueryRow(ctx, q,
		in.Sport, status, in.PlayerAName, in.PlayerBName, in.PlayerAUserID, in.PlayerBUserID,
		in.SetsToWin, in.AdvantageEnabled, in.PointsToWin, createdBy,
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
			  AND (player_a_user_id = $2 OR player_b_user_id = $2)
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
	if u.WinnerSide != nil {
		m.WinnerSide = u.WinnerSide
	}
	const q = `
		UPDATE live_matches SET
			sets_a = $2, sets_b = $3, games_a = $4, games_b = $5,
			score_a = $6, score_b = $7, is_tie_break = $8, winner_side = $9,
			updated_at = NOW()
		WHERE id = $1 AND ` + openLiveStatuses + `
		RETURNING ` + liveCols
	return scanLive(r.pool.QueryRow(ctx, q, id,
		m.SetsA, m.SetsB, m.GamesA, m.GamesB, m.ScoreA, m.ScoreB, m.IsTieBreak, m.WinnerSide,
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

func (r *repo) listOpenLiveForParticipant(ctx context.Context, userID string, limit, offset int) ([]LiveMatch, int64, error) {
	var total int64
	if err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM live_matches
		WHERE `+openLiveStatuses+`
		  AND (created_by = $1 OR player_a_user_id = $1 OR player_b_user_id = $1)`,
		userID,
	).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.pool.Query(ctx, `
		SELECT `+liveCols+` FROM live_matches
		WHERE `+openLiveStatuses+`
		  AND (created_by = $1 OR player_a_user_id = $1 OR player_b_user_id = $1)
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

// ErrWrongState is returned when a live match is not in the expected status.
var ErrWrongState = errors.New("wrong state")
