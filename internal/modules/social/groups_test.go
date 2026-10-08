package social

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestBuildStandings_WeekMembershipAndDoubles(t *testing.T) {
	now := time.Date(2026, 10, 8, 15, 0, 0, 0, time.UTC) // Thursday
	start, end := isoWeek(now)
	wantStart := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	if !start.Equal(wantStart) || !end.Equal(wantStart.AddDate(0, 0, 7)) {
		t.Fatalf("week %s .. %s", start, end)
	}
	members := []GroupMember{
		{UserID: "a", Name: "Ada"},
		{UserID: "b", Name: "Bea"},
		{UserID: "c", Name: "Cid"},
	}
	matches := []ladderMatch{
		// Counts: Ada beats Bea 2-0 on Monday.
		{Sport: "TENNIS", PlayedAt: start, PlayerA: "a", PlayerB: "b", SetsA: 2, SetsB: 0, WinnerSide: "A"},
		// Previous week does not count.
		{Sport: "TENNIS", PlayedAt: start.Add(-time.Second), PlayerA: "a", PlayerB: "b", SetsA: 2, SetsB: 0, WinnerSide: "A"},
		// Next week does not count.
		{Sport: "TENNIS", PlayedAt: end, PlayerA: "b", PlayerB: "a", SetsA: 2, SetsB: 0, WinnerSide: "A"},
		// Opponent outside the group does not count.
		{Sport: "TENNIS", PlayedAt: start.Add(time.Hour), PlayerA: "a", PlayerB: "stranger", SetsA: 2, SetsB: 0, WinnerSide: "A"},
		// Another sport does not count.
		{Sport: "PADEL", PlayedAt: start.Add(2 * time.Hour), PlayerA: "a", PlayerB: "b", SetsA: 2, SetsB: 1, WinnerSide: "A"},
		// Doubles with a non-member partner does not count.
		{Sport: "TENNIS", PlayedAt: start.Add(3 * time.Hour), PlayerA: "a", PlayerA2: "c", PlayerB: "b", PlayerB2: "outsider", SetsA: 2, SetsB: 1, WinnerSide: "A"},
		// Doubles where every registered player is a member: Cid beats Bea 2-1.
		// Ada is Cid's partner, so both take the win.
		{Sport: "TENNIS", PlayedAt: start.Add(4 * time.Hour), PlayerA: "c", PlayerA2: "a", PlayerB: "b", SetsA: 2, SetsB: 1, WinnerSide: "A"},
	}

	rows := buildStandings(members, matches, "TENNIS", start, end)
	if len(rows) != 3 {
		t.Fatalf("rows %d", len(rows))
	}
	// Ada: singles win (+2) and doubles win (+1) → 2 wins, set diff +3.
	// Cid: doubles win → 1 win, set diff +1.
	// Bea: two losses, set diff -3.
	if rows[0].UserID != "a" || rows[0].Wins != 2 || rows[0].SetDiff != 3 || rows[0].Rank != 1 {
		t.Fatalf("first %+v", rows[0])
	}
	if rows[1].UserID != "c" || rows[1].Wins != 1 || rows[1].SetDiff != 1 || rows[1].Rank != 2 {
		t.Fatalf("second %+v", rows[1])
	}
	if rows[2].UserID != "b" || rows[2].Wins != 0 || rows[2].Losses != 2 || rows[2].SetDiff != -3 {
		t.Fatalf("third %+v", rows[2])
	}
}

func TestBuildStandings_SetDiffBreaksWinTie(t *testing.T) {
	start := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 7)
	members := []GroupMember{{UserID: "a", Name: "A"}, {UserID: "b", Name: "B"}, {UserID: "c", Name: "C"}}
	matches := []ladderMatch{
		{Sport: "TENNIS", PlayedAt: start, PlayerA: "a", PlayerB: "b", SetsA: 2, SetsB: 0, WinnerSide: "A"},
		{Sport: "TENNIS", PlayedAt: start, PlayerA: "c", PlayerB: "b", SetsA: 2, SetsB: 1, WinnerSide: "A"},
	}
	rows := buildStandings(members, matches, "TENNIS", start, end)
	if rows[0].UserID != "a" || rows[1].UserID != "c" || rows[2].UserID != "b" {
		t.Fatalf("order %+v", rows)
	}
}

type fakeGroups struct {
	byID      map[string]Group
	byCode    map[string]string
	members   map[string]map[string]GroupMember
	matches   []ladderMatch
	conflicts int
	next      int
}

func newFakeGroups() *fakeGroups {
	return &fakeGroups{
		byID:    map[string]Group{},
		byCode:  map[string]string{},
		members: map[string]map[string]GroupMember{},
	}
}

func (f *fakeGroups) insertGroup(_ context.Context, name, sport, ownerID, code string) (Group, error) {
	if f.conflicts > 0 {
		f.conflicts--
		return Group{}, ErrConflict
	}
	if _, ok := f.byCode[code]; ok {
		return Group{}, ErrConflict
	}
	f.next++
	id := "g" + string(rune('0'+f.next))
	g := Group{
		ID: id, Name: name, Sport: sport, OwnerID: ownerID, InviteCode: code,
		MemberCount: 1, IsOwner: true, CreatedAt: time.Unix(int64(f.next), 0).UTC(),
	}
	f.byID[id] = g
	f.byCode[code] = id
	f.members[id] = map[string]GroupMember{
		ownerID: {UserID: ownerID, Name: ownerID, JoinedAt: g.CreatedAt},
	}
	return g, nil
}

func (f *fakeGroups) listGroups(_ context.Context, userID string) ([]Group, error) {
	var out []Group
	for id, members := range f.members {
		if _, ok := members[userID]; ok {
			g := f.byID[id]
			g.MemberCount = len(members)
			out = append(out, g)
		}
	}
	return out, nil
}

func (f *fakeGroups) getGroup(_ context.Context, id string) (Group, error) {
	g, ok := f.byID[id]
	if !ok {
		return Group{}, ErrNotFound
	}
	return g, nil
}

func (f *fakeGroups) getGroupByCode(_ context.Context, code string) (Group, error) {
	id, ok := f.byCode[code]
	if !ok {
		return Group{}, ErrNotFound
	}
	return f.byID[id], nil
}

func (f *fakeGroups) addMember(_ context.Context, groupID, userID string) error {
	if _, ok := f.byID[groupID]; !ok {
		return ErrNotFound
	}
	if _, ok := f.members[groupID][userID]; ok {
		return nil
	}
	f.members[groupID][userID] = GroupMember{UserID: userID, Name: userID, JoinedAt: time.Now().UTC()}
	return nil
}

func (f *fakeGroups) isMember(_ context.Context, groupID, userID string) (bool, error) {
	_, ok := f.members[groupID][userID]
	return ok, nil
}

func (f *fakeGroups) removeMember(_ context.Context, groupID, userID string) error {
	if _, ok := f.members[groupID][userID]; !ok {
		return ErrNotFound
	}
	delete(f.members[groupID], userID)
	return nil
}

func (f *fakeGroups) deleteGroup(_ context.Context, groupID string) error {
	if _, ok := f.byID[groupID]; !ok {
		return ErrNotFound
	}
	delete(f.byCode, f.byID[groupID].InviteCode)
	delete(f.byID, groupID)
	delete(f.members, groupID)
	return nil
}

func (f *fakeGroups) listMembers(_ context.Context, groupID string) ([]GroupMember, error) {
	var out []GroupMember
	for _, m := range f.members[groupID] {
		out = append(out, m)
	}
	return out, nil
}

func (f *fakeGroups) listWeekMatches(context.Context, string, string, time.Time, time.Time) ([]ladderMatch, error) {
	return f.matches, nil
}

func TestCreateGroup_RetriesInviteCodeCollision(t *testing.T) {
	fake := newFakeGroups()
	fake.conflicts = 1
	n := 0
	svc := &Service{groups: fake, code: func() string {
		n++
		if n == 1 {
			return "SAME"
		}
		return "OTHER"
	}}
	g, err := svc.CreateGroup(context.Background(), "owner", "Tuesday", "tennis")
	if err != nil {
		t.Fatal(err)
	}
	if g.InviteCode != "OTHER" || g.Sport != "TENNIS" || !g.IsOwner || g.MemberCount != 1 {
		t.Fatalf("%+v", g)
	}
}

func TestCreateGroup_ConflictWhenCodesKeepColliding(t *testing.T) {
	fake := newFakeGroups()
	fake.conflicts = 5
	svc := &Service{groups: fake, code: func() string { return "SAME" }}
	_, err := svc.CreateGroup(context.Background(), "owner", "Tuesday", "TENNIS")
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("got %v", err)
	}
}

func TestDeleteGroup_NonOwnerForbidden(t *testing.T) {
	fake := newFakeGroups()
	svc := &Service{groups: fake, code: func() string { return "CODE11" }}
	g, err := svc.CreateGroup(context.Background(), "owner", "Club", "PADEL")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.JoinGroup(context.Background(), "guest", "code11"); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteGroup(context.Background(), "guest", g.ID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("delete: %v", err)
	}
	if err := svc.LeaveGroup(context.Background(), "owner", g.ID); !errors.Is(err, ErrInvalid) {
		t.Fatalf("owner leave: %v", err)
	}
	if err := svc.DeleteGroup(context.Background(), "owner", g.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetGroup(context.Background(), "owner", g.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("after delete: %v", err)
	}
}

func TestGetGroup_CountsOnlyMemberMatchesThisWeek(t *testing.T) {
	now := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	start, _ := isoWeek(now)
	fake := newFakeGroups()
	svc := &Service{groups: fake, now: func() time.Time { return now }, code: func() string { return "WEEK01" }}
	g, err := svc.CreateGroup(context.Background(), "a", "Ladder", "TENNIS")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.JoinGroup(context.Background(), "b", "WEEK01"); err != nil {
		t.Fatal(err)
	}
	fake.members[g.ID]["a"] = GroupMember{UserID: "a", Name: "Ada"}
	fake.members[g.ID]["b"] = GroupMember{UserID: "b", Name: "Bea"}
	fake.matches = []ladderMatch{
		{Sport: "TENNIS", PlayedAt: start.Add(time.Hour), PlayerA: "a", PlayerB: "b", SetsA: 2, SetsB: 1, WinnerSide: "A"},
		{Sport: "TENNIS", PlayedAt: start.Add(-time.Hour), PlayerA: "a", PlayerB: "b", SetsA: 2, SetsB: 0, WinnerSide: "A"},
		{Sport: "TENNIS", PlayedAt: start.Add(2 * time.Hour), PlayerA: "a", PlayerB: "stranger", SetsA: 2, SetsB: 0, WinnerSide: "A"},
	}
	if _, err := svc.GetGroup(context.Background(), "stranger", g.ID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("stranger: %v", err)
	}
	detail, err := svc.GetGroup(context.Background(), "b", g.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Standings) != 2 {
		t.Fatalf("standings %+v", detail.Standings)
	}
	byID := map[string]Standing{}
	for _, row := range detail.Standings {
		byID[row.UserID] = row
	}
	if byID["a"].Wins != 1 || byID["a"].SetDiff != 1 || byID["b"].Losses != 1 || byID["b"].SetDiff != -1 {
		t.Fatalf("%+v", byID)
	}
}

func TestDeleteGroup_HandlerForbidsNonOwner(t *testing.T) {
	h := &handler{svc: &stubService{groupErr: ErrForbidden}}
	w := httptest.NewRecorder()
	h.deleteGroup(w, authedRequest(http.MethodDelete, "/v1/groups/g1", nil, "member"))
	if w.Code != http.StatusForbidden {
		t.Fatalf("status %d", w.Code)
	}
}

func TestCreateGroup_HandlerReturnsCode(t *testing.T) {
	h := &handler{svc: &stubService{group: Group{ID: "g1", Name: "Club", Sport: "TENNIS", InviteCode: "ABC123", IsOwner: true}}}
	body, _ := json.Marshal(map[string]string{"name": "Club", "sport": "TENNIS"})
	w := httptest.NewRecorder()
	h.createGroup(w, authedRequest(http.MethodPost, "/v1/groups", body, "owner"))
	if w.Code != http.StatusCreated {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	var got Group
	if err := json.NewDecoder(bytes.NewReader(w.Body.Bytes())).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.InviteCode != "ABC123" {
		t.Fatalf("%+v", got)
	}
}
