package score

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

const rematchCols = `id, challenger_id, opponent_id, sport, sets_to_win, advantage_enabled,
       points_to_win, set_rule, final_set, source_match_id, status, live_match_id,
       created_at, updated_at`

func scanRematch(scan func(dest ...any) error) (RematchChallenge, error) {
	var ch RematchChallenge
	err := scan(
		&ch.ID, &ch.ChallengerID, &ch.OpponentID, &ch.Sport, &ch.SetsToWin, &ch.AdvantageEnabled,
		&ch.PointsToWin, &ch.SetRule, &ch.FinalSet, &ch.SourceMatchID, &ch.Status, &ch.LiveMatchID,
		&ch.CreatedAt, &ch.UpdatedAt,
	)
	if err != nil {
		return RematchChallenge{}, err
	}
	return ch, nil
}

func (r *repo) insertRematch(ctx context.Context, challengerID string, in RematchCreateInput) (RematchChallenge, error) {
	adv := true
	if in.AdvantageEnabled != nil {
		adv = *in.AdvantageEnabled
	}
	row := r.pool.QueryRow(ctx, `
		INSERT INTO rematch_challenges (
			challenger_id, opponent_id, sport, sets_to_win, advantage_enabled, points_to_win,
			set_rule, final_set, source_match_id
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		RETURNING `+rematchCols, challengerID, in.OpponentID, in.Sport, in.SetsToWin, adv, in.PointsToWin,
		in.SetRule, in.FinalSet, in.SourceMatchID)
	ch, err := scanRematch(row.Scan)
	if isUniqueViolation(err) {
		return RematchChallenge{}, fmt.Errorf("%w: rematch already pending", ErrConflict)
	}
	if err != nil {
		return RematchChallenge{}, fmt.Errorf("insert rematch: %w", err)
	}
	return ch, nil
}

func (r *repo) getRematch(ctx context.Context, id string) (RematchChallenge, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+rematchCols+` FROM rematch_challenges WHERE id = $1`, id)
	ch, err := scanRematch(row.Scan)
	if errors.Is(err, pgx.ErrNoRows) {
		return RematchChallenge{}, ErrNotFound
	}
	if err != nil {
		return RematchChallenge{}, fmt.Errorf("get rematch: %w", err)
	}
	return ch, nil
}

func (r *repo) listIncomingRematches(ctx context.Context, opponentID string, limit, offset int) ([]RematchChallenge, int64, error) {
	return r.listRematches(ctx, `opponent_id = $1 AND status = 'pending'`, opponentID, limit, offset)
}

func (r *repo) listOutgoingRematches(ctx context.Context, challengerID string, limit, offset int) ([]RematchChallenge, int64, error) {
	return r.listRematches(ctx, `challenger_id = $1 AND status = 'pending'`, challengerID, limit, offset)
}

func (r *repo) listRematches(ctx context.Context, where, userID string, limit, offset int) ([]RematchChallenge, int64, error) {
	var total int64
	if err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM rematch_challenges WHERE `+where, userID,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count rematches: %w", err)
	}
	rows, err := r.pool.Query(ctx,
		`SELECT `+rematchCols+` FROM rematch_challenges WHERE `+where+`
		 ORDER BY created_at DESC LIMIT $2 OFFSET $3`, userID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list rematches: %w", err)
	}
	defer rows.Close()
	out := []RematchChallenge{}
	for rows.Next() {
		ch, err := scanRematch(rows.Scan)
		if err != nil {
			return nil, 0, fmt.Errorf("scan rematch: %w", err)
		}
		out = append(out, ch)
	}
	return out, total, rows.Err()
}

func (r *repo) updateRematchStatus(ctx context.Context, id, fromStatus, toStatus string) (RematchChallenge, error) {
	row := r.pool.QueryRow(ctx, `
		UPDATE rematch_challenges
		   SET status = $3, updated_at = NOW()
		 WHERE id = $1 AND status = $2
		RETURNING `+rematchCols, id, fromStatus, toStatus)
	ch, err := scanRematch(row.Scan)
	if errors.Is(err, pgx.ErrNoRows) {
		return RematchChallenge{}, ErrWrongState
	}
	if err != nil {
		return RematchChallenge{}, fmt.Errorf("update rematch: %w", err)
	}
	return ch, nil
}

func (r *repo) expirePendingRematches(ctx context.Context, olderThan time.Time) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE rematch_challenges
		   SET status = 'expired', updated_at = NOW()
		 WHERE status = 'pending' AND created_at < $1`, olderThan)
	if err != nil {
		return fmt.Errorf("expire rematches: %w", err)
	}
	return nil
}

func (r *repo) userSummary(ctx context.Context, id string) (UserSummary, error) {
	var u UserSummary
	err := r.pool.QueryRow(ctx,
		`SELECT id, nickname, name, surname, profile_icon FROM users WHERE id = $1`, id,
	).Scan(&u.ID, &u.Nickname, &u.Name, &u.Surname, &u.ProfileIcon)
	if errors.Is(err, pgx.ErrNoRows) {
		return UserSummary{}, ErrNotFound
	}
	if err != nil {
		return UserSummary{}, fmt.Errorf("user summary: %w", err)
	}
	return u, nil
}
