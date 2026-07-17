package main

import "strings"

// ============================================================================
// Layer 2: Argument layer
//
// For mapped commands, raw POSIX-ish flags are translated to the target
// OS's convention. For passthrough commands, args are passed COMPLETELY
// UNCHANGED — no dash stripping, no reordering, no interpretation.
// ============================================================================

// translateArgs converts logical args into OS-specific args.
// Passthrough (mapped=false) → returns rawArgs unchanged (copy).
func translateArgs(logicalCmd, osName string, rawArgs []string, mapped bool) []string {
	if !mapped {
		return append([]string(nil), rawArgs...) //完全不変
	}
	switch logicalCmd {
	case "ls":
		return translateLS(osName, rawArgs)
	case "rm":
		return translateRM(osName, rawArgs)
	case "mkdir":
		return translateMkdir(osName, rawArgs)
	case "touch":
		return translateTouch(osName, rawArgs)
	case "echo":
		return translateEcho(osName, rawArgs)
	case "cat", "cp", "mv", "pwd":
		// mapped but no flag translation needed — pass through unchanged
		return append([]string(nil), rawArgs...)
	default:
		return append([]string(nil), rawArgs...)
	}
}

// stripFlags removes leading dashes from short flags only, for use inside
// mapped command translators. NOT used for passthrough.
func stripFlags(args []string) []string {
	out := make([]string, 0, len(args))
	for _, a := range args {
		out = append(out, stripDash(a))
	}
	return out
}

func translateLS(osName string, rawArgs []string) []string {
	var out []string
	long := false
	all := false
	for _, a := range rawArgs {
		a = stripDash(a)
		switch a {
		case "l":
			long = true
		case "a":
			all = true
		case "al", "la":
			all = true
			long = true
		default:
			out = append(out, a)
		}
	}
	if osName == "win" {
		if all {
			out = append([]string{"-Force"}, out...)
		}
	} else {
		var flags string
		if long && all {
			flags = "-la"
		} else if long {
			flags = "-l"
		} else if all {
			flags = "-a"
		}
		if flags != "" {
			out = append([]string{flags}, out...)
		}
	}
	return out
}

func translateRM(osName string, rawArgs []string) []string {
	var out []string
	recursive := false
	force := false
	for _, a := range rawArgs {
		a = stripDash(a)
		switch a {
		case "r", "R":
			recursive = true
		case "rf":
			recursive = true
			force = true
		case "f":
			force = true
		default:
			out = append(out, a)
		}
	}
	if osName == "win" {
		if recursive {
			out = append([]string{"-Recurse"}, out...)
		}
		if force {
			out = append([]string{"-Force"}, out...)
		}
	} else {
		var flags string
		if recursive && force {
			flags = "-rf"
		} else if recursive {
			flags = "-r"
		} else if force {
			flags = "-f"
		}
		if flags != "" {
			out = append([]string{flags}, out...)
		}
	}
	return out
}

func translateMkdir(osName string, rawArgs []string) []string {
	var out []string
	for _, a := range rawArgs {
		a = stripDash(a)
		if a == "p" {
			if osName == "win" {
				out = append([]string{"-Force"}, out...)
			} else {
				out = append([]string{"-p"}, out...)
			}
			continue
		}
		out = append(out, a)
	}
	return out
}

func translateTouch(osName string, rawArgs []string) []string {
	// touch on Windows is handled at exec layer (composite PowerShell).
	// Args here are just paths — pass through unchanged.
	return append([]string(nil), rawArgs...)
}

// translateEcho handles echo args. On Windows (Write-Output), args with
// spaces need to be quoted at the exec layer; here we just pass them through
// since pwshQuote is applied in exec_layer for non-flag args.
func translateEcho(osName string, rawArgs []string) []string {
	return append([]string(nil), rawArgs...)
}

// ============================================================================
// Shell quoting — prevents arg splitting when args contain spaces
// ============================================================================

// pwshQuote wraps a single argument in PowerShell single-quote escaping.
// Prevents injection when args are joined into a -Command string.
func pwshQuote(s string) string {
	escaped := strings.ReplaceAll(s, "'", "''")
	return "'" + escaped + "'"
}

// shQuote wraps a single argument in POSIX shell single-quote escaping.
// Used for sh/zsh/wsl backends when building a -c command string.
func shQuote(s string) string {
	// Escape embedded single quotes: ' → '\''
	escaped := strings.ReplaceAll(s, "'", "'\\''")
	return "'" + escaped + "'"
}