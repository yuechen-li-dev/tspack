package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func runFormatCommand(args []string) {
	runBiomeCommand("format", args)
}

func runLintCommand(args []string) {
	runBiomeCommand("lint", args)
}

func runBiomeCommand(command string, args []string) {
	root := "."
	paths := []string{}
	useCheck := false
	useFix := false

	for i := 1; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--root":
			if i+1 >= len(args) {
				emitBiomeInvalidFlags(command, "--root requires a value")
			}
			i++
			root = args[i]
		case "--check":
			if command == "lint" {
				emitBiomeInvalidFlags(command, "--check is only valid for format")
			}
			useCheck = true
		case "--fix":
			if command == "format" {
				emitBiomeInvalidFlags(command, "--fix is only valid for lint")
			}
			useFix = true
		default:
			if strings.HasPrefix(arg, "-") {
				emitBiomeInvalidFlags(command, "unknown flag: "+arg)
			}
			paths = append(paths, arg)
		}
	}

	biomePath := resolveBiomeBackend(root)
	if biomePath == "" {
		fmt.Fprintln(os.Stderr, "TSPACK_BIOME_BACKEND_NOT_FOUND: Biome backend was not found. Add @biomejs/biome as a tool dependency and run tspack sync, or install biome on PATH.")
		os.Exit(1)
	}

	backendArgs, configPath, err := buildBiomeArgs(command, root, useCheck, useFix, paths)
	if err != nil {
		fmt.Fprintf(os.Stderr, "TSPACK_BIOME_CONFIG_FAILED: %v\n", err)
		os.Exit(1)
	}
	if configPath != "" {
		defer os.Remove(configPath)
	}

	cmd := exec.Command(biomePath, backendArgs...)
	cmd.Dir = root
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			fmt.Fprintf(os.Stderr, "TSPACK_BIOME_COMMAND_FAILED: biome %s exited with code %d\n", command, exitErr.ExitCode())
			os.Exit(exitErr.ExitCode())
		}
		fmt.Fprintf(os.Stderr, "TSPACK_BIOME_COMMAND_FAILED: %v\n", err)
		os.Exit(1)
	}
}

func emitBiomeInvalidFlags(command string, msg string) {
	if command == "format" {
		fmt.Fprintf(os.Stderr, "TSPACK_FORMAT_INVALID_FLAGS: %s\n", msg)
	} else {
		fmt.Fprintf(os.Stderr, "TSPACK_LINT_INVALID_FLAGS: %s\n", msg)
	}
	os.Exit(1)
}

func buildBiomeArgs(command, root string, useCheck, useFix bool, paths []string) ([]string, string, error) {
	args := []string{command}
	if command == "format" && !useCheck {
		args = append(args, "--write")
	}
	if command == "lint" && useFix {
		args = append(args, "--write")
	}

	configPath := ""
	if !hasProjectBiomeConfig(root) {
		tmpFile, err := os.CreateTemp("", "tspack-biome-*.json")
		if err != nil {
			return nil, "", err
		}
		configPath = tmpFile.Name()
		defaultConfig := "{\n  \"formatter\": {\n    \"enabled\": true\n  },\n  \"linter\": {\n    \"enabled\": true,\n    \"rules\": {\n      \"recommended\": true\n    }\n  }\n}\n"
		if _, err := tmpFile.WriteString(defaultConfig); err != nil {
			_ = tmpFile.Close()
			return nil, configPath, err
		}
		if err := tmpFile.Close(); err != nil {
			return nil, configPath, err
		}
		args = append(args, "--config-path", configPath)
	}

	if len(paths) == 0 {
		args = append(args, ".")
	} else {
		args = append(args, paths...)
	}
	return args, configPath, nil
}

func hasProjectBiomeConfig(root string) bool {
	if _, err := os.Stat(filepath.Join(root, "biome.json")); err == nil {
		return true
	}
	if _, err := os.Stat(filepath.Join(root, "biome.jsonc")); err == nil {
		return true
	}
	return false
}

func resolveBiomeBackend(root string) string {
	for _, candidate := range localBiomeCandidates(root) {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	pathBiome, err := exec.LookPath("biome")
	if err == nil {
		return pathBiome
	}
	return ""
}

func localBiomeCandidates(root string) []string {
	candidates := []string{
		filepath.Join(root, "node_modules", ".bin", "biome"),
	}
	if runtime.GOOS == "windows" {
		candidates = append(candidates,
			filepath.Join(root, "node_modules", ".bin", "biome.cmd"),
			filepath.Join(root, "node_modules", ".bin", "biome.exe"),
		)
	}

	directBin := filepath.Join(root, "node_modules", "@biomejs", "biome", "bin")
	candidates = append(candidates, filepath.Join(directBin, "biome"))
	if runtime.GOOS == "windows" {
		candidates = append(candidates,
			filepath.Join(directBin, "biome.cmd"),
			filepath.Join(directBin, "biome.exe"),
		)
	}
	return candidates
}
