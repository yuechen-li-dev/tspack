package bridge

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type Resolution struct {
	Path              string
	SearchedPaths     []string
	Embedded          bool
	EmbeddedSupported bool
	SelectedSource    string
	OverrideEnv       string
	OverridePath      string
	CWD               string
	ExecutablePath    string
	ProjectRoot       string
	SourceRepoRoot    string
}

type ResolveOptions struct {
	CWD            string
	ExecutablePath string
	ProjectRoot    string
	SourceRepoRoot string
}

func Resolve(name string) Resolution {
	return ResolveWithOptions(name, ResolveOptions{})
}

func ResolveWithOptions(name string, opts ResolveOptions) Resolution {
	cwd := firstNonEmpty(opts.CWD, mustGetwd())
	executablePath := firstNonEmpty(opts.ExecutablePath, mustExecutable())
	sourceRepoRoot := firstNonEmpty(opts.SourceRepoRoot, sourceRepositoryRoot())
	if !shouldUseSourceRepoRoot(sourceRepoRoot, executablePath, cwd) {
		sourceRepoRoot = ""
	}
	resolution := Resolution{
		CWD:               cwd,
		ExecutablePath:    executablePath,
		ProjectRoot:       opts.ProjectRoot,
		SourceRepoRoot:    sourceRepoRoot,
		EmbeddedSupported: embeddedSupportEnabled(),
	}

	overrideEnv, overridePath := overrideFor(name)
	if overrideEnv != "" {
		resolution.OverrideEnv = overrideEnv
		resolution.OverridePath = overridePath
		resolution.SearchedPaths = append(resolution.SearchedPaths, overridePath)
		if fileExists(overridePath) {
			resolution.Path = overridePath
			resolution.SelectedSource = "override:" + overrideEnv
		}
		return resolution
	}

	if path, ok := resolveEmbedded(name); ok {
		resolution.Path = path
		resolution.Embedded = true
		resolution.SelectedSource = "embedded"
		return resolution
	}

	candidates := filesystemCandidates(name, resolution)
	resolution.SearchedPaths = append(resolution.SearchedPaths, candidates...)
	for _, candidate := range candidates {
		if fileExists(candidate) {
			resolution.Path = candidate
			resolution.SelectedSource = describeCandidateSource(candidate, resolution)
			return resolution
		}
	}
	return resolution
}

func ResolveFilesystem(name string) Resolution {
	return ResolveWithOptions(name, ResolveOptions{})
}

func filesystemCandidates(name string, resolution Resolution) []string {
	candidates := []string{}
	executableDir := ""
	if resolution.ExecutablePath != "" {
		executableDir = filepath.Dir(resolution.ExecutablePath)
	}

	for _, root := range []string{
		executableDir,
		filepath.Join(executableDir, ".."),
		filepath.Join(executableDir, "..", ".."),
	} {
		candidates = append(candidates, manifestFrontendBridgeCandidates(root, name)...)
	}

	if executableDir != "" {
		candidates = append(candidates, manifestFrontendSharedBridgeCandidates(executableDir, name)...)
	}

	if resolution.SourceRepoRoot != "" {
		candidates = append(candidates, manifestFrontendBridgeCandidates(resolution.SourceRepoRoot, name)...)
	}

	if resolution.CWD != "" && resolution.SourceRepoRoot != "" && pathWithinRoot(resolution.CWD, resolution.SourceRepoRoot) {
		candidates = append(candidates, manifestFrontendBridgeCandidates(resolution.CWD, name)...)
	}

	if overrideDir := os.Getenv("TSPACK_MANIFEST_FRONTEND_BRIDGE_DIR"); overrideDir != "" {
		candidates = append([]string{filepath.Join(overrideDir, name)}, candidates...)
	}

	return dedupeNonEmpty(candidates)
}

func BuildNeededDetails() []string {
	return []string{
		"Build manifest frontend bridges with:",
		"  cd manifest-frontend && npm run build",
		"  npm --prefix manifest-frontend run build",
		"Build a local Windows binary with:",
		"  go build -o .\\dist\\tspack.exe .\\cmd\\tspack",
		"Override the manifest frontend CLI with:",
		"  TSPACK_MANIFEST_FRONTEND=<path-to-cli.js>",
		"or build a release binary with:",
		"  ./scripts/build-release.sh",
	}
}

func MissingMessage(code string, label string, resolution Resolution) string {
	message := fmt.Sprintf("%s: %s not found\n", code, label)
	for _, line := range ResolutionDetails(resolution) {
		message += line + "\n"
	}
	message += "searched paths:\n"
	for _, searched := range resolution.SearchedPaths {
		message += fmt.Sprintf("  %s\n", searched)
	}
	for _, line := range BuildNeededDetails() {
		message += line + "\n"
	}
	return message
}

func ResolutionDetails(resolution Resolution) []string {
	lines := []string{}
	if resolution.ProjectRoot != "" {
		lines = append(lines, "project root: "+resolution.ProjectRoot)
	}
	if resolution.CWD != "" {
		lines = append(lines, "cwd: "+resolution.CWD)
	}
	if resolution.ExecutablePath != "" {
		lines = append(lines, "executable: "+resolution.ExecutablePath)
	}
	if resolution.SourceRepoRoot != "" {
		lines = append(lines, "tspack source repo: "+resolution.SourceRepoRoot)
	}
	if resolution.OverrideEnv != "" {
		lines = append(lines, fmt.Sprintf("frontend override: %s=%s", resolution.OverrideEnv, resolution.OverridePath))
	} else {
		lines = append(lines, "frontend override: not set")
	}
	if resolution.Embedded {
		lines = append(lines, "embedded frontend: selected")
	} else if resolution.EmbeddedSupported {
		lines = append(lines, "embedded frontend: supported by this build but not selected")
	} else {
		lines = append(lines, "embedded frontend: unavailable in this build")
	}
	if resolution.SelectedSource != "" {
		lines = append(lines, "selected frontend source: "+resolution.SelectedSource)
	}
	return lines
}

func overrideFor(name string) (string, string) {
	if name == "cli.js" {
		if override := strings.TrimSpace(os.Getenv("TSPACK_MANIFEST_FRONTEND")); override != "" {
			return "TSPACK_MANIFEST_FRONTEND", override
		}
		if override := strings.TrimSpace(os.Getenv("TSPACK_MANIFEST_FRONTEND_CLI")); override != "" {
			return "TSPACK_MANIFEST_FRONTEND_CLI", override
		}
	}
	return "", ""
}

func manifestFrontendBridgeCandidates(root string, bridgeName string) []string {
	if root == "" {
		return nil
	}
	return []string{
		filepath.Join(root, "manifest-frontend", "dist", bridgeName),
		filepath.Join(root, "manifest-frontend", "dist", "src", bridgeName),
	}
}

func manifestFrontendSharedBridgeCandidates(execDir string, bridgeName string) []string {
	if execDir == "" {
		return nil
	}
	return []string{
		filepath.Join(execDir, "..", "share", "tspack", "manifest-frontend", "dist", bridgeName),
		filepath.Join(execDir, "..", "share", "tspack", "manifest-frontend", "dist", "src", bridgeName),
	}
}

func sourceRepositoryRoot() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return ""
	}
	dir := filepath.Dir(file)
	for {
		if fileExists(filepath.Join(dir, "go.mod")) && dirContainsManifestFrontend(dir) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func dirContainsManifestFrontend(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, "manifest-frontend"))
	return err == nil && info.IsDir()
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func dedupeNonEmpty(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func mustGetwd() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return cwd
}

func mustExecutable() string {
	executable, err := os.Executable()
	if err != nil {
		return ""
	}
	return executable
}

func pathWithinRoot(path string, root string) bool {
	if path == "" || root == "" {
		return false
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func shouldUseSourceRepoRoot(root string, executablePath string, cwd string) bool {
	if root == "" {
		return false
	}
	if executablePath != "" && pathWithinRoot(executablePath, root) {
		return true
	}
	if cwd != "" && pathWithinRoot(cwd, root) {
		return true
	}
	return false
}

func describeCandidateSource(candidate string, resolution Resolution) string {
	if resolution.ExecutablePath != "" {
		execDir := filepath.Dir(resolution.ExecutablePath)
		for _, path := range manifestFrontendBridgeCandidates(execDir, filepath.Base(candidate)) {
			if path == candidate {
				return "executable-dir"
			}
		}
		for _, path := range manifestFrontendBridgeCandidates(filepath.Join(execDir, ".."), filepath.Base(candidate)) {
			if path == candidate {
				return "executable-parent"
			}
		}
		for _, path := range manifestFrontendBridgeCandidates(filepath.Join(execDir, "..", ".."), filepath.Base(candidate)) {
			if path == candidate {
				return "executable-grandparent"
			}
		}
		for _, path := range manifestFrontendSharedBridgeCandidates(execDir, filepath.Base(candidate)) {
			if path == candidate {
				return "shared-install"
			}
		}
	}
	if resolution.SourceRepoRoot != "" {
		for _, path := range manifestFrontendBridgeCandidates(resolution.SourceRepoRoot, filepath.Base(candidate)) {
			if path == candidate {
				return "source-repo"
			}
		}
	}
	if resolution.CWD != "" {
		for _, path := range manifestFrontendBridgeCandidates(resolution.CWD, filepath.Base(candidate)) {
			if path == candidate {
				return "cwd-in-source-repo"
			}
		}
	}
	return "filesystem"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
