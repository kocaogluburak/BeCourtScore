package social

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type feedRepo struct{ pool *pgxpool.Pool }

const friendIDsSQL = `
	SELECT addressee_id AS fid FROM friendships
	WHERE requester_id = $1 AND status = 'accepted'
	UNION
	SELECT requester_id AS fid FROM friendships
	WHERE addressee_id = $1 AND status = 'accepted'`

func (r *feedRepo) insertActivity(ctx context.Context, rec MatchRecord) (activityRow, error) {
	sum := ActivitySummary{
		PlayerAName: rec.PlayerAName,
		PlayerBName: rec.PlayerBName,
		SetsA:       rec.SetsA,
		SetsB:       rec.SetsB,
		WinnerSide:  rec.WinnerSide,
		Winner:      rec.Winner,
		A2:          rec.PlayerA2Name,
		B2:          rec.PlayerB2Name,
		PlayedAt:    rec.PlayedAt,
	}
	raw, err := json.Marshal(sum)
	if err != nil {
		return activityRow{}, fmt.Errorf("marshal summary: %w", err)
	}
	const q = `
		INSERT INTO activities (actor_id, type, match_id, sport, summary)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, actor_id, type, match_id, sport, summary, created_at`
	var row activityRow
	var summaryJSON []byte
	err = r.pool.QueryRow(ctx, q, rec.ActorID, activityTypeMatch, rec.ID, rec.Sport, raw).
		Scan(&row.ID, &row.ActorID, &row.Type, &row.MatchID, &row.Sport, &summaryJSON, &row.CreatedAt)
	if err != nil {
		return activityRow{}, fmt.Errorf("insert activity: %w", err)
	}
	if err := json.Unmarshal(summaryJSON, &row.Summary); err != nil {
		return activityRow{}, fmt.Errorf("scan summary: %w", err)
	}
	return row, nil
}

func (r *feedRepo) deleteByMatchID(ctx context.Context, matchID string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM activities WHERE match_id = $1`, matchID)
	if err != nil {
		return fmt.Errorf("delete activity by match: %w", err)
	}
	return nil
}

func (r *feedRepo) listVisible(ctx context.Context, viewerID string, limit, offset int) ([]activityRow, int64, error) {
	const countQ = `
		WITH friends AS (` + friendIDsSQL + `)
		SELECT COUNT(*)
		FROM activities a
		JOIN users u ON u.id = a.actor_id
		WHERE a.actor_id IN (SELECT fid FROM friends)
		  AND NOT u.feed_opt_out`
	var total int64
	if err := r.pool.QueryRow(ctx, countQ, viewerID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count feed: %w", err)
	}

	const q = `
		WITH friends AS (` + friendIDsSQL + `)
		SELECT a.id, a.actor_id, a.type, a.match_id, a.sport, a.summary, a.created_at,
		       (SELECT COUNT(*) FROM activity_reactions r WHERE r.activity_id = a.id),
		       (SELECT COUNT(*) FROM activity_comments c WHERE c.activity_id = a.id),
		       EXISTS (
		           SELECT 1 FROM activity_reactions r
		           WHERE r.activity_id = a.id AND r.user_id = $1 AND r.type = 'kudos'
		       ),
		       u.id, u.nickname, u.name, u.surname, u.profile_icon
		FROM activities a
		JOIN users u ON u.id = a.actor_id
		WHERE a.actor_id IN (SELECT fid FROM friends)
		  AND NOT u.feed_opt_out
		ORDER BY a.created_at DESC
		LIMIT $2 OFFSET $3`
	rows, err := r.pool.Query(ctx, q, viewerID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list feed: %w", err)
	}
	defer rows.Close()

	out := []activityRow{}
	for rows.Next() {
		row, err := scanActivity(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, row)
	}
	return out, total, rows.Err()
}

func (r *feedRepo) getActivity(ctx context.Context, id string) (activityRow, error) {
	const q = `
		SELECT a.id, a.actor_id, a.type, a.match_id, a.sport, a.summary, a.created_at,
		       (SELECT COUNT(*) FROM activity_reactions r WHERE r.activity_id = a.id),
		       (SELECT COUNT(*) FROM activity_comments c WHERE c.activity_id = a.id),
		       FALSE,
		       u.id, u.nickname, u.name, u.surname, u.profile_icon
		FROM activities a
		JOIN users u ON u.id = a.actor_id
		WHERE a.id = $1`
	row, err := r.pool.Query(ctx, q, id)
	if err != nil {
		return activityRow{}, fmt.Errorf("get activity: %w", err)
	}
	defer row.Close()
	if !row.Next() {
		if err := row.Err(); err != nil {
			return activityRow{}, fmt.Errorf("get activity: %w", err)
		}
		return activityRow{}, ErrNotFound
	}
	return scanActivity(row)
}

func scanActivity(rows pgx.Rows) (activityRow, error) {
	var row activityRow
	var summaryJSON []byte
	if err := rows.Scan(
		&row.ID, &row.ActorID, &row.Type, &row.MatchID, &row.Sport, &summaryJSON, &row.CreatedAt,
		&row.ReactionCount, &row.CommentCount, &row.ViewerHasKudos,
		&row.Actor.ID, &row.Actor.Nickname, &row.Actor.Name, &row.Actor.Surname, &row.Actor.ProfileIcon,
	); err != nil {
		return activityRow{}, fmt.Errorf("scan activity: %w", err)
	}
	if err := json.Unmarshal(summaryJSON, &row.Summary); err != nil {
		return activityRow{}, fmt.Errorf("scan summary: %w", err)
	}
	return row, nil
}

func (r *feedRepo) addReaction(ctx context.Context, activityID, userID, typ string) (bool, int, error) {
	tag, err := r.pool.Exec(ctx, `
		INSERT INTO activity_reactions (activity_id, user_id, type)
		VALUES ($1, $2, $3)
		ON CONFLICT DO NOTHING`, activityID, userID, typ)
	if err != nil {
		return false, 0, fmt.Errorf("add reaction: %w", err)
	}
	var count int
	if err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM activity_reactions WHERE activity_id = $1`, activityID).Scan(&count); err != nil {
		return false, 0, fmt.Errorf("count reactions: %w", err)
	}
	return tag.RowsAffected() > 0, count, nil
}

func (r *feedRepo) removeReaction(ctx context.Context, activityID, userID, typ string) (int, error) {
	_, err := r.pool.Exec(ctx, `
		DELETE FROM activity_reactions
		WHERE activity_id = $1 AND user_id = $2 AND type = $3`, activityID, userID, typ)
	if err != nil {
		return 0, fmt.Errorf("remove reaction: %w", err)
	}
	var count int
	if err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM activity_reactions WHERE activity_id = $1`, activityID).Scan(&count); err != nil {
		return 0, fmt.Errorf("count reactions: %w", err)
	}
	return count, nil
}

func (r *feedRepo) listComments(ctx context.Context, activityID string, limit, offset int) ([]commentRow, int64, error) {
	var total int64
	if err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM activity_comments WHERE activity_id = $1`, activityID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count comments: %w", err)
	}
	rows, err := r.pool.Query(ctx, `
		SELECT c.id, c.activity_id, c.user_id, c.body, c.created_at,
		       u.id, u.nickname, u.name, u.surname, u.profile_icon
		FROM activity_comments c
		JOIN users u ON u.id = c.user_id
		WHERE c.activity_id = $1
		ORDER BY c.created_at ASC
		LIMIT $2 OFFSET $3`, activityID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list comments: %w", err)
	}
	defer rows.Close()
	out := []commentRow{}
	for rows.Next() {
		var row commentRow
		if err := rows.Scan(
			&row.ID, &row.ActivityID, &row.UserID, &row.Body, &row.CreatedAt,
			&row.User.ID, &row.User.Nickname, &row.User.Name, &row.User.Surname, &row.User.ProfileIcon,
		); err != nil {
			return nil, 0, fmt.Errorf("scan comment: %w", err)
		}
		out = append(out, row)
	}
	return out, total, rows.Err()
}

func (r *feedRepo) addComment(ctx context.Context, activityID, userID, body string) (commentRow, error) {
	const q = `
		INSERT INTO activity_comments (activity_id, user_id, body)
		VALUES ($1, $2, $3)
		RETURNING id, activity_id, user_id, body, created_at`
	var row commentRow
	err := r.pool.QueryRow(ctx, q, activityID, userID, body).
		Scan(&row.ID, &row.ActivityID, &row.UserID, &row.Body, &row.CreatedAt)
	if err != nil {
		return commentRow{}, fmt.Errorf("add comment: %w", err)
	}
	return row, nil
}

func (r *feedRepo) getComment(ctx context.Context, id string) (commentRow, error) {
	const q = `
		SELECT id, activity_id, user_id, body, created_at
		FROM activity_comments WHERE id = $1`
	var row commentRow
	err := r.pool.QueryRow(ctx, q, id).
		Scan(&row.ID, &row.ActivityID, &row.UserID, &row.Body, &row.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return commentRow{}, ErrNotFound
	}
	if err != nil {
		return commentRow{}, fmt.Errorf("get comment: %w", err)
	}
	return row, nil
}

func (r *feedRepo) deleteComment(ctx context.Context, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM activity_comments WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete comment: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *feedRepo) friendIDs(ctx context.Context, userID string) ([]string, error) {
	rows, err := r.pool.Query(ctx, friendIDsSQL, userID)
	if err != nil {
		return nil, fmt.Errorf("friend ids: %w", err)
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan friend id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *feedRepo) areFriends(ctx context.Context, a, b string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM friendships
			WHERE status = 'accepted'
			  AND LEAST(requester_id, addressee_id) = LEAST($1::uuid, $2::uuid)
			  AND GREATEST(requester_id, addressee_id) = GREATEST($1::uuid, $2::uuid))`, a, b).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("areFriends: %w", err)
	}
	return exists, nil
}

func (r *feedRepo) getUserSummary(ctx context.Context, id string) (UserSummary, error) {
	var u UserSummary
	err := r.pool.QueryRow(ctx, `SELECT `+userCols+` FROM users WHERE id = $1`, id).
		Scan(&u.ID, &u.Nickname, &u.Name, &u.Surname, &u.ProfileIcon)
	if errors.Is(err, pgx.ErrNoRows) {
		return UserSummary{}, ErrNotFound
	}
	if err != nil {
		return UserSummary{}, fmt.Errorf("getUserSummary: %w", err)
	}
	return u, nil
}

func (r *feedRepo) isOptedOut(ctx context.Context, userID string) (bool, error) {
	return r.getFeedOptOut(ctx, userID)
}

func (r *feedRepo) getFeedOptOut(ctx context.Context, userID string) (bool, error) {
	var opt bool
	err := r.pool.QueryRow(ctx, `SELECT feed_opt_out FROM users WHERE id = $1`, userID).Scan(&opt)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrNotFound
	}
	if err != nil {
		return false, fmt.Errorf("get feed_opt_out: %w", err)
	}
	return opt, nil
}

func (r *feedRepo) setFeedOptOut(ctx context.Context, userID string, optOut bool) error {
	tag, err := r.pool.Exec(ctx, `UPDATE users SET feed_opt_out = $2, updated_at = NOW() WHERE id = $1`, userID, optOut)
	if err != nil {
		return fmt.Errorf("set feed_opt_out: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
