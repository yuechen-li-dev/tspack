package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yuechen-li-dev/tspack/internal/manifest"
	"github.com/yuechen-li-dev/tspack/internal/manifestfrontend"
	"github.com/yuechen-li-dev/tspack/internal/nodecmd"
	"github.com/yuechen-li-dev/tspack/internal/version"
)

// Workspace is the cheap, resolved application context shared by commands.
// Loading the manifest remains an explicit operation because it invokes the
// TypeScript manifest frontend.
type Workspace struct {
	Root string
}

func openWorkspace(root string) Workspace {
	return Workspace{Root: resolveWorkspaceRoot(root)}
}

func (workspace Workspace) LoadManifest(manifestPath string) *manifest.ManifestIR {
	return loadManifestPathForRun(workspace.Root, manifestPath)
}
func loadManifestForRun(root string) *manifest.ManifestIR {
	return loadManifestPathForRun(root, filepath.Join(root, "manifest.tsx"))
}

func loadManifestPathForRun(root string, manifestPath string) *manifest.ManifestIR {
	requirement, requirementErr := version.ReadRequirement(root)
	if requirementErr != nil {
		failRun("TSPACK_VERSION_REQUIREMENT_INVALID", requirementErr.Error())
	}
	if requirement != nil && requirement.TooOld {
		failRun("TSPACK_VERSION_TOO_OLD", fmt.Sprintf("installed %s; project requires %s or newer", requirement.Current, requirement.Minimum))
	}
	cliPath := manifestFrontendCLIPath()
	parsed, err := manifestfrontend.Execute(cliPath, manifestPath)
	if err != nil {
		if nodecmd.IsNotFound(err) {
			failRun(nodecmd.DiagnosticCode, nodecmd.MessageBody())
		}
		failRun("TSPACK_PROJECT_MANIFEST_FRONTEND_FAILED", err.Error())
	}
	if !parsed.OK {
		if len(parsed.Diagnostics) > 0 {
			failRun(parsed.Diagnostics[0].Code, parsed.Diagnostics[0].Message)
		}
		failRun("TSPACK_PROJECT_MANIFEST_FRONTEND_FAILED", "manifest frontend returned failure without diagnostics")
	}
	ir, diags := manifest.LoadBytes(manifestPath, parsed.IR)
	if len(diags) > 0 {
		failRun(diags[0].Code, diags[0].Message)
	}
	if !placeholderManifestForFrontendStub(manifestPath) {
		ir.Workspace.RuntimeSpecified = workspaceRuntimeDeclaredInManifest(manifestPath)
	}
	return ir
}

func placeholderManifestForFrontendStub(manifestPath string) bool {
	contents, err := os.ReadFile(manifestPath)
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(contents)) == "export default {}"
}

func workspaceRuntimeDeclaredInManifest(manifestPath string) bool {
	contents, err := os.ReadFile(manifestPath)
	if err != nil {
		return false
	}
	text := string(contents)
	return strings.Contains(text, "<Workspace") && strings.Contains(text, "runtime=")
}

func resolveWorkspaceRoot(root string) string {
	abs, err := filepath.Abs(root)
	if err != nil {
		return root
	}
	return abs
}
