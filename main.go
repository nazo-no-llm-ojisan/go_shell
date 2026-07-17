package main

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// metaConfig holds global execution options parsed from -- flags.
type metaConfig struct {
	json             bool
	cwd              string
	timeout          time.Duration
	env              []string
	yes              bool // allow destructive operations
	allowWindowsPwsh bool // allow fallback to PowerShell 5.1
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

// parseMeta consumes leading -- flags. Stops at the first non-- arg.
// Invalid meta values (missing value, unparseable duration, malformed env)
// are fatal — fail-closed is appropriate for an agent execution runtime.
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
		case "--yes":
			meta.yes = true
			i++
		case "--allow-windows-powershell":
			meta.allowWindowsPwsh = true
			i++
		case "--cwd":
			if i+1 >= len(args) {
				fatalMeta("--cwd requires a directory path")
			}
			meta.cwd = args[i+1]
			i += 2
		case "--timeout":
			if i+1 >= len(args) {
				fatalMeta("--timeout requires a duration (e.g. 30s, 2m)")
			}
			d, err := time.ParseDuration(args[i+1])
			if err != nil {
				fatalMeta("--timeout: invalid duration " + args[i+1])
			}
			if d <= 0 {
				fatalMeta("--timeout: must be positive")
			}
			meta.timeout = d
			i += 2
		case "--env":
			if i+1 >= len(args) {
				fatalMeta("--env requires KEY=VALUE")
			}
			if !strings.Contains(args[i+1], "=") {
				fatalMeta("--env: expected KEY=VALUE, got " + args[i+1])
			}
			meta.env = append(meta.env, args[i+1])
			i += 2
		default:
			// Unknown -- flag → fatal (fail-closed)
			fatalMeta("unknown meta flag: " + a)
		}
	}
	return meta, args[i:]
}

func fatalMeta(msg string) {
	fmt.Fprintln(os.Stderr, "go_shell: "+msg)
	os.Exit(2)
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "go_shell — ilu's multi-OS command runtime")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintln(os.Stderr, "  go_shell [meta...] -<os> -<command> [args...]")
	fmt.Fprintln(os.Stderr, "  go_shell [meta...] -<function> [args...]")
	fmt.Fprintln(os.Stderr, "  go_shell [meta...] <command> [args...]  (auto OS)")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Meta:  --json --yes --cwd DIR --timeout DUR --env K=V --allow-windows-powershell")
	fmt.Fprintf(os.Stderr, "OS:    %s\n", strings.Join(osList(), ", "))
	fmt.Fprintf(os.Stderr, "Funcs: %s\n", strings.Join(funcList(), ", "))
}

func stripDash(s string) string {
	return strings.TrimPrefix(s, "-")
}
