package main

import "testing"

func TestBuildPwshCommandLineQuotesUserDataAndKeepsGeneratedFlagsRaw(t *testing.T) {
	args := []resolvedArg{
		{Value: "-Force", Raw: true},
		{Value: "name; Write-Output injected", Raw: false},
	}
	got := buildPwshCommandLine("tool; injected", args, false)
	want := "& 'tool; injected' -Force 'name; Write-Output injected'"
	if got != want {
		t.Fatalf("buildPwshCommandLine = %q, want %q", got, want)
	}
}

func TestBuildShCommandLineQuotesPassthroughCommandAndArguments(t *testing.T) {
	args := []resolvedArg{{Value: "name; rm -rf /", Raw: false}}
	got := buildShCommandLine("tool; injected", args, false)
	want := "'tool; injected' 'name; rm -rf /'"
	if got != want {
		t.Fatalf("buildShCommandLine = %q, want %q", got, want)
	}
}

func TestBuildCommandLinesKeepMappedCommandsRaw(t *testing.T) {
	args := []resolvedArg{{Value: "path with spaces", Raw: false}}
	if got := buildPwshCommandLine("Get-ChildItem", args, true); got != "Get-ChildItem 'path with spaces'" {
		t.Fatalf("mapped pwsh line = %q", got)
	}
	if got := buildShCommandLine("ls", args, true); got != "ls 'path with spaces'" {
		t.Fatalf("mapped sh line = %q", got)
	}
}
