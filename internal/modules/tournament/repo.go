package tournament

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ── Domain models ────────────────────────────────────────────────────────────

// Tournament is an organizer-run bracket or round-robin competition.
type Tournament struct {
	ID               string     `json:"id"`
	Slug             string     `json:"slug"`
	JoinCode         string     `json:"join_code,omitempty"` // only surfaced to the organizer
	Name             string     `json:"name"`
	Sport            string     `json:"sport"`
	Format           string     `json:"format"`
	Status           string     `json:"status"`
	OrganizerID      string     `json:"organizer_id"`
	OrganizerName    string     `json:"organizer_name,omitempty"`
	MaxParticipants  int        `json:"max_participants"`
	ChampionID       *string    `json:"champion_id,omitempty"`
	ChampionName     *string    `json:"champion_name,omitempty"`
	StartsAt         *time.Time `json:"starts_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	ParticipantCount int        `json:"participant_count"`
	SetsToWin        int        `json:"sets_to_win"`
	AdvantageEnabled bool       `json:"advantage_enabled"`
	PointsToWin      int        `json:"points_to_win"`
}

// ListFilter narrows a tournament listing.
type ListFilter struct {
	Query string // substring match on tournament name
	Sport string // exact match on sport type
}

// Participant is a confirmed entrant in a tournament.
type Participant struct {
	ID          string `json:"id"`
	UserID      string `json:"user_id"`
	DisplayName string `json:"display_name"`
	Seed        *int   `json:"seed,omitempty"`
	Status      string `json:"status"`
}

// TournamentMatch is a single fixture (bracket node or round-robin game).
type TournamentMatch struct {
	ID              string  `json:"id"`
	Round           int     `json:"round"`
	PositionInRound int     `json:"position_in_round"`
	ParticipantAID  *string `json:"participant_a_id"`
	ParticipantBID  *string `json:"participant_b_id"`
	ParticipantAName *string `json:"participant_a_name"`
	ParticipantBName *string `json:"participant_b_name"`
	NextMatchID     *string `json:"next_match_id"`
	WinnerID        *string `json:"winner_participant_id"`
	ScoreSummary    *string `json:"score_summary"`
	Status          string  `json:"status"`
}

// matchDetail is the internal view used for authorization and progression.
type matchDetail struct {
	ID               string
	TournamentID     string
	Status           string
	ParticipantAID   *string
	ParticipantBID   *string
	AUserID          *string
	BUserID          *string
	NextMatchID      *string
	ReportedBy       *string
	TournamentStatus string
	Format           string
	OrganizerID      string
}

type createParams struct {
	Slug             string
	JoinCode         string
	Name             string
	Sport            string
	Format           string
	OrganizerID      string
	MaxParticipants  int
	StartsAt         *time.Time
	SetsToWin        int
	AdvantageEnabled bool
	PointsToWin      int
}

type seedAssignment struct {
	ParticipantID string
	Seed          int
}

// ── Sentinel errors ──────────────────────────────────────────────────────────

var (
	ErrNotFound      = errors.New("not found")
	ErrForbidden     = errors.New("forbidden")
	ErrInvalid       = errors.New("invalid input")
	ErrWrongCode     = errors.New("invalid join code")
	ErrWrongState    = errors.New("invalid tournament state")
	ErrFull          = errors.New("tournament full")
	ErrAlreadyJoined = errors.New("already joined")

	// errSlugTaken is internal; the service retries create with a new slug.
	errSlugTaken = errors.New("slug taken")
)

// ── Repository ───────────────────────────────────────────────────────────────

type repo struct{ pool *pgxpool.Pool }

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// tournamentCols selects all tournament fields including the champion's display
// name and organizer name via LEFT JOINs.
const tournamentCols = `t.id, t.slug, t.join_code, t.name, t.sport, t.format, t.status,
	t.organizer_id, COALESCE(NULLIF(org.nickname,''), org.email, '') AS organizer_name,
	t.max_participants, t.champion_id, champ.display_name, t.starts_at, t.created_at,
	COALESCE(pc.confirmed_count, 0),
	t.sets_to_win, t.advantage_enabled, t.points_to_win`

// tournamentFrom is the FROM clause that must accompany tournamentCols.
// Participant count uses a LATERAL subquery (index-backed) instead of a
// correlated COUNT in the SELECT list, which scales better on list endpoints.
const tournamentFrom = `tournaments t
	LEFT JOIN tournament_participants champ ON champ.id = t.champion_id
	LEFT JOIN users org ON org.id = t.organizer_id
	LEFT JOIN LATERAL (
		SELECT COUNT(*)::int AS confirmed_count
		FROM tournament_participants p
		WHERE p.tournament_id = t.id AND p.status = 'CONFIRMED'
	) pc ON TRUE`

// tournamentReturningCols mirrors tournamentCols but for INSERT ... RETURNING
// clauses where no table alias or JOIN is available. Champion and organizer
// name are NULL/empty on insert, so we return literals to keep column counts aligned.
const tournamentReturningCols = `id, slug, join_code, name, sport, format, status,
	organizer_id, ''::text AS organizer_name,
	max_participants, champion_id, NULL::text, starts_at, created_at,
	COALESCE((SELECT COUNT(*) FROM tournament_participants p
	          WHERE p.tournament_id = tournaments.id AND p.status = 'CONFIRMED'), 0),
	sets_to_win, advantage_enabled, points_to_win`

func scanTournament(row pgx.Row) (Tournament, error) {
	var t Tournament
	err := row.Scan(&t.ID, &t.Slug, &t.JoinCode, &t.Name, &t.Sport, &t.Format, &t.Status,
		&t.OrganizerID, &t.OrganizerName, &t.MaxParticipants, &t.ChampionID, &t.ChampionName,
		&t.StartsAt, &t.CreatedAt, &t.ParticipantCount, &t.SetsToWin, &t.AdvantageEnabled, &t.PointsToWin)
	return t, err
}

func (r *repo) create(ctx context.Context, in createParams) (Tournament, error) {
	q := fmt.Sprintf(`
		INSERT INTO tournaments (slug, join_code, name, sport, format, organizer_id, max_participants, starts_at, sets_to_win, advantage_enabled, points_to_win)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		RETURNING %s`, tournamentReturningCols)
	t, err := scanTournament(r.pool.QueryRow(ctx, q,
		in.Slug, in.JoinCode, in.Name, in.Sport, in.Format, in.OrganizerID, in.MaxParticipants, in.StartsAt,
		in.SetsToWin, in.AdvantageEnabled, in.PointsToWin))
	if isUniqueViolation(err) {
		return Tournament{}, errSlugTaken // slug clash; caller retries with a new slug
	}
	if err != nil {
		return Tournament{}, fmt.Errorf("insert tournament: %w", err)
	}
	return t, nil
}

func (r *repo) slugExists(ctx context.Context, slug string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM tournaments WHERE slug=$1)`, slug).Scan(&exists)
	return exists, err
}

func (r *repo) getByID(ctx context.Context, id string) (Tournament, error) {
	q := fmt.Sprintf(`SELECT %s FROM %s WHERE t.id=$1`, tournamentCols, tournamentFrom)
	t, err := scanTournament(r.pool.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Tournament{}, ErrNotFound
	}
	if err != nil {
		return Tournament{}, fmt.Errorf("get tournament: %w", err)
	}
	return t, nil
}

func (r *repo) getBySlug(ctx context.Context, slug string) (Tournament, error) {
	q := fmt.Sprintf(`SELECT %s FROM %s WHERE t.slug=$1`, tournamentCols, tournamentFrom)
	t, err := scanTournament(r.pool.QueryRow(ctx, q, slug))
	if errors.Is(err, pgx.ErrNoRows) {
		return Tournament{}, ErrNotFound
	}
	if err != nil {
		return Tournament{}, fmt.Errorf("get tournament by slug: %w", err)
	}
	return t, nil
}

func (r *repo) delete(ctx context.Context, id string) error {
	// tournament_participants and tournament_matches cascade via FK ON DELETE CASCADE.
	tag, err := r.pool.Exec(ctx, `DELETE FROM tournaments WHERE id=$1`, id)
	if err != nil {
		return fmt.Errorf("delete tournament: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *repo) setStatus(ctx context.Context, id, status string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE tournaments SET status=$2, updated_at=NOW() WHERE id=$1`, id, status)
	if err != nil {
		return fmt.Errorf("set status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *repo) setChampion(ctx context.Context, id, championID string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE tournaments SET champion_id=$2, status='COMPLETED', updated_at=NOW() WHERE id=$1`,
		id, championID)
	if err != nil {
		return fmt.Errorf("set champion: %w", err)
	}
	return nil
}

// ── Participants ─────────────────────────────────────────────────────────────

const participantCols = `id, user_id, display_name, seed, status`

func scanParticipant(row pgx.Row) (Participant, error) {
	var p Participant
	err := row.Scan(&p.ID, &p.UserID, &p.DisplayName, &p.Seed, &p.Status)
	return p, err
}

func (r *repo) addParticipant(ctx context.Context, tournamentID, userID string) (Participant, error) {
	const q = `
		INSERT INTO tournament_participants (tournament_id, user_id, display_name)
		SELECT $1, $2, COALESCE(NULLIF(u.nickname,''), NULLIF(TRIM(CONCAT_WS(' ', u.name, u.surname)),''), 'Player')
		FROM users u WHERE u.id = $2
		RETURNING id, user_id, display_name, seed, status`
	p, err := scanParticipant(r.pool.QueryRow(ctx, q, tournamentID, userID))
	if isUniqueViolation(err) {
		return Participant{}, ErrAlreadyJoined
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return Participant{}, ErrNotFound
	}
	if err != nil {
		return Participant{}, fmt.Errorf("add participant: %w", err)
	}
	return p, nil
}

func (r *repo) removeParticipant(ctx context.Context, tournamentID, userID string) error {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM tournament_participants WHERE tournament_id=$1 AND user_id=$2`, tournamentID, userID)
	if err != nil {
		return fmt.Errorf("remove participant: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *repo) participantByUser(ctx context.Context, tournamentID, userID string) (Participant, bool, error) {
	q := fmt.Sprintf(`SELECT %s FROM tournament_participants WHERE tournament_id=$1 AND user_id=$2`, participantCols)
	p, err := scanParticipant(r.pool.QueryRow(ctx, q, tournamentID, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Participant{}, false, nil
	}
	if err != nil {
		return Participant{}, false, fmt.Errorf("participant by user: %w", err)
	}
	return p, true, nil
}

func (r *repo) listParticipants(ctx context.Context, tournamentID string, limit, offset int) ([]Participant, int64, error) {
	var total int64
	if err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM tournament_participants WHERE tournament_id=$1`, tournamentID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count participants: %w", err)
	}
	q := fmt.Sprintf(`SELECT %s FROM tournament_participants
		WHERE tournament_id=$1 ORDER BY COALESCE(seed, 2147483647), created_at LIMIT $2 OFFSET $3`, participantCols)
	rows, err := r.pool.Query(ctx, q, tournamentID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list participants: %w", err)
	}
	defer rows.Close()

	list := []Participant{}
	for rows.Next() {
		p, err := scanParticipant(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan participant: %w", err)
		}
		list = append(list, p)
	}
	return list, total, rows.Err()
}

func (r *repo) confirmedParticipants(ctx context.Context, tournamentID string) ([]Participant, error) {
	q := fmt.Sprintf(`SELECT %s FROM tournament_participants
		WHERE tournament_id=$1 AND status='CONFIRMED' ORDER BY COALESCE(seed, 2147483647), created_at`, participantCols)
	rows, err := r.pool.Query(ctx, q, tournamentID)
	if err != nil {
		return nil, fmt.Errorf("confirmed participants: %w", err)
	}
	defer rows.Close()

	list := []Participant{}
	for rows.Next() {
		p, err := scanParticipant(rows)
		if err != nil {
			return nil, fmt.Errorf("scan participant: %w", err)
		}
		list = append(list, p)
	}
	return list, rows.Err()
}

func (r *repo) countConfirmed(ctx context.Context, tournamentID string) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM tournament_participants WHERE tournament_id=$1 AND status='CONFIRMED'`,
		tournamentID).Scan(&n)
	return n, err
}

func (r *repo) setSeeds(ctx context.Context, seeds []seedAssignment) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin seeds tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	for _, s := range seeds {
		if _, err = tx.Exec(ctx,
			`UPDATE tournament_participants SET seed=$2 WHERE id=$1`, s.ParticipantID, s.Seed); err != nil {
			return fmt.Errorf("set seed: %w", err)
		}
	}
	return tx.Commit(ctx)
}

func (r *repo) listMine(ctx context.Context, userID string, f ListFilter, limit, offset int) ([]Tournament, int64, error) {
	// UNION of organizer-leg + participant-leg (index-backed) instead of OR/EXISTS.
	baseWhere := `t.id IN (
		SELECT id FROM tournaments WHERE organizer_id = $1
		UNION
		SELECT tournament_id FROM tournament_participants WHERE user_id = $1)`
	args := []any{userID}

	where := baseWhere
	if f.Sport != "" {
		args = append(args, f.Sport)
		where += fmt.Sprintf(" AND t.sport = $%d", len(args))
	}
	if f.Query != "" {
		args = append(args, "%"+f.Query+"%")
		where += fmt.Sprintf(" AND t.name ILIKE $%d", len(args))
	}

	var total int64
	// Count without champion/organizer/LATERAL joins.
	if err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM tournaments t WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count mine: %w", err)
	}

	args = append(args, limit, offset)
	q := fmt.Sprintf(`SELECT %s FROM %s WHERE %s ORDER BY t.created_at DESC LIMIT $%d OFFSET $%d`,
		tournamentCols, tournamentFrom, where, len(args)-1, len(args))
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list mine: %w", err)
	}
	defer rows.Close()

	list := []Tournament{}
	for rows.Next() {
		t, err := scanTournament(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan tournament: %w", err)
		}
		list = append(list, t)
	}
	return list, total, rows.Err()
}

// ── Fixtures / matches ───────────────────────────────────────────────────────

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// replaceFixtures wipes any existing fixtures and inserts the generated plan,
// wiring next_match_id links in a second pass. All within one transaction.
func (r *repo) replaceFixtures(ctx context.Context, tournamentID string, plans []MatchPlan) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin fixtures tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err = tx.Exec(ctx, `DELETE FROM tournament_matches WHERE tournament_id=$1`, tournamentID); err != nil {
		return fmt.Errorf("clear fixtures: %w", err)
	}

	key := func(round, pos int) int { return round<<16 | pos }
	ids := make(map[int]string, len(plans))

	for _, p := range plans {
		var id string
		err = tx.QueryRow(ctx, `
			INSERT INTO tournament_matches
			  (tournament_id, round, position_in_round, participant_a_id, participant_b_id, winner_participant_id, status)
			VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`,
			tournamentID, p.Round, p.Position, nilIfEmpty(p.A), nilIfEmpty(p.B), nilIfEmpty(p.Winner), p.Status,
		).Scan(&id)
		if err != nil {
			return fmt.Errorf("insert fixture: %w", err)
		}
		ids[key(p.Round, p.Position)] = id
	}

	for _, p := range plans {
		if p.NextRound < 0 {
			continue
		}
		childID := ids[key(p.Round, p.Position)]
		nextID := ids[key(p.NextRound, p.NextPos)]
		if _, err = tx.Exec(ctx,
			`UPDATE tournament_matches SET next_match_id=$2 WHERE id=$1`, childID, nextID); err != nil {
			return fmt.Errorf("link next match: %w", err)
		}
	}

	return tx.Commit(ctx)
}

func (r *repo) bracketMatches(ctx context.Context, tournamentID string) ([]TournamentMatch, error) {
	const q = `
		SELECT m.id, m.round, m.position_in_round, m.participant_a_id, m.participant_b_id,
		       pa.display_name, pb.display_name,
		       m.next_match_id, m.winner_participant_id, m.score_summary, m.status
		FROM tournament_matches m
		LEFT JOIN tournament_participants pa ON pa.id = m.participant_a_id
		LEFT JOIN tournament_participants pb ON pb.id = m.participant_b_id
		WHERE m.tournament_id=$1
		ORDER BY m.round, m.position_in_round`
	rows, err := r.pool.Query(ctx, q, tournamentID)
	if err != nil {
		return nil, fmt.Errorf("bracket matches: %w", err)
	}
	defer rows.Close()

	list := []TournamentMatch{}
	for rows.Next() {
		var m TournamentMatch
		if err := rows.Scan(&m.ID, &m.Round, &m.PositionInRound, &m.ParticipantAID, &m.ParticipantBID,
			&m.ParticipantAName, &m.ParticipantBName,
			&m.NextMatchID, &m.WinnerID, &m.ScoreSummary, &m.Status); err != nil {
			return nil, fmt.Errorf("scan match: %w", err)
		}
		list = append(list, m)
	}
	return list, rows.Err()
}

func (r *repo) matchDetail(ctx context.Context, matchID string) (matchDetail, error) {
	const q = `
		SELECT m.id, m.tournament_id, m.status, m.participant_a_id, m.participant_b_id,
		       m.next_match_id, m.reported_by, pa.user_id, pb.user_id,
		       t.status, t.format, t.organizer_id
		FROM tournament_matches m
		JOIN tournaments t ON t.id = m.tournament_id
		LEFT JOIN tournament_participants pa ON pa.id = m.participant_a_id
		LEFT JOIN tournament_participants pb ON pb.id = m.participant_b_id
		WHERE m.id=$1`
	var d matchDetail
	err := r.pool.QueryRow(ctx, q, matchID).Scan(
		&d.ID, &d.TournamentID, &d.Status, &d.ParticipantAID, &d.ParticipantBID,
		&d.NextMatchID, &d.ReportedBy, &d.AUserID, &d.BUserID,
		&d.TournamentStatus, &d.Format, &d.OrganizerID)
	if errors.Is(err, pgx.ErrNoRows) {
		return matchDetail{}, ErrNotFound
	}
	if err != nil {
		return matchDetail{}, fmt.Errorf("match detail: %w", err)
	}
	return d, nil
}

func (r *repo) getMatch(ctx context.Context, matchID string) (TournamentMatch, error) {
	const q = `
		SELECT m.id, m.round, m.position_in_round, m.participant_a_id, m.participant_b_id,
		       pa.display_name, pb.display_name,
		       m.next_match_id, m.winner_participant_id, m.score_summary, m.status
		FROM tournament_matches m
		LEFT JOIN tournament_participants pa ON pa.id = m.participant_a_id
		LEFT JOIN tournament_participants pb ON pb.id = m.participant_b_id
		WHERE m.id=$1`
	var m TournamentMatch
	err := r.pool.QueryRow(ctx, q, matchID).Scan(&m.ID, &m.Round, &m.PositionInRound,
		&m.ParticipantAID, &m.ParticipantBID, &m.ParticipantAName, &m.ParticipantBName,
		&m.NextMatchID, &m.WinnerID, &m.ScoreSummary, &m.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return TournamentMatch{}, ErrNotFound
	}
	if err != nil {
		return TournamentMatch{}, fmt.Errorf("get match: %w", err)
	}
	return m, nil
}

func (r *repo) reportMatch(ctx context.Context, matchID, winnerID, score, reportedBy string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE tournament_matches
		SET status='PENDING_CONFIRMATION', winner_participant_id=$2, score_summary=$3, reported_by=$4, confirmed_by=NULL
		WHERE id=$1`, matchID, winnerID, score, reportedBy)
	if err != nil {
		return fmt.Errorf("report match: %w", err)
	}
	return nil
}

func (r *repo) clearReport(ctx context.Context, matchID string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE tournament_matches
		SET status='READY', winner_participant_id=NULL, score_summary=NULL, reported_by=NULL, confirmed_by=NULL
		WHERE id=$1`, matchID)
	if err != nil {
		return fmt.Errorf("clear report: %w", err)
	}
	return nil
}

func (r *repo) completeMatch(ctx context.Context, matchID, winnerID, score, confirmedBy string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE tournament_matches
		SET status='COMPLETED', winner_participant_id=$2, score_summary=$3, confirmed_by=$4
		WHERE id=$1`, matchID, winnerID, score, confirmedBy)
	if err != nil {
		return fmt.Errorf("complete match: %w", err)
	}
	return nil
}

// placeIntoNext puts the winner into the first empty slot of the next match,
// flipping it to READY once both slots are filled.
func (r *repo) placeIntoNext(ctx context.Context, nextMatchID, winnerID string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE tournament_matches
		SET participant_a_id = CASE WHEN participant_a_id IS NULL THEN $2 ELSE participant_a_id END,
		    participant_b_id = CASE WHEN participant_a_id IS NOT NULL AND participant_b_id IS NULL THEN $2 ELSE participant_b_id END
		WHERE id=$1`, nextMatchID, winnerID)
	if err != nil {
		return fmt.Errorf("place into next: %w", err)
	}
	_, err = r.pool.Exec(ctx, `
		UPDATE tournament_matches SET status='READY'
		WHERE id=$1 AND status='PENDING' AND participant_a_id IS NOT NULL AND participant_b_id IS NOT NULL`,
		nextMatchID)
	if err != nil {
		return fmt.Errorf("ready next: %w", err)
	}
	return nil
}

func (r *repo) remainingMatches(ctx context.Context, tournamentID string) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM tournament_matches WHERE tournament_id=$1 AND status NOT IN ('COMPLETED','BYE')`,
		tournamentID).Scan(&n)
	return n, err
}

func (r *repo) participantUserIDs(ctx context.Context, tournamentID string) ([]string, error) {
	const q = `
		SELECT user_id FROM tournament_participants WHERE tournament_id=$1 AND status='CONFIRMED'
		UNION
		SELECT organizer_id FROM tournaments WHERE id=$1`
	rows, err := r.pool.Query(ctx, q, tournamentID)
	if err != nil {
		return nil, fmt.Errorf("participant user ids: %w", err)
	}
	defer rows.Close()

	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan user id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
