package main

import (
	"testing"
)

// ============================================================================
// Audit round 5 — flag-absorption regression tests
//
// The previous translator logic used stripDash() on every arg before
// matching, which meant a bare user value like "p", "f", or "rf" was
// indistinguishable from a real flag like "-p", "-f", "-rf". This
// silently destroyed user-supplied paths/operands.
//
// All three translators (ls, rm, mkdir) now use exact-string matching
// on the ORIGINAL arg and respect the POSIX "--" end-of-options
// terminator. These tests pin that contract.
// ============================================================================

// --- mkdir: bare "p" must NOT be absorbed as -p ---

func TestTranslateMkdir_BareP_NotFlag(t *testing.T) {
	got := translateMkdir("linux", []string{"p"})
	// Expected: ONE user value "p", no -p flag
	if len(got) != 1 {
		t.Fatalf("translateMkdir(linux, [p]) len=%d, want 1: %+v", len(got), got)
	}
	if got[0].Value != "p" {
		t.Errorf("value = %q, want %q", got[0].Value, "p")
	}
	if got[0].Raw {
		t.Errorf("bare 'p' should be user value (Raw=false), not raw flag")
	}
}

func TestTranslateMkdir_DashP_IsFlag(t *testing.T) {
	got := translateMkdir("linux", []string{"-p", "a/b"})
	// Expected: -p (raw) + a/b (user)
	if len(got) != 2 {
		t.Fatalf("translateMkdir(linux, [-p a/b]) len=%d, want 2: %+v", len(got), got)
	}
	if got[0].Value != "-p" || !got[0].Raw {
		t.Errorf("first arg should be raw '-p' flag, got %+v", got[0])
	}
	if got[1].Value != "a/b" || got[1].Raw {
		t.Errorf("second arg should be user 'a/b', got %+v", got[1])
	}
}

func TestTranslateMkdir_DashDash_DisablesFlag(t *testing.T) {
	got := translateMkdir("linux", []string{"--", "-p"})
	// After --, "-p" must be a literal path, not a flag
	values := resolvedValues(got)
	want := []string{"--", "-p"}
	if !sliceEq(values, want) {
		t.Errorf("translateMkdir(linux, [-- -p]) = %v, want %v", values, want)
	}
	// The post-"--" arg must be a user value, not a raw flag
	if got[1].Raw {
		t.Errorf("post-'--' '-p' must be user value (Raw=false), got Raw=true")
	}
}

// --- rm: bare "f" must NOT be absorbed as -f ---

func TestTranslateRM_BareF_NotFlag(t *testing.T) {
	got := translateRM("linux", []string{"f"})
	if len(got) != 1 {
		t.Fatalf("translateRM(linux, [f]) len=%d, want 1: %+v", len(got), got)
	}
	if got[0].Value != "f" {
		t.Errorf("value = %q, want %q", got[0].Value, "f")
	}
	if got[0].Raw {
		t.Errorf("bare 'f' should be user value, not raw flag")
	}
}

func TestTranslateRM_DashF_IsFlag(t *testing.T) {
	got := translateRM("linux", []string{"-f", "f"})
	// -f flag + "f" path
	if len(got) != 2 {
		t.Fatalf("translateRM(linux, [-f f]) len=%d, want 2: %+v", len(got), got)
	}
	if got[0].Value != "-f" || !got[0].Raw {
		t.Errorf("first arg should be raw '-f' flag, got %+v", got[0])
	}
	if got[1].Value != "f" || got[1].Raw {
		t.Errorf("second arg should be user 'f', got %+v", got[1])
	}
}

func TestTranslateRM_DashDash_DisablesFlag(t *testing.T) {
	got := translateRM("linux", []string{"--", "-f"})
	values := resolvedValues(got)
	want := []string{"--", "-f"}
	if !sliceEq(values, want) {
		t.Errorf("translateRM(linux, [-- -f]) = %v, want %v", values, want)
	}
	if got[1].Raw {
		t.Errorf("post-'--' '-f' must be user value (Raw=false)")
	}
}

func TestTranslateRM_DashRF_FullFlags(t *testing.T) {
	got := translateRM("linux", []string{"-rf", "dir"})
	// -rf flag + dir
	if len(got) != 2 {
		t.Fatalf("translateRM(linux, [-rf dir]) len=%d, want 2: %+v", len(got), got)
	}
	if got[0].Value != "-rf" || !got[0].Raw {
		t.Errorf("first arg should be raw '-rf', got %+v", got[0])
	}
	if got[1].Value != "dir" || got[1].Raw {
		t.Errorf("second arg should be user 'dir', got %+v", got[1])
	}
}

func TestTranslateRM_BareRF_NotFlag(t *testing.T) {
	got := translateRM("linux", []string{"rf", "dir"})
	// Bare "rf" must be a user value, not a flag
	if len(got) != 2 {
		t.Fatalf("translateRM(linux, [rf dir]) len=%d, want 2: %+v", len(got), got)
	}
	if got[0].Value != "rf" || got[0].Raw {
		t.Errorf("bare 'rf' must be user value, got %+v", got[0])
	}
	if got[1].Value != "dir" || got[1].Raw {
		t.Errorf("'dir' must be user value, got %+v", got[1])
	}
}

func TestTranslateRM_WinFallsBackToExistingFlagForm(t *testing.T) {
	// On Windows, -rf maps to BOTH -Recurse and -Force. The user value
	// "f" must still not be absorbed as a flag.
	got := translateRM("win", []string{"f"})
	line := buildPwshCommandLine("Remove-Item", got, true)
	if line != "Remove-Item -LiteralPath @('f')" {
		t.Fatalf("bare 'f' on win was not preserved as a literal path: %q", line)
	}
}

// --- ls: same fix applied for symmetry (file named "l" or "a" is not a flag) ---

func TestTranslateLS_BareL_NotFlag(t *testing.T) {
	got := translateLS("linux", []string{"l"})
	if len(got) != 1 {
		t.Fatalf("translateLS(linux, [l]) len=%d, want 1: %+v", len(got), got)
	}
	if got[0].Value != "l" || got[0].Raw {
		t.Errorf("bare 'l' must be user value, got %+v", got[0])
	}
}

func TestTranslateLS_DashL_IsFlag(t *testing.T) {
	got := translateLS("linux", []string{"-l"})
	if len(got) != 1 {
		t.Fatalf("translateLS(linux, [-l]) len=%d, want 1: %+v", len(got), got)
	}
	if got[0].Value != "-l" || !got[0].Raw {
		t.Errorf("'-l' should be raw flag, got %+v", got[0])
	}
}

func TestTranslateLS_DashDash_DisablesFlag(t *testing.T) {
	got := translateLS("linux", []string{"--", "-l"})
	values := resolvedValues(got)
	want := []string{"--", "-l"}
	if !sliceEq(values, want) {
		t.Errorf("translateLS(linux, [-- -l]) = %v, want %v", values, want)
	}
	if got[1].Raw {
		t.Errorf("post-'--' '-l' must be user value (Raw=false)")
	}
}

func TestTranslateLS_BareAl_NotFlag(t *testing.T) {
	got := translateLS("linux", []string{"al"})
	// Bare "al" must be a user value, not -la/-al flag
	if len(got) != 1 {
		t.Fatalf("translateLS(linux, [al]) len=%d, want 1: %+v", len(got), got)
	}
	if got[0].Value != "al" || got[0].Raw {
		t.Errorf("bare 'al' must be user value, got %+v", got[0])
	}
}

// --- Existing compatibility: known flag forms still work ---

func TestExistingFlags_StillWork(t *testing.T) {
	// ls
	cases := []struct {
		name    string
		args    []string
		wantRaw []string
		wantUV  []string
	}{
		// ls normalizes: -al → -la, separate -l -a → -la
		{"ls -la", []string{"-la"}, []string{"-la"}, nil},
		{"ls -al", []string{"-al"}, []string{"-la"}, nil},
		{"ls -l -a", []string{"-l", "-a"}, []string{"-la"}, nil},
		{"ls dir", []string{"dir"}, nil, []string{"dir"}},
		{"mkdir -p a/b/c", []string{"-p", "a/b/c"}, []string{"-p"}, []string{"a/b/c"}},
		{"rm -r dir", []string{"-r", "dir"}, []string{"-r"}, []string{"dir"}},
		// Unix rm normalizes: -R → -r, -fr/-Rf/-fR → -rf
		{"rm -R dir", []string{"-R", "dir"}, []string{"-r"}, []string{"dir"}},
		{"rm -rf dir", []string{"-rf", "dir"}, []string{"-rf"}, []string{"dir"}},
		{"rm -fr dir", []string{"-fr", "dir"}, []string{"-rf"}, []string{"dir"}},
		{"rm -Rf dir", []string{"-Rf", "dir"}, []string{"-rf"}, []string{"dir"}},
		{"rm -fR dir", []string{"-fR", "dir"}, []string{"-rf"}, []string{"dir"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var got []resolvedArg
			var cmd string
			switch {
			case contains(c.args, "dir") && !contains(c.args, "a/b") && !contains(c.args, "somepath"):
				cmd = "rm"
				got = translateRM("linux", c.args)
			case c.name == "ls -la" || c.name == "ls -al" || c.name == "ls -l -a" || c.name == "ls dir":
				cmd = "ls"
				got = translateLS("linux", c.args)
			default:
				cmd = "mkdir"
				got = translateMkdir("linux", c.args)
			}
			_ = cmd
			var raws, uvs []string
			for _, r := range got {
				if r.Raw {
					raws = append(raws, r.Value)
				} else {
					uvs = append(uvs, r.Value)
				}
			}
			if !sliceEq(raws, c.wantRaw) {
				t.Errorf("raw flags = %v, want %v", raws, c.wantRaw)
			}
			if !sliceEq(uvs, c.wantUV) {
				t.Errorf("user values = %v, want %v", uvs, c.wantUV)
			}
		})
	}
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
