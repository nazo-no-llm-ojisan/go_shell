package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestTranslateEchoWindowsJoinsArguments(t *testing.T) {
	got := translateEcho("win", []string{"hello", "world"})
	if len(got) != 1 || got[0].Value != "hello world" || got[0].Raw {
		t.Fatalf("translateEcho(win) = %+v, want one user value %q", got, "hello world")
	}
}

func TestTranslateTouchWindowsConsumesEndOfOptions(t *testing.T) {
	got := translateTouch("win", []string{"--", "-important"})
	if len(got) != 1 || got[0].Value != "-important" || got[0].Raw {
		t.Fatalf("translateTouch(win) = %+v, want one user path %q", got, "-important")
	}
}

func TestTranslateTouchPOSIXPreservesEndOfOptions(t *testing.T) {
	got := translateTouch("linux", []string{"--", "-important"})
	if len(got) != 2 || got[0].Value != "--" || got[1].Value != "-important" {
		t.Fatalf("translateTouch(linux) = %+v, want separator preserved", got)
	}
}

func TestPwshArrayLiteralEscapesEveryUserValue(t *testing.T) {
	got := pwshArrayLiteral([]string{"a'b.txt", "x; $(whoami).txt"})
	want := "@('a''b.txt','x; $(whoami).txt')"
	if got != want {
		t.Fatalf("pwshArrayLiteral = %q, want %q", got, want)
	}
}

func TestWindowsMappedPathCommandsUseLiteralArrays(t *testing.T) {
	cases := []struct {
		logical  string
		concrete string
		args     []string
		want     string
	}{
		{logical: "ls", concrete: "Get-ChildItem", args: []string{"-a", "[abc].txt"}, want: "Get-ChildItem -Force -LiteralPath @('[abc].txt')"},
		{logical: "rm", concrete: "Remove-Item", args: []string{"-rf", "[abc].txt", "plain.txt"}, want: "Remove-Item -Force -Recurse -LiteralPath @('[abc].txt','plain.txt')"},
		{logical: "rmdir", concrete: "Remove-Item", args: []string{"dir[1]"}, want: "Remove-Item -LiteralPath @('dir[1]')"},
		{logical: "cat", concrete: "Get-Content", args: []string{"*.txt", "plain.txt"}, want: "Get-Content -LiteralPath @('*.txt','plain.txt')"},
		{logical: "mkdir", concrete: "New-Item -ItemType Directory", args: []string{"-p", "dir[1]", "plain"}, want: "New-Item -ItemType Directory -Force -Path @('dir[1]','plain')"},
	}

	for _, tc := range cases {
		t.Run(tc.logical, func(t *testing.T) {
			args := translateArgs(tc.logical, "win", tc.args, true)
			if got := buildPwshCommandLine(tc.concrete, args, true); got != tc.want {
				t.Fatalf("line = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestWindowsCopyMoveUseAllSourcesAndFinalDestination(t *testing.T) {
	cases := []struct {
		logical  string
		concrete string
	}{
		{logical: "cp", concrete: "Copy-Item"},
		{logical: "mv", concrete: "Move-Item"},
	}
	for _, tc := range cases {
		t.Run(tc.logical, func(t *testing.T) {
			args := translateArgs(tc.logical, "win", []string{"src[1].txt", "src'2.txt", "destination"}, true)
			got := buildPwshCommandLine(tc.concrete, args, true)
			want := tc.concrete + " -LiteralPath @('src[1].txt','src''2.txt') -Destination 'destination'"
			if got != want {
				t.Fatalf("line = %q, want %q", got, want)
			}
		})
	}
}

func TestWindowsEndOfOptionsMarkerIsNotAPath(t *testing.T) {
	args := translateArgs("rm", "win", []string{"--", "-literal"}, true)
	got := buildPwshCommandLine("Remove-Item", args, true)
	want := "Remove-Item -LiteralPath @('-literal')"
	if got != want {
		t.Fatalf("line = %q, want %q", got, want)
	}
}

func useWindowsPowerShellFallback(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "windows" {
		t.Skip("PowerShell integration test is Windows-only")
	}
	saved := execLookPath
	execLookPath = func(file string) (string, error) {
		if file == "pwsh" {
			return "", errors.New("pwsh disabled for deterministic fallback test")
		}
		return exec.LookPath(file)
	}
	t.Cleanup(func() { execLookPath = saved })
}

func executeWindowsMappedForTest(t *testing.T, logical, concrete string, rawArgs []string, cwd string) *result {
	t.Helper()
	args := translateArgs(logical, "win", rawArgs, true)
	res := newResult("win", "pwsh", buildPwshCommandLine(concrete, args, true))
	return execute(res, "pwsh", concrete, args, &metaConfig{
		cwd:              cwd,
		timeout:          10 * time.Second,
		allowWindowsPwsh: true,
	}, true)
}

func TestWindowsRemoveItemDeletesOnlyLiteralBracketPath(t *testing.T) {
	useWindowsPowerShellFallback(t)
	dir := t.TempDir()
	for _, name := range []string{"[abc].txt", "a.txt", "b.txt", "c.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0644); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
	}

	res := executeWindowsMappedForTest(t, "rm", "Remove-Item", []string{"[abc].txt"}, dir)
	if !res.OK {
		t.Fatalf("Remove-Item failed: %+v", res)
	}
	if _, err := os.Stat(filepath.Join(dir, "[abc].txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("literal target still exists or stat failed: %v", err)
	}
	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("wildcard decoy %s was removed: %v", name, err)
		}
	}
}

func TestWindowsCopyMoveSupportMultipleLiteralSources(t *testing.T) {
	useWindowsPowerShellFallback(t)
	dir := t.TempDir()
	copyDest := filepath.Join(dir, "copy-dest")
	moveDest := filepath.Join(dir, "move-dest")
	if err := os.Mkdir(copyDest, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(moveDest, 0755); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"copy[1].txt", "copy'2.txt", "move[1].txt", "move'2.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0644); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
	}

	copyRes := executeWindowsMappedForTest(t, "cp", "Copy-Item", []string{"copy[1].txt", "copy'2.txt", "copy-dest"}, dir)
	if !copyRes.OK {
		t.Fatalf("Copy-Item failed: %+v", copyRes)
	}
	moveRes := executeWindowsMappedForTest(t, "mv", "Move-Item", []string{"move[1].txt", "move'2.txt", "move-dest"}, dir)
	if !moveRes.OK {
		t.Fatalf("Move-Item failed: %+v", moveRes)
	}

	for _, path := range []string{
		filepath.Join(copyDest, "copy[1].txt"),
		filepath.Join(copyDest, "copy'2.txt"),
		filepath.Join(moveDest, "move[1].txt"),
		filepath.Join(moveDest, "move'2.txt"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected destination %s: %v", path, err)
		}
	}
}
