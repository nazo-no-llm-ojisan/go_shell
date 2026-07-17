package main

import (
	"fmt"
	"os"
	"testing"
	"time"
)

func TestLimitedBufferCapsStoredBytesWithoutShortWrite(t *testing.T) {
	buf := newLimitedBuffer(5)

	if n, err := buf.Write([]byte("abc")); err != nil || n != 3 {
		t.Fatalf("first write = (%d, %v), want (3, nil)", n, err)
	}
	if n, err := buf.Write([]byte("defg")); err != nil || n != 4 {
		t.Fatalf("second write = (%d, %v), want (4, nil)", n, err)
	}
	if got := buf.String(); got != "abcde" {
		t.Fatalf("stored output = %q, want %q", got, "abcde")
	}
	if !buf.Truncated() {
		t.Fatal("Truncated() = false, want true")
	}
}

func TestLimitedBufferDoesNotMarkExactLimitAsTruncated(t *testing.T) {
	buf := newLimitedBuffer(5)
	if _, err := buf.Write([]byte("abcde")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if buf.Truncated() {
		t.Fatal("Truncated() = true at exact limit, want false")
	}
}

func TestOutputHelperProcess(t *testing.T) {
	if os.Getenv("GO_SHELL_OUTPUT_HELPER") != "1" {
		return
	}
	fmt.Print("abcdefghij")
	os.Exit(0)
}

func TestExecuteCapsOutputAndReportsTruncation(t *testing.T) {
	t.Setenv("GO_SHELL_OUTPUT_HELPER", "1")
	meta := &metaConfig{timeout: 5 * time.Second, maxOutputBytes: 5}
	res := newResult("native", "native", "output helper")
	got := execute(res, "native", os.Args[0], []resolvedArg{{Value: "-test.run=^TestOutputHelperProcess$"}}, meta, false)
	if !got.OK {
		t.Fatalf("execute failed: %+v", got)
	}
	if got.Stdout != "abcde" || !got.StdoutTruncated {
		t.Fatalf("stdout=%q truncated=%v, want %q true", got.Stdout, got.StdoutTruncated, "abcde")
	}
}
