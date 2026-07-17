package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ============================================================================
// Contract tests — verify invariants that protect agent usage
// ============================================================================

// --- Passthrough args must be COMPLETELY unchanged ---

func TestPassthrough_PreservesDashes(t *testing.T) {
	cases := []struct {
		cmd  string
		args []string
	}{
		{"git", []string{"-C", `C:\dev\repo`, "status"}},
		{"python", []string{"-m", "pytest"}},
		{"npm", []string{"--silent", "run", "build"}},
		{"rg", []string{"-n", "TODO", "."}},
		{"dotnet", []string{"--info"}},
		{"git", []string{"status", "--short"}},
	}
	for _, c := range cases {
		got := translateArgs(c.cmd, "win", c.args, false)
		if !sliceEq(got, c.args) {
			t.Errorf("translateArgs(%q, passthrough) = %v, want %v (unchanged)", c.cmd, got, c.args)
		}
	}
}

func TestPassthrough_PreservesDashes_Linux(t *testing.T) {
	got := translateArgs("git", "linux", []string{"-C", "/tmp", "status"}, false)
	want := []string{"-C", "/tmp", "status"}
	if !sliceEq(got, want) {
		t.Errorf("passthrough linux = %v, want %v", got, want)
	}
}

// --- Function mode args must NOT be stripDash'd ---

func TestFnWriteFile_PreservesDashContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")
	// content starts with "-" — must be preserved
	res := fnWriteFile([]string{path, "-hello"}, &metaConfig{})
	if res == nil || !res.OK {
		t.Fatalf("write_file failed: %+v", res)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "-hello" {
		t.Errorf("content = %q, want %q (dash must be preserved)", data, "-hello")
	}
}

func TestFnWriteFile_PreservesDashPath(t *testing.T) {
	dir := t.TempDir()
	// filename starts with "-" — must be preserved
	path := filepath.Join(dir, "-file.txt")
	res := fnWriteFile([]string{path, "content"}, &metaConfig{})
	if res == nil || !res.OK {
		t.Fatalf("write_file failed: %+v", res)
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Errorf("file %q should exist (dash in path must be preserved)", path)
	}
}

func TestFnCopyFile_PreservesDashPath(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "-src.txt")
	dst := filepath.Join(dir, "-dst.txt")
	if err := os.WriteFile(src, []byte("data"), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	res := fnCopyFile([]string{src, dst}, &metaConfig{})
	if res == nil || !res.OK {
		t.Fatalf("copy_file failed: %+v", res)
	}
	if _, err := os.Stat(dst); os.IsNotExist(err) {
		t.Errorf("dst %q should exist", dst)
	}
}

// --- mergeEnv must actually override existing keys ---

func TestMergeEnv_OverridesExisting(t *testing.T) {
	// Set a known env var, then override it
	t.Setenv("GOSHELL_TEST_VAR", "original")
	merged := mergeEnv([]string{"GOSHELL_TEST_VAR=overridden"})
	found := false
	for _, kv := range merged {
		if strings.HasPrefix(strings.ToUpper(kv), "GOSHELL_TEST_VAR=") {
			if kv == "GOSHELL_TEST_VAR=overridden" {
				found = true
			} else {
				t.Errorf("env var not overridden: %q", kv)
			}
		}
	}
	if !found {
		t.Error("GOSHELL_TEST_VAR not found in merged env")
	}
}

func TestMergeEnv_CaseInsensitiveOverride_Windows(t *testing.T) {
	// Windows env vars are case-insensitive. PATH and Path should collide.
	t.Setenv("PATH", "/original/bin")
	// Override with different case
	merged := mergeEnv([]string{"Path=/new/bin"})
	pathCount := 0
	for _, kv := range merged {
		upper := strings.ToUpper(kv)
		if strings.HasPrefix(upper, "PATH=") {
			pathCount++
			if !strings.HasSuffix(kv, "/new/bin") {
				t.Errorf("PATH not overridden case-insensitively: %q", kv)
			}
		}
	}
	if pathCount != 1 {
		t.Errorf("expected 1 PATH entry, got %d (should merge case-insensitively)", pathCount)
	}
}

func TestMergeEnv_NoExtra(t *testing.T) {
	// No --env → return os.Environ() unchanged
	merged := mergeEnv(nil)
	if len(merged) != len(os.Environ()) {
		t.Errorf("mergeEnv(nil) len=%d, want %d", len(merged), len(os.Environ()))
	}
}

// --- shQuote must prevent word-splitting ---

func TestShQuote_Basic(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"hello", "'hello'"},
		{"", "''"},
		{"hello world", "'hello world'"},
		{"it's", "'it'\\''s'"},
		{"$HOME", "'$HOME'"},
		{"path with space", "'path with space'"},
	}
	for _, c := range cases {
		if got := shQuote(c.in); got != c.want {
			t.Errorf("shQuote(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// --- Destructive operation policy ---

func TestIsDestructive_Rm(t *testing.T) {
	if !isDestructive("rm", []string{"-rf", "/tmp"}) {
		t.Error("rm should be destructive")
	}
}

func TestIsDestructive_Ls(t *testing.T) {
	if isDestructive("ls", []string{"-al"}) {
		t.Error("ls should not be destructive")
	}
}

// --- Meta flags ---

func TestParseMeta_YesFlag(t *testing.T) {
	meta, _ := parseMeta([]string{"--yes", "-win", "-rm", "-rf", "target"})
	if !meta.yes {
		t.Error("--yes should set meta.yes = true")
	}
}

func TestParseMeta_AllowWindowsPwsh(t *testing.T) {
	meta, _ := parseMeta([]string{"--allow-windows-powershell", "-win", "-ls"})
	if !meta.allowWindowsPwsh {
		t.Error("--allow-windows-powershell should set the flag")
	}
}