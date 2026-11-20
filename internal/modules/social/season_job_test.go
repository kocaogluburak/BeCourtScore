package social

import (
	"context"
	"errors"
	"testing"
	"time"

	"courtscore/internal/platform/entitlement"
	"courtscore/internal/platform/sse"
)

type fakeSeasons struct {
	byID     map[string]Season
	members  map[string][]seasonMember
	frozen   map[string][]SeasonStanding
	matches  []ladderMatch
	nights   []socialNight
	next     int
	complete int
}

func newFakeSeasons() *fakeSeasons {
	return &fakeSeasons{
		byID:    map[string]Season{},
		members: map[string][]seasonMember{},
		frozen:  map[string][]SeasonStanding{},
	}
}

func (f *fakeSeasons) createSeason(_ context.Context, s Season, members []seasonMember) (Season, error) {
	for _, existing := range f.byID {
		if existing.GroupID == s.GroupID && existing.Status == seasonStatusActive && s.Status == seasonStatusActive {
			return Season{}, ErrConflict
		}
		if existing.GroupID == s.GroupID && existing.SeasonNumber == s.SeasonNumber {
			return Season{}, ErrConflict
		}
	}
	f.next++
	s.ID = "s" + string(rune('0'+f.next))
	if s.CreatedAt.IsZero() {
		s.CreatedAt = time.Unix(int64(f.next), 0).UTC()
	}
	f.byID[s.ID] = s
	copied := append([]seasonMember(nil), members...)
	f.members[s.ID] = copied
	return s, nil
}

func (f *fakeSeasons) getActiveSeason(_ context.Context, groupID string) (Season, error) {
	for _, s := range f.byID {
		if s.GroupID == groupID && s.Status == seasonStatusActive {
			return s, nil
		}
	}
	return Season{}, ErrNoActiveSeason
}

func (f *fakeSeasons) getSeason(_ context.Context, id string) (Season, error) {
	s, ok := f.byID[id]
	if !ok {
		return Season{}, ErrNotFound
	}
	return s, nil
}

func (f *fakeSeasons) listSeasons(_ context.Context, groupID string, limit, offset int) ([]Season, int64, error) {
	var all []Season
	for _, s := range f.byID {
		if s.GroupID == groupID {
			all = append(all, s)
		}
	}
	total := int64(len(all))
	if offset > len(all) {
		return []Season{}, total, nil
	}
	all = all[offset:]
	if limit < len(all) {
		all = all[:limit]
	}
	return all, total, nil
}

func (f *fakeSeasons) listSeasonMembers(_ context.Context, seasonID string) ([]seasonMember, error) {
	return append([]seasonMember(nil), f.members[seasonID]...), nil
}

func (f *fakeSeasons) listFrozenStandings(_ context.Context, seasonID string) ([]SeasonStanding, error) {
	return append([]SeasonStanding(nil), f.frozen[seasonID]...), nil
}

func (f *fakeSeasons) freezeAndComplete(_ context.Context, seasonID, championUserID string, rows []SeasonStanding) error {
	s, ok := f.byID[seasonID]
	if !ok {
		return ErrNotFound
	}
	if s.Status != seasonStatusActive {
		return nil
	}
	f.complete++
	s.Status = seasonStatusCompleted
	if championUserID != "" {
		id := championUserID
		s.ChampionUserID = &id
	}
	f.byID[seasonID] = s
	f.frozen[seasonID] = append([]SeasonStanding(nil), rows...)
	return nil
}

func (f *fakeSeasons) listSeasonMatches(context.Context, string, string, time.Time, time.Time) ([]ladderMatch, error) {
	return f.matches, nil
}

func (f *fakeSeasons) listSeasonSocialNights(context.Context, string, time.Time, time.Time) ([]socialNight, error) {
	return f.nights, nil
}

func (f *fakeSeasons) listDueActiveSeasons(_ context.Context, now time.Time) ([]Season, error) {
	var out []Season
	for _, s := range f.byID {
		if s.Status == seasonStatusActive && !s.EndsAt.After(now) {
			out = append(out, s)
		}
	}
	return out, nil
}

func (f *fakeSeasons) listActiveSeasonsForUsers(_ context.Context, userIDs []string) ([]Season, error) {
	want := map[string]bool{}
	for _, id := range userIDs {
		want[id] = true
	}
	var out []Season
	for id, s := range f.byID {
		if s.Status != seasonStatusActive {
			continue
		}
		for _, m := range f.members[id] {
			if want[m.UserID] {
				out = append(out, s)
				break
			}
		}
	}
	return out, nil
}

func (f *fakeSeasons) maxSeasonNumber(_ context.Context, groupID string) (int, error) {
	max := 0
	for _, s := range f.byID {
		if s.GroupID == groupID && s.SeasonNumber > max {
			max = s.SeasonNumber
		}
	}
	return max, nil
}

type denyGate struct{}

func (denyGate) AllowCloudSync(context.Context, string) error  { return entitlement.ErrProRequired }
func (denyGate) AllowStats(context.Context, string) error      { return entitlement.ErrProRequired }
func (denyGate) AllowHeadToHead(context.Context, string) error { return entitlement.ErrProRequired }
func (denyGate) AllowAdvancedStats(context.Context, string) error {
	return entitlement.ErrProRequired
}
func (denyGate) AllowPublicLive(context.Context, string) error { return entitlement.ErrProRequired }
func (denyGate) AllowLargeTournament(context.Context, string, int) error {
	return entitlement.ErrProRequired
}
func (denyGate) AllowLargeLeague(_ context.Context, _ string, memberCount int) error {
	if memberCount < entitlement.FreeMaxLeagueMembers {
		return nil
	}
	return entitlement.ErrProRequired
}
func (denyGate) AllowSeasonArchive(context.Context, string) error     { return entitlement.ErrProRequired }
func (denyGate) AllowEquipment(context.Context, string) error         { return entitlement.ErrProRequired }
func (denyGate) MaxParticipants(context.Context, string) (int, error) { return 8, nil }
func (denyGate) IsPro(context.Context, string) (bool, error)          { return false, nil }

func seasonFixture() (*Service, *fakeGroups, *fakeSeasons) {
	groups := newFakeGroups()
	seasons := newFakeSeasons()
	svc := &Service{
		groups:  groups,
		seasons: seasons,
		now:     func() time.Time { return time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC) },
		code:    func() string { return "SEASON" },
	}
	return svc, groups, seasons
}

func TestRunSeasonRollover_FreezesOncePicksChampionOpensNext(t *testing.T) {
	svc, groups, seasons := seasonFixture()
	hub := sse.NewHub()
	svc.hub = hub
	g, err := svc.CreateGroup(context.Background(), "a", "Club", "TENNIS")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.JoinGroup(context.Background(), "b", "SEASON"); err != nil {
		t.Fatal(err)
	}
	groups.members[g.ID]["a"] = GroupMember{UserID: "a", Name: "Ada"}
	groups.members[g.ID]["b"] = GroupMember{UserID: "b", Name: "Bea"}
	now := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	start := now.AddDate(0, 0, -56)
	created, err := seasons.createSeason(context.Background(), Season{
		GroupID: g.ID, SeasonNumber: 1, Name: "Season 1", Status: seasonStatusActive,
		StartsAt: start, EndsAt: now, DivisionCount: 1,
	}, []seasonMember{{UserID: "a", Name: "Ada", DivisionTier: 1}, {UserID: "b", Name: "Bea", DivisionTier: 1}})
	if err != nil {
		t.Fatal(err)
	}
	seasons.matches = []ladderMatch{
		{Sport: "TENNIS", PlayedAt: start.Add(time.Hour), PlayerA: "a", PlayerB: "b", SetsA: 2, SetsB: 0, WinnerSide: "A"},
	}
	ch, unsub := hub.Subscribe("a")
	defer unsub()

	if err := svc.RunSeasonRollover(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	if seasons.complete != 1 {
		t.Fatalf("complete count %d", seasons.complete)
	}
	done, err := seasons.getSeason(context.Background(), created.ID)
	if err != nil || done.Status != seasonStatusCompleted {
		t.Fatalf("status %+v err=%v", done, err)
	}
	if done.ChampionUserID == nil || *done.ChampionUserID != "a" {
		t.Fatalf("champion %+v", done.ChampionUserID)
	}
	next, err := seasons.getActiveSeason(context.Background(), g.ID)
	if err != nil || next.SeasonNumber != 2 || next.Status != seasonStatusActive {
		t.Fatalf("next %+v err=%v", next, err)
	}

	if err := svc.RunSeasonRollover(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	if seasons.complete != 1 {
		t.Fatalf("second run should be idempotent, complete=%d", seasons.complete)
	}

	gotCompleted := false
	gotStarted := false
	for i := 0; i < 4; i++ {
		select {
		case ev := <-ch:
			if ev.Type == "season.completed" {
				gotCompleted = true
			}
			if ev.Type == "season.started" {
				gotStarted = true
			}
		default:
		}
	}
	if !gotCompleted || !gotStarted {
		t.Fatalf("sse completed=%v started=%v", gotCompleted, gotStarted)
	}
}

func TestStartSeason_ProGateOnEightMembers(t *testing.T) {
	svc, _, _ := seasonFixture()
	svc.gate = denyGate{}
	g, err := svc.CreateGroup(context.Background(), "owner", "Club", "TENNIS")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"b", "c", "d", "e", "f", "g", "h"} {
		if _, err := svc.JoinGroup(context.Background(), id, "SEASON"); err != nil {
			t.Fatal(err)
		}
	}
	_, err = svc.StartSeason(context.Background(), "owner", g.ID, SeasonConfig{Weeks: 8})
	if !errors.Is(err, entitlement.ErrProRequired) {
		t.Fatalf("err=%v", err)
	}
}

func TestStartSeason_SmallGroupFree(t *testing.T) {
	svc, _, _ := seasonFixture()
	svc.gate = denyGate{}
	g, err := svc.CreateGroup(context.Background(), "owner", "Club", "TENNIS")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.JoinGroup(context.Background(), "b", "SEASON"); err != nil {
		t.Fatal(err)
	}
	detail, err := svc.StartSeason(context.Background(), "owner", g.ID, SeasonConfig{Weeks: 6})
	if err != nil {
		t.Fatal(err)
	}
	if detail.Status != seasonStatusActive || detail.SeasonNumber != 1 || detail.IsFrozen {
		t.Fatalf("%+v", detail)
	}
}

func TestGetSeason_ArchiveProGated(t *testing.T) {
	svc, groups, seasons := seasonFixture()
	svc.gate = denyGate{}
	g, err := svc.CreateGroup(context.Background(), "owner", "Club", "TENNIS")
	if err != nil {
		t.Fatal(err)
	}
	groups.members[g.ID]["owner"] = GroupMember{UserID: "owner", Name: "Owner"}
	se, err := seasons.createSeason(context.Background(), Season{
		GroupID: g.ID, SeasonNumber: 1, Name: "Season 1", Status: seasonStatusCompleted,
		StartsAt: time.Now().AddDate(0, 0, -56), EndsAt: time.Now(), DivisionCount: 1,
	}, []seasonMember{{UserID: "owner", Name: "Owner", DivisionTier: 1}})
	if err != nil {
		t.Fatal(err)
	}
	seasons.frozen[se.ID] = []SeasonStanding{{UserID: "owner", Name: "Owner", Rank: 1, DivisionTier: 1, Outcome: outcomeChampion}}
	_, err = svc.GetSeason(context.Background(), "owner", g.ID, se.ID)
	if !errors.Is(err, entitlement.ErrProRequired) {
		t.Fatalf("archive err=%v", err)
	}
	cur, err := seasons.createSeason(context.Background(), Season{
		GroupID: g.ID, SeasonNumber: 2, Name: "Season 2", Status: seasonStatusActive,
		StartsAt: time.Now(), EndsAt: time.Now().AddDate(0, 0, 56), DivisionCount: 1,
	}, []seasonMember{{UserID: "owner", Name: "Owner", DivisionTier: 1}})
	if err != nil {
		t.Fatal(err)
	}
	live, err := svc.GetCurrentSeason(context.Background(), "owner", g.ID)
	if err != nil || live.ID != cur.ID || live.IsFrozen {
		t.Fatalf("live %+v err=%v", live, err)
	}
}

func TestOnMatchFinished_NilSafeAndPublishes(t *testing.T) {
	(&Service{}).OnMatchFinished(context.Background(), SeasonMatch{})
	svc, groups, seasons := seasonFixture()
	hub := sse.NewHub()
	svc.hub = hub
	g, err := svc.CreateGroup(context.Background(), "a", "Club", "TENNIS")
	if err != nil {
		t.Fatal(err)
	}
	groups.members[g.ID]["a"] = GroupMember{UserID: "a", Name: "Ada"}
	_, err = seasons.createSeason(context.Background(), Season{
		GroupID: g.ID, SeasonNumber: 1, Name: "Season 1", Status: seasonStatusActive,
		StartsAt: time.Now(), EndsAt: time.Now().AddDate(0, 0, 56), DivisionCount: 1,
	}, []seasonMember{{UserID: "a", Name: "Ada", DivisionTier: 1}})
	if err != nil {
		t.Fatal(err)
	}
	ch, unsub := hub.Subscribe("a")
	defer unsub()
	svc.OnMatchFinished(context.Background(), SeasonMatch{Sport: "TENNIS", PlayerIDs: []string{"a", "b"}})
	select {
	case ev := <-ch:
		if ev.Type != "season.standings_updated" {
			t.Fatalf("%+v", ev)
		}
	case <-time.After(time.Second):
		t.Fatal("expected standings_updated")
	}
}
