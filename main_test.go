package main

import "testing"

// ============================================================================
// Layer 0: OS registry
// ============================================================================

func TestIsOS(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"win", true},
		{"linux", true},
		{"macos", true},
		{"wsl", true},
		{"native", true},
		{"auto", true},
		{"windows", false}, // not in list
		{"mac", false},     // not in list (use macos)
		{"", false},
		{"create_subagent", false},
	}
	for _, c := range cases {
		if got := isOS(c.in); got != c.want {
			t.Errorf("isOS(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestIsOS_AllRegisteredAreValid(t *testing.T) {
	for _, name := range osList() {
		if !isOS(name) {
			t.Errorf("osList contains %q but isOS returns false", name)
		}
	}
}

func TestBackendFor(t *testing.T) {
	cases := []struct {
		osName string
		want   string
	}{
		{"win", "pwsh"},
		{"linux", "sh"},
		{"macos", "zsh"},
		{"wsl", "wsl"},
		{"native", "native"},
		{"unknown", "native"}, // default
	}
	for _, c := range cases {
		if got := backendFor(c.osName); got != c.want {
			t.Errorf("backendFor(%q) = %q, want %q", c.osName, got, c.want)
		}
	}
}

// ============================================================================
// Layer 1: Command layer
// ============================================================================

func TestResolveCommand_Mapped(t *testing.T) {
	cases := []struct {
		logical, osName, wantCmd string
	}{
		{"ls", "win", "Get-ChildItem"},
		{"ls", "linux", "ls"},
		{"ls", "macos", "ls"},
		{"rm", "win", "Remove-Item"},
		{"cat", "win", "Get-Content"},
		{"mkdir", "win", "New-Item -ItemType Directory"},
	}
	for _, c := range cases {
		got, mapped := resolveCommand(c.logical, c.osName)
		if !mapped {
			t.Errorf("resolveCommand(%q,%q): mapped=false, want true", c.logical, c.osName)
			continue
		}
		if got != c.wantCmd {
			t.Errorf("resolveCommand(%q,%q) = %q, want %q", c.logical, c.osName, got, c.wantCmd)
		}
	}
}

func TestResolveCommand_Passthrough(t *testing.T) {
	cases := []struct {
		logical, osName string
	}{
		{"git", "win"},    // not in table
		{"dotnet", "win"}, // not in table
		{"npm", "linux"},  // not in table
		{"rg", "macos"},   // not in table
		{"unknownxyz", "win"},
	}
	for _, c := range cases {
		got, mapped := resolveCommand(c.logical, c.osName)
		if mapped {
			t.Errorf("resolveCommand(%q,%q): mapped=true, want false (passthrough)", c.logical, c.osName)
		}
		if got != c.logical {
			t.Errorf("resolveCommand(%q,%q) = %q, want %q (passthrough)", c.logical, c.osName, got, c.logical)
		}
	}
}

func TestResolveCommand_AllCommandsHaveAllOS(t *testing.T) {
	// Every command in the table should have an entry for each concrete OS
	// (win, linux, macos, wsl). native and auto are not in the table.
	required := []string{"win", "linux", "macos", "wsl"}
	for logical, osMap := range commandTable {
		for _, osName := range required {
			if _, ok := osMap[osName]; !ok {
				t.Errorf("commandTable[%q] missing entry for %q", logical, osName)
			}
		}
	}
}

// ============================================================================
// Layer 2: Argument layer
// ============================================================================

func TestTranslateLS_Win(t *testing.T) {
	cases := []struct {
		args []string
		want []string
	}{
		{[]string{"-a"}, []string{"-Force"}},
		{[]string{"-l"}, []string{}},          // long has no effect on win (default)
		{[]string{"-al"}, []string{"-Force"}}, // long+all → -Force only
		{[]string{"-la"}, []string{"-Force"}},
		{[]string{}, []string{}},
		{[]string{"somepath"}, []string{"somepath"}},
		{[]string{"-a", "somepath"}, []string{"-Force", "somepath"}},
	}
	for _, c := range cases {
		got := translateLS("win", c.args)
		if !sliceEq(resolvedValues(got), c.want) {
			t.Errorf("translateLS(win, %v) = %v, want %v", c.args, got, c.want)
		}
	}
}

func TestTranslateLS_Linux(t *testing.T) {
	cases := []struct {
		args []string
		want []string
	}{
		{[]string{"-a"}, []string{"-a"}},
		{[]string{"-l"}, []string{"-l"}},
		{[]string{"-al"}, []string{"-la"}},
		{[]string{"-la"}, []string{"-la"}},
		{[]string{}, []string{}},
		{[]string{"somepath"}, []string{"somepath"}},
		{[]string{"-a", "somepath"}, []string{"-a", "somepath"}},
	}
	for _, c := range cases {
		got := translateLS("linux", c.args)
		if !sliceEq(resolvedValues(got), c.want) {
			t.Errorf("translateLS(linux, %v) = %v, want %v", c.args, got, c.want)
		}
	}
}

func TestTranslateRM_Win(t *testing.T) {
	cases := []struct {
		args []string
		want []string
	}{
		{[]string{"-r"}, []string{"-Recurse"}},
		{[]string{"-rf"}, []string{"-Force", "-Recurse"}},
		{[]string{"-f"}, []string{"-Force"}},
		{[]string{"target"}, []string{"target"}},
		{[]string{"-r", "target"}, []string{"-Recurse", "target"}},
	}
	for _, c := range cases {
		got := translateRM("win", c.args)
		if !sliceEq(resolvedValues(got), c.want) {
			t.Errorf("translateRM(win, %v) = %v, want %v", c.args, got, c.want)
		}
	}
}

func TestTranslateRM_Linux(t *testing.T) {
	cases := []struct {
		args []string
		want []string
	}{
		{[]string{"-r"}, []string{"-r"}},
		{[]string{"-rf"}, []string{"-rf"}},
		{[]string{"-f"}, []string{"-f"}},
		{[]string{"target"}, []string{"target"}},
		{[]string{"-r", "target"}, []string{"-r", "target"}},
	}
	for _, c := range cases {
		got := translateRM("linux", c.args)
		if !sliceEq(resolvedValues(got), c.want) {
			t.Errorf("translateRM(linux, %v) = %v, want %v", c.args, got, c.want)
		}
	}
}

func TestTranslateMkdir_Win(t *testing.T) {
	got := translateMkdir("win", []string{"-p", "newdir"})
	want := []string{"-Force", "newdir"}
	if !sliceEq(resolvedValues(got), want) {
		t.Errorf("translateMkdir(win, -p newdir) = %v, want %v", got, want)
	}
}

func TestTranslateMkdir_Linux(t *testing.T) {
	got := translateMkdir("linux", []string{"-p", "newdir"})
	want := []string{"-p", "newdir"}
	if !sliceEq(resolvedValues(got), want) {
		t.Errorf("translateMkdir(linux, -p newdir) = %v, want %v", got, want)
	}
}

func TestTranslateArgs_Passthrough(t *testing.T) {
	// unmapped command → args returned stripped of dash, no translation
	got := translateArgs("git", "win", []string{"status", "--short"}, false)
	want := []string{"status", "--short"}
	if !sliceEq(resolvedValues(got), want) {
		t.Errorf("translateArgs(git, passthrough) = %v, want %v", got, want)
	}
}

// ============================================================================
// pwshQuote
// ============================================================================

func TestPwshQuote(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"hello", "'hello'"},
		{"", "''"},
		{"it's", "'it''s'"}, // single quote doubled
		{"a'b'c", "'a''b''c'"},
		{"path with space", "'path with space'"},
		{"$HOME", "'$HOME'"}, // no variable expansion inside single quotes
	}
	for _, c := range cases {
		if got := pwshQuote(c.in); got != c.want {
			t.Errorf("pwshQuote(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// ============================================================================
// Function registry
// ============================================================================

func TestIsFunction(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"create_hermes_subagent", true},
		{"run_hermes_task", true},
		{"read_hermes_session", true},
		{"write_file", true},
		{"copy_file", true},
		{"open_url", true},
		{"create_nonexistent", false},
		{"", false},
		{"ls", false},
	}
	for _, c := range cases {
		if got := isFunction(c.name); got != c.want {
			t.Errorf("isFunction(%q) = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestFuncList_ContainsAllRegistered(t *testing.T) {
	list := funcList()
	for name := range functionRegistry {
		found := false
		for _, n := range list {
			if n == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("funcList() missing registered function %q", name)
		}
	}
	if len(list) != len(functionRegistry) {
		t.Errorf("funcList() len=%d, want %d", len(list), len(functionRegistry))
	}
}

// ============================================================================
// helpers
// ============================================================================
// sliceEq compares two string slices.
func sliceEq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// resolvedValues extracts the .Value fields from a slice of resolvedArg.
func resolvedValues(args []resolvedArg) []string {
	out := make([]string, 0, len(args))
	for _, a := range args {
		out = append(out, a.Value)
	}
	return out
}
