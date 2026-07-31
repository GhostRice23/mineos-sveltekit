package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The --remove-cli flag was declared and bound but nothing ever read it, so the
// deletion path below shipped fully written and completely unreachable. What
// surfaced it was staticcheck reporting removeCLIFromPath as unused, which is
// why that check now runs in CI.
//
// Note that staticcheck will not catch a second regression of exactly this
// shape: these tests reference the removal helpers, so the functions count as
// used even if the uninstall flow stops calling them. The tests below cover the
// deletion logic and the flag's registration -- the call site itself is only as
// safe as review.

func TestUninstallRegistersTheRemoveCLIFlag(t *testing.T) {
	cmd := NewUninstallCommand()

	flag := cmd.Flags().Lookup("remove-cli")
	if flag == nil {
		t.Fatal("uninstall no longer registers --remove-cli")
	}
	if flag.DefValue != "false" {
		t.Fatalf("--remove-cli should default to false, got %q", flag.DefValue)
	}
}

func TestMaybeRemoveCLIFromPathDoesNothingWithoutTheFlag(t *testing.T) {
	var out bytes.Buffer

	maybeRemoveCLIFromPath(&out, uninstallOptions{removeCLI: false})

	if out.Len() != 0 {
		t.Fatalf("an uninstall that was not asked to touch the CLI printed: %q", out.String())
	}
}

func TestRemoveCLIBinariesDeletesEveryInstalledCopy(t *testing.T) {
	dir := t.TempDir()
	systemBin := filepath.Join(dir, "usr-local-bin-mineos")
	userBin := filepath.Join(dir, "home-local-bin-mineos")
	for _, path := range []string{systemBin, userBin} {
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatalf("seeding %s: %v", path, err)
		}
	}

	var out bytes.Buffer
	if err := removeCLIBinaries(&out, []string{systemBin, userBin}); err != nil {
		t.Fatalf("removeCLIBinaries: %v", err)
	}

	for _, path := range []string{systemBin, userBin} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("%s survived the removal (stat error: %v)", path, err)
		}
		if !strings.Contains(out.String(), path) {
			t.Errorf("output does not name the removed copy %s: %q", path, out.String())
		}
	}
}

func TestRemoveCLIBinariesSkipsPathsThatAreNotInstalled(t *testing.T) {
	dir := t.TempDir()
	installed := filepath.Join(dir, "mineos")
	if err := os.WriteFile(installed, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("seeding %s: %v", installed, err)
	}
	missing := filepath.Join(dir, "does-not-exist", "mineos")

	var out bytes.Buffer
	// A candidate location that was never used must not turn into an error --
	// installing to one of the two locations is the normal case, not a failure.
	if err := removeCLIBinaries(&out, []string{missing, installed}); err != nil {
		t.Fatalf("a missing candidate path should not fail the removal: %v", err)
	}

	if _, err := os.Stat(installed); !os.IsNotExist(err) {
		t.Errorf("the installed copy survived (stat error: %v)", err)
	}
	if strings.Contains(out.String(), "CLI not found") {
		t.Errorf("removal reported nothing found even though it deleted a copy: %q", out.String())
	}
}

func TestRemoveCLIBinariesReportsWhenNothingIsInstalled(t *testing.T) {
	dir := t.TempDir()

	var out bytes.Buffer
	if err := removeCLIBinaries(&out, []string{filepath.Join(dir, "mineos")}); err != nil {
		t.Fatalf("an uninstall with no CLI on disk should succeed: %v", err)
	}

	if !strings.Contains(out.String(), "CLI not found") {
		t.Errorf("expected a 'not found' note, got: %q", out.String())
	}
}
