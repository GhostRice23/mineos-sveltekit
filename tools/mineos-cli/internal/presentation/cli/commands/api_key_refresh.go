package commands

import (
	"errors"
	"path/filepath"
	"strings"

	"github.com/freemancraft/mineos-sveltekit/tools/mineos-cli/internal/domain/config"
)

// errNoApiKeySource is returned when .env carries nothing the management key
// can be rebuilt from. There is deliberately no fallback to the database: the
// API stores only a SHA-256 of each key, so a key that is not in .env is not
// recoverable from anywhere, by design.
var errNoApiKeySource = errors.New(
	"no API key found in .env to refresh from.\n" +
		"MINEOS_API_KEY is rebuilt from ApiKey__StaticKey or ApiKey__SeedKey in the same file.\n" +
		"The database cannot help: keys are stored hashed, so the value cannot be read back.\n" +
		"Set ApiKey__StaticKey in .env and restart, or issue a new key from the web UI")

// refreshApiKeyFromEnv rewrites MINEOS_API_KEY from the key material already in
// .env and reports which entry it used.
//
// This used to read `SELECT Key FROM ApiKeys` out of the sqlite database. That
// worked only because the API stored keys in plaintext; now that it stores a
// hash, the query would return a hash, and writing that into MINEOS_API_KEY
// would produce a CLI that fails to authenticate with no obvious reason why.
func refreshApiKeyFromEnv(cfg config.Config) (key string, source string, err error) {
	// Static first, matching config.EffectiveApiKey's precedence: a static key
	// is accepted by the API without touching the database at all, so it is the
	// more reliable of the two.
	if key = strings.TrimSpace(cfg.ApiKeyStatic); key != "" {
		source = "ApiKey__StaticKey"
	} else if key = strings.TrimSpace(cfg.ApiKeySeed); key != "" {
		source = "ApiKey__SeedKey"
	} else {
		return "", "", errNoApiKeySource
	}

	envPath := resolveEnvPath(cfg.EnvPath)
	if err := setEnvFileValue(envPath, "MINEOS_API_KEY", key); err != nil {
		return "", "", err
	}

	return key, source, nil
}

func resolveEnvPath(path string) string {
	envPath := strings.TrimSpace(path)
	if envPath == "" {
		envPath = ".env"
	}
	envPath = filepath.Clean(envPath)
	if filepath.IsAbs(envPath) {
		return envPath
	}
	abs, err := filepath.Abs(envPath)
	if err != nil {
		return envPath
	}
	return abs
}
