package commands

import (
	"bytes"
	"io"
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
// Staticcheck alone cannot catch a second regression of this shape, though:
// these tests reference the removal helpers, so the functions count as used
// even if the uninstall flow stops calling them. That is what the call-site
// tests at the bottom of this file are for -- they drive runUninstall itself,
// and deleting either maybeRemoveCLIFromPath call turns them red.

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

// The tests above cover the deletion logic. These cover the call site, which is
// what actually regressed: --remove-cli was declared, bound, and never read.
// They drive runUninstall itself, with docker, compose and the remover all
// stubbed, so removing the maybeRemoveCLIFromPath call turns them red.

func stubUninstallDeps(t *testing.T) *bool {
	t.Helper()
	called := false

	origLookPath, origDetector, origRemover := dockerLookPath, composeDetector, cliPathRemover
	origData, origInstallDir := localDataRemover, installDirRemover
	t.Cleanup(func() {
		dockerLookPath, composeDetector, cliPathRemover = origLookPath, origDetector, origRemover
		localDataRemover, installDirRemover = origData, origInstallDir
	})

	dockerLookPath = func() error { return nil }
	composeDetector = func() (composeRunner, error) {
		return composeRunner{
			exe:  "docker",
			exec: func(_ string, _ []string, _ []string) error { return nil },
		}, nil
	}
	cliPathRemover = func(io.Writer) error {
		called = true
		return nil
	}
	localDataRemover = func(io.Writer) error { return nil }
	installDirRemover = func(io.Writer) error { return nil }

	// reportUninstallTelemetry reads ./.env; an empty dir makes it a no-op.
	t.Chdir(t.TempDir())
	return &called
}

func runUninstallForTest(t *testing.T, opts uninstallOptions) (string, error) {
	t.Helper()
	var out bytes.Buffer
	cmd := NewUninstallCommand()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	err := runUninstall(cmd, opts)
	return out.String(), err
}

func TestUninstallRemovesTheCLIWhenTheFlagIsSet(t *testing.T) {
	called := stubUninstallDeps(t)

	if _, err := runUninstallForTest(t, uninstallOptions{mode: "containers", removeCLI: true}); err != nil {
		t.Fatalf("runUninstall: %v", err)
	}

	if !*called {
		t.Fatal("--remove-cli was set but the uninstall never removed the CLI")
	}
}

func TestUninstallLeavesTheCLIAloneWithoutTheFlag(t *testing.T) {
	called := stubUninstallDeps(t)

	if _, err := runUninstallForTest(t, uninstallOptions{mode: "containers"}); err != nil {
		t.Fatalf("runUninstall: %v", err)
	}

	if *called {
		t.Fatal("the CLI was removed even though --remove-cli was not set")
	}
}

func TestCompleteUninstallAlsoHonoursTheFlag(t *testing.T) {
	// "complete" returns early on its own path, so it needs its own call.
	called := stubUninstallDeps(t)

	out, err := runUninstallForTest(t, uninstallOptions{mode: "complete", skipConfirm: true, removeCLI: true})
	if err != nil {
		t.Fatalf("runUninstall: %v", err)
	}

	if !*called {
		t.Fatal("--remove-cli was set but the complete uninstall never removed the CLI")
	}
	if !strings.Contains(out, "completely removed from your system") {
		t.Errorf("complete uninstall should claim a full removal once the CLI is gone, got: %q", out)
	}
}

func TestCompleteUninstallSaysTheCLIRemains(t *testing.T) {
	// Without the flag the binary stays, so the closing message must not claim
	// MineOS was completely removed.
	stubUninstallDeps(t)

	out, err := runUninstallForTest(t, uninstallOptions{mode: "complete", skipConfirm: true})
	if err != nil {
		t.Fatalf("runUninstall: %v", err)
	}

	if strings.Contains(out, "completely removed from your system") {
		t.Errorf("the CLI is still installed, so this must not claim a full removal: %q", out)
	}
	if !strings.Contains(out, "--remove-cli") {
		t.Errorf("output should point at --remove-cli, got: %q", out)
	}
}
