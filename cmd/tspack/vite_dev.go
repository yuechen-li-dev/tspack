package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/yuechen-li-dev/tspack/internal/manifest"
)

const (
	defaultViteDevHost = "127.0.0.1"
	defaultViteDevPort = 5173
)

type copelandViteDevProject struct {
	Package       *manifest.Package
	PackageRoot   string
	Target        manifest.Target
	WorkspaceRoot string
	ManifestPath  string
}

type viteDevFiles struct {
	SupervisorPath string
	ConfigPath     string
	ReloadPath     string
	URL            string
	Port           int
	ServeDirectory string
}

type viteDevBackend struct {
	Target  manifest.RunTarget
	Cwd     string
	Owns    bool
	Proxies []viteDevProxy
}

type viteDevProxy struct {
	Path      string
	Target    string
	WebSocket bool
	Secure    bool
}

// tryRunCopelandViteDev recognizes the compiler-owned browser development
// path. It intentionally runs before ordinary RunTarget selection: a Copeland
// browser package does not need an application-authored Vite command or config.
func tryRunCopelandViteDev(opts runCommandOptions, workspaceRoot string, manifestPath string, ir *manifest.ManifestIR) bool {
	if opts.TargetArg != "dev" || opts.List || opts.PreflightOnly {
		return false
	}

	project, err := selectCopelandViteDevProject(workspaceRoot, manifestPath, ir, opts.PackageName)
	if err != nil {
		if err == errNoCopelandViteDevProject {
			return false
		}
		failRun("TSPACK_VITE_DEV_PROJECT_INVALID", err.Error())
	}
	project, err = normalizeCopelandViteDevProject(project)
	if err != nil {
		failRun("TSPACK_VITE_DEV_PROJECT_INVALID", err.Error())
	}
	backend, err := resolveViteDevBackend(project, opts.Env)
	if err != nil {
		failRun("TSPACK_VITE_BACKEND_CONFIG_INVALID", err.Error())
	}

	frontendPort, err := viteDevPort(opts.Env)
	if err != nil {
		failRun("TSPACK_VITE_DEV_PORT_INVALID", err.Error())
	}
	files, err := writeCopelandViteDevFiles(project, backend.Proxies, frontendPort)
	if err != nil {
		failRun("TSPACK_VITE_DEV_CONFIG_FAILED", err.Error())
	}

	target := manifest.RunTarget{
		Name:    "dev",
		Runtime: "node",
		Command: []string{"node", files.SupervisorPath},
		URL:     files.URL,
		Cwd:     "workspace",
		Ready:   &manifest.RunReadyCheck{Kind: "http", Path: "/"},
	}

	fmt.Fprintf(os.Stderr, "Starting Vite-backed Copeland development for %q\n", project.Package.Name)
	fmt.Fprintf(os.Stderr, "Copeland target: %s\n", project.Target.Name)
	fmt.Fprintf(os.Stderr, "Vite config: %s\n", files.ConfigPath)
	fmt.Fprintf(os.Stderr, "Vite URL: %s\n", files.URL)
	var backendSession *RunTargetSession
	if backend.Owns {
		fmt.Fprintf(os.Stderr, "Starting development backend %q at %s\n", backend.Target.Name, backend.Target.URL)
		fmt.Fprintf(os.Stderr, "Waiting for backend: %s\n", newReadyCheck(backend.Target).waitingDescription())
		backendSession, backendReadyErr := startRunTargetInDir(project.WorkspaceRoot, backend.Cwd, backend.Target, time.Duration(opts.TimeoutSeconds)*time.Second, os.Stdout, os.Stderr, opts.Env)
		if backendReadyErr != nil {
			failRun("TSPACK_VITE_BACKEND_READINESS_FAILED", "backend="+backend.Target.URL+"; "+backendReadyErr.msg)
		}
		fmt.Fprintf(os.Stderr, "Backend ready: %s\n", backendSession.ReadyDescription)
	}
	fmt.Fprintln(os.Stderr, "Waiting for: HTTP frontend readiness")

	session, readyErr := startRunTargetInDir(project.WorkspaceRoot, project.PackageRoot, target, time.Duration(opts.TimeoutSeconds)*time.Second, os.Stdout, os.Stderr, opts.Env)
	if readyErr != nil {
		if backendSession != nil {
			_ = backendSession.Stop()
		}
		failRun(viteDevDiagnosticCode(readyErr.code), viteDevDiagnosticMessage(readyErr.msg, files.URL))
	}
	fmt.Fprintf(os.Stderr, "Ready: %s\n", session.ReadyDescription)
	fmt.Fprintf(os.Stderr, "Frontend URL: %s\n", files.URL)
	if opts.Once {
		if err := stopViteDevSessions(session, backendSession); err != nil {
			failRun("TSPACK_VITE_DEV_CLEANUP_FAILED", err.Error())
		}
		return true
	}

	waitForViteDevShutdown(session, backendSession)
	return true
}

func waitForViteDevShutdown(session *RunTargetSession, backendSession *RunTargetSession) {
	stopSignals := make(chan os.Signal, 2)
	signal.Notify(stopSignals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(stopSignals)
	go func() {
		<-stopSignals
		_ = stopViteDevSessions(session, backendSession)
	}()

	waitErr := session.Wait()
	cleanupErr := session.closeCleanupHandle()
	if waitErr != nil {
		failRun(runProcessExitCode(waitErr), "Vite development process: "+runProcessExitMessage(waitErr))
	}
	if cleanupErr != nil {
		failRun("TSPACK_VITE_DEV_CLEANUP_FAILED", cleanupErr.Error())
	}
	if backendSession != nil {
		if err := backendSession.Stop(); err != nil {
			failRun("TSPACK_VITE_BACKEND_CLEANUP_FAILED", err.Error())
		}
	}
}

func stopViteDevSessions(frontendSession *RunTargetSession, backendSession *RunTargetSession) error {
	if frontendSession != nil {
		if err := frontendSession.Stop(); err != nil {
			return err
		}
	}
	if backendSession != nil {
		return backendSession.Stop()
	}
	return nil
}

var errNoCopelandViteDevProject = fmt.Errorf("no Copeland browser target selected")

func selectCopelandViteDevProject(workspaceRoot string, manifestPath string, ir *manifest.ManifestIR, packageName string) (copelandViteDevProject, error) {
	candidates := []copelandViteDevProject{}
	for packageIndex := range ir.Packages {
		pkg := &ir.Packages[packageIndex]
		if packageName != "" && packageName != pkg.Name {
			continue
		}
		if pkg.Compiler != "tscl" {
			continue
		}
		for _, target := range pkg.Targets {
			if target.JavaScriptRuntime != "browser" {
				continue
			}
			candidates = append(candidates, copelandViteDevProject{
				Package:       pkg,
				PackageRoot:   resolvePackageRoot(workspaceRoot, manifestPath, ir, pkg),
				Target:        target,
				WorkspaceRoot: workspaceRoot,
				ManifestPath:  manifestPath,
			})
		}
	}

	if len(candidates) == 0 {
		return copelandViteDevProject{}, errNoCopelandViteDevProject
	}
	if len(candidates) > 1 {
		names := make([]string, 0, len(candidates))
		for _, candidate := range candidates {
			names = append(names, candidate.Package.Name+":"+candidate.Target.Name)
		}
		sort.Strings(names)
		return copelandViteDevProject{}, fmt.Errorf("multiple Copeland browser targets match dev: %s; use --package", strings.Join(names, ", "))
	}
	return candidates[0], nil
}

func normalizeCopelandViteDevProject(project copelandViteDevProject) (copelandViteDevProject, error) {
	absoluteWorkspaceRoot, err := filepath.Abs(project.WorkspaceRoot)
	if err != nil {
		return copelandViteDevProject{}, fmt.Errorf("resolve workspace root: %w", err)
	}
	absolutePackageRoot, err := filepath.Abs(project.PackageRoot)
	if err != nil {
		return copelandViteDevProject{}, fmt.Errorf("resolve package root: %w", err)
	}
	absoluteManifestPath, err := filepath.Abs(project.ManifestPath)
	if err != nil {
		return copelandViteDevProject{}, fmt.Errorf("resolve manifest path: %w", err)
	}
	project.WorkspaceRoot = absoluteWorkspaceRoot
	project.PackageRoot = absolutePackageRoot
	project.ManifestPath = absoluteManifestPath
	return project, nil
}

func resolveViteDevBackend(project copelandViteDevProject, overlay runEnvOverlay) (viteDevBackend, error) {
	if project.Package.DevBackend == nil {
		return viteDevBackend{}, nil
	}
	backend := project.Package.DevBackend
	target := manifest.RunTarget{
		Name:    "dev-backend",
		Runtime: "system",
		Command: backend.Command,
		URL:     backend.URL,
		Cwd:     backend.Cwd,
		Ready:   backend.Ready,
		Env:     backend.Env,
	}
	if target.Cwd == "" {
		target.Cwd = "package"
	}
	prepared, env, prepareErr := prepareRunTargetForInvocation(project.WorkspaceRoot, target, overlay)
	if prepareErr != nil {
		return viteDevBackend{}, fmt.Errorf("%s: %s", prepareErr.code, prepareErr.msg)
	}
	result := viteDevBackend{Target: prepared, Owns: backend.OwnsProcess}
	if target.Cwd == "workspace" {
		result.Cwd = project.WorkspaceRoot
	} else {
		result.Cwd = project.PackageRoot
	}
	for _, route := range backend.ProxyRoutes {
		proxyTarget := route.Target
		if proxyTarget == "" {
			proxyTarget = prepared.URL
		} else {
			interpolated, interpolationErr := interpolateRunReadyURL(prepared, proxyTarget, env)
			if interpolationErr != nil {
				return viteDevBackend{}, fmt.Errorf("%s: %s", interpolationErr.code, interpolationErr.msg)
			}
			proxyTarget = interpolated
		}
		result.Proxies = append(result.Proxies, viteDevProxy{
			Path:      route.Path,
			Target:    proxyTarget,
			WebSocket: route.WebSocket,
			Secure:    route.Secure,
		})
	}
	sort.Slice(result.Proxies, func(i int, j int) bool { return result.Proxies[i].Path < result.Proxies[j].Path })
	return result, nil
}

func writeCopelandViteDevFiles(project copelandViteDevProject, proxies []viteDevProxy, port int) (viteDevFiles, error) {

	outputDirectory, err := browserOutputDirectory(project.PackageRoot, project.Target)
	if err != nil {
		return viteDevFiles{}, err
	}
	viteBin := filepath.Join(project.PackageRoot, "node_modules", "vite", "bin", "vite.js")
	if _, err := os.Stat(viteBin); err != nil {
		return viteDevFiles{}, fmt.Errorf("Vite is not materialized for package %q at %s; declare Vite as a TSPack tool, then run `tspack update` and `tspack sync`", project.Package.Name, viteBin)
	}

	devDirectory := filepath.Join(project.PackageRoot, ".tspack", "vite-dev")
	if err := os.MkdirAll(devDirectory, 0o755); err != nil {
		return viteDevFiles{}, err
	}
	reloadPath := filepath.Join(devDirectory, "reload.txt")
	if err := os.WriteFile(reloadPath, []byte("initial\n"), 0o644); err != nil {
		return viteDevFiles{}, err
	}

	serveDirectory := filepath.Join(devDirectory, "served")
	if err := os.MkdirAll(serveDirectory, 0o755); err != nil {
		return viteDevFiles{}, err
	}
	configPath := filepath.Join(devDirectory, "vite.config.mjs")
	config := viteDevConfig(serveDirectory, reloadPath, proxies, port)
	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		return viteDevFiles{}, err
	}

	executablePath, err := os.Executable()
	if err != nil {
		return viteDevFiles{}, fmt.Errorf("locate tspack executable: %w", err)
	}
	supervisorPath := filepath.Join(devDirectory, "supervisor.mjs")
	supervisor := viteDevSupervisor(project, executablePath, viteBin, configPath, reloadPath, outputDirectory, serveDirectory, port)
	if err := os.WriteFile(supervisorPath, []byte(supervisor), 0o644); err != nil {
		return viteDevFiles{}, err
	}

	return viteDevFiles{
		SupervisorPath: supervisorPath,
		ConfigPath:     configPath,
		ReloadPath:     reloadPath,
		URL:            fmt.Sprintf("http://%s:%d", defaultViteDevHost, port),
		Port:           port,
		ServeDirectory: serveDirectory,
	}, nil
}

func browserOutputDirectory(packageRoot string, target manifest.Target) (string, error) {
	runtimePath := filepath.Clean(target.Runtime)
	if filepath.IsAbs(runtimePath) || runtimePath == "." || runtimePath == ".." || strings.HasPrefix(runtimePath, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("Copeland browser target %q has an unsafe runtime path %q", target.Name, target.Runtime)
	}
	return filepath.Join(packageRoot, filepath.Dir(runtimePath)), nil
}

func viteDevPort(overlay runEnvOverlay) (int, error) {
	value := overlay.Values["VITE_PORT"]
	if value == "" {
		return defaultViteDevPort, nil
	}
	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("VITE_PORT must be an integer from 1 through 65535")
	}
	return port, nil
}

func viteDevConfig(outputDirectory string, reloadPath string, proxies []viteDevProxy, port int) string {
	proxyConfig := viteDevProxyConfig(proxies)
	return `import { watch } from "node:fs";

const reloadPath = ` + javascriptString(reloadPath) + `;

export default {
  root: ` + javascriptString(outputDirectory) + `,
  appType: "spa",
  clearScreen: false,
  resolve: {
    alias: {
      "@copeland/browser-v1": ` + javascriptString(filepath.Join(outputDirectory, "packages", "copeland-browser-v1", "index.js")) + `,
    },
  },
  server: {
    host: "` + defaultViteDevHost + `",
    port: ` + fmt.Sprint(port) + `,
    strictPort: true,
    fs: {
      allow: [` + javascriptString(outputDirectory) + `],
    },
	` + proxyConfig + `
  },
  plugins: [{
    name: "tspack-copeland-full-reload",
    configureServer(server) {
      const watcher = watch(reloadPath, () => {
        server.ws.send({ type: "full-reload", path: "*" });
      });
      server.httpServer?.once("close", () => watcher.close());
    },
  }],
};
`
}

func viteDevProxyConfig(proxies []viteDevProxy) string {
	if len(proxies) == 0 {
		return ""
	}
	var builder strings.Builder
	builder.WriteString("proxy: {\n")
	for _, proxy := range proxies {
		builder.WriteString("  ")
		builder.WriteString(javascriptString(proxy.Path))
		builder.WriteString(": { target: ")
		builder.WriteString(javascriptString(proxy.Target))
		builder.WriteString(", changeOrigin: true, ws: ")
		builder.WriteString(fmt.Sprint(proxy.WebSocket))
		builder.WriteString(", secure: ")
		builder.WriteString(fmt.Sprint(proxy.Secure))
		builder.WriteString(" },\n")
	}
	builder.WriteString("},")
	return builder.String()
}

func viteDevSupervisor(project copelandViteDevProject, tspackPath string, viteBin string, configPath string, reloadPath string, outputDirectory string, serveDirectory string, port int) string {
	return `import { spawn } from "node:child_process";
import { watch, writeFileSync } from "node:fs";
import { cp } from "node:fs/promises";

const tspackPath = ` + javascriptString(tspackPath) + `;
const workspaceRoot = ` + javascriptString(project.WorkspaceRoot) + `;
const packageName = ` + javascriptString(project.Package.Name) + `;
const targetName = ` + javascriptString(project.Target.Name) + `;
const viteBin = ` + javascriptString(viteBin) + `;
const configPath = ` + javascriptString(configPath) + `;
const reloadPath = ` + javascriptString(reloadPath) + `;
const outputDirectory = ` + javascriptString(outputDirectory) + `;
const serveDirectory = ` + javascriptString(serveDirectory) + `;
const sourceDirectory = ` + javascriptString(filepath.Join(project.PackageRoot, "src")) + `;
const manifestPath = ` + javascriptString(project.ManifestPath) + `;

let viteProcess;
let rebuildTimer;
let stopping = false;

function run(command, args) {
  return new Promise(resolve => {
    const child = spawn(command, args, { cwd: workspaceRoot, stdio: "inherit" });
    child.once("error", error => {
      console.error("TSPACK_VITE_DEV_PROCESS_FAILED: " + error.message);
      resolve(1);
    });
    child.once("exit", code => resolve(code ?? 1));
  });
}

async function rebuild() {
  const code = await run(tspackPath, ["build", "--root", workspaceRoot, "--manifest", manifestPath, "--package", packageName, "--preserve-last-successful", targetName]);
  if (code !== 0) {
    console.error("TSPACK_VITE_DEV_COMPILE_FAILED: retained the last successful browser output; correct the Copeland source and save again.");
    return false;
  }
  await cp(outputDirectory, serveDirectory, { recursive: true, force: true });
  writeFileSync(reloadPath, new Date().toISOString() + "\n");
  return true;
}

function scheduleRebuild(reason) {
  clearTimeout(rebuildTimer);
  rebuildTimer = setTimeout(async () => {
    console.log("TSPACK_VITE_DEV_REBUILD: " + reason);
    await rebuild();
  }, 150);
}

async function main() {
  if (!await rebuild()) {
    process.exitCode = 1;
    return;
  }

  viteProcess = spawn(process.execPath, [viteBin, "--config", configPath, "--host", "` + defaultViteDevHost + `", "--port", "` + fmt.Sprint(port) + `", "--strictPort"], { cwd: workspaceRoot, stdio: "inherit" });
  viteProcess.once("error", error => {
    console.error("TSPACK_VITE_DEV_START_FAILED: " + error.message);
    process.exitCode = 1;
  });
  viteProcess.once("exit", code => {
    if (!stopping && code !== 0) {
      console.error("TSPACK_VITE_DEV_CHILD_EXITED: Vite exited with code " + code + ".");
      process.exitCode = code ?? 1;
    }
  });

  const sourceWatcher = watch(sourceDirectory, { recursive: true }, (_event, filename) => {
    const changedPath = filename ? String(filename) : "source";
    if (!changedPath.includes(".tspack")) {
      scheduleRebuild(changedPath);
    }
  });
  const manifestWatcher = watch(manifestPath, () => scheduleRebuild("manifest.tsx"));
  const stop = () => {
    if (stopping) return;
    stopping = true;
    sourceWatcher.close();
    manifestWatcher.close();
    viteProcess?.kill("SIGTERM");
  };
  process.once("SIGINT", stop);
  process.once("SIGTERM", stop);
}

await main();
`
}

func javascriptString(value string) string {
	encoded, _ := json.Marshal(filepath.ToSlash(value))
	return string(encoded)
}

func viteDevDiagnosticCode(code string) string {
	if code == "TSPACK_RUN_PROCESS_EXITED_EARLY" {
		return "TSPACK_VITE_DEV_PROCESS_EXITED_EARLY"
	}
	if code == "TSPACK_RUN_TOOL_NOT_FOUND" {
		return "TSPACK_VITE_DEV_NODE_UNAVAILABLE"
	}
	return "TSPACK_VITE_DEV_READINESS_FAILED"
}

func viteDevDiagnosticMessage(message string, frontendURL string) string {
	return message + "; frontend=" + frontendURL + "; check Copeland diagnostics above and confirm the configured Vite port is available"
}
