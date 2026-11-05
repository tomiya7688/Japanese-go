package main

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestParseCompileArgsDefaultsNextToInput(t *testing.T) {
	input, output, err := parseCompileArgs([]string{"examples/natural-syntax.jgo"})
	if err != nil {
		t.Fatalf("parseCompileArgs() error: %v", err)
	}
	if input != "examples/natural-syntax.jgo" {
		t.Fatalf("input = %q", input)
	}
	want := filepath.Join("examples", "natural-syntax.go")
	if output != want {
		t.Fatalf("output = %q, want %q", output, want)
	}
}

func TestParseBuildArgsDefaultsToBinaryName(t *testing.T) {
	input, output, err := parseBuildArgs([]string{"examples/natural-syntax.jgo"})
	if err != nil {
		t.Fatalf("parseBuildArgs() error: %v", err)
	}
	if input != "examples/natural-syntax.jgo" {
		t.Fatalf("input = %q", input)
	}
	want := "natural-syntax"
	if runtime.GOOS == "windows" {
		want += ".exe"
	}
	if output != want {
		t.Fatalf("output = %q, want %q", output, want)
	}
}

func TestParseInputOutputArgsAcceptsOutputFlag(t *testing.T) {
	input, output, err := parseInputOutputArgs([]string{"-o", "bin/app", "main.jgo"})
	if err != nil {
		t.Fatalf("parseInputOutputArgs() error: %v", err)
	}
	if input != "main.jgo" || output != "bin/app" {
		t.Fatalf("got input=%q output=%q", input, output)
	}
}

func TestParseInputOutputArgsRejectsMultipleInputs(t *testing.T) {
	if _, _, err := parseInputOutputArgs([]string{"a.jgo", "b.jgo"}); err == nil {
		t.Fatal("expected error for multiple input files")
	}
}
