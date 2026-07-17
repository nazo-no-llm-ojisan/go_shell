package main

import (
	"os"
	"strings"
	"testing"
)

func TestCIExplicitlyDisablesGoCacheWithoutDependencies(t *testing.T) {
	data, err := os.ReadFile(".github/workflows/ci.yml")
	if err != nil {
		t.Fatalf("read CI workflow: %v", err)
	}
	if got := strings.Count(string(data), "cache: false"); got != 2 {
		t.Fatalf("cache: false occurrences = %d, want 2 (Windows and Linux)", got)
	}
}
