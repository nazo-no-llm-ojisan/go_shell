package main

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
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
	t.Helper()
	cmdArgs := append([]string{"-test.run=^TestCLIHelperProcess$", "--"}, args...)
	cmd := exec.Command(os.Args[0], cmdArgs...)
	cmd.Env = append(os.Environ(), "GO_SHELL_HELPER_PROCESS=1")
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

func TestCLIJSONErrorsAreStructured(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{name: "invalid timeout", args: []string{"--json", "--timeout", "nope", "-win", "-ls"}, want: "invalid duration"},
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
