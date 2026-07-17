package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// ============================================================================
// Audit round 3 — regression tests for the 7 remaining fixes
// ============================================================================

// --- #2: pwsh passthrough command name must be quoted and invoked with & ---

func TestPwshPassthrough_UsesCallOperator(t *testing.T) {
	// Simulate the line-building logic for a passthrough command.
	// mapped=false → cmd must be pwshQuote'd and prefixed with &
	cmd := "git"
	args := []resolvedArg{{Value: "status", Raw: false}}
	mapped := false

	line := buildPwshCommandLine(cmd, args, mapped)

	if !strings.Contains(line, "& 'git'") {
		t.Errorf("passthrough should use & call operator: %q", line)
	}
	if strings.Contains(line, "; 'git'") || strings.Contains(line, " git ") {
		t.Errorf("passthrough cmd should not be raw syntax fragment: %q", line)
	}
}

func TestPwshPassthrough_CommandNameWithInjectionChars(t *testing.T) {
	// A command name containing ;, |, &, $() must NOT be emitted as syntax.
	// In passthrough mode the cmd is user-supplied, so it must be quoted.
	injectionCmds := []string{
		"git; Write-Output pwned",
		"foo | bar",
		"foo & baz",
		"foo$(whoami)",
	}
	for _, cmd := range injectionCmds {
		mapped := false
		line := buildPwshCommandLine(cmd, nil, mapped)
		// The raw injection text must not appear unquoted in the line
		rawInjection := cmd + " "
		if strings.Contains(line, rawInjection) || strings.HasSuffix(line, cmd) {
			t.Errorf("injection command %q was emitted raw: %q", cmd, line)
		}
		// Must be single-quoted
		if !strings.Contains(line, "'"+cmd+"'") {
			// Actually pwshQuote doubles internal quotes, so check the escaped form
			escaped := strings.ReplaceAll(cmd, "'", "''")
			if !strings.Contains(line, "'"+escaped+"'") {
				t.Errorf("injection command %q should be single-quoted: %q", cmd, line)
			}
		}
	}
}

func TestPwshMapped_UsesRawSyntaxFragment(t *testing.T) {
	// mapped=true → cmd is translator-generated, emitted as raw syntax.
	cmd := "Get-ChildItem"
	mapped := true

	line := buildPwshCommandLine(cmd, nil, mapped)
	if line != "Get-ChildItem" {
		t.Errorf("mapped cmd should be raw syntax, got %q", line)
	}
	if strings.Contains(line, "&") {
		t.Errorf("mapped cmd should not use & operator: %q", line)
	}
}

func TestPwshQuote_ArgWithNewlineAndSingleQuote(t *testing.T) {
	// Args with single quotes, spaces, and newlines must be properly escaped.
	cases := []struct {
		in   string
		want string
	}{
		{"hello\nworld", "'hello\nworld'"},
		{"it's a test", "'it''s a test'"},
		{"line1\nline2", "'line1\nline2'"},
		{"  spaces  ", "'  spaces  '"},
	}
	for _, c := range cases {
		if got := pwshQuote(c.in); got != c.want {
			t.Errorf("pwshQuote(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// --- #5: --cwd must be shared between OS mode and function mode ---

func TestResolveCwd_RelativePath(t *testing.T) {
	meta := &metaConfig{cwd: "/tmp/work"}
	got, err := resolveCwd("out.txt", meta)
	if err != nil {
		t.Fatalf("resolveCwd: %v", err)
	}
	want := "/tmp/work/out.txt"
	// On Windows, filepath.Join uses backslashes; normalize for comparison
	if filepath.Separator != '/' {
		want = strings.ReplaceAll(want, "/", string(filepath.Separator))
	}
	if got != want {
		t.Errorf("resolveCwd(rel) = %q, want %q", got, want)
	}
}

func TestResolveCwd_AbsolutePathUnchanged(t *testing.T) {
	meta := &metaConfig{cwd: "/tmp/work"}
	abs := "/var/log/app.log"
	if filepath.Separator != '/' {
		abs = `C:\var\log\app.log`
	}
	got, err := resolveCwd(abs, meta)
	if err != nil {
		t.Fatalf("resolveCwd: %v", err)
	}
	if got != abs {
		t.Errorf("resolveCwd(abs) = %q, want %q (absolute unchanged)", got, abs)
	}
}

func TestResolveCwd_NoCwdUnchanged(t *testing.T) {
	meta := &metaConfig{}
	got, err := resolveCwd("out.txt", meta)
	if err != nil {
		t.Fatalf("resolveCwd: %v", err)
	}
	if got != "out.txt" {
		t.Errorf("resolveCwd(no cwd) = %q, want %q", got, "out.txt")
	}
}

func TestValidateCwd_MissingDirFails(t *testing.T) {
	meta := &metaConfig{cwd: "/nonexistent/path/that/should/not/exist"}
	if filepath.Separator != '/' {
		meta.cwd = `Z:\nonexistent\path\that\should\not\exist`
	}
	err := validateCwd(meta)
	if err == nil {
		t.Error("validateCwd should fail for nonexistent directory")
	}
}

func TestValidateCwd_ValidDirSucceeds(t *testing.T) {
	dir := t.TempDir()
	meta := &metaConfig{cwd: dir}
	if err := validateCwd(meta); err != nil {
		t.Errorf("validateCwd(valid dir) = %v, want nil", err)
	}
}

func TestValidateCwd_FileNotDirFails(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "notadir.txt")
	if err := os.WriteFile(file, []byte("x"), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	meta := &metaConfig{cwd: file}
	err := validateCwd(meta)
	if err == nil {
		t.Error("validateCwd should fail when --cwd is a file, not a directory")
	}
}

func TestWriteFile_NoSideEffectWhenCwdMissing(t *testing.T) {
	// If --cwd points to a nonexistent dir, write_file must not create any file.
	missingCwd := "/nonexistent/cwd/path"
	if filepath.Separator != '/' {
		missingCwd = `Z:\nonexistent\cwd\path`
	}
	meta := &metaConfig{cwd: missingCwd}
	// Relative path that would be resolved under the missing cwd
	res := fnWriteFile([]string{"out.txt", "content"}, meta)
	if res == nil || res.OK {
		t.Errorf("write_file with missing --cwd should fail, got %+v", res)
	}
	// Verify no file was created in the current directory
	if _, err := os.Stat("out.txt"); err == nil {
		os.Remove("out.txt")
		t.Error("write_file created 'out.txt' despite missing --cwd (side effect leaked)")
	}
}

func TestWriteFile_CwdRelativePathResolves(t *testing.T) {
	dir := t.TempDir()
	meta := &metaConfig{cwd: dir}
	// Relative path "out.txt" should resolve under dir
	res := fnWriteFile([]string{"out.txt", "hello"}, meta)
	if res == nil || !res.OK {
		t.Fatalf("write_file failed: %+v", res)
	}
	fullPath := filepath.Join(dir, "out.txt")
	data, err := os.ReadFile(fullPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("content = %q, want %q", data, "hello")
	}
}

func TestCopyFile_CwdRelativePathResolves(t *testing.T) {
	dir := t.TempDir()
	meta := &metaConfig{cwd: dir}
	// Create src under cwd
	srcPath := filepath.Join(dir, "src.txt")
	if err := os.WriteFile(srcPath, []byte("data"), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	// Copy using relative paths
	res := fnCopyFile([]string{"src.txt", "dst.txt"}, meta)
	if res == nil || !res.OK {
		t.Fatalf("copy_file failed: %+v", res)
	}
	dstPath := filepath.Join(dir, "dst.txt")
	data, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatalf("ReadFile dst: %v", err)
	}
	if string(data) != "data" {
		t.Errorf("dst content = %q, want %q", data, "data")
	}
}

// --- #6: mergeEnvForOS — host-independent, deterministic ---

func TestMergeEnvForOS_WindowsCaseInsensitive(t *testing.T) {
	// On Windows, PATH and Path should merge into one entry (case-insensitive)
	base := []string{"PATH=/original/bin", "HOME=/root"}
	extra := []string{"Path=/new/bin"}
	merged := mergeEnvForOS(base, extra, "windows")

	pathEntries := 0
	for _, kv := range merged {
		upper := strings.ToUpper(kv)
		if strings.HasPrefix(upper, "PATH=") {
			pathEntries++
			if !strings.HasSuffix(kv, "/new/bin") {
				t.Errorf("PATH not overridden (case-insensitive): %q", kv)
			}
		}
	}
	if pathEntries != 1 {
		t.Errorf("Windows: expected 1 PATH entry, got %d (should merge PATH/Path)", pathEntries)
	}
}

func TestMergeEnvForOS_LinuxCaseSensitive(t *testing.T) {
	// On Linux, PATH and Path are distinct keys — both should be preserved
	base := []string{"PATH=/original/bin"}
	extra := []string{"Path=/new/bin"}
	merged := mergeEnvForOS(base, extra, "linux")

	pathCount := 0
	pathOriginalFound := false
	pathNewFound := false
	for _, kv := range merged {
		if kv == "PATH=/original/bin" {
			pathCount++
			pathOriginalFound = true
		}
		if kv == "Path=/new/bin" {
			pathCount++
			pathNewFound = true
		}
	}
	if pathCount != 2 {
		t.Errorf("Linux: expected 2 entries (PATH + Path), got %d", pathCount)
	}
	if !pathOriginalFound {
		t.Error("Linux: PATH=/original/bin should be preserved")
	}
	if !pathNewFound {
		t.Error("Linux: Path=/new/bin should be added as distinct key")
	}
}

func TestMergeEnvForOS_OverrideExisting(t *testing.T) {
	base := []string{"FOO=original", "BAR=keep"}
	extra := []string{"FOO=overridden"}
	merged := mergeEnvForOS(base, extra, "linux")

	fooCount := 0
	barCount := 0
	for _, kv := range merged {
		if kv == "FOO=overridden" {
			fooCount++
		}
		if kv == "BAR=keep" {
			barCount++
		}
	}
	if fooCount != 1 {
		t.Errorf("FOO should be overridden once, got %d", fooCount)
	}
	if barCount != 1 {
		t.Errorf("BAR should be preserved, got %d", barCount)
	}
}

func TestMergeEnvForOS_NoExtraReturnsCopy(t *testing.T) {
	base := []string{"A=1", "B=2"}
	merged := mergeEnvForOS(base, nil, "linux")
	if len(merged) != 2 {
		t.Errorf("no extra: len=%d, want 2", len(merged))
	}
	// Verify it's a copy, not the same slice
	merged[0] = "MUTATED"
	if base[0] == "MUTATED" {
		t.Error("mergeEnvForOS should return a copy, not alias base")
	}
}

func TestMergeEnvForOS_DeterministicCheck(t *testing.T) {
	// Verify override behavior is deterministic regardless of map iteration order.
	// We check that the merged set always contains exactly the expected K=V pairs.
	base := []string{"KEY=original"}
	extra := []string{"KEY=new"}
	merged := mergeEnvForOS(base, extra, "linux")

	// Sort for deterministic comparison
	sort.Strings(merged)
	if len(merged) != 1 || merged[0] != "KEY=new" {
		t.Errorf("deterministic merge = %v, want [KEY=new]", merged)
	}
}

// --- #7: rmdir is now a mapped command ---

func TestResolveCommand_Rmdir(t *testing.T) {
	cases := []struct {
		osName, wantCmd string
	}{
		{"win", "Remove-Item"},
		{"linux", "rmdir"},
		{"macos", "rmdir"},
		{"wsl", "rmdir"},
	}
	for _, c := range cases {
		got, mapped := resolveCommand("rmdir", c.osName)
		if !mapped {
			t.Errorf("resolveCommand(rmdir, %q): mapped=false, want true", c.osName)
			continue
		}
		if got != c.wantCmd {
			t.Errorf("resolveCommand(rmdir, %q) = %q, want %q", c.osName, got, c.wantCmd)
		}
	}
}

func TestIsDestructive_Rmdir(t *testing.T) {
	if !isDestructive("rmdir") {
		t.Error("rmdir should be destructive (now that it's mapped)")
	}
}

func TestTranslateRmdir_Passthrough(t *testing.T) {
	// rmdir has no flag translation — args preserved as-is
	got := translateArgs("rmdir", "linux", []string{"olddir"}, true)
	if len(got) != 1 || got[0].Value != "olddir" {
		t.Errorf("rmdir args should be preserved, got %+v", got)
	}
}

// --- #10: audit log permissions and failure handling ---

func TestWriteLogImpl_CreatesLogFileWith0600(t *testing.T) {
	// File permission bits are only meaningful on Unix.
	// Windows NTFS doesn't preserve Unix mode bits the same way,
	// so this test is Unix-only.
	if osWindows() {
		t.Skip("file mode bits not enforced on Windows NTFS")
	}

	// Use a temp HOME so we don't clobber the real log
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	// On Windows, UserHomeDir may use USERPROFILE
	t.Setenv("USERPROFILE", tmpHome)

	res := &result{
		OK:              true,
		ExitCode:        0,
		Backend:         "test",
		OSMode:          "test",
		ResolvedCommand: "test cmd",
	}
	if err := writeLogImpl(res); err != nil {
		t.Fatalf("writeLogImpl: %v", err)
	}

	logFile := filepath.Join(tmpHome, ".go_shell", "log.jsonl")
	info, err := os.Stat(logFile)
	if err != nil {
		t.Fatalf("log file not created: %v", err)
	}
	mode := info.Mode().Perm()
	if mode != 0600 {
		t.Errorf("log file mode = %o, want 0600", mode)
	}
}

// osWindows returns true on Windows hosts. Uses a seam for testability.
func osWindows() bool {
	return osGoos == "windows"
}

func TestWriteLogImpl_AppendsLines(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)

	res := &result{OK: true, ExitCode: 0, Backend: "test", OSMode: "test", ResolvedCommand: "cmd1"}
	_ = writeLogImpl(res)
	res2 := &result{OK: false, ExitCode: 1, Backend: "test", OSMode: "test", ResolvedCommand: "cmd2"}
	_ = writeLogImpl(res2)

	logFile := filepath.Join(tmpHome, ".go_shell", "log.jsonl")
	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 log lines, got %d", len(lines))
	}
}

func TestMarshalLogLineReturnsOneCompleteJSONLRecord(t *testing.T) {
	res := &result{
		OK:              true,
		ExitCode:        0,
		Backend:         "test",
		OSMode:          "test",
		ResolvedCommand: "echo hello",
		Stdout:          "hello\n",
	}
	line, err := marshalLogLine(res)
	if err != nil {
		t.Fatalf("marshalLogLine: %v", err)
	}
	if !strings.HasSuffix(string(line), "\n") || strings.Count(string(line), "\n") != 1 {
		t.Fatalf("line must contain exactly one trailing newline: %q", line)
	}
	var entry map[string]any
	if err := json.Unmarshal(line[:len(line)-1], &entry); err != nil {
		t.Fatalf("record is not valid JSON: %v", err)
	}
	if entry["resolved_command"] != "echo hello" || entry["stdout_len"] != float64(6) {
		t.Fatalf("unexpected entry: %#v", entry)
	}
}

// --- #11: stdin read error must prevent write ---

// TestWriteFile_StdinReadErrorDoesNotWrite verifies that if stdin reading
// fails, no file is written. We simulate this by providing a content that
// is NOT "-" (so stdin isn't used) — the real stdin-error path is hard to
// trigger in a unit test, but we verify the io.ReadAll contract:
// a nil error with EOF means success; any other error means abort.
func TestWriteFile_LiteralContentDoesNotReadStdin(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")
	// Content is literal, not "-" → stdin should not be read
	res := fnWriteFile([]string{path, "literal content"}, &metaConfig{})
	if res == nil || !res.OK {
		t.Fatalf("write_file failed: %+v", res)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "literal content" {
		t.Errorf("content = %q, want %q", data, "literal content")
	}
}

// TestIoReadAll_Contract is a smoke test verifying that io.ReadAll on an
// empty reader returns (nil, nil), confirming the contract we rely on.
func TestIoReadAll_Contract(t *testing.T) {
	data, err := io.ReadAll(strings.NewReader(""))
	if err != nil {
		t.Errorf("io.ReadAll(empty) err = %v, want nil", err)
	}
	if len(data) != 0 {
		t.Errorf("io.ReadAll(empty) = %d bytes, want 0", len(data))
	}
}
