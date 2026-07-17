package main

import (
	"fmt"
	"os"
	"sort"
)

// ============================================================================
// Function call mode
//
// First arg is NOT an OS → treat it as a function name.
// Functions are registered Go functions invoked directly (no shell).
// Unknown function names are rejected (no silent passthrough to shell).
//
// IMPORTANT: function args are DATA — never stripDash them.
// A path like "-file.txt" or content like "-hello" must be preserved as-is.
// ============================================================================

type shellFunc func(args []string, meta *metaConfig) *result

var functionRegistry = map[string]shellFunc{
	"create_hermes_subagent": fnCreateHermesSubagent,
	"run_hermes_task":        fnRunHermesTask,
	"read_hermes_session":    fnReadHermesSession,
	"write_file":             fnWriteFile,
	"copy_file":              fnCopyFile,
	"open_url":               fnOpenURL,
}

func isFunction(name string) bool {
	_, ok := functionRegistry[name]
	return ok
}

func funcList() []string {
	names := make([]string, 0, len(functionRegistry))
	for name := range functionRegistry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func runFunctionMode(name string, args []string, meta *metaConfig) {
	fn, ok := functionRegistry[name]
	if !ok {
		fmt.Fprintf(os.Stderr, "go_shell: unknown function: -%s\n", name)
		os.Exit(2)
	}
	res := fn(args, meta)
	if res == nil {
		res = &result{OK: true, ExitCode: 0}
	}
	res.OSMode = "function"
	res.Backend = "go-internal"
	finalize(res, meta)
}

// --- stub implementations ---
// Stubs return failure (exit 78, "function not implemented") to prevent
// agents from treating unimplemented functions as successful.

const stubExitCode = 78 // EX_CONFIG — function registered but not implemented

func stubResult(name string, args []string) *result {
	return &result{
		OK:              false,
		ExitCode:        stubExitCode,
		Backend:         "go-internal",
		OSMode:          "function",
		ResolvedCommand: fmt.Sprintf("%s(%v)", name, args),
		Stderr:          fmt.Sprintf("go_shell: %s: function not implemented (stub)\n", name),
	}
}

func fnCreateHermesSubagent(args []string, meta *metaConfig) *result {
	return stubResult("create_hermes_subagent", args)
}

func fnRunHermesTask(args []string, meta *metaConfig) *result {
	return stubResult("run_hermes_task", args)
}

func fnReadHermesSession(args []string, meta *metaConfig) *result {
	return stubResult("read_hermes_session", args)
}

// fnWriteFile: write_file <path> <content>
// content is read from stdin if "-" is given, else taken as literal.
// Args are DATA — no stripDash.
func fnWriteFile(args []string, meta *metaConfig) *result {
	if len(args) < 2 {
		return &result{
			OK:       false,
			ExitCode: 2,
			Stderr:   "write_file: requires <path> <content>",
		}
	}
	path := args[0]
	content := args[1]
	if content == "-" {
		buf := make([]byte, 0, 4096)
		tmp := make([]byte, 4096)
		for {
			n, err := os.Stdin.Read(tmp)
			if n > 0 {
				buf = append(buf, tmp[:n]...)
			}
			if err != nil {
				break
			}
		}
		content = string(buf)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return &result{OK: false, ExitCode: 1, Stderr: err.Error()}
	}
	return &result{
		OK:              true,
		ExitCode:        0,
		Backend:         "go-internal",
		ResolvedCommand: fmt.Sprintf("write_file(%s, %d bytes)", path, len(content)),
		Stdout:          fmt.Sprintf("wrote %d bytes to %s\n", len(content), path),
	}
}

func fnCopyFile(args []string, meta *metaConfig) *result {
	if len(args) < 2 {
		return &result{
			OK:       false,
			ExitCode: 2,
			Stderr:   "copy_file: requires <src> <dst>",
		}
	}
	src := args[0]
	dst := args[1]
	data, err := os.ReadFile(src)
	if err != nil {
		return &result{OK: false, ExitCode: 1, Stderr: err.Error()}
	}
	if err := os.WriteFile(dst, data, 0644); err != nil {
		return &result{OK: false, ExitCode: 1, Stderr: err.Error()}
	}
	return &result{
		OK:              true,
		ExitCode:        0,
		Backend:         "go-internal",
		ResolvedCommand: fmt.Sprintf("copy_file(%s, %s)", src, dst),
		Stdout:          fmt.Sprintf("copied %d bytes: %s → %s\n", len(data), src, dst),
	}
}

func fnOpenURL(args []string, meta *metaConfig) *result {
	if len(args) < 1 {
		return &result{OK: false, ExitCode: 2, Stderr: "open_url: requires <url>"}
	}
	// Stub — actual OS-specific open command to be wired later.
	return stubResult("open_url", args)
}