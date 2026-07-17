package main

import "strings"

// ============================================================================
// Layer 2: Argument layer
//
// Interprets arguments and absorbs OS-specific differences.
// For mapped commands, raw POSIX-ish flags are translated to the target
// OS's convention. For passthrough commands, args are passed unchanged.
// ============================================================================

func translateArgs(logicalCmd, osName string, rawArgs []string, mapped bool) []string {
	if !mapped {
		return cleanArgs(rawArgs)
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
	case "cat", "cp", "mv", "pwd", "echo":
		return cleanArgs(rawArgs)
	default:
		return cleanArgs(rawArgs)
	}
}

// cleanArgs strips a single leading dash from each arg so the execution
// layer receives bare tokens, but preserves long options (--foo) as-is
// since those are meaningful for passthrough commands (e.g. git --short).
func cleanArgs(args []string) []string {
	out := make([]string, 0, len(args))
	for _, a := range args {
		if strings.HasPrefix(a, "--") {
			out = append(out, a) // preserve long options
		} else {
			out = append(out, stripDash(a))
		}
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
	// New-Item -ItemType File already fails if file exists; touch semantics
	// differ, but for the simple case we just pass the filename.
	return cleanArgs(rawArgs)
}

// pwshQuote wraps a single argument in PowerShell single-quote escaping.
// This prevents injection when args are joined into a -Command string.
func pwshQuote(s string) string {
	// Escape embedded single quotes: ' → ''
	escaped := strings.ReplaceAll(s, "'", "''")
	return "'" + escaped + "'"
}