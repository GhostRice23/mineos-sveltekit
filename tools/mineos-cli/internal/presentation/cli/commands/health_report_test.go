package commands

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/freemancraft/mineos-sveltekit/tools/mineos-cli/internal/application/usecases"
	"github.com/freemancraft/mineos-sveltekit/tools/mineos-cli/internal/domain/ports"
)

func render(rollup usecases.HealthRollup, now time.Time) string {
	var buf bytes.Buffer
	RenderHealthRollup(&buf, rollup, now)
	return buf.String()
}

func TestReportShowsCrashesRestartsAndLastCrash(t *testing.T) {
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	out := render(usecases.HealthRollup{
		ApiReachable: true,
		Servers: []usecases.ServerHealth{{
			Name:            "lobby",
			IsMonitoring:    true,
			RestartAttempts: 3,
			CrashCount:      2,
			LastCrash: &ports.CrashEvent{
				CrashType:  "OutOfMemory",
				DetectedAt: now.Add(-90 * time.Minute),
			},
		}},
	}, now)

	for _, want := range []string{"lobby", "OutOfMemory", "1h ago", "API:      OK"} {
		if !strings.Contains(out, want) {
			t.Errorf("%q missing from report:\n%s", want, out)
		}
	}
}

func TestReportFlagsAnActiveCooldown(t *testing.T) {
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	out := render(usecases.HealthRollup{
		ApiReachable: true,
		Servers: []usecases.ServerHealth{
			{Name: "lobby", RestartAttempts: 3, InCooldown: true},
		},
	}, now)

	// A server in cooldown is not being restarted right now — the operator has
	// to be able to see that.
	if !strings.Contains(out, "3 cd") {
		t.Errorf("cooldown not flagged:\n%s", out)
	}
}

func TestReportListsUnreadAlertsWithScope(t *testing.T) {
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	server := "survival"
	out := render(usecases.HealthRollup{
		ApiReachable: true,
		Alerts: []ports.Notification{
			{Type: "warning", Title: "Low TPS", ServerName: &server, CreatedAt: now.Add(-2 * time.Minute)},
			{Type: "info", Title: "Update available", CreatedAt: now.Add(-3 * time.Hour)},
		},
	}, now)

	for _, want := range []string{"WARNING", "survival", "Low TPS", "2m ago", "global", "Update available"} {
		if !strings.Contains(out, want) {
			t.Errorf("%q missing from report:\n%s", want, out)
		}
	}
}

func TestReportTruncatesALongAlertList(t *testing.T) {
	now := time.Now()
	alerts := make([]ports.Notification, maxAlertsShown+5)
	for i := range alerts {
		alerts[i] = ports.Notification{Type: "info", Title: "alert", CreatedAt: now}
	}

	out := render(usecases.HealthRollup{ApiReachable: true, Alerts: alerts}, now)

	if !strings.Contains(out, "and 5 more") {
		t.Errorf("long alert list not truncated:\n%s", out)
	}
}

func TestReportSurfacesPartialFailures(t *testing.T) {
	now := time.Now()
	out := render(usecases.HealthRollup{
		ApiReachable: true,
		Failures: []usecases.SourceFailure{
			{Source: "watchdog status", Err: errors.New("boom")},
		},
	}, now)

	if !strings.Contains(out, "watchdog status unavailable") {
		t.Errorf("partial failure not reported:\n%s", out)
	}
	// A report with a failed source is not "nothing to act on".
	if strings.Contains(out, "Nothing to act on") {
		t.Errorf("failed source reported as all-clear:\n%s", out)
	}
}

func TestReportForUnreachableApiIsActionable(t *testing.T) {
	out := render(usecases.HealthRollup{
		ApiReachable: false,
		ApiError:     errors.New("connection refused"),
	}, time.Now())

	if !strings.Contains(out, "unreachable") || !strings.Contains(out, "connection refused") {
		t.Errorf("unreachable API not reported:\n%s", out)
	}
	if !strings.Contains(out, "mineos start") {
		t.Errorf("no next step suggested:\n%s", out)
	}
}

func TestReportSaysSoWhenAllClear(t *testing.T) {
	out := render(usecases.HealthRollup{
		ApiReachable: true,
		Servers:      []usecases.ServerHealth{{Name: "lobby", IsMonitoring: true}},
	}, time.Now())

	if !strings.Contains(out, "Nothing to act on") {
		t.Errorf("all-clear not stated:\n%s", out)
	}
	if !strings.Contains(out, "1 monitored") {
		t.Errorf("monitored count wrong:\n%s", out)
	}
}

func TestHumanAge(t *testing.T) {
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	cases := map[time.Duration]string{
		-30 * time.Second: "just now",
		-5 * time.Minute:  "5m ago",
		-3 * time.Hour:    "3h ago",
		-50 * time.Hour:   "2d ago",
		30 * time.Second:  "just now", // clock skew must not print "-1m ago"
	}
	for delta, want := range cases {
		if got := humanAge(now.Add(delta), now); got != want {
			t.Errorf("humanAge(%v) = %q, want %q", delta, got, want)
		}
	}
	if got := humanAge(time.Time{}, now); got != "unknown" {
		t.Errorf("zero time = %q, want unknown", got)
	}
}

func TestTruncateKeepsColumnsAligned(t *testing.T) {
	if got := truncate("short", 22); got != "short" {
		t.Errorf("got %q, want it untouched", got)
	}
	long := truncate(strings.Repeat("x", 40), 22)
	if len([]rune(long)) != 22 {
		t.Errorf("truncated to %d runes, want 22", len([]rune(long)))
	}
}
