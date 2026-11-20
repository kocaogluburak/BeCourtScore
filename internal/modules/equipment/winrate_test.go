package equipment

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"courtscore/internal/modules/score"
)

func tennisSetup(t *testing.T, svc *Service, user string) (racket, str, shoes Usage) {
	t.Helper()
	ctx := context.Background()
	var err error
	racket, err = svc.Create(ctx, user, CreateInput{Sport: "TENNIS", CategoryID: catTRacket, BrandID: strp(brandBab), ModelID: strp(modelPure)})
	if err != nil {
		t.Fatal(err)
	}
	str, err = svc.Create(ctx, user, CreateInput{Sport: "TENNIS", CategoryID: catTString, BrandID: strp(brandLux), ModelID: strp(modelAlu), LifespanHours: intp(15), ParentItemID: &racket.ID})
	if err != nil {
		t.Fatal(err)
	}
	shoes, err = svc.Create(ctx, user, shoesIn("Asics"))
	if err != nil {
		t.Fatal(err)
	}
	return
}

func recorded(id, user, sport, winner string) score.Match {
	u := user
	other := "opp"
	return score.Match{ID: id, Sport: sport, CreatedBy: user, PlayerAUserID: &u, PlayerBUserID: &other, WinnerSide: winner, PlayedAt: time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)}
}

func TestComboKey_SortedIdentityBased(t *testing.T) {
	n := func(s string) *string { return &s }
	racket := Item{ID: "row-1", Category: Category{Code: "RACKET"}, BrandID: n("b1"), ModelID: n("m1")}
	str := Item{ID: "row-2", Category: Category{Code: "STRING"}, BrandID: n("b2"), ModelID: n("m2")}
	if got := comboKey([]Item{str, racket}); got != "RACKET:b1:m1|STRING:b2:m2" {
		t.Fatalf("key=%q", got)
	}
	// Physical row id and nickname do not matter: restring keeps the combo.
	str2 := str
	str2.ID, str2.Nickname = "row-99", n("fresh")
	if comboKey([]Item{racket, str2}) != comboKey([]Item{racket, str}) {
		t.Fatal("combo must not depend on row id / nickname")
	}
	custom := Item{Category: Category{Code: "SHOES"}, CustomBrandName: n("Home Brand"), CustomModelName: n(" Mk2 ")}
	if got := comboPart(custom); got != "SHOES:CUSTOM:home brand:CUSTOM:mk2" {
		t.Fatalf("custom part=%q", got)
	}
	noModel := Item{Category: Category{Code: "SHOES"}, BrandID: n("b3")}
	if got := comboPart(noModel); got != "SHOES:b3:NONE" {
		t.Fatalf("no model part=%q", got)
	}
	if comboKey(nil) != "" {
		t.Fatal("empty set => empty key")
	}
}

func TestWinPercent_Rounds(t *testing.T) {
	if winPercent(8, 12) != 67 || winPercent(0, 5) != 0 || winPercent(5, 5) != 100 || winPercent(1, 0) != 0 {
		t.Fatal("win percent rounding")
	}
}

func TestMatchSide(t *testing.T) {
	a, a2, b, b2 := "ua", "ua2", "ub", "ub2"
	m := score.Match{PlayerAUserID: &a, PlayerA2UserID: &a2, PlayerBUserID: &b, PlayerB2UserID: &b2}
	for user, want := range map[string]string{"ua": "A", "ua2": "A", "ub": "B", "ub2": "B", "other": "", "": ""} {
		if got := matchSide(m, user); got != want {
			t.Errorf("side(%q)=%q want %q", user, got, want)
		}
	}
}

func TestOnMatchRecorded_WritesComboForParticipantRecorder(t *testing.T) {
	svc, st := newSvc()
	ctx := context.Background()
	racket, str, shoes := tennisSetup(t, svc, "u1")

	svc.OnMatchRecorded(ctx, recorded("m1", "u1", "TENNIS", "A"))
	svc.OnMatchRecorded(ctx, recorded("m2", "u1", "TENNIS", "B"))
	if len(st.usage) != 2 {
		t.Fatalf("usage rows=%d", len(st.usage))
	}
	won, lost := st.usage[0], st.usage[1]
	if !won.Won || lost.Won {
		t.Fatalf("won flags: %v %v", won.Won, lost.Won)
	}
	if won.Sport != "TENNIS" || won.UserID != "u1" || won.MatchID != "m1" {
		t.Fatalf("row=%+v", won)
	}
	if len(won.ItemIDs) != 3 {
		t.Fatalf("item ids=%v", won.ItemIDs)
	}
	have := map[string]bool{}
	for _, id := range won.ItemIDs {
		have[id] = true
	}
	if !have[racket.ID] || !have[str.ID] || !have[shoes.ID] {
		t.Fatalf("item ids=%v", won.ItemIDs)
	}
	if won.ComboKey != comboKey(mustActive(t, st, "u1", "TENNIS")) {
		t.Fatalf("combo=%q", won.ComboKey)
	}
	// A rerecord of the same match is deduplicated.
	svc.OnMatchRecorded(ctx, recorded("m1", "u1", "TENNIS", "A"))
	if len(st.usage) != 2 {
		t.Fatalf("duplicate usage row: %d", len(st.usage))
	}
}

func mustActive(t *testing.T, st *memStore, user, sport string) []Item {
	t.Helper()
	items, err := st.activeItemsForSport(context.Background(), user, sport)
	if err != nil {
		t.Fatal(err)
	}
	return items
}

func TestOnMatchRecorded_NoRowCases(t *testing.T) {
	svc, st := newSvc()
	ctx := context.Background()
	tennisSetup(t, svc, "u1")

	// Recorder is not a participant (scorekeeper): no row.
	other, opp := "someone", "opp"
	svc.OnMatchRecorded(ctx, score.Match{ID: "m1", Sport: "TENNIS", CreatedBy: "u1", PlayerAUserID: &other, PlayerBUserID: &opp, WinnerSide: "A"})
	// Recorder has no gear in that sport.
	svc.OnMatchRecorded(ctx, recorded("m2", "u1", "PADEL", "A"))
	// Unknown sport / missing ids.
	svc.OnMatchRecorded(ctx, recorded("m3", "u1", "GOLF", "A"))
	svc.OnMatchRecorded(ctx, score.Match{Sport: "TENNIS", CreatedBy: "u1"})
	// Participant but not the recorder: only the recorder is tracked.
	u1 := "u1"
	svc.OnMatchRecorded(ctx, score.Match{ID: "m4", Sport: "TENNIS", CreatedBy: "u9", PlayerAUserID: &u1, PlayerBUserID: &other, WinnerSide: "A"})
	if len(st.usage) != 0 {
		t.Fatalf("unexpected usage rows: %+v", st.usage)
	}
}

func TestOnMatchRecorded_SportScopedCombo(t *testing.T) {
	svc, st := newSvc()
	ctx := context.Background()
	tennisSetup(t, svc, "u1")
	if _, err := svc.Create(ctx, "u1", CreateInput{Sport: "PADEL", CategoryID: catPala, CustomBrandName: strp("Nox"), LifespanHours: intp(90)}); err != nil {
		t.Fatal(err)
	}
	svc.OnMatchRecorded(ctx, recorded("m1", "u1", "PADEL", "A"))
	if len(st.usage) != 1 || len(st.usage[0].ItemIDs) != 1 || st.usage[0].Sport != "PADEL" {
		t.Fatalf("padel usage=%+v", st.usage)
	}
}

func TestOnMatchDeleted_RemovesUsage(t *testing.T) {
	svc, st := newSvc()
	ctx := context.Background()
	tennisSetup(t, svc, "u1")
	svc.OnMatchRecorded(ctx, recorded("m1", "u1", "TENNIS", "A"))
	svc.OnMatchRecorded(ctx, recorded("m2", "u1", "TENNIS", "A"))
	svc.OnMatchDeleted(ctx, "m1")
	svc.OnMatchDeleted(ctx, "")
	svc.OnMatchDeleted(ctx, "unknown")
	if len(st.usage) != 1 || st.usage[0].MatchID != "m2" {
		t.Fatalf("usage=%+v", st.usage)
	}
}

func TestWinRate_EnoughDataThresholdAndPercent(t *testing.T) {
	svc, _ := newSvc()
	ctx := context.Background()
	tennisSetup(t, svc, "u1")

	wr, err := svc.WinRate(ctx, "u1", "TENNIS")
	if err != nil || wr.EnoughData {
		t.Fatalf("no matches: %+v err=%v", wr, err)
	}
	results := []string{"A", "A", "B", "A"} // 3-1 after four matches: below MinWinRateSample
	for i, w := range results {
		svc.OnMatchRecorded(ctx, recorded("m"+itoa(i), "u1", "TENNIS", w))
	}
	if wr, _ := svc.WinRate(ctx, "u1", "TENNIS"); wr.EnoughData || wr.MatchesPlayed != nil {
		t.Fatalf("4 matches must not be enough: %+v", wr)
	}
	svc.OnMatchRecorded(ctx, recorded("m4", "u1", "TENNIS", "A")) // 4-1 of 5
	wr, err = svc.WinRate(ctx, "u1", "tennis")
	if err != nil || !wr.EnoughData || *wr.MatchesPlayed != 5 || *wr.MatchesWon != 4 || *wr.WinPercent != 80 {
		t.Fatalf("5 matches: %+v err=%v", wr, err)
	}
	if MinWinRateSample != 5 {
		t.Fatal("AC-17 floor is 5")
	}
}

func TestWinRate_ComboSurvivesRenewButChangesWithNewBrand(t *testing.T) {
	svc, _ := newSvc()
	ctx := context.Background()
	_, str, _ := tennisSetup(t, svc, "u1")
	for i := 0; i < 5; i++ {
		svc.OnMatchRecorded(ctx, recorded("m"+itoa(i), "u1", "TENNIS", "A"))
	}
	// Restring with the same brand/model: the combination (and its sample) is kept.
	renewed, err := svc.Renew(ctx, "u1", str.ID, RenewInput{Attributes: map[string]string{"tension_kg": "25"}})
	if err != nil {
		t.Fatal(err)
	}
	wr, _ := svc.WinRate(ctx, "u1", "TENNIS")
	if !wr.EnoughData || *wr.MatchesPlayed != 5 {
		t.Fatalf("restring must keep the sample: %+v", wr)
	}
	// Switching to another string brand starts a new combination.
	if _, err := svc.Renew(ctx, "u1", renewed.ID, RenewInput{CustomBrandName: strp("Gut Co")}); err != nil {
		t.Fatal(err)
	}
	if wr, _ := svc.WinRate(ctx, "u1", "TENNIS"); wr.EnoughData {
		t.Fatalf("a new string brand is a new combo: %+v", wr)
	}
	// Archiving the active gear leaves nothing to rate.
	items, _, _ := svc.List(ctx, "u1", ListFilter{Sport: "TENNIS"}, 20, 0)
	for _, it := range items {
		if _, err := svc.Archive(ctx, "u1", it.ID); err != nil {
			t.Fatal(err)
		}
	}
	if wr, _ := svc.WinRate(ctx, "u1", "TENNIS"); wr.EnoughData {
		t.Fatalf("no active gear: %+v", wr)
	}
}

func TestWinRate_Validation(t *testing.T) {
	svc, _ := newSvc()
	for _, sport := range []string{"", "GOLF"} {
		if _, err := svc.WinRate(context.Background(), "u1", sport); !errors.Is(err, ErrInvalid) {
			t.Errorf("sport %q err=%v", sport, err)
		}
	}
}

func TestWinRateHandler_ShapesAndErrors(t *testing.T) {
	played, won, pct := 12, 8, 67
	h := &handler{svc: &stubEquip{winRate: WinRate{EnoughData: true, MatchesPlayed: &played, MatchesWon: &won, WinPercent: &pct}}}
	w := httptest.NewRecorder()
	h.winRate(w, withUser(httptest.NewRequest(http.MethodGet, "/v1/me/equipment/win-rate?sport=TENNIS", nil), "u1"))
	var got map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if w.Code != http.StatusOK || got["enough_data"] != true || got["matches_played"] != float64(12) || got["matches_won"] != float64(8) || got["win_percent"] != float64(67) {
		t.Fatalf("enough: %d %s", w.Code, w.Body.String())
	}

	h = &handler{svc: &stubEquip{}}
	w = httptest.NewRecorder()
	h.winRate(w, withUser(httptest.NewRequest(http.MethodGet, "/v1/me/equipment/win-rate?sport=TENNIS", nil), "u1"))
	if w.Body.String() != "{\"enough_data\":false}\n" && w.Body.String() != "{\"enough_data\":false}" {
		t.Fatalf("not enough must be exactly enough_data:false, got %q", w.Body.String())
	}

	// 0 wins out of enough matches still reports the zeroes.
	zero, five := 0, 5
	h = &handler{svc: &stubEquip{winRate: WinRate{EnoughData: true, MatchesPlayed: &five, MatchesWon: &zero, WinPercent: &zero}}}
	w = httptest.NewRecorder()
	h.winRate(w, withUser(httptest.NewRequest(http.MethodGet, "/v1/me/equipment/win-rate?sport=TENNIS", nil), "u1"))
	got = map[string]any{}
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got["matches_won"] != float64(0) || got["win_percent"] != float64(0) {
		t.Fatalf("zero wins must be serialized: %s", w.Body.String())
	}

	h = &handler{svc: &stubEquip{err: ErrInvalid}}
	w = httptest.NewRecorder()
	h.winRate(w, withUser(httptest.NewRequest(http.MethodGet, "/v1/me/equipment/win-rate", nil), "u1"))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("missing sport status=%d", w.Code)
	}
}
