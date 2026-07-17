package main

// ============================================================================
// Layer 1: Command layer
//
// Maps a logical command name to the OS-specific equivalent.
// If no mapping exists → passthrough (use the command name as-is).
// ============================================================================

// commandTable[logicalName][osName] = concrete command string
var commandTable = map[string]map[string]string{
	"ls": {
		"win":   "Get-ChildItem",
		"linux": "ls",
		"macos": "ls",
		"wsl":   "ls",
	},
	"mkdir": {
		"win":   "New-Item -ItemType Directory",
		"linux": "mkdir",
		"macos": "mkdir",
		"wsl":   "mkdir",
	},
	"rm": {
		"win":   "Remove-Item",
		"linux": "rm",
		"macos": "rm",
		"wsl":   "rm",
	},
	"rmdir": {
		"win":   "Remove-Item",
		"linux": "rmdir",
		"macos": "rmdir",
		"wsl":   "rmdir",
	},
	"cat": {
		"win":   "Get-Content",
		"linux": "cat",
		"macos": "cat",
		"wsl":   "cat",
	},
	"cp": {
		"win":   "Copy-Item",
		"linux": "cp",
		"macos": "cp",
		"wsl":   "cp",
	},
	"mv": {
		"win":   "Move-Item",
		"linux": "mv",
		"macos": "mv",
		"wsl":   "mv",
	},
	"touch": {
		"win":   "New-Item -ItemType File",
		"linux": "touch",
		"macos": "touch",
		"wsl":   "touch",
	},
	"pwd": {
		"win":   "Get-Location",
		"linux": "pwd",
		"macos": "pwd",
		"wsl":   "pwd",
	},
	"echo": {
		"win":   "Write-Output",
		"linux": "echo",
		"macos": "echo",
		"wsl":   "echo",
	},
}

func resolveCommand(logicalName, osName string) (string, bool) {
	osMap, ok := commandTable[logicalName]
	if !ok {
		return logicalName, false // passthrough
	}
	cmd, ok := osMap[osName]
	if !ok {
		return logicalName, false // no entry for this OS → passthrough
	}
	return cmd, true
}
