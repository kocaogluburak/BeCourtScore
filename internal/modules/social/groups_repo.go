package social

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

func (r *repo) insertGroup(ctx context.Context, name, sport, ownerID, code string) (Group, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Group{}, fmt.Errorf("insertGroup begin: %w", err)
	}
	defer tx.Rollback(ctx)

	var g Group
	err = tx.QueryRow(ctx, `
		INSERT INTO groups (name, sport, owner_id, invite_code)
		VALUES ($1, $2, $3, $4)
		RETURNING id, name, sport, owner_id, invite_code, created_at`,
		name, sport, ownerID, code).Scan(&g.ID, &g.Name, &g.Sport, &g.OwnerID, &g.InviteCode, &g.CreatedAt)
	if isUniqueViolation(err) {
		return Group{}, ErrConflict
	}
	if err != nil {
		return Group{}, fmt.Errorf("insertGroup: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO group_members (group_id, user_id) VALUES ($1, $2)`, g.ID, ownerID); err != nil {
		return Group{}, fmt.Errorf("insertGroup member: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Group{}, fmt.Errorf("insertGroup commit: %w", err)
	}
	g.MemberCount = 1
	g.IsOwner = true
	return g, nil
}

func (r *repo) listGroups(ctx context.Context, userID string) ([]Group, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT g.id, g.name, g.sport, g.owner_id, g.invite_code, g.created_at,
		       (SELECT count(*) FROM group_members m WHERE m.group_id = g.id)
		FROM groups g
		JOIN group_members me ON me.group_id = g.id AND me.user_id = $1
		ORDER BY g.created_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("listGroups: %w", err)
	}
	defer rows.Close()
	out := []Group{}
	for rows.Next() {
		var g Group
		if err := rows.Scan(&g.ID, &g.Name, &g.Sport, &g.OwnerID, &g.InviteCode, &g.CreatedAt, &g.MemberCount); err != nil {
			return nil, fmt.Errorf("listGroups scan: %w", err)
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func scanGroup(row pgx.Row) (Group, error) {
	var g Group
	err := row.Scan(&g.ID, &g.Name, &g.Sport, &g.OwnerID, &g.InviteCode, &g.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Group{}, ErrNotFound
	}
	if err != nil {
		return Group{}, err
	}
	return g, nil
}

func (r *repo) getGroup(ctx context.Context, id string) (Group, error) {
	g, err := scanGroup(r.pool.QueryRow(ctx, `
		SELECT id, name, sport, owner_id, invite_code, created_at
		FROM groups WHERE id = $1`, id))
	if err != nil && !errors.Is(err, ErrNotFound) {
		return Group{}, fmt.Errorf("getGroup: %w", err)
	}
	return g, err
}

func (r *repo) getGroupByCode(ctx context.Context, code string) (Group, error) {
	g, err := scanGroup(r.pool.QueryRow(ctx, `
		SELECT id, name, sport, owner_id, invite_code, created_at
		FROM groups WHERE invite_code = $1`, code))
	if err != nil && !errors.Is(err, ErrNotFound) {
		return Group{}, fmt.Errorf("getGroupByCode: %w", err)
	}
	return g, err
}

func (r *repo) addMember(ctx context.Context, groupID, userID string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO group_members (group_id, user_id) VALUES ($1, $2)
		ON CONFLICT DO NOTHING`, groupID, userID)
	if err != nil {
		return fmt.Errorf("addMember: %w", err)
	}
	return nil
}

func (r *repo) isMember(ctx context.Context, groupID, userID string) (bool, error) {
	var ok bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM group_members WHERE group_id = $1 AND user_id = $2)`, groupID, userID).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("isMember: %w", err)
	}
	return ok, nil
}

func (r *repo) removeMember(ctx context.Context, groupID, userID string) error {
	tag, err := r.pool.Exec(ctx, `
		DELETE FROM group_members WHERE group_id = $1 AND user_id = $2`, groupID, userID)
	if err != nil {
		return fmt.Errorf("removeMember: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *repo) deleteGroup(ctx context.Context, groupID string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM groups WHERE id = $1`, groupID)
	if err != nil {
		return fmt.Errorf("deleteGroup: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *repo) listMembers(ctx context.Context, groupID string) ([]GroupMember, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT u.id, u.nickname, u.name, u.surname, m.joined_at
		FROM group_members m
		JOIN users u ON u.id = m.user_id
		WHERE m.group_id = $1
		ORDER BY m.joined_at ASC, u.id`, groupID)
	if err != nil {
		return nil, fmt.Errorf("listMembers: %w", err)
	}
	defer rows.Close()
	out := []GroupMember{}
	for rows.Next() {
		var nickname, name, surname *string
		var userID string
		var joined time.Time
		if err := rows.Scan(&userID, &nickname, &name, &surname, &joined); err != nil {
			return nil, fmt.Errorf("listMembers scan: %w", err)
		}
		out = append(out, GroupMember{
			UserID:   userID,
			Name:     memberDisplayName(nickname, name, surname),
			Nickname: nickname,
			JoinedAt: joined,
		})
	}
	return out, rows.Err()
}

func (r *repo) listWeekMatches(ctx context.Context, groupID, sport string, start, end time.Time) ([]ladderMatch, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT sport, played_at,
		       COALESCE(player_a_user_id::text, ''),
		       COALESCE(player_a2_user_id::text, ''),
		       COALESCE(player_b_user_id::text, ''),
		       COALESCE(player_b2_user_id::text, ''),
		       sets_a, sets_b, winner_side::text
		FROM matches
		WHERE sport = $1
		  AND played_at >= $2 AND played_at < $3
		  AND (
		    player_a_user_id IN (SELECT user_id FROM group_members WHERE group_id = $4)
		    OR player_b_user_id IN (SELECT user_id FROM group_members WHERE group_id = $4)
		    OR player_a2_user_id IN (SELECT user_id FROM group_members WHERE group_id = $4)
		    OR player_b2_user_id IN (SELECT user_id FROM group_members WHERE group_id = $4)
		  )`, sport, start, end, groupID)
	if err != nil {
		return nil, fmt.Errorf("listWeekMatches: %w", err)
	}
	defer rows.Close()
	out := []ladderMatch{}
	for rows.Next() {
		var m ladderMatch
		if err := rows.Scan(&m.Sport, &m.PlayedAt, &m.PlayerA, &m.PlayerA2, &m.PlayerB, &m.PlayerB2, &m.SetsA, &m.SetsB, &m.WinnerSide); err != nil {
			return nil, fmt.Errorf("listWeekMatches scan: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
