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
		fmt.Fprintln(os.Stderr, defaultBiomeConfigStatusLine)
	}

	cmd := exec.Command(biomePath, backendArgs...)
	cmd.Dir = root
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode := exitErr.ExitCode()
			if exitCode >= 0 {
				emitBiomeExitDiagnostic(command, useCheck, useFix, exitCode)
				os.Exit(exitCode)
			}
		}

		fmt.Fprintf(os.Stderr, "TSPACK_BIOME_COMMAND_FAILED: %v\n", err)
		os.Exit(1)
	}
}

func emitBiomeExitDiagnostic(command string, useCheck bool, useFix bool, exitCode int) {
	switch {
	case command == "format" && useCheck:
		fmt.Fprintf(os.Stderr, "TSPACK_FORMAT_CHECK_FAILED: format check failed\n")
		fmt.Fprintln(os.Stderr, "Biome format found files that would change.")
		fmt.Fprintln(os.Stderr, "Run `tspack format` to apply formatting.")
	case command == "format":
		fmt.Fprintf(os.Stderr, "TSPACK_FORMAT_WRITE_FAILED: format failed\n")
		fmt.Fprintf(os.Stderr, "Biome format exited with code %d while applying formatting.\n", exitCode)
	case command == "lint" && useFix:
		fmt.Fprintf(os.Stderr, "TSPACK_LINT_FIX_INCOMPLETE: lint fix incomplete\n")
		fmt.Fprintln(os.Stderr, "Biome may have applied safe fixes, but violations remain.")
		fmt.Fprintln(os.Stderr, "Review the remaining diagnostics.")
		fmt.Fprintln(os.Stderr, "Unsafe fixes are not applied by default.")
	case command == "lint":
		fmt.Fprintf(os.Stderr, "TSPACK_LINT_CHECK_FAILED: lint check failed\n")
		fmt.Fprintln(os.Stderr, "Biome reported lint violations.")
		fmt.Fprintln(os.Stderr, "Run `tspack lint --fix` to apply safe fixes where possible.")
	default:
		fmt.Fprintf(os.Stderr, "TSPACK_BIOME_COMMAND_FAILED: biome %s exited with code %d\n", command, exitCode)
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

const defaultBiomeConfigStatusLine = "Using TSPack default Biome config: tabs, 100 columns, double quotes, organized imports, recommended lint rules. Add biome.json to customize."

const defaultBiomeStyleSummary = "tabs, 100 columns, double quotes, organizeImports, recommended rules"

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
		tmpConfigPath, err := writeDefaultBiomeConfigTempFile()
		if err != nil {
			return nil, "", err
		}
		configPath = tmpConfigPath
		args = append(args, "--config-path", configPath)
	}

	if len(paths) == 0 {
		args = append(args, ".")
	} else {
		args = append(args, paths...)
	}
	return args, configPath, nil
}

func writeDefaultBiomeConfigTempFile() (string, error) {
	tmpFile, err := os.CreateTemp("", "tspack-biome-*.json")
	if err != nil {
		return "", err
	}
	configPath := tmpFile.Name()

	if _, err := tmpFile.Write(defaultBiomeConfigBytes()); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(configPath)
		return "", err
	}
	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(configPath)
		return "", err
	}
	return configPath, nil
}

func defaultBiomeConfigBytes() []byte {
	return []byte(`{
  "formatter": {
    "enabled": true,
    "indentStyle": "tab",
    "lineWidth": 100
  },
  "organizeImports": {
    "enabled": true
  },
  "linter": {
    "enabled": true,
    "rules": {
      "recommended": true,
      "correctness": {
        "noUnusedVariables": "warn",
        "noUnusedImports": "warn"
      },
      "style": {
        "useImportType": "error"
      }
    }
  },
  "javascript": {
    "formatter": {
      "quoteStyle": "double",
      "trailingCommas": "all",
      "semicolons": "always",
      "arrowParentheses": "always",
      "bracketSpacing": true
    }
  }
}
`)
}

func hasProjectBiomeConfig(root string) bool {
	configPath, _ := projectBiomeConfigPath(root)
	return configPath != ""
}

func projectBiomeConfigPath(root string) (string, string) {
	jsonPath := filepath.Join(root, "biome.json")
	if _, err := os.Stat(jsonPath); err == nil {
		return jsonPath, "project"
	}

	jsoncPath := filepath.Join(root, "biome.jsonc")
	if _, err := os.Stat(jsoncPath); err == nil {
		return jsoncPath, "project"
	}

	return "", "tspack-default"
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
