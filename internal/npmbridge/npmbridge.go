package npmbridge

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
)

const OverrideEnv = "TSPACK_NPM"

type Options struct {
	Cwd    string
	Args   []string
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	Env    []string
}

type Result struct {
	Executable string
	ExitCode   int
}

type NotFoundError struct {
	Detail string
}

func (e NotFoundError) Error() string {
	if e.Detail == "" {
		return "npm executable was not found"
	}
	return e.Detail
}

func Locate() (string, error) {
	override := os.Getenv(OverrideEnv)
	if override != "" {
		path, err := exec.LookPath(override)
		if err != nil {
			return "", NotFoundError{Detail: fmt.Sprintf("%s=%q did not resolve to an npm executable", OverrideEnv, override)}
		}
		return path, nil
	}

	candidates := []string{"npm"}
	if runtime.GOOS == "windows" {
		candidates = []string{"npm.cmd", "npm.exe", "npm"}
	}
	for _, candidate := range candidates {
		path, err := exec.LookPath(candidate)
		if err == nil {
			return path, nil
		}
	}
	return "", NotFoundError{Detail: "npm was not found on PATH"}
}

func Run(opts Options) (Result, error) {
	executable, err := Locate()
	if err != nil {
		return Result{ExitCode: 127}, err
	}

	command := exec.Command(executable, opts.Args...)
	command.Dir = opts.Cwd
	command.Stdin = opts.Stdin
	command.Stdout = opts.Stdout
	command.Stderr = opts.Stderr
	if opts.Env != nil {
		command.Env = opts.Env
	}

	err = command.Run()
	if err == nil {
		return Result{Executable: executable, ExitCode: 0}, nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return Result{Executable: executable, ExitCode: exitErr.ExitCode()}, nil
	}
	return Result{Executable: executable, ExitCode: 1}, err
}
