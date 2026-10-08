package score

import (
	"encoding/json"
	"testing"
	"time"
)

func TestDurationSeconds_EndMinusStart(t *testing.T) {
	start := time.Date(2026, 10, 8, 18, 0, 0, 0, time.UTC)
	if got := DurationSeconds(start, start.Add(95*time.Second)); got != 95 {
		t.Fatalf("duration=%d", got)
	}
	if got := DurationSeconds(start, start.Add(-time.Second)); got != 0 {
		t.Fatalf("negative duration=%d", got)
	}
}

func TestSummarize_ServeGameAtFortyThirty(t *testing.T) {
	stats := Summarize([]PointEvent{{
		Side: "A", ScoreA: 3, ScoreB: 2, Server: "A1", Advantage: true,
	}}, nil)
	if stats.ServePlayedA != 1 || stats.ServeWonA != 1 || stats.BreakFacedB != 0 {
		t.Fatalf("40-30 serve game: %+v", stats)
	}
}

func TestSummarize_BreakPointConversion(t *testing.T) {
	stats := Summarize([]PointEvent{{
		Side: "B", ScoreA: 1, ScoreB: 3, Server: "A1", Advantage: true,
	}}, nil)
	if stats.BreakFacedB != 1 || stats.BreakWonB != 1 || stats.ServeWonA != 0 || stats.ServePlayedA != 1 {
		t.Fatalf("15-40 break: %+v", stats)
	}
}

func TestSummarize_GoldenPointBreak(t *testing.T) {
	stats := Summarize([]PointEvent{{
		Side: "B", ScoreA: 3, ScoreB: 3, Server: "A1", Advantage: false,
	}}, nil)
	if stats.BreakFacedB != 1 || stats.BreakWonB != 1 || stats.ServeWonA != 0 {
		t.Fatalf("golden break: %+v", stats)
	}
}

func TestSummarize_NoServerHidesServeRows(t *testing.T) {
	stats := Summarize([]PointEvent{{Side: "A", ScoreA: 3, ScoreB: 2}}, intPtr(40))
	if stats.HasServer || stats.TotalPoints != 1 || stats.PointsA != 1 || stats.DurationSeconds != 40 {
		t.Fatalf("%+v", stats)
	}
}

func TestSummarize_EmptyLog(t *testing.T) {
	stats := Summarize(nil, nil)
	if stats.TotalPoints != 0 || stats.HasServer || stats.DurationSeconds != 0 {
		t.Fatalf("empty: %+v", stats)
	}
}

func TestMatchJSON_OldMatchOmitsPointLog(t *testing.T) {
	raw, err := json.Marshal(baseMatch())
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if _, ok := decoded["point_log"]; ok {
		t.Fatalf("old match included point_log: %s", raw)
	}
	if _, ok := decoded["duration_seconds"]; ok {
		t.Fatalf("old match included duration: %s", raw)
	}
	stats := Summarize(nil, nil)
	if stats.TotalPoints != 0 || stats.HasServer {
		t.Fatalf("detail stats: %+v", stats)
	}
}

func intPtr(n int) *int { return &n }
