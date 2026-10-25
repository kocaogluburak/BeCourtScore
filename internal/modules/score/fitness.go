package score

const maxFitnessSamples = 240

// FitnessSample is one coarse HR/calorie snapshot during a match.
// Short JSON keys keep a long series small (same spirit as PointEvent).
type FitnessSample struct {
	T    int  `json:"t"`
	HR   *int `json:"hr,omitempty"`
	Kcal *int `json:"kcal,omitempty"`
}

// FitnessTrendPoint is one finished match on a Pro trend graph.
type FitnessTrendPoint struct {
	MatchID         string `json:"match_id"`
	Sport           string `json:"sport"`
	PlayedAt        string `json:"played_at"`
	AvgHeartRate    *int   `json:"avg_heart_rate,omitempty"`
	MaxHeartRate    *int   `json:"max_heart_rate,omitempty"`
	ActiveCalories  *int   `json:"active_calories,omitempty"`
	DurationSeconds *int   `json:"duration_seconds,omitempty"`
}

func capFitnessSamples(in []FitnessSample) []FitnessSample {
	if in == nil {
		return nil
	}
	out := make([]FitnessSample, 0, len(in))
	for _, s := range in {
		if s.T < 0 {
			continue
		}
		s.HR = clampHeartRate(s.HR)
		s.Kcal = clampCalories(s.Kcal)
		if len(out) == maxFitnessSamples {
			break
		}
		out = append(out, s)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func clampHeartRate(v *int) *int {
	if v == nil {
		return nil
	}
	if *v < 0 || *v > 250 {
		return nil
	}
	return v
}

func clampCalories(v *int) *int {
	if v == nil {
		return nil
	}
	if *v < 0 {
		return nil
	}
	return v
}

func sanitizeFitnessSource(src *string) *string {
	if src == nil {
		return nil
	}
	switch *src {
	case "apple_watch", "wear_os", "garmin":
		return src
	default:
		return nil
	}
}

func sanitizeFitnessInput(avgHR, maxHR, kcal *int, source *string, samples []FitnessSample) (*int, *int, *int, *string, []FitnessSample) {
	return clampHeartRate(avgHR), clampHeartRate(maxHR), clampCalories(kcal), sanitizeFitnessSource(source), capFitnessSamples(samples)
}
