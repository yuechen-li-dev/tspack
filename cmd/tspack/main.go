package main

import (
	"fmt"
	"os"
)

const version = "tspack 0.0.0-dev"

func main() {
	args := os.Args[1:]
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		printHelp()
		return
	}

	if args[0] == "--version" || args[0] == "version" || args[0] == "-v" {
		fmt.Println(version)
		return
	}

	fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", args[0])
	printHelp()
	os.Exit(1)
}

func printHelp() {
	fmt.Println("tspack - TypeScript-first package manager (M0 scaffold)")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  tspack help")
	fmt.Println("  tspack --version")
}
