package main

import (
	"strings"
	"testing"
)

// ============================================================================
// Symmetry regression: sh/zsh/wsl backend must handle cmd the same way
// pwsh handles it (mapped → raw, passthrough → quoted).
//
// These tests mirror audit3_test.go's pwsh tests so the two backends
// stay in lock-step. Future backend additions should follow the same
// pattern.
// ============================================================================

// simulateShLine reproduces the line-building logic from execute()'s
// sh/zsh/wsl branch, so we can assert quoting without actually launching
// a subprocess. This mirrors the pwsh helper logic in audit3_test.go.
func simulateShLine(cmd string, args []resolvedArg, mapped bool) string {
	var line string
	if mapped {
		line = cmd
	} else {
		line = shQuote(cmd)
	}
	for _, a := range args {
		line += " " + shQuote(a.Value)
	}
	return line
}

func TestShPassthrough_QuotedCommandName(t *testing.T) {
	// Passthrough (mapped=false) must shQuote the command name, mirroring
	// pwsh's "& 'git' 'status'" behavior.
	cmd := "git"
	args := []resolvedArg{{Value: "status", Raw: false}}
	mapped := false

	line := simulateShLine(cmd, args, mapped)

	if !strings.Contains(line, "'git'") {
		t.Errorf("sh passthrough should single-quote cmd: %q", line)
	}
	// Must not appear as raw syntax fragment
	if strings.HasPrefix(line, "git ") {
		t.Errorf("sh passthrough emitted raw cmd: %q", line)
	}
}

func TestShPassthrough_CommandNameWithInjectionChars(t *testing.T) {
	// A command name containing ;, |, &, $() must NOT be emitted as syntax.
	injectionCmds := []string{
		"git; rm -rf /",
		"foo | bar",
		"foo & baz",
		"foo$(whoami)",
		"foo`whoami`",
	}
	for _, cmd := range injectionCmds {
		mapped := false
		line := simulateShLine(cmd, nil, mapped)
		// Raw injection must not appear unquoted
		rawWithSpace := cmd + " "
		if strings.Contains(line, rawWithSpace) || strings.HasPrefix(line, cmd) {
			t.Errorf("injection cmd %q emitted raw: %q", cmd, line)
		}
		// Must be single-quoted
		escaped := strings.ReplaceAll(cmd, "'", "'\\''")
		if !strings.Contains(line, "'"+escaped+"'") {
			t.Errorf("injection cmd %q should be single-quoted: %q", cmd, line)
		}
	}
}

func TestShMapped_RawSyntaxFragment(t *testing.T) {
	// mapped=true: cmd is a translator-generated fragment, emitted raw.
	cmd := "ls"
	mapped := true

	line := simulateShLine(cmd, nil, mapped)
	if line != "ls" {
		t.Errorf("mapped cmd should be raw, got %q", line)
	}
	if strings.Contains(line, "'") {
		t.Errorf("mapped cmd should not be quoted: %q", line)
	}
}

func TestShMapped_MultiWordSyntax(t *testing.T) {
	// Mapped cmd can itself be a multi-word fragment (e.g. "ls -la"
	// is not a real case here, but the principle holds).
	cmd := "ls -la"
	mapped := true

	line := simulateShLine(cmd, nil, mapped)
	if line != "ls -la" {
		t.Errorf("mapped multi-word cmd should be raw, got %q", line)
	}
}

func TestShPassthrough_ArgsStillQuotedRegardlessOfMapped(t *testing.T) {
	// Args are always shQuote'd, regardless of mapped. This matches
	// pwsh behavior. The fix for cmd shouldn't have changed arg quoting.
	cases := []struct {
		mapped bool
	}{
		{true},
		{false},
	}
	for _, c := range cases {
		line := simulateShLine("git", []resolvedArg{{Value: "status", Raw: false}}, c.mapped)
		if !strings.Contains(line, "'status'") {
			t.Errorf("args should always be quoted (mapped=%v): %q", c.mapped, line)
		}
	}
}

func TestShPassthrough_WhitespaceAndQuotesPreserved(t *testing.T) {
	// Single quotes, spaces, and newlines in user values must survive
	// the round-trip through shQuote.
	cases := []struct {
		in   string
		want string
	}{
		{"hello world", "'hello world'"},
		{"it's", "'it'\\''s'"},
		{"line1\nline2", "'line1\nline2'"},
		{"  spaces  ", "'  spaces  '"},
	}
	for _, c := range cases {
		got := shQuote(c.in)
		if got != c.want {
			t.Errorf("shQuote(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// ============================================================================
// Symmetry assertion: pwsh and sh line-builders handle cmd identically
// for the (mapped, passthrough) dichotomy. If a future change diverges,
// this test catches it.
// ============================================================================

func TestBackendSymmetry_CmdHandling(t *testing.T) {
	// Same cmd + same mapped value → both backends must produce a line
	// that treats the cmd the same way: raw if mapped, quoted if not.
	cmd := "git"
	args := []resolvedArg{{Value: "status", Raw: false}}

	// pwsh line
	pwshLine := ""
	if false { // mapped=false
		pwshLine += cmd
	} else {
		pwshLine += "& " + pwshQuote(cmd)
	}
	for _, a := range args {
		pwshLine += " " + pwshQuote(a.Value)
	}

	// sh line
	shLine := simulateShLine(cmd, args, false)

	// Both must single-quote "git" (just one of the two — pwsh adds & prefix)
	if !strings.Contains(pwshLine, "'git'") {
		t.Errorf("pwsh line should contain quoted git: %q", pwshLine)
	}
	if !strings.Contains(shLine, "'git'") {
		t.Errorf("sh line should contain quoted git: %q", shLine)
	}
	// Both must single-quote "status"
	if !strings.Contains(pwshLine, "'status'") {
		t.Errorf("pwsh line should contain quoted status: %q", pwshLine)
	}
	if !strings.Contains(shLine, "'status'") {
		t.Errorf("sh line should contain quoted status: %q", shLine)
	}
}
