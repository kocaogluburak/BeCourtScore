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

func TestSummarize_OutcomeTalliesPerSide(t *testing.T) {
	stats := Summarize([]PointEvent{
		{Side: "A", ScoreA: 0, ScoreB: 0, Server: "A1", Advantage: true, Outcome: "ace", OutcomeSide: "A"},
		{Side: "B", ScoreA: 1, ScoreB: 0, Server: "A1", Advantage: true, Outcome: "df", OutcomeSide: "A"},
		{Side: "A", ScoreA: 1, ScoreB: 1, Outcome: "winner", OutcomeSide: "A"},
		{Side: "B", ScoreA: 2, ScoreB: 1, Outcome: "ue", OutcomeSide: "A"},
		{Side: "B", ScoreA: 2, ScoreB: 2, Outcome: "winner", OutcomeSide: "B"},
		{Side: "A", ScoreA: 2, ScoreB: 3, Outcome: "ue", OutcomeSide: "B"},
	}, nil)
	if stats.AcesA != 1 || stats.AcesB != 0 || stats.DoubleFaultsA != 1 || stats.DoubleFaultsB != 0 {
		t.Fatalf("serve outcomes: %+v", stats)
	}
	if stats.WinnersA != 1 || stats.WinnersB != 1 || stats.UnforcedErrorsA != 1 || stats.UnforcedErrorsB != 1 {
		t.Fatalf("rally outcomes: %+v", stats)
	}
	if stats.TaggedPoints != 6 || stats.TotalPoints != 6 {
		t.Fatalf("tagged=%d total=%d", stats.TaggedPoints, stats.TotalPoints)
	}
}

func TestSummarize_UntaggedAndUnknownIgnored(t *testing.T) {
	stats := Summarize([]PointEvent{
		{Side: "A", Outcome: "ace", OutcomeSide: "A"},
		{Side: "B"},
		{Side: "A", Outcome: "smash", OutcomeSide: "A"},
		{Side: "B", Outcome: "winner", OutcomeSide: "C"},
	}, nil)
	if stats.AcesA != 1 || stats.TaggedPoints != 1 || stats.TotalPoints != 4 || stats.WinnersA != 0 || stats.WinnersB != 0 {
		t.Fatalf("%+v", stats)
	}
}

func TestSummarize_TagsDoNotChangeServeOrBreak(t *testing.T) {
	plain := Summarize([]PointEvent{{
		Side: "B", ScoreA: 1, ScoreB: 3, Server: "A1", Advantage: true,
	}}, nil)
	tagged := Summarize([]PointEvent{{
		Side: "B", ScoreA: 1, ScoreB: 3, Server: "A1", Advantage: true,
		Outcome: "winner", OutcomeSide: "B",
	}}, nil)
	if plain.ServePlayedA != tagged.ServePlayedA || plain.ServeWonA != tagged.ServeWonA {
		t.Fatalf("serve changed: plain=%+v tagged=%+v", plain, tagged)
	}
	if plain.BreakFacedB != tagged.BreakFacedB || plain.BreakWonB != tagged.BreakWonB {
		t.Fatalf("break changed: plain=%+v tagged=%+v", plain, tagged)
	}
	if tagged.WinnersB != 1 || tagged.TaggedPoints != 1 {
		t.Fatalf("tag missing: %+v", tagged)
	}
}

func TestCapPointLog_DropsUnknownOutcomeAndCaps(t *testing.T) {
	in := []PointEvent{
		{Side: "A", Outcome: "ace", OutcomeSide: "A"},
		{Side: "B", Outcome: "smash", OutcomeSide: "B"},
		{Side: "X", Outcome: "winner", OutcomeSide: "A"},
		{Side: "A", Outcome: "winner", OutcomeSide: "Q"},
	}
	got := capPointLog(in)
	if len(got) != 3 {
		t.Fatalf("len=%d", len(got))
	}
	if got[0].Outcome != "ace" || got[0].OutcomeSide != "A" {
		t.Fatalf("kept ace: %+v", got[0])
	}
	if got[1].Outcome != "" || got[1].OutcomeSide != "" {
		t.Fatalf("unknown outcome kept: %+v", got[1])
	}
	if got[2].Outcome != "winner" || got[2].OutcomeSide != "" {
		t.Fatalf("bad side kept: %+v", got[2])
	}

	points := make([]PointEvent, maxPointLog+10)
	for i := range points {
		points[i] = PointEvent{Side: "A"}
	}
	if got := capPointLog(points); len(got) != maxPointLog {
		t.Fatalf("cap=%d", len(got))
	}
}

func TestPointEventJSON_OldEventOmitsOutcome(t *testing.T) {
	raw, err := json.Marshal(PointEvent{Side: "A", AtMs: 1, Advantage: true})
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if _, ok := decoded["o"]; ok {
		t.Fatalf("untagged event included o: %s", raw)
	}
	if _, ok := decoded["op"]; ok {
		t.Fatalf("untagged event included op: %s", raw)
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
