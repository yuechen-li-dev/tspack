package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/yuechen-li-dev/tspack/internal/capability"
	"github.com/yuechen-li-dev/tspack/internal/diag"
	"github.com/yuechen-li-dev/tspack/internal/lockfile"
	"github.com/yuechen-li-dev/tspack/internal/manifest"
	"github.com/yuechen-li-dev/tspack/internal/nodecmd"
	"github.com/yuechen-li-dev/tspack/internal/securityevidence"
)

type doctorScope string

const (
	doctorScopeAll      doctorScope = "all"
	doctorScopeFormat   doctorScope = "format"
	doctorScopeRun      doctorScope = "run"
	doctorScopeRuntime  doctorScope = "runtime"
	doctorScopeInspect  doctorScope = "inspect"
	doctorScopeSecurity doctorScope = "security"
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
		case "format", "run", "runtime", "inspect", "security":
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
	if scope != doctorScopeSecurity {
		report.Sections = append(report.Sections, doctorProject(abs).toSection("Project"))
	}
	if scope == doctorScopeAll || scope == doctorScopeFormat {
		report.Sections = append(report.Sections, doctorFormat(abs).toSection("Format/Lint"))
	}
	if scope == doctorScopeAll || scope == doctorScopeRun {
		report.Sections = append(report.Sections, doctorRun(abs).toSection("Run"))
	}
	if scope == doctorScopeAll || scope == doctorScopeRuntime {
		report.Sections = append(report.Sections, doctorRuntime(abs).toSection("Runtime profile"))
	}
	if scope == doctorScopeAll || scope == doctorScopeInspect {
		report.Sections = append(report.Sections, doctorInspect(abs).toSection("Inspect (experimental)"))
	}
	if scope == doctorScopeAll || scope == doctorScopeSecurity {
		report.Sections = append(report.Sections, doctorSecurity(abs).toSection("Security"))
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
	if (scope == doctorScopeFormat || scope == doctorScopeRun || scope == doctorScopeSecurity) && report.Summary.Errors > 0 {
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
	localBinBiome := filepath.Join(root, "node_modules", ".bin", "biome")
	directPackageBiome := filepath.Join(root, "node_modules", "@biomejs", "biome", "bin", "biome")
	pathBiome, pathErr := exec.LookPath("biome")
	selected := resolveBiomeBackend(root)
	if selected != "" {
		details := map[string]any{
			"selectedPath":      selected,
			"source":            biomeBackendSource(root, selected),
			"localPath":         localBinBiome,
			"directPackagePath": directPackageBiome,
			"pathPath":          pathBiome,
		}
		if version := commandVersion(selected, "--version"); version != "" {
			details["version"] = version
		}
		d.checks = append(d.checks, DoctorCheck{Name: "biome", Status: "ok", Message: "biome backend found", Details: details})
	} else {
		details := map[string]any{
			"localPath":         localBinBiome,
			"directPackagePath": directPackageBiome,
		}
		if pathErr == nil {
			details["pathPath"] = pathBiome
		}
		d.checks = append(d.checks, DoctorCheck{Name: "biome", Status: "error", Message: "biome backend missing", Details: details, Recommendation: "Install biome for format/lint support."})
	}
	configPath, configSource := projectBiomeConfigPath(root)
	if configSource == "project" {
		d.checks = append(d.checks, DoctorCheck{
			Name:    "config",
			Status:  "ok",
			Message: filepath.Base(configPath) + " found",
			Details: map[string]any{
				"configPath":   configPath,
				"configSource": configSource,
			},
		})
	} else {
		d.checks = append(d.checks, DoctorCheck{
			Name:    "config",
			Status:  "warning",
			Message: "biome config not found; TSPack default will be used",
			Details: map[string]any{
				"configSource": configSource,
				"defaultStyle": defaultBiomeStyleSummary,
			},
		})
	}
	return d
}

func biomeBackendSource(root string, selected string) string {
	for _, candidate := range localBiomeCandidates(root) {
		if selected != candidate {
			continue
		}
		if strings.Contains(candidate, filepath.Join("node_modules", ".bin")) {
			return "local"
		}
		return "direct-package"
	}
	return "path"
}

func doctorRun(root string) doctorBuilder {
	d := doctorBuilder{}
	d.checks = append(d.checks, runtimeCheck("node", "--version"))
	d.checks = append(d.checks, runtimeCheck("bun", "--version"))
	d.checks = append(d.checks, runtimeCheck("deno", "--version"))
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
		readyHost := ""
		readyPort := 0
		readyPattern := ""
		readyStream := ""
		if rt.Ready != nil {
			readyKind = rt.Ready.Kind
			readyPath = rt.Ready.Path
			readyHost = rt.Ready.Host
			readyPort = rt.Ready.Port
			readyPattern = rt.Ready.Pattern
			readyStream = rt.Ready.Stream
			if readyKind == "tcp" && readyHost == "" {
				readyHost = "127.0.0.1"
			}
			if readyKind == "stdout-match" && readyStream == "" {
				readyStream = "both"
			}
		}
		commandToken := runTargetCommandFirstToken(rt)
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
			"readyHost":         readyHost,
			"readyPort":         readyPort,
			"readyPattern":      readyPattern,
			"readyStream":       readyStream,
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

func doctorRuntime(root string) doctorBuilder {
	d := doctorBuilder{}
	selected := "nodejs"
	if _, err := os.Stat(filepath.Join(root, "manifest.tsx")); err != nil {
		d.checks = append(d.checks, DoctorCheck{Name: "runtime profile", Status: "warning", Message: "manifest.tsx missing; defaulting runtime profile to nodejs", Details: runtimeProfileDetails(selected)})
		return d
	}

	manifestPath := filepath.Join(root, "manifest.tsx")
	ir := loadManifestPathForRun(root, manifestPath)
	if ir.Workspace.Runtime != "" {
		selected = ir.Workspace.Runtime
	}

	details := runtimeProfileDetails(selected)
	status := "ok"
	message := "selected runtime profile: " + selected
	if available, _ := details["available"].(bool); !available {
		status = "warning"
		message = "selected runtime executable not found: " + runtimeProfileExecutable(selected)
	}
	d.checks = append(d.checks, DoctorCheck{Name: "runtime profile", Status: status, Message: message, Details: details})
	return d
}

func runtimeProfileDetails(selected string) map[string]any {
	executable := runtimeProfileExecutable(selected)
	_, err := exec.LookPath(executable)
	return map[string]any{
		"selected":                           selected,
		"executable":                         executable,
		"available":                          err == nil,
		"status":                             runtimeProfileSupportStatus(selected),
		"lifecycleOwner":                     "tspack",
		"packageManagerDelegated":            false,
		"dependencyResolution":               "TSPack",
		"lockfile":                           "ts-lock.toml",
		"materialization":                    "TSPack",
		"securityPolicy":                     "TSPack",
		"lifecyclePolicy":                    "TSPack",
		"ownershipNote":                      "TSPack owns package resolution, lockfiles, sync/materialization, check, pack, and lifecycle policy; runtime profile does not delegate those to npm/bun/deno.",
		"runTargetInheritance":               true,
		"explicitRunTargetRuntimePrecedence": true,
		"runTargetInheritanceNote":           "RunTargets without explicit runtime inherit the workspace runtime profile; explicit RunTarget runtime wins.",
	}
}

func runtimeProfileExecutable(selected string) string {
	switch selected {
	case "bun":
		return "bun"
	case "deno":
		return "deno"
	default:
		return "node"
	}
}

func runtimeProfileSupportStatus(selected string) string {
	switch selected {
	case "bun", "deno":
		return "experimental"
	default:
		return "supported"
	}
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

func runTargetCommandFirstToken(target manifest.RunTarget) string {
	if executable, ok := directRuntimeExecutable(target.Runtime); ok {
		return executable
	}
	return firstToken(target.Command)
}

func firstToken(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}

func runtimeAvailable(name string) bool {
	switch name {
	case "system":
		return true
	case "bun":
		_, err := exec.LookPath("bun")
		return err == nil
	case "deno":
		_, err := exec.LookPath("deno")
		return err == nil
	default:
		_, err := exec.LookPath(name)
		return err == nil
	}
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
	switch runtime.GOOS {
	case "windows":
		candidate = "webview2"
	case "darwin":
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
			fmt.Printf("  %s: %s\n", c.Name, c.Status)
			if c.Message != "" {
				fmt.Printf("    message: %s\n", c.Message)
			}
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
		if values, ok := details[key].([]string); ok {
			fmt.Printf("    %s:\n", key)
			for _, value := range values {
				fmt.Printf("      %s\n", value)
			}
			continue
		}
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

type doctorLifecycleCapability struct {
	PackageID   string
	Script      string
	Command     string
	Category    string
	InstallTime bool
}

func doctorSecurity(root string) doctorBuilder {
	d := doctorBuilder{}
	ir, manifestChecks := loadDoctorSecurityManifest(root)
	d.checks = append(d.checks, manifestChecks...)

	lockfilePath := filepath.Join(root, "ts-lock.toml")
	lf, lockChecks, lockfileAvailable := loadDoctorSecurityLockfile(lockfilePath)
	acks := []manifest.AcknowledgedCapability(nil)
	categoryAcks := []manifest.AcknowledgedLifecycleCategory(nil)
	if ir != nil {
		acks = append(acks, ir.Security.AcknowledgedCapabilities...)
		categoryAcks = append(categoryAcks, ir.Security.AcknowledgedLifecycleCategories...)
	}
	if !lockfileAvailable && len(acks)+len(categoryAcks) > 0 {
		annotateMissingLockfileAcknowledgements(lockChecks, len(acks)+len(categoryAcks))
	}
	d.checks = append(d.checks, lockChecks...)
	d.checks = append(d.checks, doctorBehaviorEvidenceChecks(root, acks)...)
	if !lockfileAvailable {
		d.checks = append(d.checks, doctorSecurityPostureCheck())
		return d
	}

	d.checks = append(d.checks, doctorLifecycleSecurityChecks(root, lf, acks, categoryAcks)...)
	d.checks = append(d.checks, doctorSecurityPostureCheck())
	return d
}

func annotateMissingLockfileAcknowledgements(checks []DoctorCheck, acknowledgementCount int) {
	for index := range checks {
		if checks[index].Name != "security lockfile missing" {
			continue
		}
		if checks[index].Details == nil {
			checks[index].Details = map[string]any{}
		}
		checks[index].Details["acknowledgmentsCannotBeEvaluated"] = acknowledgementCount
	}
}

func loadDoctorSecurityManifest(root string) (*manifest.ManifestIR, []DoctorCheck) {
	manifestPath := filepath.Join(root, "manifest.tsx")
	if _, err := os.Stat(manifestPath); err != nil {
		if os.IsNotExist(err) {
			return nil, []DoctorCheck{{Name: "security manifest", Status: "error", Message: "manifest.tsx missing", Recommendation: "Create manifest.tsx before auditing security acknowledgments."}}
		}
		return nil, []DoctorCheck{{Name: "security manifest", Status: "error", Message: "manifest.tsx could not be read", Details: map[string]any{"error": err.Error()}}}
	}

	cliPath := manifestFrontendCLIPath()
	if _, err := os.Stat(cliPath); err != nil {
		return nil, []DoctorCheck{{Name: "security manifest", Status: "error", Message: "manifest frontend CLI not found", Details: map[string]any{"path": cliPath}, Recommendation: "Run `cd manifest-frontend && npm run build`."}}
	}

	cmd, err := nodecmd.Command(cliPath, manifestPath)
	if err != nil {
		if nodecmd.IsNotFound(err) {
			return nil, []DoctorCheck{{
				Name:           "security manifest",
				Status:         "error",
				Message:        "Node.js was not found on PATH.",
				Recommendation: "Install Node.js or activate it in your shell. Recommended: use mise (https://mise.jdx.dev/).",
				Details: map[string]any{
					"diagnosticCode": nodecmd.DiagnosticCode,
					"guidance":       nodecmd.GuidanceLines(),
				},
			}}
		}
		return nil, []DoctorCheck{{Name: "security manifest", Status: "error", Message: "failed to prepare manifest frontend command", Details: map[string]any{"error": err.Error()}}}
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, []DoctorCheck{{Name: "security manifest", Status: "error", Message: "manifest frontend failed", Details: map[string]any{"error": err.Error(), "stderr": stderr.String()}}}
	}

	var parsed struct {
		OK          bool              `json:"ok"`
		IR          json.RawMessage   `json:"ir"`
		Diagnostics []diag.Diagnostic `json:"diagnostics"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &parsed); err != nil {
		return nil, []DoctorCheck{{Name: "security manifest", Status: "error", Message: "manifest frontend returned invalid JSON", Details: map[string]any{"error": err.Error()}}}
	}
	if !parsed.OK {
		details := map[string]any{}
		if len(parsed.Diagnostics) > 0 {
			details["diagnostics"] = parsed.Diagnostics
		}
		return nil, []DoctorCheck{{Name: "security manifest", Status: "error", Message: "manifest frontend returned diagnostics", Details: details}}
	}

	ir, diagnostics := manifest.LoadBytes(manifestPath, parsed.IR)
	if len(diagnostics) > 0 {
		return nil, doctorChecksForDiagnostics("security manifest", diagnostics)
	}
	return ir, []DoctorCheck{{Name: "security manifest", Status: "ok", Message: "security policy loaded", Details: map[string]any{"acknowledgedCapabilities": len(ir.Security.AcknowledgedCapabilities), "acknowledgedLifecycleCategories": len(ir.Security.AcknowledgedLifecycleCategories)}}}
}

func doctorChecksForDiagnostics(prefix string, diagnostics []diag.Diagnostic) []DoctorCheck {
	checks := make([]DoctorCheck, 0, len(diagnostics))
	for index, diagnostic := range diagnostics {
		status := "warning"
		if diagnostic.Severity == diag.SeverityError {
			status = "error"
		}
		details := map[string]any{"code": diagnostic.Code}
		if diagnostic.File != "" {
			details["file"] = diagnostic.File
		}
		if len(diagnostic.Details) > 0 {
			details["details"] = diagnostic.Details
		}
		checks = append(checks, DoctorCheck{
			Name:    fmt.Sprintf("%s diagnostic %d", prefix, index+1),
			Status:  status,
			Message: diagnostic.Message,
			Details: details,
		})
	}
	return checks
}

func loadDoctorSecurityLockfile(lockfilePath string) (*lockfile.Lockfile, []DoctorCheck, bool) {
	lf, diagnostics, err := lockfile.LoadFile(lockfilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, []DoctorCheck{{
				Name:    "security lockfile missing",
				Status:  "warning",
				Message: "security lockfile missing",
				Details: map[string]any{
					"lockfile": filepath.Base(lockfilePath),
					"nextStep": "run tspack update to resolve and record package capabilities",
				},
			}}, false
		}
		return nil, []DoctorCheck{{Name: "security lockfile", Status: "error", Message: "failed to read lockfile", Details: map[string]any{"error": err.Error()}}}, false
	}
	if len(diagnostics) > 0 {
		return nil, doctorChecksForDiagnostics("security lockfile", diagnostics), false
	}
	return lf, nil, true
}

func doctorLifecycleSecurityChecks(root string, lf *lockfile.Lockfile, acknowledgements []manifest.AcknowledgedCapability, categoryAcknowledgements []manifest.AcknowledgedLifecycleCategory) []DoctorCheck {
	capabilities := collectDoctorLifecycleCapabilities(lf)
	ackByExactKey := doctorAcknowledgementsByExactKey(acknowledgements)
	usedAcknowledgements := map[string]bool{}
	usedCategoryAcknowledgements := map[int]int{}
	staleByCapabilityKey := map[string]manifest.AcknowledgedCapability{}
	pathsByPackage := doctorLifecyclePulledByPathLines(lf)

	acknowledgedCount := 0
	categoryAcknowledgedCount := 0
	unacknowledgedCount := 0
	staleCount := 0
	packagesWithLifecycle := map[string]bool{}
	categoryCounts := map[string]int{
		"consumerInstall":   0,
		"maintainerPublish": 0,
		"other":             0,
	}

	checks := []DoctorCheck{}
	for _, lifecycleCapability := range capabilities {
		packagesWithLifecycle[lifecycleCapability.PackageID] = true
		switch lifecycleCapability.Category {
		case capability.LifecycleCategoryConsumerInstall:
			categoryCounts["consumerInstall"]++
		case capability.LifecycleCategoryMaintainerPublish:
			categoryCounts["maintainerPublish"]++
		default:
			categoryCounts["other"]++
		}
		exactKey := doctorLifecycleAcknowledgementKey(lifecycleCapability.PackageID, lifecycleCapability.Script, lifecycleCapability.Command)
		_, acknowledged := ackByExactKey[exactKey]
		_, categoryAcknowledgementIndex, categoryAcknowledged := doctorMatchingLifecycleCategoryAcknowledgement(lifecycleCapability, categoryAcknowledgements)
		if acknowledged {
			usedAcknowledgements[exactKey] = true
			acknowledgedCount++
		} else if categoryAcknowledged {
			usedCategoryAcknowledgements[categoryAcknowledgementIndex]++
			categoryAcknowledgedCount++
		} else {
			staleAck, stale := doctorFindStaleAcknowledgement(lifecycleCapability, acknowledgements)
			if stale {
				staleKey := doctorLifecycleStaleAcknowledgementKey(lifecycleCapability.PackageID, lifecycleCapability.Script)
				staleByCapabilityKey[staleKey] = staleAck
				usedAcknowledgements[doctorLifecycleAcknowledgementKey(staleAck.Package, staleAck.Script, staleAck.Command)] = true
				staleCount++
			} else {
				unacknowledgedCount++
			}
		}
	}

	unusedAcknowledgements := []manifest.AcknowledgedCapability{}
	for _, acknowledgement := range acknowledgements {
		if acknowledgement.Kind != capability.LifecycleScriptKind {
			continue
		}
		key := doctorLifecycleAcknowledgementKey(acknowledgement.Package, acknowledgement.Script, acknowledgement.Command)
		if !usedAcknowledgements[key] {
			unusedAcknowledgements = append(unusedAcknowledgements, acknowledgement)
		}
	}
	sort.SliceStable(unusedAcknowledgements, func(i, j int) bool {
		return unusedAcknowledgements[i].Key() < unusedAcknowledgements[j].Key()
	})

	summaryStatus := "ok"
	summaryMessage := "no lifecycle script capabilities recorded"
	if len(capabilities) > 0 {
		summaryMessage = fmt.Sprintf("%d lifecycle script capabilities found", len(capabilities))
	}
	unusedCategoryAcknowledgements := doctorUnusedLifecycleCategoryAcknowledgements(categoryAcknowledgements, usedCategoryAcknowledgements)
	staleCategoryAcknowledgements := doctorStaleLifecycleCategoryAcknowledgements(categoryAcknowledgements)

	if unacknowledgedCount > 0 || staleCount > 0 || len(unusedAcknowledgements) > 0 || len(unusedCategoryAcknowledgements) > 0 || len(staleCategoryAcknowledgements) > 0 {
		summaryStatus = "warning"
	}
	checks = append(checks, DoctorCheck{
		Name:    "lifecycle summary",
		Status:  summaryStatus,
		Message: summaryMessage,
		Details: map[string]any{
			"totalLifecycleCapabilities":             len(capabilities),
			"acknowledgedCapabilities":               acknowledgedCount,
			"acknowledged":                           acknowledgedCount,
			"acknowledgedLifecycleCategories":        len(categoryAcknowledgements),
			"categoryAcknowledgedCapabilities":       categoryAcknowledgedCount,
			"unacknowledgedCapabilities":             unacknowledgedCount,
			"unacknowledged":                         unacknowledgedCount,
			"staleAcknowledgments":                   staleCount,
			"unusedAcknowledgments":                  len(unusedAcknowledgements),
			"unusedLifecycleCategoryAcknowledgments": len(unusedCategoryAcknowledgements),
			"staleLifecycleCategoryAcknowledgments":  len(staleCategoryAcknowledgements),
			"packagesWithLifecycleScripts":           len(packagesWithLifecycle),
			"lifecycleCategories":                    categoryCounts,
			"execution":                              "blocked by default",
		},
	})

	for _, lifecycleCapability := range capabilities {
		check := doctorLifecycleCapabilityCheck(root, lifecycleCapability, ackByExactKey, categoryAcknowledgements, staleByCapabilityKey, pathsByPackage)
		checks = append(checks, check)
	}
	for _, staleAck := range staleAcknowledgementsSorted(staleByCapabilityKey) {
		actualCommand := ""
		for _, lifecycleCapability := range capabilities {
			if lifecycleCapability.PackageID == staleAck.Package && lifecycleCapability.Script == staleAck.Script {
				actualCommand = lifecycleCapability.Command
				break
			}
		}
		checks = append(checks, DoctorCheck{
			Name:    "stale acknowledgement " + staleAck.Package + " " + staleAck.Script,
			Status:  "warning",
			Message: "acknowledged lifecycle capability command no longer matches lockfile",
			Details: map[string]any{
				"package":             staleAck.Package,
				"script":              staleAck.Script,
				"acknowledgedCommand": staleAck.Command,
				"actualCommand":       actualCommand,
				"reason":              staleAck.Reason,
			},
		})
	}
	for _, acknowledgement := range unusedAcknowledgements {
		checks = append(checks, DoctorCheck{
			Name:    "unused acknowledgement " + acknowledgement.Package + " " + acknowledgement.Script,
			Status:  "warning",
			Message: "acknowledged lifecycle capability not present in lockfile",
			Details: map[string]any{
				"package": acknowledgement.Package,
				"script":  acknowledgement.Script,
				"command": acknowledgement.Command,
				"reason":  acknowledgement.Reason,
			},
		})
	}
	for index, categoryAck := range categoryAcknowledgements {
		checks = append(checks, doctorLifecycleCategoryAcknowledgementCheck(categoryAck, usedCategoryAcknowledgements[index]))
	}
	for _, categoryAck := range staleCategoryAcknowledgements {
		checks = append(checks, doctorLifecycleCategoryStaleCheck(categoryAck))
	}
	for _, categoryAck := range unusedCategoryAcknowledgements {
		checks = append(checks, doctorLifecycleCategoryUnusedCheck(categoryAck))
	}
	return checks
}

func doctorLifecycleCategoryAcknowledgementCheck(acknowledgement manifest.AcknowledgedLifecycleCategory, matchedCapabilities int) DoctorCheck {
	return DoctorCheck{
		Name:    "lifecycle category acknowledgement " + acknowledgement.Category,
		Status:  "ok",
		Message: "lifecycle category acknowledgment audited; execution remains blocked",
		Details: map[string]any{
			"category":            acknowledgement.Category,
			"scripts":             acknowledgement.Scripts,
			"reason":              acknowledgement.Reason,
			"matchedCapabilities": matchedCapabilities,
		},
	}
}

func doctorLifecycleCategoryStaleCheck(acknowledgement manifest.AcknowledgedLifecycleCategory) DoctorCheck {
	return DoctorCheck{
		Name:    "stale lifecycle category acknowledgement " + acknowledgement.Category,
		Status:  "warning",
		Message: "acknowledged lifecycle category includes script outside that category",
		Details: map[string]any{
			"category": acknowledgement.Category,
			"scripts":  acknowledgement.Scripts,
			"reason":   acknowledgement.Reason,
		},
	}
}

func doctorLifecycleCategoryUnusedCheck(acknowledgement manifest.AcknowledgedLifecycleCategory) DoctorCheck {
	return DoctorCheck{
		Name:    "unused lifecycle category acknowledgement " + acknowledgement.Category,
		Status:  "warning",
		Message: "acknowledged lifecycle category did not match any lockfile capabilities",
		Details: map[string]any{
			"category": acknowledgement.Category,
			"scripts":  acknowledgement.Scripts,
			"reason":   acknowledgement.Reason,
		},
	}
}
func collectDoctorLifecycleCapabilities(lf *lockfile.Lockfile) []doctorLifecycleCapability {
	if lf == nil {
		return nil
	}
	capabilities := []doctorLifecycleCapability{}
	for _, pkg := range lf.Packages {
		for _, pkgCapability := range pkg.Capabilities {
			if !doctorIsLifecycleCapability(pkgCapability) {
				continue
			}
			script := pkgCapability.Script
			if script == "" {
				script = pkgCapability.Detail
			}
			classification := capability.ClassifyLifecycleScript(script)
			capabilities = append(capabilities, doctorLifecycleCapability{
				PackageID:   pkg.ID,
				Script:      script,
				Command:     pkgCapability.Command,
				Category:    classification.LifecycleCategory,
				InstallTime: classification.ConsumerInstallTime,
			})
		}
	}
	sort.SliceStable(capabilities, func(i, j int) bool {
		if capabilities[i].PackageID != capabilities[j].PackageID {
			return capabilities[i].PackageID < capabilities[j].PackageID
		}
		if capabilities[i].Script != capabilities[j].Script {
			return capabilities[i].Script < capabilities[j].Script
		}
		return capabilities[i].Command < capabilities[j].Command
	})
	return capabilities
}

func doctorLifecycleCapabilityCheck(root string, lifecycleCapability doctorLifecycleCapability, ackByExactKey map[string]manifest.AcknowledgedCapability, categoryAcknowledgements []manifest.AcknowledgedLifecycleCategory, staleByCapabilityKey map[string]manifest.AcknowledgedCapability, pathsByPackage map[string][]string) DoctorCheck {
	exactKey := doctorLifecycleAcknowledgementKey(lifecycleCapability.PackageID, lifecycleCapability.Script, lifecycleCapability.Command)
	acknowledgement, acknowledged := ackByExactKey[exactKey]
	categoryAcknowledgement, _, categoryAcknowledged := doctorMatchingLifecycleCategoryAcknowledgement(lifecycleCapability, categoryAcknowledgements)
	staleKey := doctorLifecycleStaleAcknowledgementKey(lifecycleCapability.PackageID, lifecycleCapability.Script)
	staleAck, stale := staleByCapabilityKey[staleKey]

	status := "warning"
	message := "package declares lifecycle script; execution is blocked by default"
	details := map[string]any{
		"package":             lifecycleCapability.PackageID,
		"lifecycleScriptName": lifecycleCapability.Script,
		"script":              lifecycleCapability.Script,
		"command":             lifecycleCapability.Command,
		"lifecycleCategory":   lifecycleCapability.Category,
		"consumerInstallTime": lifecycleCapability.InstallTime,
		"execution":           "blocked",
		"acknowledged":        acknowledged || categoryAcknowledged,
	}
	if !acknowledged && categoryAcknowledged {
		status = "ok"
		message = "lifecycle script capability acknowledged by category policy; execution remains blocked"
		details["acknowledgmentKind"] = "lifecycle-category"
		details["acknowledgedByCategory"] = categoryAcknowledgement.Category
		details["acknowledgedByScripts"] = categoryAcknowledgement.Scripts
		details["reason"] = categoryAcknowledgement.Reason
	}
	if acknowledged {
		status = "ok"
		message = "acknowledged lifecycle script capability; execution remains blocked"
		details["acknowledgmentKind"] = "capability"
		details["reason"] = acknowledgement.Reason
		addDoctorEvidenceDetails(root, details, acknowledgement)
	}
	if stale {
		details["stale"] = true
		details["acknowledgedCommand"] = staleAck.Command
		details["actualCommand"] = lifecycleCapability.Command
		details["reason"] = staleAck.Reason
	}
	if pulledBy := pathsByPackage[lifecycleCapability.PackageID]; len(pulledBy) > 0 {
		details["pulledBy"] = pulledBy
	}
	return DoctorCheck{
		Name:    "lifecycle " + lifecycleCapability.PackageID + " " + lifecycleCapability.Script,
		Status:  status,
		Message: message,
		Details: details,
	}
}

func doctorMatchingLifecycleCategoryAcknowledgement(lifecycleCapability doctorLifecycleCapability, acknowledgements []manifest.AcknowledgedLifecycleCategory) (manifest.AcknowledgedLifecycleCategory, int, bool) {
	for index, acknowledgement := range acknowledgements {
		if acknowledgement.Category != lifecycleCapability.Category {
			continue
		}
		if len(acknowledgement.Scripts) == 0 {
			return acknowledgement, index, true
		}
		for _, script := range acknowledgement.Scripts {
			if script == lifecycleCapability.Script {
				return acknowledgement, index, true
			}
		}
	}
	return manifest.AcknowledgedLifecycleCategory{}, -1, false
}

func doctorUnusedLifecycleCategoryAcknowledgements(acknowledgements []manifest.AcknowledgedLifecycleCategory, used map[int]int) []manifest.AcknowledgedLifecycleCategory {
	out := []manifest.AcknowledgedLifecycleCategory{}
	for index, acknowledgement := range acknowledgements {
		if used[index] == 0 {
			out = append(out, acknowledgement)
		}
	}
	return out
}

func doctorStaleLifecycleCategoryAcknowledgements(acknowledgements []manifest.AcknowledgedLifecycleCategory) []manifest.AcknowledgedLifecycleCategory {
	out := []manifest.AcknowledgedLifecycleCategory{}
	for _, acknowledgement := range acknowledgements {
		for _, script := range acknowledgement.Scripts {
			classification := capability.ClassifyLifecycleScript(script)
			if classification.LifecycleCategory != acknowledgement.Category {
				out = append(out, acknowledgement)
				break
			}
		}
	}
	return out
}
func doctorAcknowledgementsByExactKey(acknowledgements []manifest.AcknowledgedCapability) map[string]manifest.AcknowledgedCapability {
	byKey := map[string]manifest.AcknowledgedCapability{}
	for _, acknowledgement := range acknowledgements {
		if acknowledgement.Kind != capability.LifecycleScriptKind {
			continue
		}
		key := doctorLifecycleAcknowledgementKey(acknowledgement.Package, acknowledgement.Script, acknowledgement.Command)
		byKey[key] = acknowledgement
	}
	return byKey
}

func doctorFindStaleAcknowledgement(lifecycleCapability doctorLifecycleCapability, acknowledgements []manifest.AcknowledgedCapability) (manifest.AcknowledgedCapability, bool) {
	for _, acknowledgement := range acknowledgements {
		if acknowledgement.Kind != capability.LifecycleScriptKind {
			continue
		}
		if acknowledgement.Package != lifecycleCapability.PackageID {
			continue
		}
		if acknowledgement.Script != lifecycleCapability.Script {
			continue
		}
		if acknowledgement.Command == lifecycleCapability.Command {
			continue
		}
		return acknowledgement, true
	}
	return manifest.AcknowledgedCapability{}, false
}

func staleAcknowledgementsSorted(staleByCapabilityKey map[string]manifest.AcknowledgedCapability) []manifest.AcknowledgedCapability {
	keys := make([]string, 0, len(staleByCapabilityKey))
	for key := range staleByCapabilityKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]manifest.AcknowledgedCapability, 0, len(keys))
	for _, key := range keys {
		out = append(out, staleByCapabilityKey[key])
	}
	return out
}

func doctorIsLifecycleCapability(pkgCapability lockfile.Capability) bool {
	return pkgCapability.Kind == capability.LifecycleScriptKind || pkgCapability.Kind == "lifecycle-script"
}

func doctorLifecycleAcknowledgementKey(packageID string, script string, command string) string {
	return packageID + "|" + capability.LifecycleScriptKind + "|" + script + "|" + command
}

func doctorLifecycleStaleAcknowledgementKey(packageID string, script string) string {
	return packageID + "|" + capability.LifecycleScriptKind + "|" + script
}

func doctorLifecyclePulledByPathLines(lf *lockfile.Lockfile) map[string][]string {
	edgesByFrom := map[string][]lockfile.Edge{}
	for _, edge := range lf.Edges {
		edgesByFrom[edge.From] = append(edgesByFrom[edge.From], edge)
	}
	for from := range edgesByFrom {
		sort.SliceStable(edgesByFrom[from], func(i, j int) bool {
			if edgesByFrom[from][i].To != edgesByFrom[from][j].To {
				return edgesByFrom[from][i].To < edgesByFrom[from][j].To
			}
			return edgesByFrom[from][i].Kind < edgesByFrom[from][j].Kind
		})
	}

	roots := []string{}
	for from := range edgesByFrom {
		if strings.Contains(from, ":target:") || strings.HasSuffix(from, ":tool") {
			roots = append(roots, from)
		}
	}
	sort.Strings(roots)

	pathsByPackage := map[string][]string{}
	for _, root := range roots {
		queue := [][]string{{root}}
		seen := map[string]bool{}
		for len(queue) > 0 {
			path := queue[0]
			queue = queue[1:]
			current := path[len(path)-1]
			for _, edge := range edgesByFrom[current] {
				if seen[edge.To] {
					continue
				}
				seen[edge.To] = true
				nextPath := append(append([]string(nil), path...), edge.To)
				pathsByPackage[edge.To] = append(pathsByPackage[edge.To], strings.Join(nextPath, " -> "))
				queue = append(queue, nextPath)
			}
		}
	}
	for packageID := range pathsByPackage {
		sort.Strings(pathsByPackage[packageID])
	}
	return pathsByPackage
}

func doctorBehaviorEvidenceChecks(root string, acknowledgements []manifest.AcknowledgedCapability) []DoctorCheck {
	if len(acknowledgements) == 0 {
		return nil
	}
	presentFixtures := 0
	missingFixtures := 0
	presentReports := 0
	missingReports := 0
	invalidReports := 0
	checks := []DoctorCheck{}
	for _, acknowledgement := range acknowledgements {
		evidence := securityevidence.Evaluate(root, acknowledgement)
		if evidence.BehaviorFixtureStatus == securityevidence.StatusPresent {
			presentFixtures++
		}
		if evidence.BehaviorFixtureStatus == securityevidence.StatusMissing {
			missingFixtures++
			checks = append(checks, DoctorCheck{
				Name:    "behavior fixture missing " + acknowledgement.Package + " " + acknowledgement.Script,
				Status:  "warning",
				Message: "acknowledged lifecycle behavior fixture is missing",
				Details: doctorEvidenceDetails(root, acknowledgement),
			})
		}
		if evidence.BehaviorReportStatus == securityevidence.StatusPresent {
			presentReports++
		}
		if evidence.BehaviorReportStatus == securityevidence.StatusMissing {
			missingReports++
			checks = append(checks, DoctorCheck{
				Name:    "behavior report missing " + acknowledgement.Package + " " + acknowledgement.Script,
				Status:  "warning",
				Message: "acknowledged lifecycle behavior report is missing",
				Details: doctorEvidenceDetails(root, acknowledgement),
			})
		}
		if evidence.BehaviorReportStatus == securityevidence.StatusInvalid {
			invalidReports++
			checks = append(checks, DoctorCheck{
				Name:    "behavior report invalid " + acknowledgement.Package + " " + acknowledgement.Script,
				Status:  "warning",
				Message: "acknowledged lifecycle behavior report is not valid JSON",
				Details: doctorEvidenceDetails(root, acknowledgement),
			})
		}
	}
	status := "ok"
	if missingFixtures > 0 || missingReports > 0 || invalidReports > 0 {
		status = "warning"
	}
	checks = append([]DoctorCheck{{
		Name:    "behavior evidence summary",
		Status:  status,
		Message: "behavior evidence references are validated without running fixtures",
		Details: map[string]any{
			"behaviorFixturesPresent": presentFixtures,
			"behaviorFixturesMissing": missingFixtures,
			"behaviorReportsPresent":  presentReports,
			"behaviorReportsMissing":  missingReports,
			"behaviorReportsInvalid":  invalidReports,
			"execution":               "not run by doctor security",
		},
	}}, checks...)
	return checks
}

func addDoctorEvidenceDetails(root string, details map[string]any, acknowledgement manifest.AcknowledgedCapability) {
	for key, value := range doctorEvidenceDetails(root, acknowledgement) {
		details[key] = value
	}
}

func doctorEvidenceDetails(root string, acknowledgement manifest.AcknowledgedCapability) map[string]any {
	evidence := securityevidence.Evaluate(root, acknowledgement)
	details := map[string]any{
		"package": acknowledgement.Package,
		"script":  acknowledgement.Script,
		"command": acknowledgement.Command,
		"reason":  acknowledgement.Reason,
	}
	if evidence.BehaviorFixture != "" {
		details["behaviorFixture"] = evidence.BehaviorFixture
		details["behaviorFixtureStatus"] = evidence.BehaviorFixtureStatus
	}
	if evidence.BehaviorReport != "" {
		details["behaviorReport"] = evidence.BehaviorReport
		details["behaviorReportStatus"] = evidence.BehaviorReportStatus
	}
	return details
}

func doctorSecurityPostureCheck() DoctorCheck {
	return DoctorCheck{
		Name:    "lifecycle execution posture",
		Status:  "ok",
		Message: "update, sync, and materialization do not execute lifecycle scripts",
		Details: map[string]any{
			"execution":        "blocked by default",
			"normalOperations": "update/sync/materialization do not run lifecycle scripts",
			"explicitTesting":  "lifecycle behavior probes are available through native xTest lifecycle.runScript; doctor does not run probes",
		},
	}
}
