package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// ============================================================================
// Layer 3: Execution layer
//
// Decides HOW to run the resolved command and produces structured results.
// Backends:  pwsh | sh | zsh | wsl | native
// ============================================================================

type result struct {
	OK              bool   `json:"ok"`
	ExitCode        int    `json:"exit_code"`
	Stdout          string `json:"stdout"`
	Stderr          string `json:"stderr"`
	Backend         string `json:"backend"`
	OSMode          string `json:"os_mode"`
	ResolvedCommand string `json:"resolved_command"`
	Duration        string `json:"duration"`
	Warning         string `json:"warning,omitempty"`
	DryRun          bool   `json:"dry_run,omitempty"`
}

func newResult(osName, backend, resolved string) *result {
	return &result{
		OK:              true,
		ExitCode:        0,
		Backend:         backend,
		OSMode:          osName,
		ResolvedCommand: resolved,
	}
}

// runOSMode drives the 4-layer pipeline.
func runOSMode(osName string, args []string, meta *metaConfig) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "go_shell: no command given")
		os.Exit(2)
	}

	concrete := resolveAutoOSIfAuto(osName)
	logicalCmd := stripDash(args[0])
	rawArgs := args[1:]

	concreteCmd, mapped := resolveCommand(logicalCmd, concrete)
	translatedArgs := translateArgs(logicalCmd, concrete, rawArgs, mapped)

	// Destructive operation check
	if isDestructive(logicalCmd) && !meta.yes {
		dryRunResult := newResult(concrete, backendFor(concrete), joinCommand(concreteCmd, translatedArgs))
		dryRunResult.OK = true
		dryRunResult.DryRun = true
		dryRunResult.Stdout = fmt.Sprintf("[dry-run] destructive operation blocked without --yes\n  resolved: %s\n  args: %s\n", dryRunResult.ResolvedCommand, argSummary(translatedArgs))
		finalize(dryRunResult, meta)
		return
	}

	// touch on Windows needs composite PowerShell (not a simple cmdlet)
	if logicalCmd == "touch" && concrete == "win" {
		runTouchWindows(translatedArgs, meta, concrete)
		return
	}

	backend := backendFor(concrete)
	resolvedStr := joinCommand(concreteCmd, translatedArgs)

	res := newResult(concrete, backend, resolvedStr)
	res = execute(res, backend, concreteCmd, translatedArgs, meta)

	// Warn if fell back to Windows PowerShell 5.1
	if backend == "pwsh" && strings.HasSuffix(res.Backend, "5.1") {
		res.Warning = "PowerShell 7 (pwsh) was not found; using Windows PowerShell 5.1 — output encoding may differ."
	}

	finalize(res, meta)
}

func resolveAutoOSIfAuto(osName string) string {
	if osName == "auto" {
		return resolveAutoOS()
	}
	return osName
}

// joinCommand builds a display string from cmd + resolvedArgs.
func joinCommand(cmd string, args []resolvedArg) string {
	parts := []string{cmd}
	for _, a := range args {
		parts = append(parts, a.Value)
	}
	return strings.Join(parts, " ")
}

// argSummary returns a debug summary of resolvedArgs (value + raw flag).
func argSummary(args []resolvedArg) string {
	parts := make([]string, 0, len(args))
	for _, a := range args {
		if a.Raw {
			parts = append(parts, fmt.Sprintf("%s(raw)", a.Value))
		} else {
			parts = append(parts, a.Value)
		}
	}
	return "[" + strings.Join(parts, " ") + "]"
}

// isDestructive returns true for mapped commands that can delete data.
// NOTE: This is a UX guard against accidental mapped -rm/-rmdir, NOT a
// security boundary. Passthrough commands (Remove-Item, cmd /c del, etc.)
// are not intercepted. See README Security section.
func isDestructive(logicalCmd string) bool {
	switch logicalCmd {
	case "rm", "rmdir":
		return true
	}
	return false
}

// runTouchWindows implements Unix-compatible touch on Windows via composite PowerShell.
func runTouchWindows(args []resolvedArg, meta *metaConfig, osName string) {
	shellPath, shellArgs, err := shellPathFor("pwsh", meta.allowWindowsPwsh)
	if err != nil {
		res := &result{OK: false, ExitCode: 127, Stderr: err.Error(), OSMode: osName, Backend: "pwsh"}
		finalize(res, meta)
		return
	}

	// Build: foreach ($p in 'path1','path2') { if (Test-Path -LiteralPath $p) { ... } else { ... } }
	// All user paths are pwshQuote'd to prevent injection.
	var paths []string
	var displayPaths []string
	for _, a := range args {
		paths = append(paths, pwshQuote(a.Value))
		displayPaths = append(displayPaths, a.Value)
	}
	script := fmt.Sprintf(
		"[Console]::OutputEncoding=[System.Text.Encoding]::UTF8; foreach ($p in %s) { if (Test-Path -LiteralPath $p) { (Get-Item -LiteralPath $p).LastWriteTime = Get-Date } else { New-Item -ItemType File -Path $p | Out-Null } }",
		strings.Join(paths, ","))
	fullArgs := append(append([]string{}, shellArgs...), script)

	ctx, cancel := context.WithTimeout(context.Background(), meta.timeout)
	defer cancel()
	c := exec.CommandContext(ctx, shellPath, fullArgs...)
	if meta.cwd != "" {
		c.Dir = meta.cwd
	}
	c.Env = mergeEnv(meta.env)
	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr
	c.Stdin = os.Stdin

	backend := "pwsh"
	if !strings.Contains(shellPath, "pwsh") && !strings.Contains(shellPath, "pwsh.exe") {
		backend = "powershell-5.1"
	}
	res := &result{
		OK:              true,
		Backend:         backend,
		OSMode:          osName,
		ResolvedCommand: "touch(composite) " + strings.Join(displayPaths, " "),
	}

	start := time.Now()
	runErr := c.Run()
	res.Duration = time.Since(start).String()
	res.Stdout = stdout.String()
	res.Stderr = stderr.String()
	if runErr != nil {
		res.OK = false
		// Check timeout FIRST — CommandContext kill often surfaces as ExitError
		if ctx.Err() == context.DeadlineExceeded {
			res.ExitCode = 124
			res.Stderr = stderr.String() + "go_shell: timeout after " + meta.timeout.String()
		} else if exitErr, ok := runErr.(*exec.ExitError); ok {
			res.ExitCode = exitErr.ExitCode()
		} else {
			res.ExitCode = 1
			res.Stderr = stderr.String() + runErr.Error()
		}
	}
	if backend == "powershell-5.1" {
		res.Warning = "PowerShell 7 (pwsh) was not found; using Windows PowerShell 5.1 — output encoding may differ."
	}
	writeLog(res)
	finalize(res, meta)
}

func execute(res *result, backend, cmd string, args []resolvedArg, meta *metaConfig) *result {
	ctx, cancel := context.WithTimeout(context.Background(), meta.timeout)
	defer cancel()

	var c *exec.Cmd
	switch backend {
	case "pwsh":
		// Build a single -Command string. cmd is a translator-generated
		// syntax fragment (e.g. "Get-ChildItem", "New-Item -ItemType Directory").
		// Each arg is either Raw (translator flag like -Force) or a user value
		// (always pwshQuote'd to prevent injection).
		line := "[Console]::OutputEncoding=[System.Text.Encoding]::UTF8; [Console]::InputEncoding=[System.Text.Encoding]::UTF8; " + cmd
		for _, a := range args {
			if a.Raw {
				line += " " + a.Value
			} else {
				line += " " + pwshQuote(a.Value)
			}
		}
		shellPath, shellArgs, err := shellPathFor(backend, meta.allowWindowsPwsh)
		if err != nil {
			res.OK = false
			res.ExitCode = 127
			res.Stderr = err.Error()
			return res
		}
		if !strings.Contains(shellPath, "pwsh") {
			res.Backend = "powershell-5.1"
		}
		fullArgs := append(append([]string{}, shellArgs...), line)
		c = exec.CommandContext(ctx, shellPath, fullArgs...)
	case "native":
		// Direct exec — no shell. args are passed as argv (no quoting needed).
		rawArgs := make([]string, 0, len(args))
		for _, a := range args {
			rawArgs = append(rawArgs, a.Value)
		}
		c = exec.CommandContext(ctx, cmd, rawArgs...)
	case "sh", "zsh", "wsl":
		shellPath, shellArgs, err := shellPathFor(backend, meta.allowWindowsPwsh)
		if err != nil {
			res.OK = false
			res.ExitCode = 127
			res.Stderr = err.Error()
			return res
		}
		// Quote each arg with shQuote. Raw flags are also quoted for POSIX
		// shells since "-Force" is just a string to sh.
		line := cmd
		for _, a := range args {
			line += " " + shQuote(a.Value)
		}
		fullArgs := append(append([]string{}, shellArgs...), line)
		c = exec.CommandContext(ctx, shellPath, fullArgs...)
	default:
		res.OK = false
		res.ExitCode = 127
		res.Stderr = "unknown backend: " + backend
		return res
	}

	if meta.cwd != "" {
		c.Dir = meta.cwd
	}
	c.Env = mergeEnv(meta.env)

	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr
	c.Stdin = os.Stdin

	start := time.Now()
	err := c.Run()
	res.Duration = time.Since(start).String()
	res.Stdout = stdout.String()
	res.Stderr = stderr.String()

	if err != nil {
		res.OK = false
		// Check timeout FIRST — CommandContext kill often surfaces as ExitError
		if ctx.Err() == context.DeadlineExceeded {
			res.ExitCode = 124
			res.Stderr = stderr.String() + "go_shell: timeout after " + meta.timeout.String()
		} else if exitErr, ok := err.(*exec.ExitError); ok {
			res.ExitCode = exitErr.ExitCode()
		} else {
			res.ExitCode = 1
			if res.Stderr == "" {
				res.Stderr = err.Error()
			}
		}
	}

	writeLog(res)
	return res
}

func finalize(res *result, meta *metaConfig) {
	if meta.json {
		out, _ := json.MarshalIndent(res, "", "  ")
		fmt.Println(string(out))
	} else {
		os.Stdout.WriteString(res.Stdout)
		os.Stderr.WriteString(res.Stderr)
	}
	if !res.OK {
		os.Exit(res.ExitCode)
	}
}

// mergeEnv merges extra K=V pairs into the current environment.
// On Windows, env var names are case-insensitive (PATH == Path).
// On Linux/macOS, names are case-sensitive (PATH != Path).
func mergeEnv(extra []string) []string {
	if len(extra) == 0 {
		return os.Environ()
	}
	envMap := make(map[string]string)
	keyOrder := make(map[string]string)
	normalize := envKeyNormalizer()

	for _, kv := range os.Environ() {
		key := kv
		if idx := strings.Index(kv, "="); idx >= 0 {
			key = kv[:idx]
		}
		nk := normalize(key)
		envMap[nk] = kv
		if _, exists := keyOrder[nk]; !exists {
			keyOrder[nk] = key
		}
	}
	for _, kv := range extra {
		key := kv
		val := ""
		if idx := strings.Index(kv, "="); idx >= 0 {
			key = kv[:idx]
			val = kv[idx+1:]
		}
		nk := normalize(key)
		if _, exists := keyOrder[nk]; !exists {
			keyOrder[nk] = key
		}
		envMap[nk] = key + "=" + val
	}
	out := make([]string, 0, len(envMap))
	for _, kv := range envMap {
		out = append(out, kv)
	}
	return out
}

func envKeyNormalizer() func(string) string {
	if runtime.GOOS == "windows" {
		return strings.ToUpper
	}
	return func(s string) string { return s }
}

// ============================================================================
// Execution log
// ============================================================================

func logPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".go_shell", "log.jsonl")
}

func writeLog(res *result) {
	entry := map[string]any{
		"ts":               time.Now().Format(time.RFC3339),
		"ok":               res.OK,
		"exit_code":        res.ExitCode,
		"backend":          res.Backend,
		"os_mode":          res.OSMode,
		"resolved_command": res.ResolvedCommand,
		"duration":         res.Duration,
		"stdout_len":       len(res.Stdout),
		"stderr_len":       len(res.Stderr),
	}
	line, _ := json.Marshal(entry)
	_ = os.MkdirAll(filepath.Dir(logPath()), 0755)
	f, err := os.OpenFile(logPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	f.Write(line)
	f.Write([]byte("\n"))
}