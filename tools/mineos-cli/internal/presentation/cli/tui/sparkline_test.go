package tui

import (
	"math"
	"strings"
	"testing"

	"github.com/freemancraft/mineos-sveltekit/tools/mineos-cli/internal/infrastructure/api"
)

func TestStatsOverASeries(t *testing.T) {
	got := Stats([]float64{4, 1, 10, 5})

	if got.Min != 1 || got.Max != 10 || got.Count != 4 {
		t.Errorf("min/max/count = %v/%v/%d", got.Min, got.Max, got.Count)
	}
	if got.Avg != 5 {
		t.Errorf("avg = %v, want 5", got.Avg)
	}
}

func TestStatsOfEmptySeries(t *testing.T) {
	if got := Stats(nil); got.Count != 0 {
		t.Errorf("count = %d, want 0", got.Count)
	}
}

func TestSparklineUsesFullGlyphRange(t *testing.T) {
	spark := Sparkline([]float64{0, 1, 2, 3, 4, 5, 6, 7}, 8, math.NaN(), math.NaN())

	if got := []rune(spark); len(got) != 8 {
		t.Fatalf("got %d columns, want 8", len(got))
	}
	if !strings.HasPrefix(spark, string(sparkRunes[0])) {
		t.Errorf("minimum should render as the lowest block, got %q", spark)
	}
	if !strings.HasSuffix(spark, string(sparkRunes[len(sparkRunes)-1])) {
		t.Errorf("maximum should render as the highest block, got %q", spark)
	}
}

func TestSparklineDrawsFlatSeriesOnTheBaseline(t *testing.T) {
	// Normalising a zero range to the top would make an idle server look pegged.
	spark := Sparkline([]float64{20, 20, 20}, 8, math.NaN(), math.NaN())

	want := strings.Repeat(string(sparkRunes[0]), 3)
	if spark != want {
		t.Errorf("got %q, want %q", spark, want)
	}
}

func TestSparklineHonoursAFixedScale(t *testing.T) {
	// On a 0..100 CPU scale, a series hovering near 10% must stay low rather
	// than being stretched to full height by its own narrow range.
	fixed := Sparkline([]float64{9, 10, 11}, 3, 0, 100)
	for _, r := range fixed {
		if r > sparkRunes[2] {
			t.Errorf("expected columns in the lower third, got %q", fixed)
			break
		}
	}

	// Without the fixed scale the same data is normalised to its own range and
	// reaches the top — which is exactly what the scale exists to prevent.
	auto := Sparkline([]float64{9, 10, 11}, 3, math.NaN(), math.NaN())
	if !strings.ContainsRune(auto, sparkRunes[len(sparkRunes)-1]) {
		t.Errorf("auto-scaled series should reach full height, got %q", auto)
	}
}

func TestSparklineWidensScaleForOutOfRangeData(t *testing.T) {
	// A TPS reading above the nominal 20 must still be drawable.
	spark := Sparkline([]float64{0, 25}, 2, 0, 20)

	if got := []rune(spark); len(got) != 2 || got[1] != sparkRunes[len(sparkRunes)-1] {
		t.Errorf("got %q, want the out-of-range point at full height", spark)
	}
}

func TestSparklineDownsamplesToWidth(t *testing.T) {
	values := make([]float64, 500)
	for i := range values {
		values[i] = float64(i)
	}

	spark := Sparkline(values, 20, math.NaN(), math.NaN())

	if got := len([]rune(spark)); got != 20 {
		t.Errorf("got %d columns, want 20", got)
	}
}

func TestSparklineDownsamplingKeepsTheShape(t *testing.T) {
	// A rise-then-fall must not come out monotonic just because it was bucketed.
	values := make([]float64, 100)
	for i := range values {
		if i < 50 {
			values[i] = float64(i)
		} else {
			values[i] = float64(100 - i)
		}
	}

	runes := []rune(Sparkline(values, 10, math.NaN(), math.NaN()))
	peak := 0
	for i, r := range runes {
		if r > runes[peak] {
			peak = i
		}
	}
	if peak == 0 || peak == len(runes)-1 {
		t.Errorf("peak landed at the edge (%d) — the shape was lost: %q", peak, string(runes))
	}
}

func TestSparklineHandlesDegenerateInput(t *testing.T) {
	if got := Sparkline(nil, 10, math.NaN(), math.NaN()); got != "" {
		t.Errorf("empty series should render nothing, got %q", got)
	}
	if got := Sparkline([]float64{1, 2}, 0, math.NaN(), math.NaN()); got != "" {
		t.Errorf("zero width should render nothing, got %q", got)
	}
	if got := Sparkline([]float64{1, 2}, -5, math.NaN(), math.NaN()); got != "" {
		t.Errorf("negative width should render nothing, got %q", got)
	}
}

func TestFormatSeriesStats(t *testing.T) {
	got := FormatSeriesStats(Stats([]float64{1, 2, 3}), 1, "%")
	for _, want := range []string{"min 1.0%", "avg 2.0%", "max 3.0%"} {
		if !strings.Contains(got, want) {
			t.Errorf("%q missing from %q", want, got)
		}
	}

	if got := FormatSeriesStats(Stats(nil), 1, ""); got != "no data" {
		t.Errorf("empty stats = %q, want \"no data\"", got)
	}
}

func TestPerfSeriesSkipsMissingTps(t *testing.T) {
	tps20 := 20.0
	history := []api.PerfSample{
		{CpuPercent: 10, PlayerCount: 1, Tps: nil},
		{CpuPercent: 20, PlayerCount: 2, Tps: &tps20},
	}

	gotTps, gotCpu, gotPlayers := perfSeries(history)

	// A sample without TPS is a gap, not a zero — plotting it as 0 would look
	// like the server stalled.
	if len(gotTps) != 1 || gotTps[0] != 20 {
		t.Errorf("tps = %v, want just the reported sample", gotTps)
	}
	if len(gotCpu) != 2 || len(gotPlayers) != 2 {
		t.Errorf("cpu/players = %v/%v, want both samples", gotCpu, gotPlayers)
	}
}

func TestMetricsPanelSeedsFromHistoryBeforeAnyLiveSample(t *testing.T) {
	m := TuiModel{PerfHistory: []api.PerfSample{
		{CpuPercent: 10, PlayerCount: 1},
		{CpuPercent: 40, PlayerCount: 3},
	}}

	rendered := strings.Join(m.renderMetricsLines(), "\n")

	// The whole point of #130: the panel is useful before the first SSE frame.
	if strings.Contains(rendered, "waiting for data") {
		t.Errorf("panel still reports no data despite history:\n%s", rendered)
	}
	if !strings.Contains(rendered, "CPU") {
		t.Errorf("no CPU row rendered:\n%s", rendered)
	}
}

func TestMetricsPanelWaitsWhenThereIsNothingAtAll(t *testing.T) {
	rendered := strings.Join(TuiModel{}.renderMetricsLines(), "\n")

	if !strings.Contains(rendered, "waiting for data") {
		t.Errorf("expected the waiting state, got:\n%s", rendered)
	}
}

func TestSingleHistoryPointIsNotDrawnAsATrend(t *testing.T) {
	m := TuiModel{PerfHistory: []api.PerfSample{{CpuPercent: 10}}}

	rendered := strings.Join(m.renderSparklines(), "\n")

	if !strings.Contains(rendered, "collecting") {
		t.Errorf("expected a collecting notice for one point, got %q", rendered)
	}
}

func TestAppendPerfSamplesTrimsOldestFirst(t *testing.T) {
	older := make([]api.PerfSample, MaxPerfHistory)
	for i := range older {
		older[i].PlayerCount = i
	}
	newest := api.PerfSample{PlayerCount: 999}

	got := appendPerfSamples(older, []api.PerfSample{newest})

	if len(got) != MaxPerfHistory {
		t.Errorf("len = %d, want %d", len(got), MaxPerfHistory)
	}
	if got[len(got)-1].PlayerCount != 999 {
		t.Error("newest sample was trimmed instead of the oldest")
	}
}

func TestPerfHistoryForAnotherServerIsIgnored(t *testing.T) {
	// A slow history response for a server the user already left must not land
	// in the panel now on screen.
	m := TuiModel{PerfServer: "survival"}

	updated, _ := m.Update(PerfHistoryMsg{
		Server:  "lobby",
		Samples: []api.PerfSample{{CpuPercent: 50}},
	})

	if got := updated.(TuiModel).PerfHistory; len(got) != 0 {
		t.Errorf("history = %v, want it discarded", got)
	}
}

func TestPerfHistoryErrorIsIgnored(t *testing.T) {
	m := TuiModel{PerfServer: "lobby"}

	updated, _ := m.Update(PerfHistoryMsg{Server: "lobby", Err: errStreamClosed})

	if got := updated.(TuiModel).PerfHistory; len(got) != 0 {
		t.Errorf("history = %v, want it discarded", got)
	}
}

func TestLiveSampleExtendsTheHistory(t *testing.T) {
	m := TuiModel{PerfServer: "lobby", PerfHistory: []api.PerfSample{{CpuPercent: 10}}}

	updated, _ := m.Update(PerfSampleMsg{Sample: api.PerfSample{CpuPercent: 20}})
	next := updated.(TuiModel)

	if len(next.PerfHistory) != 2 {
		t.Fatalf("history = %v, want the live sample appended", next.PerfHistory)
	}
	if next.PerfHistory[1].CpuPercent != 20 {
		t.Error("live sample not appended at the end")
	}
	if next.PerfSample == nil || next.PerfSample.CpuPercent != 20 {
		t.Error("live sample not set as current")
	}
}
