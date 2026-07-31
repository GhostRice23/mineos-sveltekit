package ports

import (
	"context"
	"time"
)

// WatchdogStatus mirrors the API's ServerWatchdogStatus.
type WatchdogStatus struct {
	ServerName      string     `json:"serverName"`
	IsMonitoring    bool       `json:"isMonitoring"`
	WasRunning      bool       `json:"wasRunning"`
	RestartAttempts int        `json:"restartAttempts"`
	LastCrashTime   *time.Time `json:"lastCrashTime"`
	LastRestart     *time.Time `json:"lastRestartAttempt"`
	CooldownEndsAt  *time.Time `json:"cooldownEndsAt"`
}

// InCooldown reports whether the watchdog is holding off restarts at now.
func (s WatchdogStatus) InCooldown(now time.Time) bool {
	return s.CooldownEndsAt != nil && s.CooldownEndsAt.After(now)
}

// CrashEvent mirrors the API's CrashEventDto.
type CrashEvent struct {
	ID                   int       `json:"id"`
	ServerName           string    `json:"serverName"`
	DetectedAt           time.Time `json:"detectedAt"`
	CrashType            string    `json:"crashType"`
	CrashDetails         *string   `json:"crashDetails"`
	AutoRestartAttempted bool      `json:"autoRestartAttempted"`
	AutoRestartSucceeded bool      `json:"autoRestartSucceeded"`
}

// Notification mirrors the API's SystemNotification. PerformanceService raises
// low-TPS alerts through this channel.
type Notification struct {
	ID         int       `json:"id"`
	Type       string    `json:"type"`
	Title      string    `json:"title"`
	Message    string    `json:"message"`
	CreatedAt  time.Time `json:"createdAt"`
	IsRead     bool      `json:"isRead"`
	ServerName *string   `json:"serverName"`
}

// HealthClient is the API surface the health roll-up consumes.
type HealthClient interface {
	Health(ctx context.Context) error
	WatchdogStatuses(ctx context.Context) ([]WatchdogStatus, error)
	Crashes(ctx context.Context, limit int) ([]CrashEvent, error)
	UnreadNotifications(ctx context.Context) ([]Notification, error)
}
