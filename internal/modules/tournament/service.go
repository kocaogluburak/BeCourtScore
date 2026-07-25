package tournament

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"time"

	"courtscore/internal/platform/sse"

	"github.com/jackc/pgx/v5/pgxpool"
)

var validSports = map[string]bool{
	"TENNIS": true, "PADEL": true, "SQUASH": true, "PING_PONG": true,
}
var validFormats = map[string]bool{
	"SINGLE_ELIM": true, "ROUND_ROBIN": true,
}

// store is the persistence surface the Service depends on. Implemented by *repo;
// stubbed in tests so business rules can be verified without a database.
type store interface {
	create(ctx context.Context, in createParams) (Tournament, error)
	slugExists(ctx context.Context, slug string) (bool, error)
	getByID(ctx context.Context, id string) (Tournament, error)
	getBySlug(ctx context.Context, slug string) (Tournament, error)
	delete(ctx context.Context, id string) error
	setStatus(ctx context.Context, id, status string) error
	setChampion(ctx context.Context, id, championID string) error

	addParticipant(ctx context.Context, tournamentID, userID string) (Participant, error)
	removeParticipant(ctx context.Context, tournamentID, userID string) error
	participantByUser(ctx context.Context, tournamentID, userID string) (Participant, bool, error)
	listParticipants(ctx context.Context, tournamentID string, limit, offset int) ([]Participant, int64, error)
	confirmedParticipants(ctx context.Context, tournamentID string) ([]Participant, error)
	countConfirmed(ctx context.Context, tournamentID string) (int, error)
	setSeeds(ctx context.Context, seeds []seedAssignment) error
	listMine(ctx context.Context, userID string, f ListFilter, limit, offset int) ([]Tournament, int64, error)

	replaceFixtures(ctx context.Context, tournamentID string, plans []MatchPlan) error
	bracketMatches(ctx context.Context, tournamentID string) ([]TournamentMatch, error)
	matchDetail(ctx context.Context, matchID string) (matchDetail, error)
	getMatch(ctx context.Context, matchID string) (TournamentMatch, error)
	reportMatch(ctx context.Context, matchID, winnerID, score, reportedBy string) error
	clearReport(ctx context.Context, matchID string) error
	completeMatch(ctx context.Context, matchID, winnerID, score, confirmedBy string) error
	placeIntoNext(ctx context.Context, nextMatchID, winnerID string) error
	remainingMatches(ctx context.Context, tournamentID string) (int, error)
	participantUserIDs(ctx context.Context, tournamentID string) ([]string, error)
}

// Service is the tournament domain's business logic layer.
type Service struct {
	store store
	hub   *sse.Hub
}

// NewService wires the Service to Postgres and the SSE hub.
func NewService(pool *pgxpool.Pool, hub *sse.Hub) *Service {
	return &Service{store: &repo{pool: pool}, hub: hub}
}

// ── View types ───────────────────────────────────────────────────────────────

// TournamentView adds caller-relative flags to a Tournament.
type TournamentView struct {
	Tournament
	IsOrganizer bool `json:"is_organizer"`
	HasJoined   bool `json:"has_joined"`
}

// BracketRound groups fixtures of a single round.
type BracketRound struct {
	Round   int               `json:"round"`
	Matches []TournamentMatch `json:"matches"`
}

// Standing is a computed row of the round-robin table.
type Standing struct {
	ParticipantID string `json:"participant_id"`
	DisplayName   string `json:"display_name"`
	Played        int    `json:"played"`
	Won           int    `json:"won"`
	Lost          int    `json:"lost"`
	Diff          int    `json:"diff"`
	Points        int    `json:"points"`
}

// Bracket is the full fixture view returned to clients.
type Bracket struct {
	Format    string         `json:"format"`
	Rounds    []BracketRound `json:"rounds"`
	Standings []Standing     `json:"standings"`
}

// CreateInput holds the fields required to open a tournament.
type CreateInput struct {
	Name             string
	Sport            string
	Format           string
	MaxParticipants  int
	StartsAt         *time.Time
	SetsToWin        int
	AdvantageEnabled bool
	PointsToWin      int
}

// ── Create / read ────────────────────────────────────────────────────────────

func (s *Service) CreateTournament(ctx context.Context, userID string, in CreateInput) (Tournament, error) {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return Tournament{}, fmt.Errorf("%w: name required", ErrInvalid)
	}
	if !validSports[in.Sport] {
		return Tournament{}, fmt.Errorf("%w: unknown sport %q", ErrInvalid, in.Sport)
	}
	if !validFormats[in.Format] {
		return Tournament{}, fmt.Errorf("%w: unknown format %q", ErrInvalid, in.Format)
	}
	if in.MaxParticipants == 0 {
		in.MaxParticipants = 32
	}
	if in.MaxParticipants < 2 || in.MaxParticipants > 128 {
		return Tournament{}, fmt.Errorf("%w: max_participants must be 2..128", ErrInvalid)
	}
	if in.SetsToWin <= 0 {
		in.SetsToWin = 1
	}
	if in.PointsToWin <= 0 {
		in.PointsToWin = 11
	}

	code := makeJoinCode(in.Sport)
	for attempt := 0; attempt < 6; attempt++ {
		slug := makeSlug(in.Name)
		t, err := s.store.create(ctx, createParams{
			Slug:             slug,
			JoinCode:         code,
			Name:             in.Name,
			Sport:            in.Sport,
			Format:           in.Format,
			OrganizerID:      userID,
			MaxParticipants:  in.MaxParticipants,
			StartsAt:         in.StartsAt,
			SetsToWin:        in.SetsToWin,
			AdvantageEnabled: in.AdvantageEnabled,
			PointsToWin:      in.PointsToWin,
		})
		if errors.Is(err, errSlugTaken) {
			continue
		}
		if err != nil {
			return Tournament{}, err
		}
		return t, nil
	}
	return Tournament{}, fmt.Errorf("could not allocate unique slug")
}

func (s *Service) GetTournament(ctx context.Context, userID, slug string) (TournamentView, error) {
	t, err := s.store.getBySlug(ctx, slug)
	if err != nil {
		return TournamentView{}, err
	}
	view := TournamentView{Tournament: t, IsOrganizer: t.OrganizerID == userID}
	_, joined, err := s.store.participantByUser(ctx, t.ID, userID)
	if err != nil {
		return TournamentView{}, err
	}
	view.HasJoined = joined
	if !view.IsOrganizer {
		view.JoinCode = ""
	}
	return view, nil
}

// Delete removes a tournament and (via cascade) its participants and fixtures.
// Only the organizer may delete, and only before the draw (REGISTRATION/LOCKED),
// so an ongoing or finished tournament's history can't be silently wiped.
func (s *Service) Delete(ctx context.Context, userID, tournamentID string) error {
	t, err := s.store.getByID(ctx, tournamentID)
	if err != nil {
		return err
	}
	if t.OrganizerID != userID {
		return ErrForbidden
	}
	if t.Status != "REGISTRATION" && t.Status != "LOCKED" {
		return fmt.Errorf("%w: cannot delete once the tournament has started", ErrWrongState)
	}
	// Capture recipients before the row (and its participants) are deleted.
	recipients, _ := s.store.participantUserIDs(ctx, tournamentID)
	if err := s.store.delete(ctx, tournamentID); err != nil {
		return err
	}
	if s.hub != nil {
		ev := sse.Event{Type: "tournament.deleted", Data: map[string]any{"tournament_id": tournamentID}}
		for _, id := range recipients {
			s.hub.Publish(id, ev)
		}
	}
	return nil
}

func (s *Service) ListMine(ctx context.Context, userID string, f ListFilter, limit, offset int) ([]Tournament, int64, error) {
	if f.Sport != "" && !validSports[f.Sport] {
		return nil, 0, fmt.Errorf("%w: unknown sport %q", ErrInvalid, f.Sport)
	}
	list, total, err := s.store.listMine(ctx, userID, f, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	for i := range list {
		if list[i].OrganizerID != userID {
			list[i].JoinCode = ""
		}
	}
	return list, total, nil
}

// ── Join / participants ──────────────────────────────────────────────────────

func (s *Service) JoinTournament(ctx context.Context, userID, slug, code string) (Participant, error) {
	t, err := s.store.getBySlug(ctx, slug)
	if err != nil {
		return Participant{}, err
	}
	if t.Status != "REGISTRATION" {
		return Participant{}, fmt.Errorf("%w: registration closed", ErrWrongState)
	}
	if !strings.EqualFold(strings.TrimSpace(code), t.JoinCode) {
		return Participant{}, ErrWrongCode
	}
	if _, joined, err := s.store.participantByUser(ctx, t.ID, userID); err != nil {
		return Participant{}, err
	} else if joined {
		return Participant{}, ErrAlreadyJoined
	}
	n, err := s.store.countConfirmed(ctx, t.ID)
	if err != nil {
		return Participant{}, err
	}
	if n >= t.MaxParticipants {
		return Participant{}, ErrFull
	}

	p, err := s.store.addParticipant(ctx, t.ID, userID)
	if err != nil {
		return Participant{}, err
	}
	s.publishAll(ctx, t.ID, sse.Event{
		Type: "tournament.participant_joined",
		Data: map[string]any{"tournament_id": t.ID, "participant": p},
	})
	return p, nil
}

func (s *Service) ListParticipants(ctx context.Context, tournamentID string, limit, offset int) ([]Participant, int64, error) {
	if _, err := s.store.getByID(ctx, tournamentID); err != nil {
		return nil, 0, err
	}
	return s.store.listParticipants(ctx, tournamentID, limit, offset)
}

func (s *Service) Leave(ctx context.Context, userID, tournamentID string) error {
	t, err := s.store.getByID(ctx, tournamentID)
	if err != nil {
		return err
	}
	if t.Status != "REGISTRATION" {
		return fmt.Errorf("%w: cannot leave after registration", ErrWrongState)
	}
	return s.store.removeParticipant(ctx, tournamentID, userID)
}

// ── Lock / draw ──────────────────────────────────────────────────────────────

func (s *Service) Lock(ctx context.Context, userID, tournamentID string) (Tournament, error) {
	t, err := s.store.getByID(ctx, tournamentID)
	if err != nil {
		return Tournament{}, err
	}
	if t.OrganizerID != userID {
		return Tournament{}, ErrForbidden
	}
	if t.Status != "REGISTRATION" {
		return Tournament{}, fmt.Errorf("%w: not in registration", ErrWrongState)
	}
	n, err := s.store.countConfirmed(ctx, tournamentID)
	if err != nil {
		return Tournament{}, err
	}
	if n < 2 {
		return Tournament{}, fmt.Errorf("%w: need at least 2 participants", ErrInvalid)
	}
	if err := s.store.setStatus(ctx, tournamentID, "LOCKED"); err != nil {
		return Tournament{}, err
	}
	s.publishAll(ctx, tournamentID, sse.Event{Type: "tournament.locked", Data: map[string]any{"tournament_id": tournamentID}})
	return s.store.getByID(ctx, tournamentID)
}

func (s *Service) Draw(ctx context.Context, userID, tournamentID, mode string, seeding []string) (Bracket, error) {
	t, err := s.store.getByID(ctx, tournamentID)
	if err != nil {
		return Bracket{}, err
	}
	if t.OrganizerID != userID {
		return Bracket{}, ErrForbidden
	}
	if t.Status != "LOCKED" {
		return Bracket{}, fmt.Errorf("%w: lock the tournament before drawing", ErrWrongState)
	}
	participants, err := s.store.confirmedParticipants(ctx, tournamentID)
	if err != nil {
		return Bracket{}, err
	}
	if len(participants) < 2 {
		return Bracket{}, fmt.Errorf("%w: need at least 2 participants", ErrInvalid)
	}

	var ordered []string
	switch mode {
	case "RANDOM":
		ordered = participantIDs(participants)
		rand.Shuffle(len(ordered), func(i, j int) { ordered[i], ordered[j] = ordered[j], ordered[i] })
	case "MANUAL":
		if !sameSet(seeding, participants) {
			return Bracket{}, fmt.Errorf("%w: seeding must list every participant exactly once", ErrInvalid)
		}
		ordered = seeding
		seeds := make([]seedAssignment, len(ordered))
		for i, pid := range ordered {
			seeds[i] = seedAssignment{ParticipantID: pid, Seed: i + 1}
		}
		if err := s.store.setSeeds(ctx, seeds); err != nil {
			return Bracket{}, err
		}
	default:
		return Bracket{}, fmt.Errorf("%w: mode must be RANDOM or MANUAL", ErrInvalid)
	}

	var plans []MatchPlan
	switch t.Format {
	case "SINGLE_ELIM":
		plans = buildSingleElim(ordered)
	case "ROUND_ROBIN":
		plans = buildRoundRobin(ordered)
	default:
		return Bracket{}, fmt.Errorf("%w: unsupported format", ErrInvalid)
	}

	if err := s.store.replaceFixtures(ctx, tournamentID, plans); err != nil {
		return Bracket{}, err
	}
	if err := s.store.setStatus(ctx, tournamentID, "ONGOING"); err != nil {
		return Bracket{}, err
	}
	s.publishAll(ctx, tournamentID, sse.Event{Type: "tournament.draw_completed", Data: map[string]any{"tournament_id": tournamentID}})
	return s.buildBracket(ctx, t.Format, tournamentID)
}

func (s *Service) GetBracket(ctx context.Context, tournamentID string) (Bracket, error) {
	t, err := s.store.getByID(ctx, tournamentID)
	if err != nil {
		return Bracket{}, err
	}
	return s.buildBracket(ctx, t.Format, tournamentID)
}

func (s *Service) buildBracket(ctx context.Context, format, tournamentID string) (Bracket, error) {
	matches, err := s.store.bracketMatches(ctx, tournamentID)
	if err != nil {
		return Bracket{}, err
	}
	b := Bracket{Format: format, Rounds: groupRounds(matches), Standings: []Standing{}}
	if format == "ROUND_ROBIN" {
		parts, err := s.store.confirmedParticipants(ctx, tournamentID)
		if err != nil {
			return Bracket{}, err
		}
		b.Standings = computeStandings(parts, matches)
	}
	return b, nil
}

// ── Result / confirmation ────────────────────────────────────────────────────

func (s *Service) SubmitResult(ctx context.Context, userID, matchID, winnerID, score string) (TournamentMatch, error) {
	d, err := s.store.matchDetail(ctx, matchID)
	if err != nil {
		return TournamentMatch{}, err
	}
	if d.TournamentStatus != "ONGOING" {
		return TournamentMatch{}, fmt.Errorf("%w: tournament not ongoing", ErrWrongState)
	}
	if d.Status != "READY" {
		return TournamentMatch{}, fmt.Errorf("%w: match is not ready for a result", ErrWrongState)
	}
	if !matchHasParticipant(d, winnerID) {
		return TournamentMatch{}, fmt.Errorf("%w: winner must be a match participant", ErrInvalid)
	}

	isOrganizer := d.OrganizerID == userID
	isPlayer := userIsPlayer(d, userID)

	switch {
	case isOrganizer:
		if err := s.store.completeMatch(ctx, matchID, winnerID, score, userID); err != nil {
			return TournamentMatch{}, err
		}
		if err := s.advance(ctx, d, winnerID); err != nil {
			return TournamentMatch{}, err
		}
		s.publishAll(ctx, d.TournamentID, matchEvent("tournament.match_completed", d.TournamentID, matchID))
	case isPlayer:
		if err := s.store.reportMatch(ctx, matchID, winnerID, score, userID); err != nil {
			return TournamentMatch{}, err
		}
		if opp := opponentUserID(d, userID); opp != "" {
			s.publishUser(opp, sse.Event{
				Type: "tournament.match_pending_confirmation",
				Data: map[string]any{"tournament_id": d.TournamentID, "match_id": matchID, "reported_by": userID},
			})
		}
	default:
		return TournamentMatch{}, ErrForbidden
	}

	return s.store.getMatch(ctx, matchID)
}

func (s *Service) ConfirmResult(ctx context.Context, userID, matchID string, approve bool) (TournamentMatch, error) {
	d, err := s.store.matchDetail(ctx, matchID)
	if err != nil {
		return TournamentMatch{}, err
	}
	if d.Status != "PENDING_CONFIRMATION" {
		return TournamentMatch{}, fmt.Errorf("%w: match is not awaiting confirmation", ErrWrongState)
	}
	isOrganizer := d.OrganizerID == userID
	isPlayer := userIsPlayer(d, userID)
	if !isOrganizer && !isPlayer {
		return TournamentMatch{}, ErrForbidden
	}
	// A player cannot confirm the result they reported themselves.
	if !isOrganizer && d.ReportedBy != nil && *d.ReportedBy == userID {
		return TournamentMatch{}, ErrForbidden
	}

	if approve {
		m, err := s.store.getMatch(ctx, matchID)
		if err != nil {
			return TournamentMatch{}, err
		}
		if m.WinnerID == nil {
			return TournamentMatch{}, fmt.Errorf("%w: reported match has no winner", ErrInvalid)
		}
		if err := s.store.completeMatch(ctx, matchID, *m.WinnerID, deref(m.ScoreSummary), userID); err != nil {
			return TournamentMatch{}, err
		}
		if err := s.advance(ctx, d, *m.WinnerID); err != nil {
			return TournamentMatch{}, err
		}
		s.publishAll(ctx, d.TournamentID, matchEvent("tournament.match_completed", d.TournamentID, matchID))
	} else {
		if err := s.store.clearReport(ctx, matchID); err != nil {
			return TournamentMatch{}, err
		}
		s.publishUser(d.OrganizerID, sse.Event{
			Type: "tournament.match_disputed",
			Data: map[string]any{"tournament_id": d.TournamentID, "match_id": matchID},
		})
	}
	return s.store.getMatch(ctx, matchID)
}

// advance moves a winner into the next fixture (single-elim) or checks for
// tournament completion (both formats).
func (s *Service) advance(ctx context.Context, d matchDetail, winnerID string) error {
	if d.Format == "SINGLE_ELIM" {
		if d.NextMatchID != nil {
			return s.store.placeIntoNext(ctx, *d.NextMatchID, winnerID)
		}
		// No next match → this was the final.
		s.publishAll(ctx, d.TournamentID, sse.Event{
			Type: "tournament.completed",
			Data: map[string]any{"tournament_id": d.TournamentID, "champion_id": winnerID},
		})
		return s.store.setChampion(ctx, d.TournamentID, winnerID)
	}

	// ROUND_ROBIN: complete when no fixtures remain.
	remaining, err := s.store.remainingMatches(ctx, d.TournamentID)
	if err != nil {
		return err
	}
	if remaining > 0 {
		return nil
	}
	parts, err := s.store.confirmedParticipants(ctx, d.TournamentID)
	if err != nil {
		return err
	}
	matches, err := s.store.bracketMatches(ctx, d.TournamentID)
	if err != nil {
		return err
	}
	standings := computeStandings(parts, matches)
	if len(standings) == 0 {
		return nil
	}
	champion := standings[0].ParticipantID
	s.publishAll(ctx, d.TournamentID, sse.Event{
		Type: "tournament.completed",
		Data: map[string]any{"tournament_id": d.TournamentID, "champion_id": champion},
	})
	return s.store.setChampion(ctx, d.TournamentID, champion)
}

// ── SSE helpers ──────────────────────────────────────────────────────────────

func (s *Service) publishAll(ctx context.Context, tournamentID string, ev sse.Event) {
	if s.hub == nil {
		return
	}
	ids, err := s.store.participantUserIDs(ctx, tournamentID)
	if err != nil {
		return
	}
	for _, id := range ids {
		s.hub.Publish(id, ev)
	}
}

func (s *Service) publishUser(userID string, ev sse.Event) {
	if s.hub == nil {
		return
	}
	s.hub.Publish(userID, ev)
}

func matchEvent(typ, tournamentID, matchID string) sse.Event {
	return sse.Event{Type: typ, Data: map[string]any{"tournament_id": tournamentID, "match_id": matchID}}
}

// ── Pure helpers ─────────────────────────────────────────────────────────────

func participantIDs(ps []Participant) []string {
	ids := make([]string, len(ps))
	for i, p := range ps {
		ids[i] = p.ID
	}
	return ids
}

func sameSet(seeding []string, ps []Participant) bool {
	if len(seeding) != len(ps) {
		return false
	}
	want := make(map[string]bool, len(ps))
	for _, p := range ps {
		want[p.ID] = true
	}
	seen := make(map[string]bool, len(seeding))
	for _, id := range seeding {
		if !want[id] || seen[id] {
			return false
		}
		seen[id] = true
	}
	return true
}

func matchHasParticipant(d matchDetail, participantID string) bool {
	return (d.ParticipantAID != nil && *d.ParticipantAID == participantID) ||
		(d.ParticipantBID != nil && *d.ParticipantBID == participantID)
}

func userIsPlayer(d matchDetail, userID string) bool {
	return (d.AUserID != nil && *d.AUserID == userID) ||
		(d.BUserID != nil && *d.BUserID == userID)
}

func opponentUserID(d matchDetail, userID string) string {
	if d.AUserID != nil && *d.AUserID == userID {
		if d.BUserID != nil {
			return *d.BUserID
		}
		return ""
	}
	if d.AUserID != nil {
		return *d.AUserID
	}
	return ""
}

func groupRounds(matches []TournamentMatch) []BracketRound {
	rounds := []BracketRound{}
	idx := map[int]int{}
	for _, m := range matches {
		i, ok := idx[m.Round]
		if !ok {
			i = len(rounds)
			idx[m.Round] = i
			rounds = append(rounds, BracketRound{Round: m.Round, Matches: []TournamentMatch{}})
		}
		rounds[i].Matches = append(rounds[i].Matches, m)
	}
	return rounds
}

// computeStandings tallies wins/losses and set-diff (from "A-B" score tokens)
// for a round-robin tournament, sorted by wins then diff.
func computeStandings(participants []Participant, matches []TournamentMatch) []Standing {
	byID := map[string]*Standing{}
	order := make([]string, 0, len(participants))
	for _, p := range participants {
		byID[p.ID] = &Standing{ParticipantID: p.ID, DisplayName: p.DisplayName}
		order = append(order, p.ID)
	}

	for _, m := range matches {
		if m.Status != "COMPLETED" || m.WinnerID == nil || m.ParticipantAID == nil || m.ParticipantBID == nil {
			continue
		}
		a, aok := byID[*m.ParticipantAID]
		b, bok := byID[*m.ParticipantBID]
		if !aok || !bok {
			continue
		}
		a.Played++
		b.Played++
		forA, forB := parseScore(m.ScoreSummary)
		a.Diff += forA - forB
		b.Diff += forB - forA
		if *m.WinnerID == *m.ParticipantAID {
			a.Won++
			b.Lost++
		} else {
			b.Won++
			a.Lost++
		}
	}

	out := make([]Standing, 0, len(order))
	for _, id := range order {
		st := byID[id]
		st.Points = st.Won
		out = append(out, *st)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Won != out[j].Won {
			return out[i].Won > out[j].Won
		}
		return out[i].Diff > out[j].Diff
	})
	return out
}

// parseScore sums game counts from a summary like "6-4 3-6 7-5" into
// (games for A, games for B). Unparseable tokens are ignored.
func parseScore(score *string) (int, int) {
	if score == nil {
		return 0, 0
	}
	var a, b int
	for _, tok := range strings.Fields(*score) {
		parts := strings.SplitN(tok, "-", 2)
		if len(parts) != 2 {
			continue
		}
		x, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
		y, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err1 != nil || err2 != nil {
			continue
		}
		a += x
		b += y
	}
	return a, b
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// ── Slug / code generation ───────────────────────────────────────────────────

var trReplacer = strings.NewReplacer(
	"ç", "c", "ğ", "g", "ı", "i", "ö", "o", "ş", "s", "ü", "u",
	"Ç", "c", "Ğ", "g", "İ", "i", "Ö", "o", "Ş", "s", "Ü", "u",
)

func makeSlug(name string) string {
	s := trReplacer.Replace(strings.ToLower(strings.TrimSpace(name)))
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	base := strings.Trim(b.String(), "-")
	if base == "" {
		base = "tournament"
	}
	return base + "-" + strings.ToLower(randomString(4))
}

func makeJoinCode(sport string) string {
	return sport + "-" + randomString(4)
}

// randomString uses an alphabet without easily-confused characters (no O/0/I/1).
const codeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

func randomString(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = codeAlphabet[rand.Intn(len(codeAlphabet))]
	}
	return string(b)
}
