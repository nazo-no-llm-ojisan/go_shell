package main

import (
	"runtime"
	"strings"
)

// ============================================================================
// Layer 0: OS registry
//
// The OS layer orients the pipeline but does NOT restrict which commands
// are allowed. It's a modifier that picks interpretation + execution backend.
//
// -auto and -native are special:
//   -auto   → resolve from runtime.GOOS at execution time
//   -native → bypass shell entirely, exec directly (no /bin/sh, no pwsh)
// ============================================================================

func osList() []string {
	return []string{"win", "linux", "macos", "wsl", "native", "auto"}
}

func isOS(name string) bool {
	for _, o := range osList() {
		if o == name {
			return true
		}
	}
	return false
}

// resolveAutoOS maps runtime.GOOS to a concrete OS mode.
func resolveAutoOS() string {
	switch runtime.GOOS {
	case "windows":
		return "win"
	case "darwin":
		return "macos"
	case "linux":
		// Could be WSL; we treat bare linux as linux unless detected otherwise.
		return "linux"
	default:
		return "linux"
	}
}

// normalizeOS resolves -auto into a concrete OS mode.
func normalizeOS(osName string) string {
	if osName == "auto" {
		return resolveAutoOS()
	}
	return osName
}

// backendFor returns the execution backend kind for a given OS.
// native is resolved at this point (no longer "auto").
func backendFor(osName string) string {
	switch osName {
	case "win":
		return "pwsh"
	case "linux":
		return "sh"
	case "macos":
		return "zsh"
	case "wsl":
		return "wsl"
	case "native":
		return "native"
	default:
		return "native"
	}
}

// shellFor returns the preferred shell binary + default args for the backend.
// Second return is whether the shell exists in PATH.
func shellPathFor(backend string, allowWindowsPwsh bool) (string, []string, error) {
	switch backend {
	case "pwsh":
		return lookupPwsh(allowWindowsPwsh)
	case "sh":
		return "/bin/sh", []string{"-c"}, nil
	case "zsh":
		return "/bin/zsh", []string{"-c"}, nil
	case "wsl":
		return "wsl.exe", []string{"-e", "sh", "-c"}, nil
	case "native":
		return "", nil, nil
	default:
		return "", nil, errUnknownBackend(backend)
	}
}

type errUnknownBackend string

func (e errUnknownBackend) Error() string { return "unknown backend: " + string(e) }

func lookupPwsh(allowWindowsPwsh bool) (string, []string, error) {
	// Prefer pwsh (PowerShell 7+) — UTF-8 safe, no encoding surprises.
	if p, err := execLookPath("pwsh"); err == nil {
		return p, []string{"-NoProfile", "-NonInteractive", "-Command"}, nil
	}
	// Fallback to Windows PowerShell 5.1 only if explicitly allowed.
	// 5.1 has encoding quirks that can corrupt non-ASCII output.
	if allowWindowsPwsh {
		if p, err := execLookPath("powershell"); err == nil {
			return p, []string{"-NoProfile", "-NonInteractive", "-Command"}, nil
		}
	}
	return "", nil, errNoPwsh{}
}

type errNoPwsh struct{}

func (errNoPwsh) Error() string {
	return "pwsh (PowerShell 7) not found in PATH; install PowerShell 7 or pass --allow-windows-powershell"
}

// execLookPath is a seam for testing; real impl in exec_layer.go
var execLookPath = realLookPath

func realLookPath(file string) (string, error) {
	return lookPathImpl(file)
}

// lookPathImpl is set in exec_layer.go to avoid import cycle.
var lookPathImpl = func(file string) (string, error) { return file, nil }

// detectWSL checks if we're running inside WSL by inspecting /proc/version.
// Returns true on WSL environments.
func detectWSL() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	data, err := osReadFile("/proc/version")
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(data)), "microsoft")
}

// osReadFile seam for testing
var osReadFile = realReadFile

func realReadFile(path string) ([]byte, error) {
	return readFileImpl(path)
}

var readFileImpl = func(path string) ([]byte, error) { return nil, nil }