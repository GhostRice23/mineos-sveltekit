package usecases

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/freemancraft/mineos-sveltekit/tools/mineos-cli/internal/domain/ports"
)

var errUnavailable = errors.New("unavailable")

// fakeHealthClient lets each source fail independently, which is the case the
// roll-up has to survive.
type fakeHealthClient struct {
	healthErr   error
	statuses    []ports.WatchdogStatus
	statusesErr error
	crashes     []ports.CrashEvent
	crashesErr  error
	alerts      []ports.Notification
	alertsErr   error
	crashLimit  int
}

func (f *fakeHealthClient) Health(context.Context) error { return f.healthErr }

func (f *fakeHealthClient) WatchdogStatuses(context.Context) ([]ports.WatchdogStatus, error) {
	return f.statuses, f.statusesErr
}

func (f *fakeHealthClient) Crashes(_ context.Context, limit int) ([]ports.CrashEvent, error) {
	f.crashLimit = limit
	return f.crashes, f.crashesErr
}

func (f *fakeHealthClient) UnreadNotifications(context.Context) ([]ports.Notification, error) {
	return f.alerts, f.alertsErr
}

func at(base time.Time, minutes int) time.Time {
	return base.Add(time.Duration(minutes) * time.Minute)
}

func rollupAt(client HealthRollupClient, now time.Time) HealthRollup {
	uc := NewHealthRollupUseCase(client)
	uc.now = func() time.Time { return now }
	return uc.Execute(context.Background())
}

func TestRollupJoinsWatchdogStateWithCrashHistory(t *testing.T) {
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	cooldown := at(now, 5)

	client := &fakeHealthClient{
		statuses: []ports.WatchdogStatus{
			{ServerName: "lobby", IsMonitoring: true, RestartAttempts: 2, CooldownEndsAt: &cooldown},
			{ServerName: "creative", IsMonitoring: true},
		},
		crashes: []ports.CrashEvent{
			{ServerName: "lobby", DetectedAt: at(now, -30), CrashType: "ProcessDeath"},
			{ServerName: "lobby", DetectedAt: at(now, -10), CrashType: "OutOfMemory"},
		},
	}

	got := rollupAt(client, now)

	if !got.ApiReachable {
		t.Fatal("API should be reachable")
	}
	if got.TotalCrashes() != 2 {
		t.Errorf("TotalCrashes = %d, want 2", got.TotalCrashes())
	}
	if len(got.Servers) != 2 {
		t.Fatalf("got %d servers, want 2", len(got.Servers))
	}

	// Most-troubled first.
	lobby := got.Servers[0]
	if lobby.Name != "lobby" {
		t.Fatalf("first server = %q, want lobby (most crashes)", lobby.Name)
	}
	if lobby.CrashCount != 2 {
		t.Errorf("CrashCount = %d, want 2", lobby.CrashCount)
	}
	if lobby.LastCrash == nil || lobby.LastCrash.CrashType != "OutOfMemory" {
		t.Errorf("LastCrash = %+v, want the most recent one", lobby.LastCrash)
	}
	if !lobby.InCooldown {
		t.Error("lobby should be in cooldown")
	}
	if got.Servers[1].InCooldown {
		t.Error("creative should not be in cooldown")
	}
}

func TestRollupIncludesServersKnownOnlyFromCrashHistory(t *testing.T) {
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	client := &fakeHealthClient{
		statuses: []ports.WatchdogStatus{{ServerName: "lobby", IsMonitoring: true}},
		crashes:  []ports.CrashEvent{{ServerName: "deleted-server", DetectedAt: at(now, -5)}},
	}

	got := rollupAt(client, now)

	names := map[string]bool{}
	for _, s := range got.Servers {
		names[s.Name] = true
	}
	// The watchdog only knows what it currently monitors; a server that crashed
	// and was removed must not vanish from the report.
	if !names["deleted-server"] {
		t.Errorf("servers = %v, want the crash-only server included", names)
	}
	if !names["lobby"] {
		t.Errorf("servers = %v, want the monitored server included", names)
	}
}

func TestRollupSurvivesPartialSourceFailure(t *testing.T) {
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	client := &fakeHealthClient{
		statusesErr: errUnavailable,
		crashes:     []ports.CrashEvent{{ServerName: "lobby", DetectedAt: at(now, -1)}},
		alerts:      []ports.Notification{{ID: 1, Type: "warning", Title: "Low TPS"}},
	}

	got := rollupAt(client, now)

	if !got.ApiReachable {
		t.Fatal("a single failed source must not mark the API unreachable")
	}
	if len(got.Failures) != 1 || got.Failures[0].Source != "watchdog status" {
		t.Errorf("Failures = %+v, want the watchdog recorded", got.Failures)
	}
	// The data that did load must still be reported.
	if got.TotalCrashes() != 1 {
		t.Errorf("TotalCrashes = %d, want the crash history that loaded", got.TotalCrashes())
	}
	if len(got.Alerts) != 1 {
		t.Errorf("Alerts = %v, want the alerts that loaded", got.Alerts)
	}
}

func TestRollupReportsUnreachableApiWithoutQueryingTheRest(t *testing.T) {
	client := &fakeHealthClient{healthErr: errUnavailable}

	got := rollupAt(client, time.Now())

	if got.ApiReachable {
		t.Error("ApiReachable = true, want false")
	}
	if !errors.Is(got.ApiError, errUnavailable) {
		t.Errorf("ApiError = %v, want %v", got.ApiError, errUnavailable)
	}
	if client.crashLimit != 0 {
		t.Error("crash history should not be requested when the API is down")
	}
}

func TestRollupRequestsTheDocumentedCrashLimit(t *testing.T) {
	client := &fakeHealthClient{}

	rollupAt(client, time.Now())

	if client.crashLimit != CrashHistoryLimit {
		t.Errorf("crash limit = %d, want %d", client.crashLimit, CrashHistoryLimit)
	}
}

func TestHealthyOnlyWhenThereIsNothingToActOn(t *testing.T) {
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)

	quiet := rollupAt(&fakeHealthClient{
		statuses: []ports.WatchdogStatus{{ServerName: "lobby", IsMonitoring: true}},
	}, now)
	if !quiet.Healthy() {
		t.Error("a monitored server with no crashes or alerts should be healthy")
	}

	withAlert := rollupAt(&fakeHealthClient{
		alerts: []ports.Notification{{ID: 1, Title: "Low TPS"}},
	}, now)
	if withAlert.Healthy() {
		t.Error("an unread alert means there is something to act on")
	}

	withCrash := rollupAt(&fakeHealthClient{
		crashes: []ports.CrashEvent{{ServerName: "lobby", DetectedAt: at(now, -1)}},
	}, now)
	if withCrash.Healthy() {
		t.Error("a recorded crash means there is something to act on")
	}

	down := rollupAt(&fakeHealthClient{healthErr: errUnavailable}, now)
	if down.Healthy() {
		t.Error("an unreachable API is not healthy")
	}
}

func TestCooldownIsRelativeToNow(t *testing.T) {
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	past := at(now, -1)
	future := at(now, 1)

	if (ports.WatchdogStatus{CooldownEndsAt: &past}).InCooldown(now) {
		t.Error("an elapsed cooldown must not read as active")
	}
	if !(ports.WatchdogStatus{CooldownEndsAt: &future}).InCooldown(now) {
		t.Error("a future cooldown must read as active")
	}
	if (ports.WatchdogStatus{}).InCooldown(now) {
		t.Error("no cooldown set must not read as active")
	}
}
