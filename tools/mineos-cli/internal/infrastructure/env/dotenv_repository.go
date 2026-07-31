package env

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"

	"github.com/freemancraft/mineos-sveltekit/tools/mineos-cli/internal/domain/config"
)

// envFileMode is the permission the .env file is written with. It holds the
// API key and the seeded admin password, so it must not be world-readable.
const envFileMode os.FileMode = 0o600

// DotenvRepository is the single owner of .env reads and writes.
//
// Every caller goes through here. Previously three near-identical hand-rolled
// writers existed side by side (commands.setEnvFileValue, tui.writeEnvValue and
// a whole-file godotenv.Write in commands/config.go); they disagreed about
// quoting, so a value containing '#' or a space was written by one path and
// silently truncated when read back, and the godotenv.Write path additionally
// discarded every comment and blank line in the user's file.
type DotenvRepository struct {
	path string
}

func NewDotenvRepository(path string) *DotenvRepository {
	return &DotenvRepository{path: path}
}

// KeyValue is a single assignment to apply to the .env file.
type KeyValue struct {
	Key   string
	Value string
}

func (r *DotenvRepository) Load(_ context.Context) (config.Config, error) {
	cfg := config.Config{EnvPath: r.path}

	values, err := r.Values()
	if err != nil {
		return cfg, err
	}

	cfg.ApiPort = values["API_PORT"]
	cfg.WebOrigin = values["WEB_ORIGIN_PROD"]
	if cfg.WebOrigin == "" {
		cfg.WebOrigin = values["ORIGIN"]
	}
	cfg.NetworkMode = values["MINEOS_NETWORK_MODE"]
	cfg.BuildFromSource = values["MINEOS_BUILD_FROM_SOURCE"]
	cfg.ImageTag = values["MINEOS_IMAGE_TAG"]
	cfg.ApiKeySeed = values["ApiKey__SeedKey"]
	cfg.ApiKeyStatic = values["ApiKey__StaticKey"]
	cfg.ManagementApiKey = values["MINEOS_API_KEY"]
	cfg.MinecraftHost = values["PUBLIC_MINECRAFT_HOST"]
	cfg.BodySizeLimit = values["BODY_SIZE_LIMIT"]
	cfg.DatabaseType = values["DB_TYPE"]
	cfg.DatabaseConnection = values["ConnectionStrings__DefaultConnection"]
	cfg.DataDirectory = values["Data__Directory"]
	cfg.ShutdownTimeout = values["MINEOS_SHUTDOWN_TIMEOUT"]
	cfg.PreReleaseUpdates = values["MINEOS_CLI_PRERELEASE_UPDATES"]
	cfg.TelemetryEnabled = values["MINEOS_TELEMETRY_ENABLED"]
	cfg.TelemetryEndpoint = values["MINEOS_TELEMETRY_ENDPOINT"]
	cfg.InstallationID = values["MINEOS_INSTALLATION_ID"]
	cfg.TelemetryKey = values["MINEOS_TELEMETRY_KEY"]

	return cfg, nil
}

// Values returns the parsed contents of the .env file.
func (r *DotenvRepository) Values() (map[string]string, error) {
	return godotenv.Read(r.resolvedPath())
}

// Set writes a single key, creating the file if it does not exist.
func (r *DotenvRepository) Set(key, value string) error {
	return r.SetAll([]KeyValue{{Key: key, Value: value}})
}

// SetAll applies several assignments in one read/rewrite cycle.
//
// Existing lines are edited in place, so comments, ordering and any keys not
// mentioned survive untouched; unknown keys are appended. The file is replaced
// atomically at mode 0600.
func (r *DotenvRepository) SetAll(pairs []KeyValue) error {
	if len(pairs) == 0 {
		return nil
	}

	path := r.resolvedPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		data = nil
	}

	// Render everything first: a value that cannot be encoded must abort before
	// the file is touched, not halfway through.
	rendered := make([]string, len(pairs))
	for i, pair := range pairs {
		line, err := FormatAssignment(pair.Key, pair.Value)
		if err != nil {
			return err
		}
		rendered[i] = line
	}

	newline := detectNewline(string(data))
	lines := splitLines(string(data))

	for i, pair := range pairs {
		lines = applyAssignment(lines, pair.Key, rendered[i])
	}

	return writeAtomic(path, strings.Join(lines, newline)+newline)
}

// EnsureComment appends a comment (or any literal line) unless the file
// already contains it.
func (r *DotenvRepository) EnsureComment(line string) error {
	path := r.resolvedPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		data = nil
	}
	if strings.Contains(string(data), line) {
		return nil
	}

	newline := detectNewline(string(data))
	content := string(data)
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += newline
	}
	content += newline + line + newline
	return writeAtomic(path, content)
}

func (r *DotenvRepository) SetPath(path string) {
	r.path = path
}

func (r *DotenvRepository) Path() string {
	return r.path
}

func (r *DotenvRepository) resolvedPath() string {
	return ResolvePath(r.path)
}

// ResolvePath normalises an .env path, defaulting to ./.env when empty.
func ResolvePath(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ".env"
	}
	return filepath.Clean(trimmed)
}

// FormatAssignment renders a single `KEY=value` line that godotenv reads back
// as exactly `value`.
//
// Three encodings are tried in order of how readable they leave the file, and
// each is parsed back before being accepted. Verifying rather than trusting is
// not paranoia: godotenv 1.5.1 cannot parse its own `Marshal` output when a
// value ends in a quote (`K="a \""` reads back as `a "\`), so picking an
// encoding by inspection alone reintroduces exactly the silent corruption this
// replaces — `KEY=a # b` reading back as `a`, `KEY=$HOME` as HOME's value.
//
// A value no encoding survives yields an error, so the caller fails loudly
// instead of writing something that reads back different.
func FormatAssignment(key, value string) (string, error) {
	candidates := make([]string, 0, 3)

	if isBareSafe(value) {
		candidates = append(candidates, key+"="+value)
	}
	// Single quotes are literal to godotenv — no escapes to get wrong — but
	// cannot carry a quote, backslash or line break of their own.
	if !strings.ContainsAny(value, "'\\\n\r") {
		candidates = append(candidates, key+"='"+value+"'")
	}
	if marshaled, err := godotenv.Marshal(map[string]string{key: value}); err == nil {
		candidates = append(candidates, marshaled)
	}

	for _, candidate := range candidates {
		parsed, err := godotenv.Unmarshal(candidate)
		if err == nil && parsed[key] == value {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("env: value for %s cannot be represented in a .env file", key)
}

// isBareSafe reports whether a value survives being written without quotes.
// Deliberately conservative: only characters with no meaning to godotenv's
// unquoted-value path, Docker Compose interpolation or a POSIX shell.
func isBareSafe(value string) bool {
	if value == "" {
		return true
	}
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9':
			continue
		}
		if !strings.ContainsRune("_.-:/@+,=", r) {
			return false
		}
	}
	return true
}

// applyAssignment rewrites every existing assignment of key, or appends one.
//
// Rewriting all occurrences (rather than only the last, which is the one
// godotenv would return) leaves no stale duplicate behind to confuse a human
// reading the file.
func applyAssignment(lines []string, key, rendered string) []string {
	out := make([]string, 0, len(lines)+1)
	found := false

	for i := 0; i < len(lines); i++ {
		parsed, ok := parseAssignment(lines[i])
		if !ok {
			out = append(out, lines[i])
			continue
		}

		// A quoted value may continue over following physical lines; they
		// belong to this assignment and are replaced or kept along with it.
		last := i
		for state := openQuoteAfter(parsed.value); state != 0 && last+1 < len(lines); {
			last++
			state = closeQuote(lines[last], state)
		}

		if parsed.key == key {
			out = append(out, parsed.prefix+rendered)
			found = true
		} else {
			out = append(out, lines[i:last+1]...)
		}
		i = last
	}

	if !found {
		if len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
			out[len(out)-1] = rendered
		} else {
			out = append(out, rendered)
		}
	}
	return out
}

type assignment struct {
	// key assigned by the line.
	key string
	// prefix is the literal text before the key (indentation, `export `) so it
	// can be preserved when the line is rewritten.
	prefix string
	// value is everything after the '='.
	value string
}

// parseAssignment reports whether a line is a `KEY=value` assignment.
func parseAssignment(line string) (assignment, bool) {
	rest := strings.TrimLeft(line, " \t")
	prefix := line[:len(line)-len(rest)]

	if rest == "" || strings.HasPrefix(rest, "#") {
		return assignment{}, false
	}

	if after, cut := strings.CutPrefix(rest, "export"); cut && strings.HasPrefix(after, " ") {
		trimmed := strings.TrimLeft(after, " \t")
		prefix += rest[:len(rest)-len(trimmed)]
		rest = trimmed
	}

	eq := strings.IndexByte(rest, '=')
	if eq <= 0 {
		return assignment{}, false
	}

	key := strings.TrimRight(rest[:eq], " \t")
	if key == "" || !isKeyChars(key) {
		return assignment{}, false
	}
	return assignment{key: key, prefix: prefix, value: rest[eq+1:]}, true
}

func isKeyChars(key string) bool {
	for _, r := range key {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			continue
		}
		if r != '_' && r != '.' {
			return false
		}
	}
	return true
}

// openQuoteAfter returns the quote character a value leaves open at end of
// line, or 0. Only a quoted value can span lines; an unquoted one always ends
// with its line, so a stray quote inside it (or inside a trailing comment)
// never starts a continuation.
func openQuoteAfter(value string) rune {
	trimmed := strings.TrimLeft(value, " \t")
	if trimmed == "" {
		return 0
	}
	quote := rune(trimmed[0])
	if quote != '"' && quote != '\'' {
		return 0
	}
	return closeQuote(trimmed[1:], quote)
}

// closeQuote returns the quote still open after scanning s, given that quote
// was open when it started.
func closeQuote(s string, quote rune) rune {
	escaped := false
	for _, r := range s {
		if escaped {
			escaped = false
			continue
		}
		switch {
		case r == '\\' && quote == '"':
			escaped = true
		case r == quote:
			return 0
		}
	}
	return quote
}

func detectNewline(content string) string {
	if strings.Contains(content, "\r\n") {
		return "\r\n"
	}
	return "\n"
}

// splitLines splits content into lines without inventing a trailing empty one.
func splitLines(content string) []string {
	if content == "" {
		return nil
	}
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	normalized = strings.TrimSuffix(normalized, "\n")
	return strings.Split(normalized, "\n")
}

// writeAtomic replaces path via a temp file + rename so a crash or a full disk
// can never leave a half-written .env behind.
func writeAtomic(path, content string) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".env-*.tmp")
	if err != nil {
		// A read-only or missing directory leaves nothing to fall back to.
		return err
	}
	tmpName := tmp.Name()

	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}

	if err := tmp.Chmod(envFileMode); err != nil {
		cleanup()
		return err
	}
	if _, err := tmp.WriteString(content); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	// CreateTemp honours umask, and Rename keeps the temp file's mode, so
	// re-assert it on the final path.
	return os.Chmod(path, envFileMode)
}
