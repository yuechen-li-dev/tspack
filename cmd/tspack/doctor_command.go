package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

type doctorScope string

const (
	doctorScopeAll     doctorScope = "all"
	doctorScopeFormat  doctorScope = "format"
	doctorScopeRun     doctorScope = "run"
	doctorScopeInspect doctorScope = "inspect"
)

type DoctorReport struct {
	Root     string          `json:"root"`
	Sections []DoctorSection `json:"sections"`
	Summary  DoctorSummary   `json:"summary"`
}
type DoctorSection struct {
	Name   string        `json:"name"`
	Checks []DoctorCheck `json:"checks"`
}
type DoctorCheck struct {
	Name           string         `json:"name"`
	Status         string         `json:"status"`
	Message        string         `json:"message"`
	Details        map[string]any `json:"details,omitempty"`
	Recommendation string         `json:"recommendation,omitempty"`
}
type DoctorSummary struct {
	Ok       int `json:"ok"`
	Warnings int `json:"warnings"`
	Errors   int `json:"errors"`
}

func runDoctorCommand(args []string) {
	scope := doctorScopeAll
	root := "."
	jsonOut := false
	for i := 1; i < len(args); i++ {
		a := args[i]
		switch a {
		case "format", "run", "inspect":
			scope = doctorScope(a)
		case "--root":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "TSPACK_DOCTOR_ROOT_INVALID: --root requires a value")
				os.Exit(1)
			}
			i++
			root = args[i]
		case "--json":
			jsonOut = true
		default:
			if strings.HasPrefix(a, "-") {
				fmt.Fprintf(os.Stderr, "TSPACK_DOCTOR_INVALID_SCOPE: unknown flag %s\n", a)
				os.Exit(1)
			}
			fmt.Fprintf(os.Stderr, "TSPACK_DOCTOR_INVALID_SCOPE: unknown scope %s\n", a)
			os.Exit(1)
		}
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "TSPACK_DOCTOR_ROOT_INVALID: %v\n", err)
		os.Exit(1)
	}
	if st, err := os.Stat(abs); err != nil || !st.IsDir() {
		fmt.Fprintln(os.Stderr, "TSPACK_DOCTOR_ROOT_INVALID: root directory does not exist")
		os.Exit(1)
	}

	report := DoctorReport{Root: abs}
	report.Sections = append(report.Sections, doctorProject(abs).toSection("Project"))
	if scope == doctorScopeAll || scope == doctorScopeFormat {
		report.Sections = append(report.Sections, doctorFormat(abs).toSection("Format/Lint"))
	}
	if scope == doctorScopeAll || scope == doctorScopeRun {
		report.Sections = append(report.Sections, doctorRun(abs).toSection("Run"))
	}
	if scope == doctorScopeAll || scope == doctorScopeInspect {
		report.Sections = append(report.Sections, doctorInspect(abs).toSection("Inspect (experimental)"))
	}
	for _, s := range report.Sections {
		for _, c := range s.Checks {
			if c.Status == "ok" {
				report.Summary.Ok++
			}
			if c.Status == "warning" {
				report.Summary.Warnings++
			}
			if c.Status == "error" {
				report.Summary.Errors++
			}
		}
	}
	if jsonOut {
		b, _ := json.MarshalIndent(report, "", "  ")
		os.Stdout.Write(append(b, '\n'))
	} else {
		printDoctorText(report)
	}
	if (scope == doctorScopeFormat || scope == doctorScopeRun) && report.Summary.Errors > 0 {
		os.Exit(1)
	}
}

type doctorBuilder struct{ checks []DoctorCheck }

func (d doctorBuilder) toSection(name string) DoctorSection {
	return DoctorSection{Name: name, Checks: d.checks}
}

func doctorProject(root string) doctorBuilder {
	d := doctorBuilder{}
	manifestPath := filepath.Join(root, "manifest.tsx")
	if _, err := os.Stat(manifestPath); err == nil {
		d.checks = append(d.checks, DoctorCheck{Name: "manifest", Status: "ok", Message: "manifest.tsx found"})
	} else {
		d.checks = append(d.checks, DoctorCheck{Name: "manifest", Status: "error", Message: "manifest.tsx missing", Recommendation: "Create manifest.tsx at project root."})
	}
	if _, err := os.Stat(filepath.Join(root, "ts-lock.toml")); err == nil {
		d.checks = append(d.checks, DoctorCheck{Name: "lockfile", Status: "ok", Message: "ts-lock.toml found"})
	} else {
		d.checks = append(d.checks, DoctorCheck{Name: "lockfile", Status: "warning", Message: "ts-lock.toml missing"})
	}
	if _, err := os.Stat(filepath.Join(root, "node_modules")); err == nil {
		d.checks = append(d.checks, DoctorCheck{Name: "node_modules", Status: "ok", Message: "node_modules present"})
	} else {
		d.checks = append(d.checks, DoctorCheck{Name: "node_modules", Status: "warning", Message: "node_modules missing"})
	}
	return d
}

func doctorFormat(root string) doctorBuilder {
	d := doctorBuilder{}
	localBiome := filepath.Join(root, "node_modules", ".bin", "biome")
	pathBiome, pathErr := exec.LookPath("biome")
	selected := resolveBiomeBackend(root)
	if selected != "" {
		details := map[string]any{
			"selectedPath": selected,
			"source":       "path",
			"localPath":    localBiome,
			"pathPath":     pathBiome,
		}
		if localBiome != "" && selected == localBiome {
			details["source"] = "local"
		}
		if version := commandVersion(selected, "--version"); version != "" {
			details["version"] = version
		}
		d.checks = append(d.checks, DoctorCheck{Name: "biome", Status: "ok", Message: "biome backend found", Details: details})
	} else {
		details := map[string]any{"localPath": localBiome}
		if pathErr == nil {
			details["pathPath"] = pathBiome
		}
		d.checks = append(d.checks, DoctorCheck{Name: "biome", Status: "error", Message: "biome backend missing", Details: details, Recommendation: "Install biome for format/lint support."})
	}
	if _, err := os.Stat(filepath.Join(root, "biome.json")); err == nil {
		d.checks = append(d.checks, DoctorCheck{Name: "config", Status: "ok", Message: "biome.json found", Details: map[string]any{"configPath": filepath.Join(root, "biome.json")}})
	} else if _, err := os.Stat(filepath.Join(root, "biome.jsonc")); err == nil {
		d.checks = append(d.checks, DoctorCheck{Name: "config", Status: "ok", Message: "biome.jsonc found", Details: map[string]any{"configPath": filepath.Join(root, "biome.jsonc")}})
	} else {
		d.checks = append(d.checks, DoctorCheck{Name: "config", Status: "warning", Message: "biome config not found; TSPack defaults will be used"})
	}
	return d
}

func doctorRun(root string) doctorBuilder {
	d := doctorBuilder{}
	d.checks = append(d.checks, runtimeCheck("node", "--version"))
	d.checks = append(d.checks, reservedRuntimeCheck("bun"))
	d.checks = append(d.checks, reservedRuntimeCheck("deno"))
	d.checks = append(d.checks, DoctorCheck{
		Name:    "runtime:system",
		Status:  "ok",
		Message: "system runtime support available",
		Details: map[string]any{
			"available": true,
			"builtIn":   true,
			"status":    "built in; executes declared argv directly",
		},
	})
	if _, err := os.Stat(filepath.Join(root, "manifest.tsx")); err != nil {
		d.checks = append(d.checks, DoctorCheck{Name: "runTargets", Status: "error", Message: "manifest.tsx missing"})
		return d
	}
	manifestPath := filepath.Join(root, "manifest.tsx")
	ir := loadManifestPathForRun(root, manifestPath)
	count := 0
	for _, ref := range collectRunTargets(root, manifestPath, ir, "") {
		rt := ref.Target
		count++
		readyKind := ""
		readyPath := ""
		if rt.Ready != nil {
			readyKind = rt.Ready.Kind
			readyPath = rt.Ready.Path
		}
		commandToken := firstToken(rt.Command)
		targetID := ref.PackageName + ":" + rt.Name
		cwdPath, cwdErr := resolveRunTargetCwd(ref)
		status := "ok"
		message := fmt.Sprintf("%s runtime=%s url=%s", targetID, rt.Runtime, rt.URL)
		if cwdErr != nil {
			status = "error"
			message = cwdErr.msg
		}
		details := map[string]any{
			"id":                targetID,
			"name":              rt.Name,
			"runtime":           rt.Runtime,
			"url":               rt.URL,
			"readyKind":         readyKind,
			"package":           ref.PackageName,
			"readyPath":         readyPath,
			"cwd":               effectiveRunTargetCwd(rt),
			"cwdPath":           cwdPath,
			"commandFirstToken": commandToken,
			"commandAvailable":  commandAvailable(commandToken),
			"runtimeAvailable":  runtimeAvailable(rt.Runtime),
		}
		if effectiveRunTargetCwd(rt) == "package" {
			details["packageRoot"] = ref.PackageRoot
		}
		if cwdErr != nil {
			details["cwdError"] = cwdErr.code
		}
		d.checks = append(d.checks, DoctorCheck{
			Name:    "runTarget:" + targetID,
			Status:  status,
			Message: message,
			Details: details,
		})
	}
	if count == 0 {
		d.checks = append(d.checks, DoctorCheck{Name: "runTargets", Status: "error", Message: "no run targets declared"})
	}
	return d
}

func doctorInspect(root string) doctorBuilder {
	_ = root
	d := doctorBuilder{}
	d.checks = append(d.checks, DoctorCheck{Name: "inspect", Status: "warning", Message: "inspect is experimental"})
	d.checks = append(d.checks, platformWebviewCheck())
	d.checks = append(d.checks, DoctorCheck{Name: "cdp", Status: "not_applicable", Message: "explicit endpoint required", Details: map[string]any{"policy": "explicit endpoint only; no port scanning"}})
	d.checks = append(d.checks, DoctorCheck{Name: "host-path", Status: "not_applicable", Message: "explicit path required", Details: map[string]any{"policy": "explicit host executable path required"}})
	d.checks = append(d.checks, doctorPlaywrightProvider(root))
	d.checks = append(d.checks, doctorVSCodeFamily())
	return d
}

func runtimeCheck(command string, versionFlag string) DoctorCheck {
	path, err := exec.LookPath(command)
	if err != nil {
		return DoctorCheck{
			Name:    command,
			Status:  "warning",
			Message: command + " not found",
			Details: map[string]any{"available": false},
		}
	}
	details := map[string]any{"available": true, "path": path}
	version := commandVersion(path, versionFlag)
	if version != "" {
		details["version"] = version
	}
	return DoctorCheck{Name: command, Status: "ok", Message: command + " found", Details: details}
}

func reservedRuntimeCheck(name string) DoctorCheck {
	return DoctorCheck{
		Name:    name,
		Status:  "not_applicable",
		Message: "reserved runtime backend; not implemented yet",
		Details: map[string]any{
			"available":   false,
			"implemented": false,
			"status":      "reserved runtime backend; not implemented yet",
		},
	}
}

func commandVersion(path string, flag string) string {
	cmd := exec.Command(path, flag)
	cmd.Env = os.Environ()
	out, err := runCommandWithTimeout(cmd, time.Second)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func runCommandWithTimeout(cmd *exec.Cmd, timeout time.Duration) ([]byte, error) {
	timer := time.AfterFunc(timeout, func() {
		_ = cmd.Process.Kill()
	})
	defer timer.Stop()
	return cmd.Output()
}

func firstToken(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}

func runtimeAvailable(name string) bool {
	if name == "system" {
		return true
	}
	_, err := exec.LookPath(name)
	return err == nil
}

func commandAvailable(name string) bool {
	if name == "" {
		return false
	}
	_, err := exec.LookPath(name)
	return err == nil
}

func platformWebviewCheck() DoctorCheck {
	candidate := "webkitgtk"
	if runtime.GOOS == "windows" {
		candidate = "webview2"
	} else if runtime.GOOS == "darwin" {
		candidate = "wkwebview"
	}

	details := map[string]any{
		"candidate":             candidate,
		"display":               os.Getenv("DISPLAY"),
		"waylandDisplay":        os.Getenv("WAYLAND_DISPLAY"),
		"dbusSessionBusAddress": os.Getenv("DBUS_SESSION_BUS_ADDRESS"),
	}
	if runtime.GOOS == "linux" && os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		return DoctorCheck{
			Name:           "platform-webview",
			Status:         "warning",
			Message:        "display session missing (DISPLAY/WAYLAND_DISPLAY)",
			Details:        details,
			Recommendation: "Run in a desktop session to enable WebKitGTK-based inspect backend checks.",
		}
	}
	return DoctorCheck{Name: "platform-webview", Status: "warning", Message: "platform-webview backend remains experimental", Details: details}
}

func doctorPlaywrightProvider(root string) DoctorCheck {
	modulePath, source := resolvePlaywrightCoreProviderPath(root)
	if modulePath == "" {
		return DoctorCheck{
			Name:           "playwright-core-provider",
			Status:         "warning",
			Message:        "playwright-core provider missing",
			Recommendation: "Set TSPACK_PLAYWRIGHT_CORE_PATH or install playwright-core.",
		}
	}
	return DoctorCheck{
		Name:    "playwright-core-provider",
		Status:  "ok",
		Message: "playwright-core provider found",
		Details: map[string]any{"source": source, "path": modulePath, "loadable": true},
	}
}
func resolvePlaywrightCoreProviderPath(root string) (string, string) {
	envPath := os.Getenv("TSPACK_PLAYWRIGHT_CORE_PATH")
	if envPath != "" && fileExists(filepath.Join(envPath, "package.json")) {
		return envPath, "TSPACK_PLAYWRIGHT_CORE_PATH"
	}
	localCandidates := []string{
		filepath.Join(root, "node_modules", "playwright-core"),
		filepath.Join(root, "node_modules", "playwright"),
	}
	for _, candidate := range localCandidates {
		if fileExists(filepath.Join(candidate, "package.json")) {
			return candidate, "project-local"
		}
	}
	vscodeCandidates := []string{
		"/usr/share/code/resources/app/node_modules/playwright-core",
		"/usr/share/code-insiders/resources/app/node_modules/playwright-core",
		"/usr/share/codium/resources/app/node_modules/playwright-core",
		"/usr/share/code-oss/resources/app/node_modules/playwright-core",
	}
	for _, candidate := range vscodeCandidates {
		if fileExists(filepath.Join(candidate, "package.json")) {
			return candidate, "VS Code bundle"
		}
	}
	return "", ""
}
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func doctorVSCodeFamily() DoctorCheck {
	names := []string{"code", "code-insiders", "code-oss", "codium", "vscodium"}
	found := []map[string]any{}
	for _, name := range names {
		path, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		entry := map[string]any{"name": name, "path": path}
		version := commandVersion(path, "--version")
		if version != "" {
			entry["version"] = strings.Split(version, "\n")[0]
		}
		found = append(found, entry)
	}
	if len(found) == 0 {
		return DoctorCheck{Name: "vscode-family", Status: "warning", Message: "no VS Code-family executables found"}
	}
	return DoctorCheck{Name: "vscode-family", Status: "ok", Message: "VS Code-family executable(s) found", Details: map[string]any{"executables": found}}
}

func printDoctorText(report DoctorReport) {
	fmt.Println("TSPack Doctor")
	fmt.Println()
	fmt.Printf("Project Root\n  root: %s\n\n", report.Root)
	for _, s := range report.Sections {
		fmt.Println(s.Name)
		for _, c := range s.Checks {
			fmt.Printf("  %s: %s\n", c.Name, c.Message)
			printDoctorDetails(c.Details)
		}
		fmt.Println()
	}
	fmt.Println("Summary")
	fmt.Printf("  ok: %d\n  warnings: %d\n  errors: %d\n", report.Summary.Ok, report.Summary.Warnings, report.Summary.Errors)
}

func printDoctorDetails(details map[string]any) {
	if len(details) == 0 {
		return
	}
	keys := make([]string, 0, len(details))
	for key := range details {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		fmt.Printf("    %s: %s\n", key, doctorDetailValue(details[key]))
	}
}

func doctorDetailValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case bool:
		if typed {
			return "true"
		}
		return "false"
	case int:
		return fmt.Sprintf("%d", typed)
	case nil:
		return ""
	default:
		b, err := json.Marshal(typed)
		if err != nil {
			return fmt.Sprint(typed)
		}
		return string(b)
	}
}
