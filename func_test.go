package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ============================================================================
// Function mode: write_file / copy_file IO tests
// ============================================================================

func TestFnWriteFile_Basic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")

	res := fnWriteFile([]string{path, "hello world"}, &metaConfig{})

	if res == nil || !res.OK {
		t.Fatalf("write_file failed: %+v", res)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "hello world" {
		t.Errorf("file content = %q, want %q", data, "hello world")
	}
}

func TestFnWriteFile_MissingArgs(t *testing.T) {
	res := fnWriteFile([]string{"only-one-arg"}, &metaConfig{})
	if res == nil || res.OK {
		t.Errorf("write_file with 1 arg should fail, got %+v", res)
	}
	if res != nil && res.ExitCode != 2 {
		t.Errorf("exit_code = %d, want 2", res.ExitCode)
	}
}

func TestFnCopyFile_Basic(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")

	if err := os.WriteFile(src, []byte("copy me"), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	res := fnCopyFile([]string{src, dst}, &metaConfig{})

	if res == nil || !res.OK {
		t.Fatalf("copy_file failed: %+v", res)
	}
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("ReadFile dst: %v", err)
	}
	if string(data) != "copy me" {
		t.Errorf("dst content = %q, want %q", data, "copy me")
	}
}

func TestFnCopyFile_MissingSrc(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "dst.txt")
	res := fnCopyFile([]string{filepath.Join(dir, "nonexistent"), dst}, &metaConfig{})
	if res == nil || res.OK {
		t.Errorf("copy_file with missing src should fail, got %+v", res)
	}
}

func TestFnOpenURL_Stub(t *testing.T) {
	res := fnOpenURL([]string{"https://example.com"}, &metaConfig{})
	if res == nil || !res.OK {
		t.Fatalf("open_url stub failed: %+v", res)
	}
	if !strings.Contains(res.Stdout, "https://example.com") {
		t.Errorf("open_url stdout = %q, want URL in output", res.Stdout)
	}
}

// ============================================================================
// Stub functions exist and return results
// ============================================================================

func TestStubFunctions_ReturnResults(t *testing.T) {
	stubs := []string{"create_hermes_subagent", "run_hermes_task", "read_hermes_session"}
	for _, name := range stubs {
		fn, ok := functionRegistry[name]
		if !ok {
			t.Errorf("stub %q not registered", name)
			continue
		}
		res := fn([]string{"-arg1"}, &metaConfig{})
		if res == nil {
			t.Errorf("stub %q returned nil result", name)
			continue
		}
		if !res.OK {
			t.Errorf("stub %q returned OK=false: %+v", name, res)
		}
		if !strings.Contains(res.Stdout, "[stub]") {
			t.Errorf("stub %q stdout = %q, want [stub] marker", name, res.Stdout)
		}
	}
}

// ============================================================================
// Meta parsing
// ============================================================================

func TestParseMeta_Defaults(t *testing.T) {
	meta, rest := parseMeta([]string{"-win", "-ls"})
	if meta.json {
		t.Error("default json should be false")
	}
	if meta.cwd != "" {
		t.Errorf("default cwd = %q, want empty", meta.cwd)
	}
	// default timeout is 60s
	if meta.timeout.Seconds() != 60 {
		t.Errorf("default timeout = %v, want 60s", meta.timeout)
	}
	if len(rest) != 2 || rest[0] != "-win" || rest[1] != "-ls" {
		t.Errorf("rest = %v, want [-win -ls]", rest)
	}
}

func TestParseMeta_JsonFlag(t *testing.T) {
	meta, rest := parseMeta([]string{"--json", "-win", "-ls"})
	if !meta.json {
		t.Error("--json should set meta.json = true")
	}
	if len(rest) != 2 {
		t.Errorf("rest = %v, want 2 items", rest)
	}
}

func TestParseMeta_Cwd(t *testing.T) {
	meta, rest := parseMeta([]string{"--cwd", "/tmp", "-win", "-ls"})
	if meta.cwd != "/tmp" {
		t.Errorf("cwd = %q, want /tmp", meta.cwd)
	}
	if len(rest) != 2 {
		t.Errorf("rest = %v, want 2 items", rest)
	}
}

func TestParseMeta_Timeout(t *testing.T) {
	meta, _ := parseMeta([]string{"--timeout", "5s", "-win", "-ls"})
	if meta.timeout.Seconds() != 5 {
		t.Errorf("timeout = %v, want 5s", meta.timeout)
	}
}

func TestParseMeta_Env(t *testing.T) {
	meta, _ := parseMeta([]string{"--env", "FOO=bar", "-win", "-ls"})
	if len(meta.env) != 1 || meta.env[0] != "FOO=bar" {
		t.Errorf("env = %v, want [FOO=bar]", meta.env)
	}
}

func TestParseMeta_NoMeta(t *testing.T) {
	// no -- flags at all → all args are rest
	meta, rest := parseMeta([]string{"-win", "-ls", "-a"})
	if meta.json || meta.cwd != "" || len(meta.env) != 0 {
		t.Errorf("meta should be zero-valued: %+v", meta)
	}
	if len(rest) != 3 {
		t.Errorf("rest = %v, want 3 items", rest)
	}
}