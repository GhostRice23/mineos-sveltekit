package commands

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/freemancraft/mineos-sveltekit/tools/mineos-cli/internal/application/usecases"
	"github.com/freemancraft/mineos-sveltekit/tools/mineos-cli/internal/domain/config"
	"github.com/freemancraft/mineos-sveltekit/tools/mineos-cli/internal/infrastructure/env"
)

// recorder captures compose invocations instead of running docker.
type recorder struct {
	calls [][]string
	err   error
}

func (r *recorder) runner() composeRunner {
	return composeRunner{
		exe:      "docker",
		baseArgs: []string{"compose"},
		exec: func(_ string, args []string, _ []string) error {
			r.calls = append(r.calls, args)
			return r.err
		},
	}
}

func seedEnvFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func argsOf(runner composeRunner) string {
	return strings.Join(runner.baseArgs, " ")
}

func TestComposeWithConfig_PassesTheEnvFileWhenItExists(t *testing.T) {
	envPath := seedEnvFile(t, "API_PORT=5078\n")
	base := composeRunner{exe: "docker", baseArgs: []string{"compose"}}

	got := argsOf(composeWithConfig(base, config.Config{EnvPath: envPath}))

	if !strings.Contains(got, "--env-file "+envPath) {
		t.Errorf("args %q missing --env-file %s", got, envPath)
	}
}

func TestComposeWithConfig_OmitsTheEnvFileWhenItIsMissing(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent.env")
	base := composeRunner{exe: "docker", baseArgs: []string{"compose"}}

	got := argsOf(composeWithConfig(base, config.Config{EnvPath: missing}))

	// Passing --env-file for a file that is not there makes compose fail
	// outright, which is worse than falling back to its own defaults.
	if strings.Contains(got, "--env-file") {
		t.Errorf("args %q should not reference a missing env file", got)
	}
}

func TestComposeWithConfig_SelectsOverlayFiles(t *testing.T) {
	envPath := seedEnvFile(t, "")
	dir := filepath.Dir(envPath)
	base := composeRunner{exe: "docker", baseArgs: []string{"compose"}}

	cases := []struct {
		name    string
		cfg     config.Config
		want    []string
		notWant []string
	}{
		{
			name:    "defaults to the base file only",
			cfg:     config.Config{EnvPath: envPath},
			want:    []string{filepath.Join(dir, "docker-compose.yml")},
			notWant: []string{"docker-compose.host.yml", "docker-compose.build.yml"},
		},
		{
			name:    "host networking adds the host overlay",
			cfg:     config.Config{EnvPath: envPath, NetworkMode: "host"},
			want:    []string{filepath.Join(dir, "docker-compose.host.yml")},
			notWant: []string{"docker-compose.build.yml"},
		},
		{
			name:    "host networking is matched case-insensitively",
			cfg:     config.Config{EnvPath: envPath, NetworkMode: "  HOST "},
			want:    []string{filepath.Join(dir, "docker-compose.host.yml")},
			notWant: nil,
		},
		{
			name:    "build from source adds the build overlay",
			cfg:     config.Config{EnvPath: envPath, BuildFromSource: "true"},
			want:    []string{filepath.Join(dir, "docker-compose.build.yml")},
			notWant: []string{"docker-compose.host.yml"},
		},
		{
			name: "both overlays can apply at once",
			cfg:  config.Config{EnvPath: envPath, NetworkMode: "host", BuildFromSource: "true"},
			want: []string{
				filepath.Join(dir, "docker-compose.host.yml"),
				filepath.Join(dir, "docker-compose.build.yml"),
			},
		},
		{
			name:    "a non-boolean build flag is not treated as true",
			cfg:     config.Config{EnvPath: envPath, BuildFromSource: "maybe"},
			notWant: []string{"docker-compose.build.yml"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := argsOf(composeWithConfig(base, tc.cfg))
			for _, want := range tc.want {
				if !strings.Contains(got, "-f "+want) {
					t.Errorf("args %q missing -f %s", got, want)
				}
			}
			for _, notWant := range tc.notWant {
				if strings.Contains(got, notWant) {
					t.Errorf("args %q should not include %s", got, notWant)
				}
			}
		})
	}
}

func TestComposeWithConfig_OrdersTheBaseFileFirst(t *testing.T) {
	envPath := seedEnvFile(t, "")
	base := composeRunner{exe: "docker", baseArgs: []string{"compose"}}

	got := argsOf(composeWithConfig(base, config.Config{EnvPath: envPath, NetworkMode: "host"}))

	// Compose merges later -f files over earlier ones, so the overlay has to
	// come after the base file or it has no effect.
	baseIdx := strings.Index(got, "docker-compose.yml")
	hostIdx := strings.Index(got, "docker-compose.host.yml")
	if baseIdx < 0 || hostIdx < 0 || baseIdx > hostIdx {
		t.Errorf("overlay must follow the base file, got %q", got)
	}
}

func TestComposeWithConfig_DoesNotMutateTheBaseRunner(t *testing.T) {
	envPath := seedEnvFile(t, "")
	base := composeRunner{exe: "docker", baseArgs: []string{"compose"}}

	composeWithConfig(base, config.Config{EnvPath: envPath, NetworkMode: "host"})

	if len(base.baseArgs) != 1 || base.baseArgs[0] != "compose" {
		t.Errorf("base runner was mutated: %v", base.baseArgs)
	}
}

func TestComposeWithConfig_KeepsTheInjectedExecutor(t *testing.T) {
	envPath := seedEnvFile(t, "")
	rec := &recorder{}

	derived := composeWithConfig(rec.runner(), config.Config{EnvPath: envPath})
	if err := derived.run([]string{"ps"}); err != nil {
		t.Fatal(err)
	}

	if len(rec.calls) != 1 {
		t.Fatalf("got %d calls, want 1", len(rec.calls))
	}
	if last := rec.calls[0][len(rec.calls[0])-1]; last != "ps" {
		t.Errorf("last arg = %q, want ps", last)
	}
}

func TestParseBool(t *testing.T) {
	for _, truthy := range []string{"true", "TRUE", " 1 ", "t"} {
		if !parseBool(truthy) {
			t.Errorf("parseBool(%q) = false, want true", truthy)
		}
	}
	for _, falsy := range []string{"", "false", "0", "maybe", "yes"} {
		if parseBool(falsy) {
			t.Errorf("parseBool(%q) = true, want false", falsy)
		}
	}
}

func TestFileExists(t *testing.T) {
	path := seedEnvFile(t, "x")
	if !fileExists(path) {
		t.Error("an existing file should report true")
	}
	if fileExists(filepath.Dir(path)) {
		t.Error("a directory is not a file")
	}
	if fileExists("") {
		t.Error("an empty path is not a file")
	}
}

// loadConfigFor builds a use case over a throwaway .env, so gracefulStop can
// run without touching the caller's working directory.
func loadConfigFor(t *testing.T, envPath string) *usecases.LoadConfigUseCase {
	t.Helper()
	return usecases.NewLoadConfigUseCase(env.NewDotenvRepository(envPath))
}

func TestGracefulStop_UsesAShortDockerTimeoutAfterStoppingServers(t *testing.T) {
	// No API key in the .env, so the server-stop step fails fast and locally.
	envPath := seedEnvFile(t, "API_PORT=5078\n")
	rec := &recorder{}
	var out bytes.Buffer

	err := gracefulStop(context.Background(), loadConfigFor(t, envPath), rec.runner(),
		config.Config{EnvPath: envPath}, 300, false, &out)
	if err != nil {
		t.Fatalf("gracefulStop: %v", err)
	}

	if len(rec.calls) != 1 {
		t.Fatalf("got %d compose calls, want 1: %v", len(rec.calls), rec.calls)
	}
	got := strings.Join(rec.calls[0], " ")
	// Minecraft servers are stopped through the API first, so the containers
	// only need a short grace period.
	if !strings.HasSuffix(got, "stop -t 30") {
		t.Errorf("compose args = %q, want a graceful stop with -t 30", got)
	}
}

func TestGracefulStop_ForceStopsImmediately(t *testing.T) {
	envPath := seedEnvFile(t, "API_PORT=5078\n")
	rec := &recorder{}
	var out bytes.Buffer

	err := gracefulStop(context.Background(), loadConfigFor(t, envPath), rec.runner(),
		config.Config{EnvPath: envPath}, 300, true, &out)
	if err != nil {
		t.Fatalf("gracefulStop: %v", err)
	}

	got := strings.Join(rec.calls[0], " ")
	if !strings.HasSuffix(got, "stop -t 0") {
		t.Errorf("compose args = %q, want an immediate stop", got)
	}
	if !strings.Contains(out.String(), "Force stop") {
		t.Errorf("force stop should be announced, got %q", out.String())
	}
}

func TestGracefulStop_ContinuesWhenTheApiIsUnavailable(t *testing.T) {
	// The whole point of the warning path: an unreachable API must not leave
	// the containers running.
	envPath := seedEnvFile(t, "API_PORT=5078\n")
	rec := &recorder{}
	var out bytes.Buffer

	if err := gracefulStop(context.Background(), loadConfigFor(t, envPath), rec.runner(),
		config.Config{EnvPath: envPath}, 300, false, &out); err != nil {
		t.Fatalf("gracefulStop: %v", err)
	}

	if len(rec.calls) == 0 {
		t.Fatal("containers were never stopped")
	}
	if !strings.Contains(out.String(), "Warning") {
		t.Errorf("expected a warning about the API, got %q", out.String())
	}
}

func TestGracefulStop_ReportsAComposeFailure(t *testing.T) {
	envPath := seedEnvFile(t, "API_PORT=5078\n")
	rec := &recorder{err: os.ErrPermission}
	var out bytes.Buffer

	err := gracefulStop(context.Background(), loadConfigFor(t, envPath), rec.runner(),
		config.Config{EnvPath: envPath}, 300, false, &out)

	if err == nil {
		t.Fatal("a failing compose stop must be reported, not swallowed")
	}
}
