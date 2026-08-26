package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
)

// App owns the process-facing streams used by the command-line application.
// Run returns the status that the outer executable should use as its exit code.
type App struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

// SetIO replaces the streams used by subsequent Run calls. It exists so test
// harnesses can configure App without importing CLI internals.
func (app *App) SetIO(stdin io.Reader, stdout io.Writer, stderr io.Writer) {
	app.Stdin = stdin
	app.Stdout = stdout
	app.Stderr = stderr
}

// NewDefaultApp creates an application connected to the current process.
func NewDefaultApp() *App {
	return &App{
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}
}

// Main preserves the package's small embedding surface while returning process
// status to its caller. cmd/tspack is responsible for calling os.Exit.
func Main(args []string) int {
	return NewDefaultApp().Run(context.Background(), args)
}

// exitStatus is the compatibility boundary for legacy handlers while they are
// converted from immediate process termination to returned status. It never
// crosses Run: only an explicit command exit is recovered.
type exitStatus struct {
	code int
}

func exit(code int) {
	panic(exitStatus{code: code})
}

var appRunMutex sync.Mutex

// Run dispatches one command with injected process IO and returns its exit
// status. Runs are serialized while legacy renderers are connected to the
// injected streams; parser, application, and renderer helpers remain directly
// testable without this compatibility bridge.
func (app *App) Run(ctx context.Context, args []string) (code int) {
	if app == nil {
		app = NewDefaultApp()
	}
	if ctx == nil {
		ctx = context.Background()
	}

	appRunMutex.Lock()
	defer appRunMutex.Unlock()

	restoreIO := connectProcessIO(app)
	defer func() {
		restoreIO()
		if recovered := recover(); recovered != nil {
			status, ok := recovered.(exitStatus)
			if !ok {
				panic(recovered)
			}
			code = status.code
		}
	}()

	return runCommand(args)
}

func runCommand(args []string) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		printDefaultHelp()
		return 0
	}
	if args[0] == "help" {
		topic := ""
		if len(args) > 1 {
			topic = args[1]
		}
		if !printHelpTopic(topic) {
			fmt.Fprintf(os.Stderr, "unknown help topic: %s\n\n", topic)
			printDefaultHelp()
			return 1
		}
		return 0
	}

	if len(args) > 1 && (args[1] == "--help" || args[1] == "-h" || args[1] == "help") {
		if printHelpTopic(args[0]) {
			return 0
		}
	}

	if args[0] == "--version" || args[0] == "version" || args[0] == "-v" {
		printVersion()
		return 0
	}
	if handler, ok := commandHandlers[args[0]]; ok {
		handler(args)
		return 0
	}

	fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", args[0])
	printDefaultHelp()
	return 1
}

type processIOConnection struct {
	restore func()
	wait    func()
}

func connectProcessIO(app *App) func() {
	connections := []processIOConnection{}

	if app.Stdin != nil && app.Stdin != os.Stdin {
		original := os.Stdin
		reader, writer, err := os.Pipe()
		if err != nil {
			panic(fmt.Errorf("connect CLI stdin: %w", err))
		}
		os.Stdin = reader
		done := make(chan struct{})
		go func() {
			_, _ = io.Copy(writer, app.Stdin)
			_ = writer.Close()
			close(done)
		}()
		connections = append(connections, processIOConnection{
			restore: func() {
				os.Stdin = original
				_ = reader.Close()
			},
			wait: func() {
				<-done
			},
		})
	}

	connections = append(connections, connectOutput(&os.Stdout, app.Stdout, "stdout"))
	connections = append(connections, connectOutput(&os.Stderr, app.Stderr, "stderr"))

	return func() {
		for index := len(connections) - 1; index >= 0; index-- {
			connections[index].restore()
		}
		for _, connection := range connections {
			connection.wait()
		}
	}
}

func connectOutput(processFile **os.File, destination io.Writer, name string) processIOConnection {
	if destination == nil || destination == *processFile {
		return processIOConnection{
			restore: func() {},
			wait:    func() {},
		}
	}

	original := *processFile
	reader, writer, err := os.Pipe()
	if err != nil {
		panic(fmt.Errorf("connect CLI %s: %w", name, err))
	}
	*processFile = writer
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(destination, reader)
		_ = reader.Close()
		close(done)
	}()
	return processIOConnection{
		restore: func() {
			*processFile = original
			_ = writer.Close()
		},
		wait: func() {
			<-done
		},
	}
}
