package main

import (
	"os"
	"os/exec"
	"runtime"
)

// ============================================================================
// exec seams — wired here to keep os_layer.go free of direct os/exec import.
// ============================================================================

func init() {
	lookPathImpl = exec.LookPath
	readFileImpl = os.ReadFile
}

// osGoos is a seam for runtime.GOOS, set at init time. Tests can read it
// to decide platform-specific behavior without importing runtime directly.
var osGoos = runtime.GOOS
