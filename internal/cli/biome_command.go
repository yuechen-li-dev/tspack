package cli

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/yuechen-li-dev/tspack/internal/diag"
)

func runFormatCommand(args []string) {
	runBiomeCommand("format", args)
}

func runLintCommand(args []string) {
	runBiomeCommand("lint", args)
}

type biomeCommandOptions struct {
	Command                  string
	Root                     string
	Paths                    []string
	UseCheck                 bool
	UseFix                   bool
	UseUnsafe                bool
	CaptureOutput            bool
	PrintDefaultConfigStatus bool
}

type biomeCommandResult struct {
	Diagnostics []diag.Diagnostic
	ExitCode    int
	Stdout      string
	Stderr      string
}

func runBiomeCommand(command string, args []string) {
	options := biomeCommandOptions{
		Command:                  command,
		Root:                     ".",
		Paths:                    []string{},
		PrintDefaultConfigStatus: true,
	}

	for i := 1; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--root":
			if i+1 >= len(args) {
				emitBiomeInvalidFlags(command, "--root requires a value")
			}
			i++
			options.Root = args[i]
		case "--check":
			if command == "lint" {
				emitBiomeInvalidFlags(command, "--check is only valid for format")
			}
			options.UseCheck = true
		case "--fix":
			if command == "format" {
				emitBiomeInvalidFlags(command, "--fix is only valid for lint")
			}
			options.UseFix = true
		case "--unsafe":
			if command == "format" {
				emitBiomeInvalidFlags(command, "--unsafe is only valid for lint --fix")
			}
			options.UseUnsafe = true
		default:
			if strings.HasPrefix(arg, "-") {
				emitBiomeInvalidFlags(command, "unknown flag: "+arg)
			}
			options.Paths = append(options.Paths, arg)
		}
	}

	if options.UseUnsafe && !options.UseFix {
		emitBiomeInvalidFlags(command, "--unsafe requires --fix")
	}
	if len(options.Paths) == 0 {
		options.Paths = deriveCheckFormatPaths(options.Root)
	}

	result := runBiomeCommandWithOptions(options)
	for _, diagnostic := range result.Diagnostics {
		fmt.Fprintf(os.Stderr, "%s: %s\n", diagnostic.Code, diagnostic.Message)
		for _, detail := range diagnostic.Details {
			fmt.Fprintf(os.Stderr, "%s\n", detail)
		}
	}
	if result.ExitCode != 0 {
		exit(result.ExitCode)
	}
}

func runBiomeCommandWithOptions(options biomeCommandOptions) biomeCommandResult {
	biomePath := resolveBiomeBackend(options.Root)
	if biomePath == "" {
		return biomeCommandResult{
			ExitCode:    1,
			Diagnostics: []diag.Diagnostic{newBiomeBackendNotFoundDiagnostic()},
		}
	}

	backendArgs, configPath, err := buildBiomeArgs(
		options.Command,
		options.Root,
		options.UseCheck,
		options.UseFix,
		options.UseUnsafe,
		options.Paths,
	)
	if err != nil {
		return biomeCommandResult{
			ExitCode: 1,
			Diagnostics: []diag.Diagnostic{{
				Code:     "TSPACK_BIOME_CONFIG_FAILED",
				Severity: diag.SeverityError,
				Message:  "Biome config setup failed.",
				Details:  []string{err.Error()},
			}},
		}
	}
	if configPath != "" {
		defer os.Remove(configPath)
		if options.PrintDefaultConfigStatus {
			fmt.Fprintln(os.Stderr, defaultBiomeConfigStatusLine)
		}
	}

	cmd := exec.Command(biomePath, backendArgs...)
	cmd.Dir = options.Root

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if options.CaptureOutput {
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
	} else {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}

	if err := cmd.Run(); err != nil {
		result := biomeCommandResult{
			ExitCode: 1,
			Stdout:   stdout.String(),
			Stderr:   stderr.String(),
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode := exitErr.ExitCode()
			if exitCode >= 0 {
				result.ExitCode = exitCode
				result.Diagnostics = append(result.Diagnostics, newBiomeExitDiagnostic(options, exitCode, result.Stdout, result.Stderr))
				return result
			}
		}

		result.Diagnostics = append(result.Diagnostics, diag.Diagnostic{
			Code:     "TSPACK_BIOME_COMMAND_FAILED",
			Severity: diag.SeverityError,
			Message:  "Biome command failed.",
			Details:  []string{err.Error()},
		})
		return result
	}

	return biomeCommandResult{
		ExitCode: 0,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
	}
}

func newBiomeBackendNotFoundDiagnostic() diag.Diagnostic {
	return diag.Diagnostic{
		Code:     "TSPACK_BIOME_BACKEND_NOT_FOUND",
		Severity: diag.SeverityError,
		Message:  "Biome backend was not found.",
		Details: []string{
			"Add @biomejs/biome as a tool dependency and run tspack sync, or install biome on PATH.",
		},
	}
}

func newBiomeExitDiagnostic(options biomeCommandOptions, exitCode int, stdout string, stderr string) diag.Diagnostic {
	diagnostic := biomeExitDiagnostic(options.Command, options.UseCheck, options.UseFix, options.UseUnsafe, exitCode)
	if options.CaptureOutput {
		diagnostic.Details = appendCapturedBiomeOutput(diagnostic.Details, stdout, stderr)
	}
	return diagnostic
}

func biomeExitDiagnostic(command string, useCheck bool, useFix bool, useUnsafe bool, exitCode int) diag.Diagnostic {
	switch {
	case command == "format" && useCheck:
		return diag.Diagnostic{
			Code:     "TSPACK_FORMAT_CHECK_FAILED",
			Severity: diag.SeverityError,
			Message:  "format check failed",
			Details: []string{
				"Biome format found files that would change.",
				"Run `tspack format` to apply formatting.",
			},
		}
	case command == "format":
		return diag.Diagnostic{
			Code:     "TSPACK_FORMAT_WRITE_FAILED",
			Severity: diag.SeverityError,
			Message:  "format failed",
			Details:  []string{fmt.Sprintf("Biome format exited with code %d while applying formatting.", exitCode)},
		}
	case command == "lint" && useFix:
		details := []string{}
		if useUnsafe {
			details = append(details, "Biome may have applied safe and unsafe fixes, but violations remain.")
			details = append(details, "Unsafe fixes were enabled for this run.")
		} else {
			details = append(details, "Biome may have applied safe fixes, but violations remain.")
			details = append(details, "Unsafe fixes are not applied by default.")
		}
		details = append(details, "Review the remaining diagnostics.")
		return diag.Diagnostic{
			Code:     "TSPACK_LINT_FIX_INCOMPLETE",
			Severity: diag.SeverityError,
			Message:  "lint fix incomplete",
			Details:  details,
		}
	case command == "lint":
		return diag.Diagnostic{
			Code:     "TSPACK_LINT_CHECK_FAILED",
			Severity: diag.SeverityError,
			Message:  "lint check failed",
			Details: []string{
				"Biome reported lint violations.",
				"Run `tspack lint --fix` to apply safe fixes where possible.",
			},
		}
	default:
		return diag.Diagnostic{
			Code:     "TSPACK_BIOME_COMMAND_FAILED",
			Severity: diag.SeverityError,
			Message:  fmt.Sprintf("biome %s exited with code %d", command, exitCode),
		}
	}
}

var ansiCSISequencePattern = regexp.MustCompile("\x1b\\[[0-9;?]*[ -/]*[@-~]")

func appendCapturedBiomeOutput(details []string, stdout string, stderr string) []string {
	cleanStdout := sanitizeTerminalOutput(stdout)
	cleanStderr := sanitizeTerminalOutput(stderr)
	if strings.TrimSpace(cleanStdout) != "" {
		details = append(details, "Biome stdout:", cleanStdout)
	}
	if strings.TrimSpace(cleanStderr) != "" {
		details = append(details, "Biome stderr:", cleanStderr)
	}
	return details
}

func sanitizeTerminalOutput(output string) string {
	return ansiCSISequencePattern.ReplaceAllString(output, "")
}

func emitBiomeInvalidFlags(command string, msg string) {
	if command == "format" {
		fmt.Fprintf(os.Stderr, "TSPACK_FORMAT_INVALID_FLAGS: %s\n", msg)
	} else {
		fmt.Fprintf(os.Stderr, "TSPACK_LINT_INVALID_FLAGS: %s\n", msg)
	}
	exit(1)
}

const defaultBiomeConfigStatusLine = "Using TSPack default Biome config: tabs, 100 columns, double quotes, organized imports, recommended lint rules. Add biome.json to customize."

const defaultBiomeStyleSummary = "tabs, 100 columns, double quotes, organizeImports, recommended rules"

func buildBiomeArgs(command, root string, useCheck, useFix, useUnsafe bool, paths []string) ([]string, string, error) {
	args := []string{command}
	if command == "format" && !useCheck {
		args = append(args, "--write")
	}
	if command == "lint" && useFix {
		args = append(args, "--write")
		if useUnsafe {
			args = append(args, "--unsafe")
		}
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
  "files": {
    "experimentalScannerIgnores": [
      ".tspack",
      "node_modules",
      "dist",
      "tspack-artifacts",
      "coverage",
      ".git",
      "build",
      ".turbo",
      ".vite"
    ],
    "includes": [
      "**",
      "!.tspack/**",
      "!node_modules/**",
      "!dist/**",
      "!tspack-artifacts/**",
      "!coverage/**",
      "!.git/**",
      "!build/**",
      "!.turbo/**",
      "!.vite/**"
    ]
  },
  "formatter": {
    "enabled": true,
    "indentStyle": "tab",
    "lineWidth": 100
  },
  "assist": {
    "enabled": true,
    "actions": {
      "source": {
        "organizeImports": "on"
      }
    }
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
