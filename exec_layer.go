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
	if isDestructive(logicalCmd, translatedArgs) && !meta.yes {
		dryRunResult := newResult(concrete, backendFor(concrete), joinCommand(concreteCmd, translatedArgs))
		dryRunResult.OK = true
		dryRunResult.DryRun = true
		dryRunResult.Stdout = fmt.Sprintf("[dry-run] destructive operation blocked without --yes\n  resolved: %s\n  args: %v\n", dryRunResult.ResolvedCommand, translatedArgs)
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

func joinCommand(cmd string, args []string) string {
	parts := append([]string{cmd}, args...)
	return strings.Join(parts, " ")
}

// isDestructive returns true for commands that can delete or overwrite
// significant amounts of data, requiring --yes to execute.
func isDestructive(logicalCmd string, args []string) bool {
	switch logicalCmd {
	case "rm":
		return true // any rm is destructive
	case "rmdir":
		return true
	}
	return false
}

// runTouchWindows implements Unix-compatible touch on Windows via composite PowerShell.
func runTouchWindows(args []string, meta *metaConfig, osName string) {
	shellPath, shellArgs, err := shellPathFor("pwsh", meta.allowWindowsPwsh)
	if err != nil {
		res := &result{OK: false, ExitCode: 127, Stderr: err.Error(), OSMode: osName, Backend: "pwsh"}
		finalize(res, meta)
		return
	}

	// Build: foreach ($p in 'path1','path2') { if (Test-Path -LiteralPath $p) { (Get-Item -LiteralPath $p).LastWriteTime = Get-Date } else { New-Item -ItemType File -Path $p } | Out-Null }
	var paths []string
	for _, a := range args {
		paths = append(paths, pwshQuote(a))
	}
	script := fmt.Sprintf(
		"foreach ($p in %s) { if (Test-Path -LiteralPath $p) { (Get-Item -LiteralPath $p).LastWriteTime = Get-Date } else { New-Item -ItemType File -Path $p | Out-Null } }",
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
		ResolvedCommand: "touch(composite) " + strings.Join(args, " "),
	}
	start := time.Now()
	runErr := c.Run()
	res.Duration = time.Since(start).String()
	// Capture stdout/stderr FIRST, then layer error/timeout info on top.
	res.Stdout = stdout.String()
	res.Stderr = stderr.String()
	if runErr != nil {
		res.OK = false
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			res.ExitCode = exitErr.ExitCode()
		} else if ctx.Err() == context.DeadlineExceeded {
			res.ExitCode = 124
			res.Stderr = stderr.String() + "go_shell: timeout after " + meta.timeout.String()
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

func execute(res *result, backend, cmd string, args []string, meta *metaConfig) *result {
	ctx, cancel := context.WithTimeout(context.Background(), meta.timeout)
	defer cancel()

	var c *exec.Cmd
	switch backend {
	case "pwsh":
		// Build a single -Command string with proper single-quote escaping.
		// cmd itself may contain spaces (e.g. "New-Item -ItemType Directory"),
		// so we treat it as a command fragment and quote each arg.
		// Prefix with UTF-8 encoding setup to prevent CJK/coded text corruption.
		line := "[Console]::OutputEncoding=[System.Text.Encoding]::UTF8; [Console]::InputEncoding=[System.Text.Encoding]::UTF8; " + cmd
		for _, a := range args {
			if strings.HasPrefix(a, "-") {
				line += " " + a // flag-style: pass as-is
			} else {
				line += " " + pwshQuote(a)
			}
		}
		shellPath, shellArgs, err := shellPathFor(backend, meta.allowWindowsPwsh)
		if err != nil {
			res.OK = false
			res.ExitCode = 127
			res.Stderr = err.Error()
			return res
		}
		// Track if we fell back to 5.1
		if !strings.Contains(shellPath, "pwsh") {
			res.Backend = "powershell-5.1"
		}
		fullArgs := append(append([]string{}, shellArgs...), line)
		c = exec.CommandContext(ctx, shellPath, fullArgs...)
	case "native":
		c = exec.CommandContext(ctx, cmd, args...)
	case "sh", "zsh", "wsl":
		shellPath, shellArgs, err := shellPathFor(backend, meta.allowWindowsPwsh)
		if err != nil {
			res.OK = false
			res.ExitCode = 127
			res.Stderr = err.Error()
			return res
		}
		// Quote each arg with shQuote to prevent word-splitting on spaces.
		line := cmd
		for _, a := range args {
			line += " " + shQuote(a)
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
		if exitErr, ok := err.(*exec.ExitError); ok {
			res.ExitCode = exitErr.ExitCode()
		} else if ctx.Err() == context.DeadlineExceeded {
			res.ExitCode = 124
			res.Stderr = "go_shell: timeout after " + meta.timeout.String()
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
// Go's runtime.GOOS is used to pick the correct comparison strategy.
func mergeEnv(extra []string) []string {
	if len(extra) == 0 {
		return os.Environ()
	}
	// Build a map keyed by the comparison form (upper on Windows, as-is elsewhere).
	envMap := make(map[string]string) // compareKey → "KEY=VALUE" string
	keyOrder := make(map[string]string) // compareKey → original key (first seen)
	normalize := envKeyNormalizer() // func(string) string

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

// envKeyNormalizer returns a function that normalizes env var keys for
// comparison. On Windows, keys are upper-cased (case-insensitive).
// On Linux/macOS, keys are returned as-is (case-sensitive).
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