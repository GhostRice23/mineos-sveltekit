package commands

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/freemancraft/mineos-sveltekit/tools/mineos-cli/internal/domain/config"
)

// refresh used to read `SELECT Key FROM ApiKeys` out of the sqlite database.
// That only worked while the API stored keys in plaintext; it stores a SHA-256
// now, so the query would hand back a hash and MINEOS_API_KEY would be quietly
// set to something that cannot authenticate. The key is rebuilt from .env
// instead, and there is deliberately no database fallback.

func envFile(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("seeding %s: %v", path, err)
	}
	return path
}

func readEnvValue(t *testing.T, path, key string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	for _, line := range strings.Split(string(raw), "\n") {
		name, value, found := strings.Cut(strings.TrimSpace(line), "=")
		if found && name == key {
			return strings.Trim(strings.TrimSpace(value), `"'`)
		}
	}
	return ""
}

func TestRefreshPrefersTheStaticKey(t *testing.T) {
	// Matches config.EffectiveApiKey's precedence: a static key is accepted by
	// the API without a database lookup at all, so it is the more reliable one.
	path := envFile(t, "MINEOS_API_KEY=stale\n")
	cfg := config.Config{EnvPath: path, ApiKeyStatic: "static-key", ApiKeySeed: "seed-key"}

	key, source, err := refreshApiKeyFromEnv(cfg)
	if err != nil {
		t.Fatalf("refreshApiKeyFromEnv: %v", err)
	}
	if key != "static-key" {
		t.Errorf("key = %q, want static-key", key)
	}
	if source != "ApiKey__StaticKey" {
		t.Errorf("source = %q, want ApiKey__StaticKey", source)
	}
	if got := readEnvValue(t, path, "MINEOS_API_KEY"); got != "static-key" {
		t.Errorf("MINEOS_API_KEY in .env = %q, want static-key", got)
	}
}

func TestRefreshFallsBackToTheSeedKey(t *testing.T) {
	path := envFile(t, "MINEOS_API_KEY=stale\n")
	cfg := config.Config{EnvPath: path, ApiKeySeed: "seed-key"}

	key, source, err := refreshApiKeyFromEnv(cfg)
	if err != nil {
		t.Fatalf("refreshApiKeyFromEnv: %v", err)
	}
	if key != "seed-key" || source != "ApiKey__SeedKey" {
		t.Errorf("got (%q, %q), want (seed-key, ApiKey__SeedKey)", key, source)
	}
	if got := readEnvValue(t, path, "MINEOS_API_KEY"); got != "seed-key" {
		t.Errorf("MINEOS_API_KEY in .env = %q, want seed-key", got)
	}
}

func TestRefreshWritesTheKeyEvenWhenTheEntryIsAbsent(t *testing.T) {
	// The reason to run refresh at all is usually that MINEOS_API_KEY is gone.
	path := envFile(t, "ApiKey__SeedKey=seed-key\n")
	cfg := config.Config{EnvPath: path, ApiKeySeed: "seed-key"}

	if _, _, err := refreshApiKeyFromEnv(cfg); err != nil {
		t.Fatalf("refreshApiKeyFromEnv: %v", err)
	}
	if got := readEnvValue(t, path, "MINEOS_API_KEY"); got != "seed-key" {
		t.Errorf("MINEOS_API_KEY in .env = %q, want seed-key", got)
	}
}

func TestRefreshFailsWhenThereIsNothingToRefreshFrom(t *testing.T) {
	path := envFile(t, "MINEOS_API_KEY=stale\n")
	cfg := config.Config{EnvPath: path}

	_, _, err := refreshApiKeyFromEnv(cfg)
	if !errors.Is(err, errNoApiKeySource) {
		t.Fatalf("err = %v, want errNoApiKeySource", err)
	}
	// The message has to say why the database is not consulted, or the next
	// person will "fix" this by reading the key back out of it.
	if !strings.Contains(err.Error(), "hashed") {
		t.Errorf("error should explain that stored keys are hashed, got: %v", err)
	}
	// A failed refresh must not clobber whatever was already there.
	if got := readEnvValue(t, path, "MINEOS_API_KEY"); got != "stale" {
		t.Errorf("MINEOS_API_KEY = %q, want it left untouched", got)
	}
}

func TestRefreshIgnoresWhitespaceOnlyKeys(t *testing.T) {
	path := envFile(t, "MINEOS_API_KEY=stale\n")
	cfg := config.Config{EnvPath: path, ApiKeyStatic: "   ", ApiKeySeed: "seed-key"}

	key, source, err := refreshApiKeyFromEnv(cfg)
	if err != nil {
		t.Fatalf("refreshApiKeyFromEnv: %v", err)
	}
	if key != "seed-key" || source != "ApiKey__SeedKey" {
		t.Errorf("got (%q, %q), want (seed-key, ApiKey__SeedKey)", key, source)
	}
}
