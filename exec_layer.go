package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

	backend := backendFor(concrete)
	resolvedStr := joinCommand(concreteCmd, translatedArgs)

	res := newResult(concrete, backend, resolvedStr)
	res = execute(res, backend, concreteCmd, translatedArgs, meta)

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

func execute(res *result, backend, cmd string, args []string, meta *metaConfig) *result {
	ctx, cancel := context.WithTimeout(context.Background(), meta.timeout)
	defer cancel()

	var c *exec.Cmd
	switch backend {
	case "pwsh":
		// Build a single -Command string with proper single-quote escaping.
		// cmd itself may contain spaces (e.g. "New-Item -ItemType Directory"),
		// so we treat it as a command fragment and quote each arg.
		line := cmd
		for _, a := range args {
			if strings.HasPrefix(a, "-") {
				line += " " + a // flag-style: pass as-is
			} else {
				line += " " + pwshQuote(a)
			}
		}
		shellPath, shellArgs, err := shellPathFor(backend)
		if err != nil {
			res.OK = false
			res.ExitCode = 127
			res.Stderr = err.Error()
			return res
		}
		fullArgs := append(append([]string{}, shellArgs...), line)
		c = exec.CommandContext(ctx, shellPath, fullArgs...)
	case "native":
		c = exec.CommandContext(ctx, cmd, args...)
	case "sh", "zsh", "wsl":
		shellPath, shellArgs, err := shellPathFor(backend)
		if err != nil {
			res.OK = false
			res.ExitCode = 127
			res.Stderr = err.Error()
			return res
		}
		line := joinCommand(cmd, args)
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
	// stdin passthrough
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

	// Write execution log (best-effort, never fatal)
	writeLog(res)

	return res
}

func finalize(res *result, meta *metaConfig) {
	if meta.json {
		out, _ := json.MarshalIndent(res, "", "  ")
		fmt.Println(string(out))
	} else {
		// passthrough: write stdout/stderr to streams, exit with code
		os.Stdout.WriteString(res.Stdout)
		os.Stderr.WriteString(res.Stderr)
	}
	if !res.OK {
		os.Exit(res.ExitCode)
	}
}

func mergeEnv(extra []string) []string {
	env := os.Environ()
	for _, kv := range extra {
		// override existing key if present
		key := kv
		if idx := strings.Index(kv, "="); idx >= 0 {
			key = kv[:idx]
		}
		env = append(env, kv)
		// Remove duplicates: keep last
		_ = key
	}
	return env
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
		"ts":              time.Now().Format(time.RFC3339),
		"ok":              res.OK,
		"exit_code":       res.ExitCode,
		"backend":         res.Backend,
		"os_mode":         res.OSMode,
		"resolved_command": res.ResolvedCommand,
		"duration":        res.Duration,
		"stdout_len":      len(res.Stdout),
		"stderr_len":      len(res.Stderr),
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