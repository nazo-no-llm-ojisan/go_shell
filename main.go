package main

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// metaConfig holds global execution options parsed from -- flags.
type metaConfig struct {
	json    bool
	cwd     string
	timeout time.Duration
	env     []string
}

func main() {
	rawArgs := os.Args[1:]
	meta, rest := parseMeta(rawArgs)

	if len(rest) == 0 {
		printUsage()
		os.Exit(2)
	}

	first := rest[0]

	if !strings.HasPrefix(first, "-") {
		// bare first arg → auto OS mode (command with no OS specifier)
		runOSMode("auto", rest, meta)
		return
	}

	stripped := stripDash(first)
	switch {
	case isOS(stripped):
		runOSMode(stripped, rest[1:], meta)
	case isFunction(stripped):
		runFunctionMode(stripped, rest[1:], meta)
	default:
		// not OS, not function → reject as likely hallucinated call
		fmt.Fprintf(os.Stderr, "go_shell: unknown OS or function: -%s\n", stripped)
		os.Exit(2)
	}
}

// parseMeta consumes leading -- flags. Stops at the first non-- arg or
// unknown -- arg (which becomes part of the command).
func parseMeta(args []string) (*metaConfig, []string) {
	meta := &metaConfig{timeout: 60 * time.Second}
	i := 0
	for i < len(args) {
		a := args[i]
		if !strings.HasPrefix(a, "--") {
			break
		}
		switch a {
		case "--json":
			meta.json = true
			i++
		case "--cwd":
			if i+1 < len(args) {
				meta.cwd = args[i+1]
				i += 2
			} else {
				i++
			}
		case "--timeout":
			if i+1 < len(args) {
				if d, err := time.ParseDuration(args[i+1]); err == nil {
					meta.timeout = d
				}
				i += 2
			} else {
				i++
			}
		case "--env":
			if i+1 < len(args) {
				meta.env = append(meta.env, args[i+1])
				i += 2
			} else {
				i++
			}
		default:
			return meta, args[i:]
		}
	}
	return meta, args[i:]
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "go_shell — ilu's multi-OS command runtime")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintln(os.Stderr, "  go_shell [meta...] -<os> -<command> [args...]")
	fmt.Fprintln(os.Stderr, "  go_shell [meta...] -<function> [args...]")
	fmt.Fprintln(os.Stderr, "  go_shell [meta...] <command> [args...]  (auto OS)")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Meta:  --json --cwd DIR --timeout DUR --env K=V")
	fmt.Fprintf(os.Stderr, "OS:    %s\n", strings.Join(osList(), ", "))
	fmt.Fprintf(os.Stderr, "Funcs: %s\n", strings.Join(funcList(), ", "))
}

func stripDash(s string) string {
	return strings.TrimPrefix(s, "-")
}