package tui

import (
	"fmt"
	"math"
	"strings"
)

// sparkRunes are the eight block heights a sparkline is drawn from, lowest first.
var sparkRunes = []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

// SeriesStats summarises a series alongside its sparkline. Without these the
// glyphs are unreadable: a sparkline is normalised to its own range, so a flat
// line at 19 TPS and one at 3 TPS draw identically.
type SeriesStats struct {
	Min   float64
	Max   float64
	Avg   float64
	Count int
}

// Stats computes min/avg/max over values. Count is 0 for an empty series.
func Stats(values []float64) SeriesStats {
	if len(values) == 0 {
		return SeriesStats{}
	}
	stats := SeriesStats{Min: values[0], Max: values[0], Count: len(values)}
	sum := 0.0
	for _, v := range values {
		if v < stats.Min {
			stats.Min = v
		}
		if v > stats.Max {
			stats.Max = v
		}
		sum += v
	}
	stats.Avg = sum / float64(len(values))
	return stats
}

// Sparkline renders values as block glyphs, at most width wide.
//
// When there are more points than columns the series is downsampled by taking
// the mean of each bucket, so the shape survives instead of the tail being
// dropped. A flat series renders on the baseline rather than at full height —
// normalising a zero range to the top would make an idle server look pegged.
//
// scaleMin/scaleMax fix the vertical scale when a series has a meaningful
// absolute range (TPS is 0..20, CPU 0..100); pass NaN for either to take it
// from the data.
func Sparkline(values []float64, width int, scaleMin, scaleMax float64) string {
	if width <= 0 || len(values) == 0 {
		return ""
	}

	points := downsample(values, width)

	lo, hi := scaleMin, scaleMax
	observed := Stats(points)
	if math.IsNaN(lo) {
		lo = observed.Min
	}
	if math.IsNaN(hi) {
		hi = observed.Max
	}
	// Data outside the fixed scale still has to be drawable.
	lo = math.Min(lo, observed.Min)
	hi = math.Max(hi, observed.Max)

	var b strings.Builder
	span := hi - lo
	for _, v := range points {
		if span <= 0 {
			b.WriteRune(sparkRunes[0])
			continue
		}
		idx := int(math.Round((v - lo) / span * float64(len(sparkRunes)-1)))
		if idx < 0 {
			idx = 0
		}
		if idx >= len(sparkRunes) {
			idx = len(sparkRunes) - 1
		}
		b.WriteRune(sparkRunes[idx])
	}
	return b.String()
}

// downsample reduces values to at most width points by averaging buckets.
// Series shorter than width are returned unchanged — stretching them would
// invent detail that was never measured.
func downsample(values []float64, width int) []float64 {
	if len(values) <= width {
		return values
	}

	out := make([]float64, 0, width)
	for i := 0; i < width; i++ {
		start := i * len(values) / width
		end := (i + 1) * len(values) / width
		if end <= start {
			end = start + 1
		}
		if end > len(values) {
			end = len(values)
		}
		sum := 0.0
		for _, v := range values[start:end] {
			sum += v
		}
		out = append(out, sum/float64(end-start))
	}
	return out
}

// FormatSeriesStats renders "min X · avg Y · max Z" with the given precision.
func FormatSeriesStats(stats SeriesStats, decimals int, suffix string) string {
	if stats.Count == 0 {
		return "no data"
	}
	// The suffix is interpolated into a format string, so a literal '%' in it
	// (as for CPU) has to be escaped or fmt reads it as a verb.
	escaped := strings.ReplaceAll(suffix, "%", "%%")
	value := fmt.Sprintf("%%.%df%s", decimals, escaped)
	format := fmt.Sprintf("min %s · avg %s · max %s", value, value, value)
	return fmt.Sprintf(format, stats.Min, stats.Avg, stats.Max)
}
