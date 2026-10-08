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
}
