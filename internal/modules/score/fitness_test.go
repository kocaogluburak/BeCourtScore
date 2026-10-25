package score

import "testing"

func TestCapFitnessSamples_DropsNegativesAndCaps(t *testing.T) {
	hr := 140
	kcal := 20
	badHR := 400
	negKcal := -3
	samples := make([]FitnessSample, maxFitnessSamples+10)
	for i := range samples {
		samples[i] = FitnessSample{T: i, HR: &hr, Kcal: &kcal}
	}
	samples[0] = FitnessSample{T: -1, HR: &hr}
	samples[1] = FitnessSample{T: 1, HR: &badHR, Kcal: &negKcal}
	got := capFitnessSamples(samples)
	if len(got) != maxFitnessSamples {
		t.Fatalf("len=%d want %d", len(got), maxFitnessSamples)
	}
	if got[0].T < 0 {
		t.Fatal("negative t should be dropped")
	}
	for _, s := range got {
		if s.HR != nil && (*s.HR < 0 || *s.HR > 250) {
			t.Fatalf("hr not clamped: %+v", s)
		}
		if s.Kcal != nil && *s.Kcal < 0 {
			t.Fatalf("kcal not clamped: %+v", s)
		}
	}
}

func TestSanitizeFitnessInput_DropsInvalid(t *testing.T) {
	neg := -4
	high := 300
	src := "fitbit"
	okSrc := "apple_watch"
	avg, max, kcal, source, samples := sanitizeFitnessInput(&neg, &high, &neg, &src, []FitnessSample{{T: 0, HR: &high}})
	if avg != nil || max != nil || kcal != nil || source != nil {
		t.Fatalf("expected nils, got avg=%v max=%v kcal=%v src=%v", avg, max, kcal, source)
	}
	if samples == nil || samples[0].HR != nil {
		t.Fatalf("sample hr should be dropped: %+v", samples)
	}
	avg, max, kcal, source, _ = sanitizeFitnessInput(intPtr(120), intPtr(180), intPtr(45), &okSrc, nil)
	if avg == nil || *avg != 120 || max == nil || *max != 180 || kcal == nil || *kcal != 45 || source == nil || *source != "apple_watch" {
		t.Fatalf("kept valid fields: avg=%v max=%v kcal=%v src=%v", avg, max, kcal, source)
	}
}
