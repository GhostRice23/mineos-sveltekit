package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWatchdogStatuses_FlattensTheMapAndSortsByName(t *testing.T) {
	mux := http.NewServeMux()
	// Registered at /api/watchdog, outside the /api/v1 group.
	mux.HandleFunc("/api/watchdog/status", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"survival": {"serverName":"survival","isMonitoring":true,"restartAttempts":2,"cooldownEndsAt":"2026-07-30T12:05:00Z"},
			"creative": {"serverName":"creative","isMonitoring":false,"restartAttempts":0}
		}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	statuses, err := NewClient(srv.URL, "k").WatchdogStatuses(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(statuses) != 2 {
		t.Fatalf("got %d statuses, want 2", len(statuses))
	}
	if statuses[0].ServerName != "creative" || statuses[1].ServerName != "survival" {
		t.Errorf("not sorted by name: %q %q", statuses[0].ServerName, statuses[1].ServerName)
	}
	if statuses[1].RestartAttempts != 2 {
		t.Errorf("restartAttempts = %d, want 2", statuses[1].RestartAttempts)
	}
	if statuses[1].CooldownEndsAt == nil {
		t.Error("cooldownEndsAt not decoded")
	}
}

func TestWatchdogStatuses_FallsBackToTheMapKeyForTheName(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/watchdog/status", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"lobby": {"isMonitoring": true}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	statuses, err := NewClient(srv.URL, "k").WatchdogStatuses(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(statuses) != 1 || statuses[0].ServerName != "lobby" {
		t.Errorf("got %+v, want the key used as the name", statuses)
	}
}

func TestCrashes_SortsNewestFirstAndPassesTheLimit(t *testing.T) {
	var gotQuery string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/watchdog/crashes", func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(`[
			{"id":1,"serverName":"lobby","detectedAt":"2026-07-30T10:00:00Z","crashType":"ProcessDeath"},
			{"id":2,"serverName":"lobby","detectedAt":"2026-07-30T11:00:00Z","crashType":"OutOfMemory"}
		]`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	crashes, err := NewClient(srv.URL, "k").Crashes(context.Background(), 25)
	if err != nil {
		t.Fatal(err)
	}
	if gotQuery != "limit=25" {
		t.Errorf("query = %q, want limit=25", gotQuery)
	}
	if len(crashes) != 2 || crashes[0].ID != 2 {
		t.Errorf("not sorted newest first: %+v", crashes)
	}
}

func TestCrashes_DefaultsANonPositiveLimit(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	if _, err := NewClient(srv.URL, "k").Crashes(context.Background(), 0); err != nil {
		t.Fatal(err)
	}
	if gotQuery != "limit=50" {
		t.Errorf("query = %q, want the default limit", gotQuery)
	}
}

func TestUnreadNotifications_AsksOnlyForActionableOnes(t *testing.T) {
	var gotQuery string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/notifications/", func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(`[
			{"id":1,"type":"info","title":"older","createdAt":"2026-07-30T10:00:00Z"},
			{"id":2,"type":"warning","title":"newer","createdAt":"2026-07-30T11:00:00Z","serverName":"lobby"}
		]`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	alerts, err := NewClient(srv.URL, "k").UnreadNotifications(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"includeRead=false", "includeDismissed=false"} {
		if !strings.Contains(gotQuery, want) {
			t.Errorf("query %q missing %q", gotQuery, want)
		}
	}
	if len(alerts) != 2 || alerts[0].ID != 2 {
		t.Errorf("not sorted newest first: %+v", alerts)
	}
	if alerts[0].ServerName == nil || *alerts[0].ServerName != "lobby" {
		t.Error("serverName not decoded")
	}
	if alerts[1].ServerName != nil {
		t.Error("a global notification should decode with a nil serverName")
	}
}

func TestHealthEndpointsRequireAnApiKey(t *testing.T) {
	c := NewClient("http://example.invalid", "")
	if _, err := c.WatchdogStatuses(context.Background()); err != ErrApiKeyMissing {
		t.Errorf("WatchdogStatuses err = %v", err)
	}
	if _, err := c.Crashes(context.Background(), 10); err != ErrApiKeyMissing {
		t.Errorf("Crashes err = %v", err)
	}
	if _, err := c.UnreadNotifications(context.Background()); err != ErrApiKeyMissing {
		t.Errorf("UnreadNotifications err = %v", err)
	}
}

func TestHealthEndpointsSurfaceAuthFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	if _, err := NewClient(srv.URL, "k").WatchdogStatuses(context.Background()); err != ErrApiKeyInvalid {
		t.Errorf("err = %v, want ErrApiKeyInvalid", err)
	}
}
