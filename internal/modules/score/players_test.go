package score

import (
	"errors"
	"strings"
	"testing"
)

func strPtr(s string) *string { return &s }

func TestSideLabel_BlankPartnerStaysSingleName(t *testing.T) {
	if got := sideLabel("Ada", nil); got != "Ada" {
		t.Fatalf("nil partner %q", got)
	}
	blank := "   "
	if got := sideLabel("Ada", &blank); got != "Ada" {
		t.Fatalf("blank partner %q", got)
	}
	partner := " Cara "
	if got := sideLabel("Ada", &partner); got != "Ada / Cara" {
		t.Fatalf("combined %q", got)
	}
	slash := sideLabel("A/B", strPtr("C"))
	if slash != "A/B / C" || strings.Count(slash, " / ") != 1 {
		t.Fatalf("slash name %q", slash)
	}
	if sideLabel("Ada", strPtr("Ada")) != "Ada / Ada" {
		t.Fatal("identical names must both stay")
	}
}

func TestInviteeIDs_SkipsStarterBlankAndDuplicate(t *testing.T) {
	got := inviteeIDs("starter", strPtr("starter"), nil, strPtr(""), strPtr("p"), strPtr("p"), strPtr("o"))
	if strings.Join(got, ",") != "p,o" {
		t.Fatalf("invitees=%v", got)
	}
	if inviteeIDs("starter", nil, nil) != nil {
		t.Fatalf("guest match invited someone")
	}
}

func TestNormalizePartners_RejectsDuplicateAndNamelessID(t *testing.T) {
	name := "Cara"
	_, _, _, _, _, _, err := normalizePartners(&name, nil, strPtr("u1"), strPtr("u1"), nil, nil)
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("duplicate err=%v", err)
	}
	_, _, _, _, _, _, err = normalizePartners(nil, nil, nil, nil, strPtr("p"), nil)
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("nameless partner err=%v", err)
	}
	a2, _, _, _, id, _, err := normalizePartners(strPtr("  Cara  "), strPtr("  "), nil, nil, strPtr(" p "), nil)
	if err != nil || a2 == nil || *a2 != "Cara" || id == nil || *id != "p" {
		t.Fatalf("a2=%v id=%v err=%v", a2, id, err)
	}
}
