package commands

import (
	"fmt"
	"io"
	"strings"

	"github.com/google/uuid"

	"github.com/freemancraft/mineos-sveltekit/tools/mineos-cli/internal/infrastructure/env"
)

// envDefault defines a required env var and its default value.
// If generator is set, it's called to produce the default (e.g., UUID generation).
type envDefault struct {
	key       string
	value     string
	generator func() string
	comment   string // optional comment line to add before the key
}

// requiredEnvDefaults lists env vars that must exist in .env.
// New features should add entries here so upgrades pick them up automatically.
var requiredEnvDefaults = []envDefault{
	{key: "MINEOS_SHUTDOWN_TIMEOUT", value: "300", comment: "# Server shutdown timeout (seconds)"},
	{key: "MINEOS_TELEMETRY_ENABLED", value: "true", comment: "# Telemetry"},
	{key: "MINEOS_TELEMETRY_ENDPOINT", value: "https://mineos.net"},
	{key: "MINEOS_INSTALLATION_ID", generator: func() string { return uuid.New().String() }},
	{key: "MINEOS_CLI_PRERELEASE_UPDATES", value: "false"},
}

// ensureEnvDefaults adds any missing required env vars to the .env file.
// Returns the list of keys that were added.
func ensureEnvDefaults(envPath string, out io.Writer) ([]string, error) {
	repo := envRepo(envPath)

	values, err := repo.Values()
	if err != nil {
		return nil, err
	}

	var added []string
	for _, d := range requiredEnvDefaults {
		if _, exists := values[d.key]; exists {
			continue
		}

		val := d.value
		if d.generator != nil {
			val = d.generator()
		}

		// Comment first, then the key, so each lands directly under its heading.
		if d.comment != "" {
			if err := repo.EnsureComment(d.comment); err != nil {
				return nil, err
			}
		}
		if err := repo.Set(d.key, val); err != nil {
			return nil, err
		}
		added = append(added, d.key)
	}

	if len(added) > 0 && out != nil {
		fmt.Fprintf(out, "Added new configuration: %s\n", strings.Join(added, ", "))
	}
	return added, nil
}

// envRepo builds the single .env reader/writer for a path. All .env access in
// the CLI goes through it — see infrastructure/env for why.
func envRepo(path string) *env.DotenvRepository {
	return env.NewDotenvRepository(env.ResolvePath(path))
}

func loadEnvValues(path string) (map[string]string, error) {
	return envRepo(path).Values()
}

func setEnvFileValue(path, key, value string) error {
	return envRepo(path).Set(key, value)
}
