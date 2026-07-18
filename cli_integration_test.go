package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIHelperProcess(t *testing.T) {
	if os.Getenv("GO_SHELL_HELPER_PROCESS") != "1" {
		return
	}

	separator := -1
	for i, arg := range os.Args {
		if arg == "--" {
			separator = i
			break
		}
	}
	if separator == -1 {
		os.Exit(99)
	}

	os.Args = append([]string{"go_shell"}, os.Args[separator+1:]...)
	main()
	os.Exit(0)
}

func runCLIForTest(t *testing.T, args ...string) ([]byte, []byte, int) {
	home := t.TempDir()
	return runCLIForTestEnv(t, []string{"HOME=" + home, "USERPROFILE=" + home}, args...)
}

func runCLIForTestEnv(t *testing.T, extraEnv []string, args ...string) ([]byte, []byte, int) {
	t.Helper()
	cmdArgs := append([]string{"-test.run=^TestCLIHelperProcess$", "--"}, args...)
	cmd := exec.Command(os.Args[0], cmdArgs...)
	cmd.Env = mergeEnvForOS(os.Environ(), append(extraEnv, "GO_SHELL_HELPER_PROCESS=1"), osGoos)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("run CLI: %v", err)
		}
		exitCode = exitErr.ExitCode()
	}
	return []byte(stdout.String()), []byte(stderr.String()), exitCode
}

func readAuditResults(t *testing.T, home string) []result {
	t.Helper()
	f, err := os.Open(filepath.Join(home, ".go_shell", "log.jsonl"))
	if err != nil {
		t.Fatalf("open audit log: %v", err)
	}
	defer f.Close()

	var results []result
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var got result
		if err := json.Unmarshal(scanner.Bytes(), &got); err != nil {
			t.Fatalf("invalid JSONL entry %q: %v", scanner.Text(), err)
		}
		results = append(results, got)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan audit log: %v", err)
	}
	return results
}

func TestCLIJSONErrorsAreStructured(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{name: "invalid timeout", args: []string{"--json", "--timeout", "nope", "-win", "-ls"}, want: "invalid duration"},
		{name: "json after invalid timeout", args: []string{"--timeout", "nope", "--json", "-win", "-ls"}, want: "invalid duration"},
		{name: "missing command", args: []string{"--json", "-win"}, want: "no command given"},
		{name: "unknown selector", args: []string{"--json", "-unknown"}, want: "unknown OS or function"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, exitCode := runCLIForTest(t, tc.args...)
			if exitCode != 2 {
				t.Fatalf("exit code = %d, want 2; stderr=%q", exitCode, stderr)
			}
			if len(stderr) != 0 {
				t.Fatalf("stderr = %q, want empty", stderr)
			}
			var got result
			if err := json.Unmarshal(stdout, &got); err != nil {
				t.Fatalf("stdout is not JSON: %q: %v", stdout, err)
			}
			if got.OK || got.ExitCode != 2 || !strings.Contains(got.Stderr, tc.want) {
				t.Fatalf("result = %+v, want exit 2 containing %q", got, tc.want)
			}
		})
	}
}

func TestCLIFunctionAndDryRunAreAudited(t *testing.T) {
	cases := []struct {
		name       string
		args       func(string) []string
		wantPrefix string
	}{
		{
			name: "write_file function",
			args: func(work string) []string {
				return []string{"--json", "--cwd", work, "-write_file", "out.txt", "content"}
			},
			wantPrefix: "write_file(",
		},
		{
			name: "destructive dry run",
			args: func(string) []string {
				return []string{"--json", "-native", "-rm", "target"}
			},
			wantPrefix: "rm target",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			work := t.TempDir()
			env := []string{"HOME=" + home, "USERPROFILE=" + home}
			_, stderr, exitCode := runCLIForTestEnv(t, env, tc.args(work)...)
			if exitCode != 0 {
				t.Fatalf("exit code = %d, want 0; stderr=%q", exitCode, stderr)
			}
			entries := readAuditResults(t, home)
			if len(entries) != 1 {
				t.Fatalf("audit entries = %d, want 1", len(entries))
			}
			if !strings.HasPrefix(entries[0].ResolvedCommand, tc.wantPrefix) {
				t.Fatalf("resolved command = %q, want prefix %q", entries[0].ResolvedCommand, tc.wantPrefix)
			}
		})
	}
}

func TestCLIWindowsTouchWithoutPathIsStructuredUsageError(t *testing.T) {
	stdout, stderr, exitCode := runCLIForTest(t, "--json", "-win", "-touch")
	if exitCode != 2 {
		t.Fatalf("exit code = %d, want 2; stdout=%q stderr=%q", exitCode, stdout, stderr)
	}
	if len(stderr) != 0 {
		t.Fatalf("stderr = %q, want empty in JSON mode", stderr)
	}
	var got result
	if err := json.Unmarshal(stdout, &got); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if got.OK || !strings.Contains(got.Stderr, "touch: requires at least one path") {
		t.Fatalf("result = %+v, want touch usage error", got)
	}
}

func TestCLIJSONAuditLogFailureUsesWarningField(t *testing.T) {
	dir := t.TempDir()
	badHome := filepath.Join(dir, "not-a-directory")
	if err := os.WriteFile(badHome, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	env := []string{"HOME=" + badHome, "USERPROFILE=" + badHome}
	stdout, stderr, exitCode := runCLIForTestEnv(t, env, "--json", "-open_url", "https://example.invalid")
	if exitCode != stubExitCode {
		t.Fatalf("exit code = %d, want %d; stdout=%q stderr=%q", exitCode, stubExitCode, stdout, stderr)
	}
	if len(stderr) != 0 {
		t.Fatalf("stderr = %q, want empty in JSON mode", stderr)
	}
	var got result
	if err := json.Unmarshal(stdout, &got); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if !strings.Contains(got.Warning, "audit log write failed") {
		t.Fatalf("warning = %q, want audit log failure", got.Warning)
	}
}
