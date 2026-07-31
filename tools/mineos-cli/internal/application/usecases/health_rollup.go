package usecases

import (
	"context"
	"sort"
	"time"

	"github.com/freemancraft/mineos-sveltekit/tools/mineos-cli/internal/domain/ports"
)

// HealthRollup is the consolidated health picture the CLI reports.
//
// The API has computed all of this for a while — the watchdog tracks crashes,
// restart attempts and cooldowns, and PerformanceService raises low-TPS alerts
// into the notification stream — but the CLI only ever showed a health badge.
type HealthRollup struct {
	ApiReachable bool
	ApiError     error

	Servers []ServerHealth

	// Alerts are undismissed, unread notifications, newest first.
	Alerts []ports.Notification
	// Partial sources that failed. The roll-up still renders what it has:
	// a watchdog outage should not hide the alerts, and vice versa.
	Failures []SourceFailure
}

// ServerHealth is one server's line in the roll-up.
type ServerHealth struct {
	Name            string
	IsMonitoring    bool
	RestartAttempts int
	CrashCount      int
	LastCrash       *ports.CrashEvent
	CooldownEndsAt  *time.Time
	InCooldown      bool
}

// SourceFailure records that one of the roll-up's inputs could not be read.
type SourceFailure struct {
	Source string
	Err    error
}

// HealthRollupClient is the slice of the API client the roll-up needs.
type HealthRollupClient interface {
	Health(ctx context.Context) error
	WatchdogStatuses(ctx context.Context) ([]ports.WatchdogStatus, error)
	Crashes(ctx context.Context, limit int) ([]ports.CrashEvent, error)
	UnreadNotifications(ctx context.Context) ([]ports.Notification, error)
}

// HealthRollupUseCase assembles the roll-up from the three API sources.
type HealthRollupUseCase struct {
	client HealthRollupClient
	now    func() time.Time
}

func NewHealthRollupUseCase(client HealthRollupClient) *HealthRollupUseCase {
	return &HealthRollupUseCase{client: client, now: time.Now}
}

// CrashHistoryLimit is how many recent crash events the roll-up requests.
const CrashHistoryLimit = 50

// Execute gathers the roll-up. It never returns an error for a partially
// available API: each failed source is recorded in Failures so the caller can
// show what did load.
func (u *HealthRollupUseCase) Execute(ctx context.Context) HealthRollup {
	now := u.now()
	rollup := HealthRollup{ApiReachable: true}

	if err := u.client.Health(ctx); err != nil {
		// Without the API nothing else will answer either; report once.
		return HealthRollup{ApiReachable: false, ApiError: err}
	}

	statuses, err := u.client.WatchdogStatuses(ctx)
	if err != nil {
		rollup.Failures = append(rollup.Failures, SourceFailure{Source: "watchdog status", Err: err})
	}

	crashes, err := u.client.Crashes(ctx, CrashHistoryLimit)
	if err != nil {
		rollup.Failures = append(rollup.Failures, SourceFailure{Source: "crash history", Err: err})
	}

	alerts, err := u.client.UnreadNotifications(ctx)
	if err != nil {
		rollup.Failures = append(rollup.Failures, SourceFailure{Source: "notifications", Err: err})
	}
	rollup.Alerts = alerts

	rollup.Servers = buildServerHealth(statuses, crashes, now)
	return rollup
}

// buildServerHealth joins watchdog state with crash history.
//
// A server can appear in either source alone: the watchdog only knows servers
// it currently monitors, while crash history outlives them. Both are included
// so a crashed-and-removed server does not silently vanish from the report.
func buildServerHealth(statuses []ports.WatchdogStatus, crashes []ports.CrashEvent, now time.Time) []ServerHealth {
	byName := make(map[string]*ServerHealth)

	for _, status := range statuses {
		byName[status.ServerName] = &ServerHealth{
			Name:            status.ServerName,
			IsMonitoring:    status.IsMonitoring,
			RestartAttempts: status.RestartAttempts,
			CooldownEndsAt:  status.CooldownEndsAt,
			InCooldown:      status.InCooldown(now),
		}
	}

	for i := range crashes {
		crash := crashes[i]
		entry, ok := byName[crash.ServerName]
		if !ok {
			entry = &ServerHealth{Name: crash.ServerName}
			byName[crash.ServerName] = entry
		}
		entry.CrashCount++
		if entry.LastCrash == nil || crash.DetectedAt.After(entry.LastCrash.DetectedAt) {
			entry.LastCrash = &crash
		}
	}

	out := make([]ServerHealth, 0, len(byName))
	for _, entry := range byName {
		out = append(out, *entry)
	}
	// Most-troubled first: crashes, then restart attempts, then name so the
	// order is stable for equal servers.
	sort.Slice(out, func(i, j int) bool {
		if out[i].CrashCount != out[j].CrashCount {
			return out[i].CrashCount > out[j].CrashCount
		}
		if out[i].RestartAttempts != out[j].RestartAttempts {
			return out[i].RestartAttempts > out[j].RestartAttempts
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// TotalCrashes sums crash events across servers.
func (r HealthRollup) TotalCrashes() int {
	total := 0
	for _, s := range r.Servers {
		total += s.CrashCount
	}
	return total
}

// Healthy reports whether there is nothing to act on.
func (r HealthRollup) Healthy() bool {
	return r.ApiReachable && r.TotalCrashes() == 0 && len(r.Alerts) == 0
}
