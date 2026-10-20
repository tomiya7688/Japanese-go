package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/tomiya7688/Japanese-go/internal/transpile"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	var code int
	switch os.Args[1] {
	case "compile":
		code = compileCommand(os.Args[2:])
	case "run":
		code = runCommand(os.Args[2:])
	case "help", "-h", "--help":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "jgo: unknown command %q\n", os.Args[1])
		usage()
		code = 2
	}
	os.Exit(code)
}

func compileCommand(args []string) int {
	input, output, err := parseCompileArgs(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "jgo compile:", err)
		return 2
	}

	src, err := os.ReadFile(input)
	if err != nil {
		fmt.Fprintln(os.Stderr, "jgo compile:", err)
		return 1
	}
	goSrc, err := transpile.Compile(src)
	if err != nil {
		fmt.Fprintln(os.Stderr, "jgo compile:", err)
		return 1
	}
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil && filepath.Dir(output) != "." {
		fmt.Fprintln(os.Stderr, "jgo compile:", err)
		return 1
	}
	if err := os.WriteFile(output, goSrc, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "jgo compile:", err)
		return 1
	}
	return 0
}

func runCommand(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "jgo run: input .jgo file is required")
		return 2
	}
	input := args[0]
	programArgs := args[1:]
	if len(programArgs) > 0 && programArgs[0] == "--" {
		programArgs = programArgs[1:]
	}

	src, err := os.ReadFile(input)
	if err != nil {
		fmt.Fprintln(os.Stderr, "jgo run:", err)
		return 1
	}
	goSrc, err := transpile.Compile(src)
	if err != nil {
		fmt.Fprintln(os.Stderr, "jgo run:", err)
		return 1
	}

	dir, err := os.MkdirTemp("", "jgo-run-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, "jgo run:", err)
		return 1
	}
	defer os.RemoveAll(dir)
	mainPath := filepath.Join(dir, "main.go")
	if err := os.WriteFile(mainPath, goSrc, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "jgo run:", err)
		return 1
	}

	cmdArgs := append([]string{"run", mainPath}, programArgs...)
	cmd := exec.Command("go", cmdArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode()
		}
		fmt.Fprintln(os.Stderr, "jgo run:", err)
		return 1
	}
	return 0
}

func parseCompileArgs(args []string) (input, output string, err error) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-o":
			if i+1 >= len(args) {
				return "", "", fmt.Errorf("-o requires a path")
			}
			output = args[i+1]
			i++
		default:
			if input != "" {
				return "", "", fmt.Errorf("only one input file is supported for now")
			}
			input = args[i]
		}
	}
	if input == "" {
		return "", "", fmt.Errorf("input .jgo file is required")
	}
	if output == "" {
		base := input[:len(input)-len(filepath.Ext(input))]
		output = base + ".go"
	}
	return input, output, nil
}

func usage() {
	fmt.Fprintln(os.Stderr, `Japanese-go (JGo)

Usage:
  jgo compile <file.jgo> -o <file.go>
  jgo run <file.jgo> [-- program args...]`)
}
