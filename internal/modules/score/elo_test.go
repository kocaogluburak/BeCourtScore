package score

import "testing"

func TestNextRatings_EqualStartMovesSixteen(t *testing.T) {
	win, lose := nextRatings(1500, 1500, 0, 0, true)
	if win != 1516 || lose != 1484 {
		t.Fatalf("got %d / %d, want 1516 / 1484", win, lose)
	}
}

func TestNextRatings_KDropsAfterTwenty(t *testing.T) {
	win, lose := nextRatings(1500, 1500, 20, 20, true)
	if win != 1508 || lose != 1492 {
		t.Fatalf("got %d / %d, want 1508 / 1492", win, lose)
	}
	// The 20th rated match (count 19) still uses K=32.
	win, lose = nextRatings(1500, 1500, 19, 19, true)
	if win != 1516 || lose != 1484 {
		t.Fatalf("count 19 got %d / %d", win, lose)
	}
}

func TestNextRatings_FavoriteGainsLess(t *testing.T) {
	win, lose := nextRatings(1600, 1500, 0, 0, true)
	if win != 1612 || lose != 1488 {
		t.Fatalf("got %d / %d, want 1612 / 1488", win, lose)
	}
}

func TestSinglesEloPair_SkipsGuestAndDoubles(t *testing.T) {
	a, b := "u1", "u2"
	guest := Match{PlayerAUserID: &a, PlayerAName: "A", PlayerBName: "Guest", WinnerSide: "A"}
	if _, _, ok := singlesEloPair(guest); ok {
		t.Fatal("guest opponent should not rate")
	}
	partner := "Bea"
	doubles := Match{
		PlayerAUserID: &a, PlayerBUserID: &b, PlayerA2Name: &partner,
		WinnerSide: "A",
	}
	if _, _, ok := singlesEloPair(doubles); ok {
		t.Fatal("doubles should not rate")
	}
	okMatch := Match{PlayerAUserID: &a, PlayerBUserID: &b, WinnerSide: "B"}
	winner, loser, ok := singlesEloPair(okMatch)
	if !ok || winner != "u2" || loser != "u1" {
		t.Fatalf("pair=%s %s ok=%v", winner, loser, ok)
	}
}

func TestAttachRatings_SportsStaySeparate(t *testing.T) {
	tennis := "TENNIS"
	stats := UserStats{
		Sport: &tennis,
		BySport: []SportStats{
			{Sport: "TENNIS"},
			{Sport: "PADEL"},
		},
	}
	got := attachRatings(stats, []UserRating{
		{Sport: "TENNIS", Rating: 1516},
		{Sport: "PADEL", Rating: 1480},
	})
	if got.Rating != 1516 {
		t.Fatalf("filtered rating=%d", got.Rating)
	}
	if got.BySport[0].Rating != 1516 || got.BySport[1].Rating != 1480 {
		t.Fatalf("by sport=%+v", got.BySport)
	}
	if got.DoublesRating != initialRating || got.BySport[0].DoublesRating != initialRating {
		t.Fatalf("doubles default=%d %+v", got.DoublesRating, got.BySport)
	}
}

func TestNextDoublesRatings_EqualTeamsMoveSixteen(t *testing.T) {
	side := [2]lockedRating{{rating: 1500}, {rating: 1500}}
	win, lose := nextDoublesRatings(side, side, true)
	if win[0] != 1516 || win[1] != 1516 || lose[0] != 1484 || lose[1] != 1484 {
		t.Fatalf("win=%v lose=%v", win, lose)
	}
}

func TestDoublesEloSides_RequiresFourDistinctRegistered(t *testing.T) {
	a, b, c, d := "u1", "u2", "u3", "u4"
	guest := "Guest"
	if _, _, _, ok := doublesEloSides(Match{
		PlayerAUserID: &a, PlayerA2UserID: &b, PlayerBUserID: &c, PlayerB2Name: &guest, WinnerSide: "A",
	}); ok {
		t.Fatal("guest partner should not rate")
	}
	if _, _, _, ok := doublesEloSides(Match{
		PlayerAUserID: &a, PlayerA2UserID: &b, PlayerBUserID: &c, PlayerB2UserID: &a, WinnerSide: "A",
	}); ok {
		t.Fatal("repeated user should not rate")
	}
	sideA, sideB, aWon, ok := doublesEloSides(Match{
		PlayerAUserID: &a, PlayerA2UserID: &b, PlayerBUserID: &c, PlayerB2UserID: &d, WinnerSide: "B",
	})
	if !ok || aWon || sideA != [2]string{a, b} || sideB != [2]string{c, d} {
		t.Fatalf("sides=%v %v won=%v ok=%v", sideA, sideB, aWon, ok)
	}
}

func TestAttachRatings_DoublesStaySeparate(t *testing.T) {
	padel := "PADEL"
	stats := UserStats{Sport: &padel, BySport: []SportStats{{Sport: "PADEL"}}}
	got := attachRatings(stats, []UserRating{
		{Sport: "PADEL", Discipline: DisciplineSingles, Rating: 1510},
		{Sport: "PADEL", Discipline: DisciplineDoubles, Rating: 1604},
	})
	if got.Rating != 1510 || got.DoublesRating != 1604 {
		t.Fatalf("rating=%d doubles=%d", got.Rating, got.DoublesRating)
	}
	if got.BySport[0].Rating != 1510 || got.BySport[0].DoublesRating != 1604 {
		t.Fatalf("by sport=%+v", got.BySport[0])
	}
}
