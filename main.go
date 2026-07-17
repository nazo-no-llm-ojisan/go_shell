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
	meta, rest, err := parseMetaChecked(rawArgs)
	if err != nil {
		fail(meta, 2, err.Error())
		return
	}

	if len(rest) == 0 {
		if meta.json {
			fail(meta, 2, "no command given")
			return
		}
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
		fail(meta, 2, fmt.Sprintf("unknown OS or function: -%s", stripped))
	}
}

// parseMeta consumes leading -- flags. Stops at the first non-- arg.
// Invalid meta values (missing value, unparseable duration, malformed env)
// are fatal — fail-closed is appropriate for an agent execution runtime.
func parseMeta(args []string) (*metaConfig, []string) {
	meta, rest, err := parseMetaChecked(args)
	if err != nil {
		fatalMeta(err.Error())
	}
	return meta, rest
}

func parseMetaChecked(args []string) (*metaConfig, []string, error) {
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
				return meta, nil, fmt.Errorf("--cwd requires a directory path")
			}
			meta.cwd = args[i+1]
			i += 2
		case "--timeout":
			if i+1 >= len(args) {
				return meta, nil, fmt.Errorf("--timeout requires a duration (e.g. 30s, 2m)")
			}
			d, err := time.ParseDuration(args[i+1])
			if err != nil {
				return meta, nil, fmt.Errorf("--timeout: invalid duration %s", args[i+1])
			}
			if d <= 0 {
				return meta, nil, fmt.Errorf("--timeout: must be positive")
			}
			meta.timeout = d
			i += 2
		case "--env":
			if i+1 >= len(args) {
				return meta, nil, fmt.Errorf("--env requires KEY=VALUE")
			}
			if !strings.Contains(args[i+1], "=") {
				return meta, nil, fmt.Errorf("--env: expected KEY=VALUE, got %s", args[i+1])
			}
			meta.env = append(meta.env, args[i+1])
			i += 2
		default:
			// Unknown -- flag → fatal (fail-closed)
			return meta, nil, fmt.Errorf("unknown meta flag: %s", a)
		}
	}
	return meta, args[i:], nil
}

func fail(meta *metaConfig, exitCode int, message string) {
	finalize(&result{
		OK:       false,
		ExitCode: exitCode,
		Stderr:   "go_shell: " + message + "\n",
		Backend:  "go-internal",
		OSMode:   "meta",
	}, meta)
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
