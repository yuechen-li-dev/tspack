package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
)

// inProcessCommand is a narrow migration adapter for older CLI tests that used
// exec.Cmd only for Run, Output, or CombinedOutput. It intentionally does not
// implement Start, Wait, signals, or process access: those contracts must keep
// using a real subprocess and say so in their test name.
type inProcessCommand struct {
	Args   []string
	Dir    string
	Env    []string
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

var inProcessCommandMutex sync.Mutex

func newInProcessCommand(args ...string) *inProcessCommand {
	return &inProcessCommand{Args: append([]string(nil), args...)}
}

func (command *inProcessCommand) Run() error {
	stdout := command.Stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := command.Stderr
	if stderr == nil {
		stderr = io.Discard
	}
	return command.run(stdout, stderr)
}

func (command *inProcessCommand) Output() ([]byte, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := command.run(&stdout, &stderr)
	return stdout.Bytes(), err
}

func (command *inProcessCommand) CombinedOutput() ([]byte, error) {
	combined := &synchronizedBuffer{}
	err := command.run(combined, combined)
	return combined.Bytes(), err
}

type synchronizedBuffer struct {
	mutex sync.Mutex
	data  bytes.Buffer
}

func (buffer *synchronizedBuffer) Write(data []byte) (int, error) {
	buffer.mutex.Lock()
	defer buffer.mutex.Unlock()
	return buffer.data.Write(data)
}

func (buffer *synchronizedBuffer) Bytes() []byte {
	buffer.mutex.Lock()
	defer buffer.mutex.Unlock()
	return append([]byte(nil), buffer.data.Bytes()...)
}

func (command *inProcessCommand) run(stdout io.Writer, stderr io.Writer) error {
	inProcessCommandMutex.Lock()
	defer inProcessCommandMutex.Unlock()

	restoreDirectory, err := useCommandDirectory(command.Dir)
	if err != nil {
		return err
	}
	defer restoreDirectory()

	restoreEnvironment := useCommandEnvironment(command.Env)
	defer restoreEnvironment()

	stdin := command.Stdin
	if stdin == nil {
		stdin = strings.NewReader("")
	}
	app := &App{Stdin: stdin, Stdout: stdout, Stderr: stderr}
	code := app.Run(context.Background(), command.Args)
	if code != 0 {
		return fmt.Errorf("tspack exited with status %d", code)
	}
	return nil
}

func useCommandDirectory(directory string) (func(), error) {
	if directory == "" {
		return func() {}, nil
	}
	original, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("resolve test working directory: %w", err)
	}
	if err := os.Chdir(directory); err != nil {
		return nil, fmt.Errorf("enter test working directory %s: %w", directory, err)
	}
	return func() {
		_ = os.Chdir(original)
	}, nil
}

func useCommandEnvironment(environment []string) func() {
	if environment == nil {
		return func() {}
	}
	original := os.Environ()
	os.Clearenv()
	setEnvironment(environment)
	return func() {
		os.Clearenv()
		setEnvironment(original)
	}
}

func setEnvironment(environment []string) {
	for _, entry := range environment {
		name, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		_ = os.Setenv(name, value)
	}
}
