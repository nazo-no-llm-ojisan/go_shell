package main

import "testing"

func TestTranslateEchoWindowsJoinsArguments(t *testing.T) {
	got := translateEcho("win", []string{"hello", "world"})
	if len(got) != 1 || got[0].Value != "hello world" || got[0].Raw {
		t.Fatalf("translateEcho(win) = %+v, want one user value %q", got, "hello world")
	}
}

func TestTranslateTouchWindowsConsumesEndOfOptions(t *testing.T) {
	got := translateTouch("win", []string{"--", "-important"})
	if len(got) != 1 || got[0].Value != "-important" || got[0].Raw {
		t.Fatalf("translateTouch(win) = %+v, want one user path %q", got, "-important")
	}
}

func TestTranslateTouchPOSIXPreservesEndOfOptions(t *testing.T) {
	got := translateTouch("linux", []string{"--", "-important"})
	if len(got) != 2 || got[0].Value != "--" || got[1].Value != "-important" {
		t.Fatalf("translateTouch(linux) = %+v, want separator preserved", got)
	}
}
