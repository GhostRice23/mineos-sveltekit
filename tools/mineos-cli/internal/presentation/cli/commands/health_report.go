package commands

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/freemancraft/mineos-sveltekit/tools/mineos-cli/internal/application/usecases"
)

// maxAlertsShown bounds the alert list so one noisy server cannot bury the
// per-server summary underneath it.
const maxAlertsShown = 10

// RenderHealthRollup writes the consolidated health report.
//
// Written against an io.Writer and a caller-supplied clock so it can be tested
// without a terminal or a real API.
func RenderHealthRollup(out io.Writer, rollup usecases.HealthRollup, now time.Time) {
	if !rollup.ApiReachable {
		fmt.Fprintf(out, "API:      unreachable (%v)\n", rollup.ApiError)
		fmt.Fprintln(out, "\nStart the stack with 'mineos start', then re-run 'mineos health --all'.")
		return
	}

	fmt.Fprintln(out, "API:      OK")
	fmt.Fprintf(out, "Servers:  %d monitored\n", countMonitored(rollup.Servers))
	fmt.Fprintf(out, "Crashes:  %d recorded\n", rollup.TotalCrashes())
	fmt.Fprintf(out, "Alerts:   %d unread\n", len(rollup.Alerts))

	for _, failure := range rollup.Failures {
		fmt.Fprintf(out, "  ! %s unavailable: %v\n", failure.Source, failure.Err)
	}

	if len(rollup.Servers) > 0 {
		fmt.Fprintln(out, "\nSERVER                 CRASHES  RESTARTS  LAST CRASH")
		for _, server := range rollup.Servers {
			fmt.Fprintf(out, "%-22s %7d  %8s  %s\n",
				truncate(server.Name, 22),
				server.CrashCount,
				restartCell(server),
				lastCrashCell(server, now))
		}
	}

	if len(rollup.Alerts) > 0 {
		fmt.Fprintln(out, "\nUNREAD ALERTS")
		for i, alert := range rollup.Alerts {
			if i == maxAlertsShown {
				fmt.Fprintf(out, "  … and %d more\n", len(rollup.Alerts)-maxAlertsShown)
				break
			}
			scope := "global"
			if alert.ServerName != nil && *alert.ServerName != "" {
				scope = *alert.ServerName
			}
			fmt.Fprintf(out, "  [%s] %s — %s (%s)\n",
				strings.ToUpper(alert.Type), scope, alert.Title, humanAge(alert.CreatedAt, now))
		}
	}

	if rollup.Healthy() && len(rollup.Failures) == 0 {
		fmt.Fprintln(out, "\nNothing to act on.")
	}
}

func countMonitored(servers []usecases.ServerHealth) int {
	n := 0
	for _, s := range servers {
		if s.IsMonitoring {
			n++
		}
	}
	return n
}

// restartCell shows the restart attempts, flagging an active cooldown — a
// server in cooldown is not being restarted right now, which is the thing an
// operator most needs to know.
func restartCell(server usecases.ServerHealth) string {
	if server.InCooldown {
		return fmt.Sprintf("%d cd", server.RestartAttempts)
	}
	return fmt.Sprintf("%d", server.RestartAttempts)
}

func lastCrashCell(server usecases.ServerHealth, now time.Time) string {
	if server.LastCrash == nil {
		return "—"
	}
	return fmt.Sprintf("%s (%s)", server.LastCrash.CrashType, humanAge(server.LastCrash.DetectedAt, now))
}

// humanAge renders how long ago t was, coarsely.
func humanAge(t time.Time, now time.Time) string {
	if t.IsZero() {
		return "unknown"
	}
	d := now.Sub(t)
	switch {
	case d < 0:
		return "just now"
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 1 {
		return s[:max]
	}
	return s[:max-1] + "…"
}
