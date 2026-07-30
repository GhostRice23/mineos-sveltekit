package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/freemancraft/mineos-sveltekit/tools/mineos-cli/internal/domain/ports"
)

// getJSON performs an authenticated GET and decodes the body into out.
//
// The path is absolute rather than relative to /api/v1: the watchdog endpoints
// are registered at /api/watchdog, outside that group.
func (c *Client) getJSON(ctx context.Context, path string, out any) error {
	if strings.TrimSpace(c.apiKey) == "" {
		return ErrApiKeyMissing
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Api-Key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		return ErrApiKeyInvalid
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("GET %s failed: %s", path, readBody(resp.Body))
	}

	return json.NewDecoder(resp.Body).Decode(out)
}

// WatchdogStatuses returns the watchdog's per-server state, sorted by name.
func (c *Client) WatchdogStatuses(ctx context.Context) ([]ports.WatchdogStatus, error) {
	// The API returns a map keyed by server name.
	var byName map[string]ports.WatchdogStatus
	if err := c.getJSON(ctx, "/api/watchdog/status", &byName); err != nil {
		return nil, err
	}

	statuses := make([]ports.WatchdogStatus, 0, len(byName))
	for name, status := range byName {
		if status.ServerName == "" {
			status.ServerName = name
		}
		statuses = append(statuses, status)
	}
	sort.Slice(statuses, func(i, j int) bool { return statuses[i].ServerName < statuses[j].ServerName })
	return statuses, nil
}

// Crashes returns recent crash events across all servers, newest first.
func (c *Client) Crashes(ctx context.Context, limit int) ([]ports.CrashEvent, error) {
	if limit <= 0 {
		limit = 50
	}
	var events []ports.CrashEvent
	if err := c.getJSON(ctx, fmt.Sprintf("/api/watchdog/crashes?limit=%d", limit), &events); err != nil {
		return nil, err
	}
	sort.Slice(events, func(i, j int) bool { return events[i].DetectedAt.After(events[j].DetectedAt) })
	return events, nil
}

// UnreadNotifications returns undismissed, unread alerts, newest first.
func (c *Client) UnreadNotifications(ctx context.Context) ([]ports.Notification, error) {
	var notifications []ports.Notification
	err := c.getJSON(ctx,
		"/api/v1/notifications/?includeRead=false&includeDismissed=false",
		&notifications)
	if err != nil {
		return nil, err
	}
	sort.Slice(notifications, func(i, j int) bool {
		return notifications[i].CreatedAt.After(notifications[j].CreatedAt)
	})
	return notifications, nil
}
