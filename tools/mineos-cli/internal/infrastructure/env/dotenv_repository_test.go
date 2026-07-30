package env

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/joho/godotenv"
)

func writeEnv(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("seed .env: %v", err)
	}
	return path
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

// The bug issue #134 is about: the hand-rolled writers wrote values verbatim,
// so anything godotenv treats specially came back different from what was set.
func TestSetRoundTripsValuesThroughGodotenv(t *testing.T) {
	cases := []struct {
		name  string
		value string
	}{
		{"plain", "bridge"},
		{"empty", ""},
		{"number", "5078"},
		{"url", "http://localhost:3000"},
		{"path", "/var/games/minecraft"},
		{"hash", "s3cret#value"},
		{"hash after space", "s3cret # not a comment"},
		{"spaces", "my server name"},
		{"leading space", "  padded"},
		{"trailing space", "padded  "},
		{"double quote", `say "hi"`},
		{"single quote", "it's fine"},
		{"backslash", `C:\games\minecraft`},
		{"dollar", "$HOME/servers"},
		{"braced var", "${HOME}"},
		{"backtick", "a`b`c"},
		{"bang", "hunter2!"},
		{"newline", "line1\nline2"},
		{"equals", "key=value"},
		{"base64 key", "aGVsbG8=+/word"},
		{"only whitespace", "   "},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := writeEnv(t, "API_PORT=5078\n")
			repo := NewDotenvRepository(path)

			if err := repo.Set("SECRET", tc.value); err != nil {
				t.Fatalf("Set: %v", err)
			}

			values, err := godotenv.Read(path)
			if err != nil {
				t.Fatalf("godotenv.Read after Set: %v\nfile:\n%s", err, readFile(t, path))
			}
			if got := values["SECRET"]; got != tc.value {
				t.Errorf("round trip: set %q, read back %q\nfile:\n%s", tc.value, got, readFile(t, path))
			}
			if values["API_PORT"] != "5078" {
				t.Errorf("unrelated key changed: %q", values["API_PORT"])
			}
		})
	}
}

func TestSetPreservesCommentsBlankLinesAndOrder(t *testing.T) {
	const original = `# MineOS configuration
# Ports
API_PORT=5078
WEB_PORT=3000

# Telemetry
MINEOS_TELEMETRY_ENABLED=true
`
	path := writeEnv(t, original)

	if err := NewDotenvRepository(path).Set("WEB_PORT", "8080"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	want := strings.Replace(original, "WEB_PORT=3000", "WEB_PORT=8080", 1)
	if got := readFile(t, path); got != want {
		t.Errorf("file rewritten unexpectedly:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestSetAppendsUnknownKey(t *testing.T) {
	path := writeEnv(t, "API_PORT=5078\n")

	if err := NewDotenvRepository(path).Set("NEW_KEY", "value"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	if got, want := readFile(t, path), "API_PORT=5078\nNEW_KEY=value\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSetCreatesMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")

	if err := NewDotenvRepository(path).Set("API_PORT", "5078"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	if got, want := readFile(t, path), "API_PORT=5078\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSetUsesOwnerOnlyPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX file modes are not meaningful on Windows")
	}
	// The .env holds the API key and the seeded admin password (issue #121).
	path := writeEnv(t, "API_PORT=5078\n")

	if err := NewDotenvRepository(path).Set("MINEOS_API_KEY", "topsecret"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("mode = %o, want 600", got)
	}
}

func TestSetLeavesNoTempFileBehind(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("A=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := NewDotenvRepository(path).Set("A", "2"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != ".env" {
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		t.Errorf("directory contains %v, want only .env", names)
	}
}

func TestSetRewritesEveryOccurrence(t *testing.T) {
	path := writeEnv(t, "DUPE=one\nOTHER=x\nDUPE=two\n")

	if err := NewDotenvRepository(path).Set("DUPE", "three"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	if got, want := readFile(t, path), "DUPE=three\nOTHER=x\nDUPE=three\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSetHandlesExportPrefixAndIndentation(t *testing.T) {
	path := writeEnv(t, "export API_PORT=5078\n  WEB_PORT=3000\n")
	repo := NewDotenvRepository(path)

	if err := repo.SetAll([]KeyValue{{Key: "API_PORT", Value: "6000"}, {Key: "WEB_PORT", Value: "4000"}}); err != nil {
		t.Fatalf("SetAll: %v", err)
	}

	if got, want := readFile(t, path), "export API_PORT=6000\n  WEB_PORT=4000\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSetIgnoresCommentedOutAndPartialKeyMatches(t *testing.T) {
	path := writeEnv(t, "# API_PORT=1111\nAPI_PORT_EXTRA=x\nAPI_PORT=5078\n")

	if err := NewDotenvRepository(path).Set("API_PORT", "6000"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	want := "# API_PORT=1111\nAPI_PORT_EXTRA=x\nAPI_PORT=6000\n"
	if got := readFile(t, path); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSetReplacesMultiLineQuotedValueWholesale(t *testing.T) {
	path := writeEnv(t, "MOTD=\"line one\nline two\"\nAPI_PORT=5078\n")

	if err := NewDotenvRepository(path).Set("MOTD", "short"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// The continuation line must go with the assignment it belonged to,
	// not survive as an orphan that breaks parsing.
	if got, want := readFile(t, path), "MOTD=short\nAPI_PORT=5078\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSetSkipsOverAnUnrelatedMultiLineValue(t *testing.T) {
	path := writeEnv(t, "MOTD=\"line one\nAPI_PORT=decoy\"\nAPI_PORT=5078\n")

	if err := NewDotenvRepository(path).Set("API_PORT", "6000"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// The `API_PORT=decoy` text lives inside MOTD's quoted value; only the
	// real assignment on the last line may change.
	want := "MOTD=\"line one\nAPI_PORT=decoy\"\nAPI_PORT=6000\n"
	if got := readFile(t, path); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSetPreservesCRLFLineEndings(t *testing.T) {
	path := writeEnv(t, "API_PORT=5078\r\nWEB_PORT=3000\r\n")

	if err := NewDotenvRepository(path).Set("WEB_PORT", "8080"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	if got, want := readFile(t, path), "API_PORT=5078\r\nWEB_PORT=8080\r\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSetAllAppliesEveryPair(t *testing.T) {
	path := writeEnv(t, "A=1\n")

	err := NewDotenvRepository(path).SetAll([]KeyValue{
		{Key: "A", Value: "one"},
		{Key: "B", Value: "two"},
		{Key: "C", Value: "three # not a comment"},
	})
	if err != nil {
		t.Fatalf("SetAll: %v", err)
	}

	values, err := godotenv.Read(path)
	if err != nil {
		t.Fatalf("godotenv.Read: %v", err)
	}
	for key, want := range map[string]string{"A": "one", "B": "two", "C": "three # not a comment"} {
		if values[key] != want {
			t.Errorf("%s = %q, want %q", key, values[key], want)
		}
	}
}

func TestSetAllWithNoPairsIsANoOp(t *testing.T) {
	path := writeEnv(t, "A=1\n")

	if err := NewDotenvRepository(path).SetAll(nil); err != nil {
		t.Fatalf("SetAll: %v", err)
	}

	if got, want := readFile(t, path), "A=1\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestEnsureCommentIsIdempotent(t *testing.T) {
	path := writeEnv(t, "A=1\n")
	repo := NewDotenvRepository(path)

	if err := repo.EnsureComment("# Telemetry"); err != nil {
		t.Fatalf("EnsureComment: %v", err)
	}
	first := readFile(t, path)

	if err := repo.EnsureComment("# Telemetry"); err != nil {
		t.Fatalf("EnsureComment (second): %v", err)
	}
	if got := readFile(t, path); got != first {
		t.Errorf("second call changed the file:\n%q\nvs\n%q", got, first)
	}
	if !strings.Contains(first, "# Telemetry") {
		t.Errorf("comment missing from %q", first)
	}
}

func TestFormatAssignmentKeepsSimpleValuesUnquoted(t *testing.T) {
	// Quoting everything would churn every line of an existing .env and make
	// diffs unreadable, so plain values stay bare.
	cases := map[string]string{
		"bridge":                "MINEOS_NETWORK_MODE=bridge",
		"5078":                  "MINEOS_NETWORK_MODE=5078",
		"":                      "MINEOS_NETWORK_MODE=",
		"http://localhost:3000": "MINEOS_NETWORK_MODE=http://localhost:3000",
		"/var/games/minecraft":  "MINEOS_NETWORK_MODE=/var/games/minecraft",
		"a-b_c.d":               "MINEOS_NETWORK_MODE=a-b_c.d",
		"user@example.com":      "MINEOS_NETWORK_MODE=user@example.com",
		// ';' is a shell metacharacter, so this one gets quoted — literal
		// single quotes are preferred over escaping wherever they fit.
		"Server=db;Trusted=true": `MINEOS_NETWORK_MODE='Server=db;Trusted=true'`,
		`C:\games`:               `MINEOS_NETWORK_MODE="C:\\games"`,
	}
	for value, want := range cases {
		got, err := FormatAssignment("MINEOS_NETWORK_MODE", value)
		if err != nil {
			t.Errorf("FormatAssignment(%q): %v", value, err)
			continue
		}
		if got != want {
			t.Errorf("FormatAssignment(%q) = %q, want %q", value, got, want)
		}
	}
}

func TestFormatAssignmentRejectsUnrepresentableValues(t *testing.T) {
	// godotenv 1.5.1 mis-parses its own escaping when a value ends in a quote
	// *and* contains an apostrophe, so single quoting is unavailable too.
	// Failing is the point: the alternative is writing something that reads
	// back different.
	if _, err := FormatAssignment("SECRET", `it's "quoted"`); err == nil {
		t.Fatal("expected an error for a value godotenv cannot round trip")
	}
}

func TestSetAllRejectsUnrepresentableValueWithoutTouchingTheFile(t *testing.T) {
	path := writeEnv(t, "A=1\n")

	err := NewDotenvRepository(path).SetAll([]KeyValue{
		{Key: "A", Value: "2"},
		{Key: "SECRET", Value: `it's "quoted"`},
	})
	if err == nil {
		t.Fatal("expected SetAll to fail")
	}
	if got, want := readFile(t, path), "A=1\n"; got != want {
		t.Errorf("file was modified despite the error: %q", got)
	}
}

func TestResolvePathDefaultsToDotEnv(t *testing.T) {
	if got := ResolvePath(""); got != ".env" {
		t.Errorf("ResolvePath(\"\") = %q, want .env", got)
	}
	if got := ResolvePath("   "); got != ".env" {
		t.Errorf("ResolvePath(spaces) = %q, want .env", got)
	}
	if got := ResolvePath("  /tmp/x/.env "); got != filepath.Clean("/tmp/x/.env") {
		t.Errorf("ResolvePath trimmed path = %q", got)
	}
}

func TestLoadMapsKnownKeys(t *testing.T) {
	path := writeEnv(t, strings.Join([]string{
		"API_PORT=5078",
		"ORIGIN=http://fallback:3000",
		"MINEOS_API_KEY=abc123",
		"MINEOS_NETWORK_MODE=host",
		"Data__Directory=/data",
		"",
	}, "\n"))

	cfg, err := NewDotenvRepository(path).Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.ApiPort != "5078" {
		t.Errorf("ApiPort = %q", cfg.ApiPort)
	}
	// WEB_ORIGIN_PROD is absent, so ORIGIN is the fallback.
	if cfg.WebOrigin != "http://fallback:3000" {
		t.Errorf("WebOrigin = %q", cfg.WebOrigin)
	}
	if cfg.ManagementApiKey != "abc123" {
		t.Errorf("ManagementApiKey = %q", cfg.ManagementApiKey)
	}
	if cfg.NetworkMode != "host" {
		t.Errorf("NetworkMode = %q", cfg.NetworkMode)
	}
	if cfg.DataDirectory != "/data" {
		t.Errorf("DataDirectory = %q", cfg.DataDirectory)
	}
	if cfg.EnvPath != path {
		t.Errorf("EnvPath = %q, want %q", cfg.EnvPath, path)
	}
}

func TestLoadPrefersWebOriginProd(t *testing.T) {
	path := writeEnv(t, "WEB_ORIGIN_PROD=http://prod:3000\nORIGIN=http://fallback:3000\n")

	cfg, err := NewDotenvRepository(path).Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.WebOrigin != "http://prod:3000" {
		t.Errorf("WebOrigin = %q", cfg.WebOrigin)
	}
}

func TestLoadReportsMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent.env")

	cfg, err := NewDotenvRepository(path).Load(context.Background())
	if !os.IsNotExist(err) {
		t.Fatalf("err = %v, want a not-exist error", err)
	}
	// The path is still reported so callers can offer to create it.
	if cfg.EnvPath != path {
		t.Errorf("EnvPath = %q, want %q", cfg.EnvPath, path)
	}
}
