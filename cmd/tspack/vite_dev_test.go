package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yuechen-li-dev/tspack/internal/manifest"
)

func TestSelectCopelandViteDevProjectSelectsOneBrowserTarget(t *testing.T) {
	root := t.TempDir()
	ir := &manifest.ManifestIR{Packages: []manifest.Package{{
		Name:     "copeland-app",
		Compiler: "tscl",
		Targets: []manifest.Target{{
			Name:              "browser",
			Runtime:           "dist/browser/main.js",
			JavaScriptRuntime: "browser",
		}},
	}}}

	project, err := selectCopelandViteDevProject(root, filepath.Join(root, "manifest.tsx"), ir, "")
	if err != nil {
		t.Fatal(err)
	}
	if project.Package.Name != "copeland-app" || project.Target.Name != "browser" {
		t.Fatalf("selected project = %#v", project)
	}
}

func TestWriteCopelandViteDevFilesKeepsViteDownstreamOfCopelandOutput(t *testing.T) {
	root := t.TempDir()
	outputDirectory := filepath.Join(root, "dist", "browser")
	if err := os.MkdirAll(filepath.Join(root, "node_modules", "vite", "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(outputDirectory, "packages", "copeland-browser-v1"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "node_modules", "vite", "bin", "vite.js"), []byte("// fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	project := copelandViteDevProject{
		Package:       &manifest.Package{Name: "copeland-app"},
		PackageRoot:   root,
		WorkspaceRoot: root,
		ManifestPath:  filepath.Join(root, "manifest.tsx"),
		Target: manifest.Target{
			Name:              "browser",
			Runtime:           "dist/browser/main.js",
			JavaScriptRuntime: "browser",
		},
	}
	files, err := writeCopelandViteDevFiles(project, nil, defaultViteDevPort)
	if err != nil {
		t.Fatal(err)
	}
	config, err := os.ReadFile(files.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	supervisor, err := os.ReadFile(files.SupervisorPath)
	if err != nil {
		t.Fatal(err)
	}

	configText := string(config)
	if !strings.Contains(configText, "@copeland/browser-v1") || !strings.Contains(configText, "tspack-copeland-full-reload") {
		t.Fatalf("generated config did not provide the browser runtime/reload infrastructure:\n%s", configText)
	}
	supervisorText := string(supervisor)
	for _, expected := range []string{
		"--preserve-last-successful",
		"TSPACK_VITE_DEV_COMPILE_FAILED",
		"await cp(outputDirectory, serveDirectory",
		"writeFileSync(reloadPath",
		"watch(sourceDirectory",
	} {
		if !strings.Contains(supervisorText, expected) {
			t.Fatalf("generated supervisor is missing %q:\n%s", expected, supervisorText)
		}
	}
}

func TestBrowserOutputDirectoryRejectsEscapingRuntime(t *testing.T) {
	_, err := browserOutputDirectory(t.TempDir(), manifest.Target{Name: "browser", Runtime: "../outside/main.js"})
	if err == nil {
		t.Fatal("expected unsafe runtime path to fail")
	}
}

func TestResolveViteDevBackendInterpolatesTargetAndGeneratesWebSocketProxy(t *testing.T) {
	project := copelandViteDevProject{
		Package: &manifest.Package{
			DevBackend: &manifest.DevBackend{
				Kind:        "aspnet",
				Command:     []string{"dotnet", "run"},
				URL:         "http://127.0.0.1:${BACKEND_PORT}",
				Ready:       &manifest.RunReadyCheck{Kind: "http", Path: "/api/status"},
				OwnsProcess: true,
				Env:         []manifest.RunTargetEnv{{Name: "BACKEND_PORT", Default: stringPointer("5187")}},
				ProxyRoutes: []manifest.DevProxyRoute{{Path: "/api"}, {Path: "/hub", Target: "ws://127.0.0.1:${BACKEND_PORT}", WebSocket: true}},
			},
		},
		PackageRoot:   t.TempDir(),
		WorkspaceRoot: t.TempDir(),
	}

	backend, err := resolveViteDevBackend(project, runEnvOverlay{})
	if err != nil {
		t.Fatal(err)
	}
	if backend.Target.URL != "http://127.0.0.1:5187" || len(backend.Proxies) != 2 {
		t.Fatalf("resolved backend = %#v", backend)
	}
	config := viteDevConfig(t.TempDir(), t.TempDir(), backend.Proxies, 5190)
	for _, expected := range []string{"\"/api\"", "http://127.0.0.1:5187", "\"/hub\"", "ws://127.0.0.1:5187", "ws: true", "secure: false"} {
		if !strings.Contains(config, expected) {
			t.Fatalf("proxy config missing %q:\n%s", expected, config)
		}
	}
}

func TestViteDevPortUsesOverlayAndRejectsInvalidValues(t *testing.T) {
	overlay := runEnvOverlay{Values: map[string]string{"VITE_PORT": "5190"}}
	port, err := viteDevPort(overlay)
	if err != nil || port != 5190 {
		t.Fatalf("port = %d, err = %v", port, err)
	}
	_, err = viteDevPort(runEnvOverlay{Values: map[string]string{"VITE_PORT": "invalid"}})
	if err == nil {
		t.Fatal("expected invalid VITE_PORT to fail")
	}
}

func stringPointer(value string) *string {
	return &value
}
