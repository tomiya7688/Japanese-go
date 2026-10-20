package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/tomiya7688/Japanese-go/internal/transpile"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	var code int
	switch os.Args[1] {
	case "build":
		code = buildCommand(os.Args[2:])
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

func buildCommand(args []string) int {
	input, output, err := parseBuildArgs(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "jgo build:", err)
		return 2
	}
	goFile, cleanup, err := transpileToTemp(input)
	if err != nil {
		fmt.Fprintln(os.Stderr, "jgo build:", err)
		return 1
	}
	defer cleanup()

	if dir := filepath.Dir(output); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			fmt.Fprintln(os.Stderr, "jgo build:", err)
			return 1
		}
	}
	cmd := exec.Command("go", "build", "-o", output, goFile)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return exitCode("jgo build", err)
	}
	return 0
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
	if dir := filepath.Dir(output); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			fmt.Fprintln(os.Stderr, "jgo compile:", err)
			return 1
		}
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

	goFile, cleanup, err := transpileToTemp(input)
	if err != nil {
		fmt.Fprintln(os.Stderr, "jgo run:", err)
		return 1
	}
	defer cleanup()

	cmdArgs := append([]string{"run", goFile}, programArgs...)
	cmd := exec.Command("go", cmdArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return exitCode("jgo run", err)
	}
	return 0
}

func transpileToTemp(input string) (string, func(), error) {
	src, err := os.ReadFile(input)
	if err != nil {
		return "", nil, err
	}
	goSrc, err := transpile.Compile(src)
	if err != nil {
		return "", nil, err
	}
	dir, err := os.MkdirTemp("", "jgo-*")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	mainPath := filepath.Join(dir, "main.go")
	if err := os.WriteFile(mainPath, goSrc, 0o644); err != nil {
		cleanup()
		return "", nil, err
	}
	return mainPath, cleanup, nil
}

func exitCode(prefix string, err error) int {
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode()
	}
	fmt.Fprintln(os.Stderr, prefix+":", err)
	return 1
}

func parseBuildArgs(args []string) (input, output string, err error) {
	input, output, err = parseInputOutputArgs(args)
	if err != nil {
		return "", "", err
	}
	if output == "" {
		output = stringsTrimExt(filepath.Base(input))
		if runtime.GOOS == "windows" {
			output += ".exe"
		}
	}
	return input, output, nil
}

func parseCompileArgs(args []string) (input, output string, err error) {
	input, output, err = parseInputOutputArgs(args)
	if err != nil {
		return "", "", err
	}
	if output == "" {
		output = stringsTrimExt(input) + ".go"
	}
	return input, output, nil
}

func parseInputOutputArgs(args []string) (input, output string, err error) {
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
	return input, output, nil
}

func stringsTrimExt(path string) string {
	ext := filepath.Ext(path)
	return path[:len(path)-len(ext)]
}

func usage() {
	fmt.Fprintln(os.Stderr, `Japanese-go (JGo)

Usage:
  jgo build <file.jgo> [-o binary]
  jgo compile <file.jgo> [-o file.go]
  jgo run <file.jgo> [-- program args...]`)
}
