package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildTouchWindowsScriptRejectsMissingPath(t *testing.T) {
	_, _, err := buildTouchWindowsScript(nil)
	if err == nil || !strings.Contains(err.Error(), "requires at least one path") {
		t.Fatalf("error = %v, want missing-path error", err)
	}
}

func TestBuildTouchWindowsScriptUsesNonTruncatingLiteralCreation(t *testing.T) {
	script, display, err := buildTouchWindowsScript([]resolvedArg{{Value: "a'b[1].txt"}})
	if err != nil {
		t.Fatalf("buildTouchWindowsScript: %v", err)
	}
	if len(display) != 1 || display[0] != "a'b[1].txt" {
		t.Fatalf("display paths = %v", display)
	}
	for _, want := range []string{"'a''b[1].txt'", "-LiteralPath", "FileMode]::OpenOrCreate"} {
		if !strings.Contains(script, want) {
			t.Fatalf("script missing %q: %s", want, script)
		}
	}
	for _, forbidden := range []string{"WriteAllBytes", "New-Item"} {
		if strings.Contains(script, forbidden) {
			t.Fatalf("script contains truncating/provider-dependent creation %q: %s", forbidden, script)
		}
	}
}

func TestWindowsTouchPreservesExistingContentAndCreatesLiteralPath(t *testing.T) {
	useWindowsPowerShellFallback(t)
	dir := t.TempDir()
	existing := filepath.Join(dir, "existing[1].txt")
	created := filepath.Join(dir, "created[1].txt")
	if err := os.WriteFile(existing, []byte("keep me"), 0644); err != nil {
		t.Fatal(err)
	}

	script, _, err := buildTouchWindowsScript([]resolvedArg{
		{Value: "existing[1].txt"},
		{Value: "created[1].txt"},
	})
	if err != nil {
		t.Fatal(err)
	}
	shellPath, shellArgs, err := shellPathFor("pwsh", true)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(shellPath, append(shellArgs, script)...)
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("touch script failed: %v: %s", err, output)
	}
	data, err := os.ReadFile(existing)
	if err != nil || string(data) != "keep me" {
		t.Fatalf("existing content changed: %q, %v", data, err)
	}
	if _, err := os.Stat(created); err != nil {
		t.Fatalf("literal path was not created: %v", err)
	}
}
