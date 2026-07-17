package main

import (
	"os"
	"strings"
	"testing"
	"time"
)

// ============================================================================
// ChatGPT audit round 2 — contract tests for the 6 fixes
// ============================================================================

// --- P1: pwsh must quote ALL user args, only translator flags are raw ---

func TestPwshArgQuoting_UserValueWithDashIsQuoted(t *testing.T) {
	// A user value starting with "-" must still be quoted (not emitted raw).
	// This prevents injection like: -foo; Write-Output injected
	arg := resolvedArg{Value: "-foo; Write-Output injected", Raw: false}
	// Simulate the pwsh line building logic
	line := buildPwshCommandLine("cmd", []resolvedArg{arg}, true)
	// The result must contain the quoted form, not the raw injection
	if strings.Contains(line, arg.Value+" ") || strings.HasSuffix(line, arg.Value) {
		t.Errorf("user value with dash was emitted raw (injection risk): %q", line)
	}
	if !strings.Contains(line, "'-foo; Write-Output injected'") {
		t.Errorf("user value should be single-quoted: %q", line)
	}
}

func TestPwshArgQuoting_TranslatorFlagIsRaw(t *testing.T) {
	// Translator-generated flags (e.g. -Force) are Raw=true and emitted as-is.
	arg := resolvedArg{Value: "-Force", Raw: true}
	line := buildPwshCommandLine("cmd", []resolvedArg{arg}, true)
	if !strings.Contains(line, "cmd -Force") {
		t.Errorf("translator flag should be raw: %q", line)
	}
}

func TestTranslateLS_PreservesUnknownArgs(t *testing.T) {
	// --color=auto is not a known ls flag — must be preserved as-is
	got := translateLS("linux", []string{"--color=auto", "src"})
	if len(got) != 2 {
		t.Fatalf("expected 2 args, got %d: %+v", len(got), got)
	}
	// first might be -la if flags were set, but --color=auto must appear verbatim
	found := false
	for _, a := range got {
		if a.Value == "--color=auto" {
			found = true
			if a.Raw {
				t.Errorf("--color=auto should be Raw=false (user value)")
			}
		}
	}
	if !found {
		t.Errorf("--color=auto not preserved, got: %+v", got)
	}
}

func TestTranslateRM_PreservesDashFilenames(t *testing.T) {
	// A file named "-important" must NOT become "important"
	got := translateRM("linux", []string{"-important"})
	if len(got) != 1 {
		t.Fatalf("expected 1 arg, got %d: %+v", len(got), got)
	}
	if got[0].Value != "-important" {
		t.Errorf("filename '-important' was corrupted to %q", got[0].Value)
	}
}

func TestTranslateRM_PresainsDoubleDashSeparator(t *testing.T) {
	// "--" is a POSIX argument separator, not a flag — must be preserved
	got := translateRM("linux", []string{"--", "-important"})
	values := resolvedValues(got)
	// "--" should be preserved (not recognized as a known flag)
	foundDD := false
	foundFile := false
	for _, v := range values {
		if v == "--" {
			foundDD = true
		}
		if v == "-important" {
			foundFile = true
		}
	}
	if !foundDD {
		t.Errorf("'--' separator was not preserved, got: %v", values)
	}
	if !foundFile {
		t.Errorf("filename '-important' after '--' was not preserved, got: %v", values)
	}
}

func TestTranslateMkdir_PreservesUnknownDashArg(t *testing.T) {
	// A directory named "-weird" must be preserved
	got := translateMkdir("linux", []string{"-weird"})
	if len(got) != 1 {
		t.Fatalf("expected 1 arg, got %d: %+v", len(got), got)
	}
	if got[0].Value != "-weird" {
		t.Errorf("dirname '-weird' was corrupted to %q", got[0].Value)
	}
}

// --- P1: timeout must produce exit code 124 ---

func TestTimeout_ProducesExit124(t *testing.T) {
	if os.Getenv("GO_SHELL_TIMEOUT_HELPER") == "1" {
		time.Sleep(10 * time.Second)
		os.Exit(0)
	}
	t.Setenv("GO_SHELL_TIMEOUT_HELPER", "1")
	res := newResult("native", "native", "timeout helper")
	got := execute(res, "native", os.Args[0], []resolvedArg{{Value: "-test.run=^TestTimeout_ProducesExit124$"}}, &metaConfig{timeout: 50 * time.Millisecond}, false)
	if got.ExitCode != 124 || got.OK {
		t.Fatalf("timeout result = %+v, want exit 124 failure", got)
	}
}

// --- P2: meta validation must be fail-closed ---

func TestParseMeta_InvalidTimeoutIsFatal(t *testing.T) {
	_, _, err := parseMetaChecked([]string{"--timeout", "not-a-duration", "-win", "-ls"})
	if err == nil {
		t.Fatal("parseMetaChecked accepted invalid timeout")
	}
}

func TestParseMeta_NegativeTimeoutRejected(t *testing.T) {
	_, _, err := parseMetaChecked([]string{"--timeout", "-5s", "-win", "-ls"})
	if err == nil {
		t.Fatal("parseMetaChecked accepted negative timeout")
	}
}

func TestParseMeta_EnvWithoutEqualsIsFatal(t *testing.T) {
	_, _, err := parseMetaChecked([]string{"--env", "NOEQUALSHERE", "-win", "-ls"})
	if err == nil {
		t.Fatal("parseMetaChecked accepted malformed environment override")
	}
}

// --- P2: stubs must return failure (exit 78) ---

func TestAllStubsReturnFailure(t *testing.T) {
	// Every function that uses stubResult must return OK=false, exit 78
	stubFnNames := []string{"create_hermes_subagent", "run_hermes_task", "read_hermes_session", "open_url"}
	for _, name := range stubFnNames {
		fn, ok := functionRegistry[name]
		if !ok {
			t.Errorf("%q not registered", name)
			continue
		}
		res := fn([]string{"arg"}, &metaConfig{})
		if res == nil {
			t.Errorf("%q returned nil", name)
			continue
		}
		if res.OK {
			t.Errorf("%q: stub returned OK=true, want false", name)
		}
		if res.ExitCode != stubExitCode {
			t.Errorf("%q: exit=%d, want %d", name, res.ExitCode, stubExitCode)
		}
	}
}
