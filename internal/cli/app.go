package cli

import (
	"fmt"
	"os"
)

// Main dispatches the tspack command-line application. Process bootstrap belongs
// in cmd/tspack; command parsing, presentation, and orchestration live here.
func Main(args []string) {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		printDefaultHelp()
		return
	}
	if args[0] == "help" {
		topic := ""
		if len(args) > 1 {
			topic = args[1]
		}
		if !printHelpTopic(topic) {
			fmt.Fprintf(os.Stderr, "unknown help topic: %s\n\n", topic)
			printDefaultHelp()
			os.Exit(1)
		}
		return
	}

	if len(args) > 1 && (args[1] == "--help" || args[1] == "-h" || args[1] == "help") {
		if printHelpTopic(args[0]) {
			return
		}
	}

	if args[0] == "--version" || args[0] == "version" || args[0] == "-v" {
		printVersion()
		return
	}
	if handler, ok := commandHandlers[args[0]]; ok {
		handler(args)
		return
	}

	fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", args[0])
	printDefaultHelp()
	os.Exit(1)
}
