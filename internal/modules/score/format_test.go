package score

import "testing"

func TestNormalizeSetFormat(t *testing.T) {
	rule, final, err := normalizeSetFormat("", "")
	if err != nil || rule != "STANDARD_6" || final != "REGULAR" {
		t.Fatalf("defaults: %s %s %v", rule, final, err)
	}
	rule, final, err = normalizeSetFormat("FAST4", "MATCH_TIEBREAK")
	if err != nil || rule != "FAST4" || final != "MATCH_TIEBREAK" {
		t.Fatalf("known: %s %s %v", rule, final, err)
	}
	if _, _, err = normalizeSetFormat("STAR", "REGULAR"); err == nil {
		t.Fatal("expected invalid set rule")
	}
}
