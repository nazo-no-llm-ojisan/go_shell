package main

import (
	"os"
	"os/exec"
)

// ============================================================================
// exec seams — wired here to keep os_layer.go free of direct os/exec import.
// ============================================================================

func init() {
	lookPathImpl = exec.LookPath
	readFileImpl = os.ReadFile
}