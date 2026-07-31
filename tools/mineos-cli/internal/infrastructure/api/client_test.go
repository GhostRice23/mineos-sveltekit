package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListServers_DecodesRichFieldsAndDerivesStatus(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/host/servers", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"name":"lobby","up":true,"playersOnline":3,"playersMax":20,"memoryBytes":536870912,"needsRestart":false},
			{"name":"survival","up":false,"needsRestart":true}
		]`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	servers, err := NewClient(srv.URL, "k").ListServers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 2 {
		t.Fatalf("want 2 servers, got %d", len(servers))
	}
	if servers[0].Status != "running" || servers[1].Status != "stopped" {
		t.Fatalf("status derivation wrong: %q %q", servers[0].Status, servers[1].Status)
	}
	if servers[0].PlayersOnline == nil || *servers[0].PlayersOnline != 3 {
		t.Fatal("playersOnline not decoded")
	}
	if servers[0].MemoryBytes == nil || *servers[0].MemoryBytes != 536870912 {
		t.Fatal("memoryBytes not decoded")
	}
	if !servers[1].NeedsRestart {
		t.Fatal("needsRestart not decoded")
	}
}

func TestListServers_MissingKey(t *testing.T) {
	if _, err := NewClient("http://example.invalid", "").ListServers(context.Background()); err != ErrApiKeyMissing {
		t.Fatalf("want ErrApiKeyMissing, got %v", err)
	}
}

func TestStreamPerformance_ParsesSSE(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, `data: {"isRunning":true,"cpuPercent":12.5,"ramUsedMb":512,"ramTotalMb":1024,"tps":19.9,"playerCount":2}`+"\n\n")
		_, _ = io.WriteString(w, `data: {"isRunning":true,"cpuPercent":30,"ramUsedMb":600,"ramTotalMb":1024,"tps":null,"playerCount":0}`+"\n\n")
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	samples, _ := NewClient(srv.URL, "k").StreamPerformance(ctx, "lobby")

	first, ok := <-samples
	if !ok {
		t.Fatal("expected a first sample")
	}
	if !first.IsRunning || first.CpuPercent != 12.5 || first.RamUsedMb != 512 || first.PlayerCount != 2 {
		t.Fatalf("bad first sample: %+v", first)
	}
	if first.Tps == nil || *first.Tps != 19.9 {
		t.Fatal("tps not decoded on first sample")
	}
	second, ok := <-samples
	if !ok {
		t.Fatal("expected a second sample")
	}
	if second.Tps != nil {
		t.Fatalf("expected nil tps on second sample, got %v", *second.Tps)
	}
}

func TestPerformanceHistory_DecodesSamplesOldestFirst(t *testing.T) {
	var gotQuery string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/servers/lobby/performance/history", func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"timestamp":"2026-07-30T10:00:00Z","isRunning":true,"cpuPercent":12.5,"ramUsedMb":1024,"ramTotalMb":4096,"tps":19.8,"playerCount":3},
			{"timestamp":"2026-07-30T10:01:00Z","isRunning":true,"cpuPercent":30,"ramUsedMb":2048,"ramTotalMb":4096,"tps":null,"playerCount":5}
		]`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	samples, err := NewClient(srv.URL, "k").PerformanceHistory(context.Background(), "lobby", 60)
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 2 {
		t.Fatalf("want 2 samples, got %d", len(samples))
	}
	if gotQuery != "minutes=60" {
		t.Errorf("query = %q, want minutes=60", gotQuery)
	}
	if samples[0].Tps == nil || *samples[0].Tps != 19.8 {
		t.Error("tps not decoded")
	}
	if samples[1].Tps != nil {
		t.Error("null tps should decode as nil, not 0")
	}
	if samples[0].Timestamp.IsZero() {
		t.Error("timestamp not decoded")
	}
	if samples[1].CpuPercent != 30 || samples[1].PlayerCount != 5 {
		t.Errorf("sample not decoded: %+v", samples[1])
	}
}

func TestPerformanceHistory_EscapesServerName(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// EscapedPath, not Path: net/http hands the handler the decoded form,
		// so Path would show the space back and prove nothing.
		gotPath = r.URL.EscapedPath()
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	if _, err := NewClient(srv.URL, "k").PerformanceHistory(context.Background(), "my server", 60); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/api/v1/servers/my%20server/performance/history" {
		t.Errorf("path = %q, want the name percent-encoded", gotPath)
	}
}

func TestPerformanceHistory_RequiresKeyAndName(t *testing.T) {
	if _, err := NewClient("http://example.invalid", "").PerformanceHistory(context.Background(), "lobby", 60); err != ErrApiKeyMissing {
		t.Errorf("err = %v, want ErrApiKeyMissing", err)
	}
	if _, err := NewClient("http://example.invalid", "k").PerformanceHistory(context.Background(), "", 60); err == nil {
		t.Error("expected an error for an empty server name")
	}
}

func TestPerformanceHistory_SurfacesAuthFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	if _, err := NewClient(srv.URL, "k").PerformanceHistory(context.Background(), "lobby", 60); err != ErrApiKeyInvalid {
		t.Errorf("err = %v, want ErrApiKeyInvalid", err)
	}
}
