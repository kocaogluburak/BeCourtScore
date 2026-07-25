package tournament

import (
	"context"
	"errors"
	"testing"
)

// ── fake store ───────────────────────────────────────────────────────────────

type fakeStore struct {
	tournament        Tournament
	getErr            error
	participantExists bool
	confirmed         []Participant
	countN            int
	md                matchDetail
	mdErr             error
	match             TournamentMatch
	matches           []TournamentMatch
	remaining         int

	// recorders
	completeCalled bool
	reportCalled   bool
	clearCalled    bool
	placeCalled    bool
	fixturesCalled bool
	deleteCalled   bool
	champion       string
	statusSet      string
	seedsSet       []seedAssignment
}

func (f *fakeStore) create(context.Context, createParams) (Tournament, error) {
	return f.tournament, f.getErr
}
func (f *fakeStore) slugExists(context.Context, string) (bool, error) { return false, nil }
func (f *fakeStore) getByID(context.Context, string) (Tournament, error) {
	return f.tournament, f.getErr
}
func (f *fakeStore) getBySlug(context.Context, string) (Tournament, error) {
	return f.tournament, f.getErr
}
func (f *fakeStore) delete(context.Context, string) error {
	f.deleteCalled = true
	return nil
}
func (f *fakeStore) setStatus(_ context.Context, _, status string) error {
	f.statusSet = status
	return nil
}
func (f *fakeStore) setChampion(_ context.Context, _, championID string) error {
	f.champion = championID
	return nil
}
func (f *fakeStore) addParticipant(context.Context, string, string) (Participant, error) {
	return Participant{ID: "new", DisplayName: "Ada"}, nil
}
func (f *fakeStore) removeParticipant(context.Context, string, string) error { return nil }
func (f *fakeStore) participantByUser(context.Context, string, string) (Participant, bool, error) {
	return Participant{}, f.participantExists, nil
}
func (f *fakeStore) listParticipants(context.Context, string, int, int) ([]Participant, int64, error) {
	return f.confirmed, int64(len(f.confirmed)), nil
}
func (f *fakeStore) confirmedParticipants(context.Context, string) ([]Participant, error) {
	return f.confirmed, nil
}
func (f *fakeStore) countConfirmed(context.Context, string) (int, error) { return f.countN, nil }
func (f *fakeStore) setSeeds(_ context.Context, seeds []seedAssignment) error {
	f.seedsSet = seeds
	return nil
}
func (f *fakeStore) listMine(_ context.Context, _ string, _ ListFilter, _, _ int) ([]Tournament, int64, error) {
	return []Tournament{f.tournament}, 1, nil
}
func (f *fakeStore) replaceFixtures(context.Context, string, []MatchPlan) error {
	f.fixturesCalled = true
	return nil
}
func (f *fakeStore) bracketMatches(context.Context, string) ([]TournamentMatch, error) {
	return f.matches, nil
}
func (f *fakeStore) matchDetail(context.Context, string) (matchDetail, error) {
	return f.md, f.mdErr
}
func (f *fakeStore) getMatch(context.Context, string) (TournamentMatch, error) {
	return f.match, nil
}
func (f *fakeStore) reportMatch(_ context.Context, _, _, _, _ string) error {
	f.reportCalled = true
	return nil
}
func (f *fakeStore) clearReport(context.Context, string) error {
	f.clearCalled = true
	return nil
}
func (f *fakeStore) completeMatch(_ context.Context, _, _, _, _ string) error {
	f.completeCalled = true
	return nil
}
func (f *fakeStore) placeIntoNext(context.Context, string, string) error {
	f.placeCalled = true
	return nil
}
func (f *fakeStore) remainingMatches(context.Context, string) (int, error) { return f.remaining, nil }
func (f *fakeStore) participantUserIDs(context.Context, string) ([]string, error) {
	return nil, nil
}

func newService(f *fakeStore) *Service { return &Service{store: f, hub: nil} }

func strptr(s string) *string { return &s }

// ── Join ─────────────────────────────────────────────────────────────────────

func regTournament() Tournament {
	return Tournament{ID: "t1", Slug: "s", Status: "REGISTRATION", JoinCode: "PADEL-ABCD", MaxParticipants: 4, OrganizerID: "org", Format: "SINGLE_ELIM"}
}

func TestJoin_Happy(t *testing.T) {
	f := &fakeStore{tournament: regTournament(), countN: 1}
	_, err := newService(f).JoinTournament(context.Background(), "u1", "s", "padel-abcd") // case-insensitive
	if err != nil {
		t.Fatalf("join: %v", err)
	}
}

func TestJoin_WrongState(t *testing.T) {
	tt := regTournament()
	tt.Status = "LOCKED"
	f := &fakeStore{tournament: tt}
	_, err := newService(f).JoinTournament(context.Background(), "u1", "s", "PADEL-ABCD")
	if !errors.Is(err, ErrWrongState) {
		t.Fatalf("got %v, want ErrWrongState", err)
	}
}

func TestJoin_WrongCode(t *testing.T) {
	f := &fakeStore{tournament: regTournament()}
	_, err := newService(f).JoinTournament(context.Background(), "u1", "s", "NOPE")
	if !errors.Is(err, ErrWrongCode) {
		t.Fatalf("got %v, want ErrWrongCode", err)
	}
}

func TestJoin_AlreadyJoined(t *testing.T) {
	f := &fakeStore{tournament: regTournament(), participantExists: true}
	_, err := newService(f).JoinTournament(context.Background(), "u1", "s", "PADEL-ABCD")
	if !errors.Is(err, ErrAlreadyJoined) {
		t.Fatalf("got %v, want ErrAlreadyJoined", err)
	}
}

func TestJoin_Full(t *testing.T) {
	f := &fakeStore{tournament: regTournament(), countN: 4}
	_, err := newService(f).JoinTournament(context.Background(), "u1", "s", "PADEL-ABCD")
	if !errors.Is(err, ErrFull) {
		t.Fatalf("got %v, want ErrFull", err)
	}
}

// ── Lock ─────────────────────────────────────────────────────────────────────

func TestLock_Forbidden(t *testing.T) {
	f := &fakeStore{tournament: regTournament(), countN: 4}
	_, err := newService(f).Lock(context.Background(), "not-org", "t1")
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("got %v, want ErrForbidden", err)
	}
}

func TestLock_TooFewParticipants(t *testing.T) {
	f := &fakeStore{tournament: regTournament(), countN: 1}
	_, err := newService(f).Lock(context.Background(), "org", "t1")
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("got %v, want ErrInvalid", err)
	}
}

func TestLock_Happy(t *testing.T) {
	f := &fakeStore{tournament: regTournament(), countN: 2}
	if _, err := newService(f).Lock(context.Background(), "org", "t1"); err != nil {
		t.Fatalf("lock: %v", err)
	}
	if f.statusSet != "LOCKED" {
		t.Errorf("status: got %q, want LOCKED", f.statusSet)
	}
}

// ── Delete ───────────────────────────────────────────────────────────────────

func TestDelete_Forbidden(t *testing.T) {
	f := &fakeStore{tournament: regTournament()}
	err := newService(f).Delete(context.Background(), "not-org", "t1")
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("got %v, want ErrForbidden", err)
	}
	if f.deleteCalled {
		t.Error("delete should not be called when forbidden")
	}
}

func TestDelete_WrongStateAfterStart(t *testing.T) {
	tt := regTournament()
	tt.Status = "ONGOING"
	f := &fakeStore{tournament: tt}
	err := newService(f).Delete(context.Background(), "org", "t1")
	if !errors.Is(err, ErrWrongState) {
		t.Fatalf("got %v, want ErrWrongState", err)
	}
	if f.deleteCalled {
		t.Error("delete should not be called for an ongoing tournament")
	}
}

func TestDelete_Happy(t *testing.T) {
	f := &fakeStore{tournament: regTournament()}
	if err := newService(f).Delete(context.Background(), "org", "t1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if !f.deleteCalled {
		t.Error("expected delete to be called")
	}
}

// ── Draw ─────────────────────────────────────────────────────────────────────

func lockedTournament() Tournament {
	tt := regTournament()
	tt.Status = "LOCKED"
	return tt
}

func TestDraw_Forbidden(t *testing.T) {
	f := &fakeStore{tournament: lockedTournament()}
	_, err := newService(f).Draw(context.Background(), "not-org", "t1", "RANDOM", nil)
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("got %v, want ErrForbidden", err)
	}
}

func TestDraw_WrongState(t *testing.T) {
	f := &fakeStore{tournament: regTournament()} // still REGISTRATION
	_, err := newService(f).Draw(context.Background(), "org", "t1", "RANDOM", nil)
	if !errors.Is(err, ErrWrongState) {
		t.Fatalf("got %v, want ErrWrongState", err)
	}
}

func TestDraw_ManualInvalidSeeding(t *testing.T) {
	f := &fakeStore{
		tournament: lockedTournament(),
		confirmed:  []Participant{{ID: "a"}, {ID: "b"}, {ID: "c"}, {ID: "d"}},
	}
	_, err := newService(f).Draw(context.Background(), "org", "t1", "MANUAL", []string{"a", "b"}) // incomplete
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("got %v, want ErrInvalid", err)
	}
}

func TestDraw_RandomHappy(t *testing.T) {
	f := &fakeStore{
		tournament: lockedTournament(),
		confirmed:  []Participant{{ID: "a"}, {ID: "b"}, {ID: "c"}, {ID: "d"}},
	}
	if _, err := newService(f).Draw(context.Background(), "org", "t1", "RANDOM", nil); err != nil {
		t.Fatalf("draw: %v", err)
	}
	if !f.fixturesCalled {
		t.Errorf("replaceFixtures not called")
	}
	if f.statusSet != "ONGOING" {
		t.Errorf("status: got %q, want ONGOING", f.statusSet)
	}
}

func TestDraw_ManualSetsSeeds(t *testing.T) {
	f := &fakeStore{
		tournament: lockedTournament(),
		confirmed:  []Participant{{ID: "a"}, {ID: "b"}, {ID: "c"}, {ID: "d"}},
	}
	_, err := newService(f).Draw(context.Background(), "org", "t1", "MANUAL", []string{"d", "c", "b", "a"})
	if err != nil {
		t.Fatalf("draw: %v", err)
	}
	if len(f.seedsSet) != 4 || f.seedsSet[0].ParticipantID != "d" || f.seedsSet[0].Seed != 1 {
		t.Errorf("seeds not assigned in listed order: %+v", f.seedsSet)
	}
}

// ── SubmitResult ─────────────────────────────────────────────────────────────

func readyMatchDetail() matchDetail {
	return matchDetail{
		ID: "m1", TournamentID: "t1", Status: "READY",
		ParticipantAID: strptr("pa"), ParticipantBID: strptr("pb"),
		AUserID: strptr("ua"), BUserID: strptr("ub"),
		TournamentStatus: "ONGOING", Format: "SINGLE_ELIM", OrganizerID: "org",
	}
}

func TestSubmitResult_OrganizerCompletesAndAdvances(t *testing.T) {
	f := &fakeStore{md: readyMatchDetail(), match: TournamentMatch{ID: "m1", Status: "COMPLETED"}}
	_, err := newService(f).SubmitResult(context.Background(), "org", "m1", "pa", "6-4 6-2")
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if !f.completeCalled {
		t.Errorf("completeMatch not called for organizer")
	}
	if f.champion != "pa" { // final has no next → champion set
		t.Errorf("champion: got %q, want pa", f.champion)
	}
}

func TestSubmitResult_PlayerReportsPending(t *testing.T) {
	f := &fakeStore{md: readyMatchDetail(), match: TournamentMatch{ID: "m1", Status: "PENDING_CONFIRMATION"}}
	_, err := newService(f).SubmitResult(context.Background(), "ua", "m1", "pa", "6-4 6-2")
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if !f.reportCalled {
		t.Errorf("reportMatch not called for player")
	}
	if f.completeCalled {
		t.Errorf("player submission must not complete the match")
	}
}

func TestSubmitResult_StrangerForbidden(t *testing.T) {
	f := &fakeStore{md: readyMatchDetail()}
	_, err := newService(f).SubmitResult(context.Background(), "zzz", "m1", "pa", "")
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("got %v, want ErrForbidden", err)
	}
}

func TestSubmitResult_NotReady(t *testing.T) {
	md := readyMatchDetail()
	md.Status = "PENDING"
	f := &fakeStore{md: md}
	_, err := newService(f).SubmitResult(context.Background(), "org", "m1", "pa", "")
	if !errors.Is(err, ErrWrongState) {
		t.Fatalf("got %v, want ErrWrongState", err)
	}
}

func TestSubmitResult_InvalidWinner(t *testing.T) {
	f := &fakeStore{md: readyMatchDetail()}
	_, err := newService(f).SubmitResult(context.Background(), "org", "m1", "unknown", "")
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("got %v, want ErrInvalid", err)
	}
}

func TestSubmitResult_AdvancesIntoNext(t *testing.T) {
	md := readyMatchDetail()
	md.NextMatchID = strptr("m2") // not the final
	f := &fakeStore{md: md, match: TournamentMatch{ID: "m1"}}
	_, err := newService(f).SubmitResult(context.Background(), "org", "m1", "pa", "")
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if !f.placeCalled {
		t.Errorf("winner not placed into next match")
	}
	if f.champion != "" {
		t.Errorf("champion should not be set for non-final match")
	}
}

// ── ConfirmResult ────────────────────────────────────────────────────────────

func pendingMatchDetail() matchDetail {
	md := readyMatchDetail()
	md.Status = "PENDING_CONFIRMATION"
	md.ReportedBy = strptr("ua")
	return md
}

func TestConfirm_ReporterCannotConfirm(t *testing.T) {
	f := &fakeStore{md: pendingMatchDetail()}
	_, err := newService(f).ConfirmResult(context.Background(), "ua", "m1", true)
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("got %v, want ErrForbidden", err)
	}
}

func TestConfirm_OpponentApprovesCompletes(t *testing.T) {
	f := &fakeStore{
		md:    pendingMatchDetail(),
		match: TournamentMatch{ID: "m1", WinnerID: strptr("pa"), ScoreSummary: strptr("6-4 6-2")},
	}
	_, err := newService(f).ConfirmResult(context.Background(), "ub", "m1", true)
	if err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if !f.completeCalled {
		t.Errorf("completeMatch not called on approval")
	}
	if f.champion != "pa" {
		t.Errorf("champion: got %q, want pa", f.champion)
	}
}

func TestConfirm_RejectClearsReport(t *testing.T) {
	f := &fakeStore{md: pendingMatchDetail(), match: TournamentMatch{ID: "m1"}}
	_, err := newService(f).ConfirmResult(context.Background(), "ub", "m1", false)
	if err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if !f.clearCalled {
		t.Errorf("clearReport not called on rejection")
	}
}

func TestConfirm_WrongState(t *testing.T) {
	md := readyMatchDetail() // still READY, not PENDING_CONFIRMATION
	f := &fakeStore{md: md}
	_, err := newService(f).ConfirmResult(context.Background(), "ub", "m1", true)
	if !errors.Is(err, ErrWrongState) {
		t.Fatalf("got %v, want ErrWrongState", err)
	}
}

// ── Standings ────────────────────────────────────────────────────────────────

func TestComputeStandings_RanksByWinsThenDiff(t *testing.T) {
	parts := []Participant{
		{ID: "a", DisplayName: "A"},
		{ID: "b", DisplayName: "B"},
		{ID: "c", DisplayName: "C"},
	}
	matches := []TournamentMatch{
		{Status: "COMPLETED", ParticipantAID: strptr("a"), ParticipantBID: strptr("b"), WinnerID: strptr("a"), ScoreSummary: strptr("6-0 6-0")},
		{Status: "COMPLETED", ParticipantAID: strptr("a"), ParticipantBID: strptr("c"), WinnerID: strptr("a"), ScoreSummary: strptr("6-3 6-3")},
		{Status: "COMPLETED", ParticipantAID: strptr("b"), ParticipantBID: strptr("c"), WinnerID: strptr("b"), ScoreSummary: strptr("6-4 6-4")},
	}
	st := computeStandings(parts, matches)
	if len(st) != 3 {
		t.Fatalf("standings len: got %d, want 3", len(st))
	}
	if st[0].ParticipantID != "a" || st[0].Won != 2 {
		t.Errorf("first place: got %s (won %d), want a (2)", st[0].ParticipantID, st[0].Won)
	}
	if st[1].ParticipantID != "b" || st[1].Won != 1 {
		t.Errorf("second place: got %s (won %d), want b (1)", st[1].ParticipantID, st[1].Won)
	}
	if st[2].ParticipantID != "c" {
		t.Errorf("third place: got %s, want c", st[2].ParticipantID)
	}
}

// ── ListMine filter ──────────────────────────────────────────────────────────

func TestListMine_UnknownSportReturnsInvalid(t *testing.T) {
	f := &fakeStore{tournament: regTournament()}
	_, _, err := newService(f).ListMine(context.Background(), "u1", ListFilter{Sport: "CHESS"}, 20, 0)
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("got %v, want ErrInvalid for unknown sport", err)
	}
}

func TestListMine_ValidSportPassesThrough(t *testing.T) {
	f := &fakeStore{tournament: regTournament()}
	list, total, err := newService(f).ListMine(context.Background(), "u1", ListFilter{Sport: "PADEL"}, 20, 0)
	if err != nil {
		t.Fatalf("list mine with sport: %v", err)
	}
	if total != 1 || len(list) != 1 {
		t.Errorf("expected 1 result, got %d (total %d)", len(list), total)
	}
}

func TestListMine_EmptyFilterPassesThrough(t *testing.T) {
	f := &fakeStore{tournament: regTournament()}
	_, _, err := newService(f).ListMine(context.Background(), "u1", ListFilter{}, 20, 0)
	if err != nil {
		t.Fatalf("list mine no filter: %v", err)
	}
}

// ── Slug / code ──────────────────────────────────────────────────────────────

func TestMakeSlug_TurkishAndUnique(t *testing.T) {
	s1 := makeSlug("35 Yaş Üstü Padel")
	if len(s1) == 0 {
		t.Fatal("empty slug")
	}
	for _, r := range s1 {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-'
		if !ok {
			t.Errorf("slug has invalid rune %q in %q", r, s1)
		}
	}
	if s1 == makeSlug("35 Yaş Üstü Padel") {
		t.Errorf("slugs should differ by random suffix")
	}
}
