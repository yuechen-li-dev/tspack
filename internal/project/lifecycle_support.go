package project

import (
	"context"
	"fmt"

	"github.com/yuechen-li-dev/tspack/internal/bridge"
	capmodel "github.com/yuechen-li-dev/tspack/internal/capability"
	"github.com/yuechen-li-dev/tspack/internal/diag"
	"github.com/yuechen-li-dev/tspack/internal/graph"
	"github.com/yuechen-li-dev/tspack/internal/lockfile"
	"github.com/yuechen-li-dev/tspack/internal/manifest"
	"github.com/yuechen-li-dev/tspack/internal/manifestfrontend"
	"github.com/yuechen-li-dev/tspack/internal/nodecmd"
	"github.com/yuechen-li-dev/tspack/internal/resolver"
	"github.com/yuechen-li-dev/tspack/internal/version"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func loadManifestAndGraph(opts Options) (*manifest.ManifestIR, *graph.WorkspaceGraph, []diag.Diagnostic) {
	ir, d := loadManifestIR(opts)
	if len(d) > 0 {
		return nil, nil, d
	}
	g, gd := graph.Build(ir)
	return ir, g, append([]diag.Diagnostic{}, gd...)
}

func manifestFrontendCLIPath() string {
	resolution := bridge.Resolve("cli.js")
	if resolution.Path != "" {
		return resolution.Path
	}
	if len(resolution.SearchedPaths) > 0 {
		return resolution.SearchedPaths[0]
	}
	return filepath.Join("manifest-frontend", "dist", "cli.js")
}

func loadManifestIR(opts Options) (*manifest.ManifestIR, []diag.Diagnostic) {
	requirement, requirementErr := version.ReadRequirement(opts.RootDir)
	if requirementErr != nil {
		return nil, []diag.Diagnostic{errDiag("TSPACK_VERSION_REQUIREMENT_INVALID", "invalid TSPack minimum-version contract", requirementErr.Error())}
	}
	if requirement != nil && requirement.TooOld {
		return nil, []diag.Diagnostic{{
			Code:     "TSPACK_VERSION_TOO_OLD",
			Severity: diag.SeverityError,
			File:     requirement.Path,
			Message:  "this project requires a newer TSPack release",
			Details:  []string{"installed: " + requirement.Current, "minimum: " + requirement.Minimum},
			Fixes:    []string{"Install TSPack " + requirement.Minimum + " or newer.", "Update the setup-tspack version in CI before retrying."},
		}}
	}
	if opts.ManifestIRPath != "" {
		b, err := os.ReadFile(opts.ManifestIRPath)
		if err != nil {
			return nil, []diag.Diagnostic{errDiag("TSPACK_PROJECT_MANIFEST_FRONTEND_FAILED", "failed to read manifest IR", err.Error())}
		}
		return manifest.LoadBytes(opts.ManifestIRPath, b)
	}
	resolution := bridge.ResolveWithOptions("cli.js", bridge.ResolveOptions{
		ProjectRoot: opts.RootDir,
	})
	cliPath := opts.FrontendCLIPath
	if cliPath == "" {
		cliPath = resolution.Path
	}
	if cliPath == "" {
		details := append([]string{
			"manifest frontend CLI not found",
		}, bridge.ResolutionDetails(resolution)...)
		if len(resolution.SearchedPaths) > 0 {
			details = append(details, "frontend path candidates tried:")
			for _, candidate := range resolution.SearchedPaths {
				details = append(details, "  "+candidate)
			}
		}
		details = append(details, bridge.BuildNeededDetails()...)
		return nil, []diag.Diagnostic{errDiag("TSPACK_PROJECT_MANIFEST_FRONTEND_FAILED", "manifest frontend CLI not found", details...)}
	}
	if _, err := os.Stat(cliPath); err != nil {
		details := append([]string{
			"selected frontend path: " + cliPath,
			"selected frontend path error: " + err.Error(),
		}, bridge.ResolutionDetails(resolution)...)
		details = append(details, bridge.BuildNeededDetails()...)
		return nil, []diag.Diagnostic{errDiag("TSPACK_PROJECT_MANIFEST_FRONTEND_FAILED", "manifest frontend CLI not found", details...)}
	}
	parsed, err := manifestfrontend.Execute(cliPath, opts.ManifestPath)
	if err != nil {
		if nodecmd.IsNotFound(err) {
			return nil, []diag.Diagnostic{{
				Code:     nodecmd.DiagnosticCode,
				Severity: diag.SeverityError,
				Message:  "Node.js was not found on PATH.",
				Details:  nodecmd.GuidanceLines(),
			}}
		}
		details := append([]string{
			"selected frontend path: " + cliPath,
			"node execution error: " + err.Error(),
			"node stderr:",
			err.Error(),
		}, bridge.ResolutionDetails(resolution)...)
		return nil, []diag.Diagnostic{{Code: "TSPACK_PROJECT_MANIFEST_FRONTEND_FAILED", Severity: diag.SeverityError, Message: "manifest frontend failed", Details: details}}
	}
	if !parsed.OK {
		if len(parsed.Diagnostics) == 0 {
			return nil, []diag.Diagnostic{errDiag("TSPACK_PROJECT_MANIFEST_FRONTEND_FAILED", "manifest frontend returned failure")}
		}
		return nil, parsed.Diagnostics
	}
	ir, d := manifest.LoadBytes(opts.ManifestPath, parsed.IR)
	return ir, d
}

func errDiag(code, msg string, details ...string) diag.Diagnostic {
	return diag.Diagnostic{Code: code, Severity: diag.SeverityError, Message: msg, Details: details}
}

func FormatResult(r Result) string { return fmt.Sprintf("diagnostics=%d", len(r.Diagnostics)) }

func findTarballURL(pkg *lockfile.Package, client resolver.NPMRegistryClient) string {
	meta, err := client.PackageMetadata(context.Background(), pkg.Name)
	if err != nil {
		return ""
	}
	pv, ok := meta.Versions[pkg.Version]
	if !ok {
		return ""
	}
	return pv.Dist.Tarball
}

func hasErrors(diags []diag.Diagnostic) bool {
	for _, d := range diags {
		if d.Severity == diag.SeverityError {
			return true
		}
	}
	return false
}

func lifecycleCapabilityDiagnostics(lf *lockfile.Lockfile, acknowledgements map[string]manifest.AcknowledgedCapability, categoryAcknowledgements []manifest.AcknowledgedLifecycleCategory) []diag.Diagnostic {
	if lf == nil {
		return nil
	}
	diagnostics := []diag.Diagnostic{}
	usedAcknowledgements := map[string]bool{}
	usedCategoryAcknowledgements := map[int]int{}
	staleAcknowledgements := map[string]staleLifecycleAcknowledgement{}
	pathsByPackage := lifecyclePulledByPaths(lf)
	for _, pkg := range lf.Packages {
		for _, capability := range pkg.Capabilities {
			if !isLifecycleCapability(capability) {
				continue
			}
			ackKey := lifecycleAcknowledgementKey(pkg.ID, capability.Script, capability.Command)
			if _, ok := acknowledgements[ackKey]; ok {
				usedAcknowledgements[ackKey] = true
				continue
			}
			classification := capmodel.ClassifyLifecycleScript(capability.Script)
			categoryAcknowledgement, categoryAcknowledgementIndex, categoryAcknowledged := matchingLifecycleCategoryAcknowledgement(classification.LifecycleCategory, capability.Script, categoryAcknowledgements)
			staleKey := lifecycleStaleAcknowledgementKey(pkg.ID, capability.Script)
			for _, acknowledgement := range acknowledgements {
				if acknowledgement.Package == pkg.ID && acknowledgement.Script == capability.Script && acknowledgement.Command != capability.Command {
					acknowledgementKey := lifecycleAcknowledgementKey(acknowledgement.Package, acknowledgement.Script, acknowledgement.Command)
					usedAcknowledgements[acknowledgementKey] = true
					staleAcknowledgements[staleKey] = staleLifecycleAcknowledgement{Acknowledgement: acknowledgement, ActualCommand: capability.Command}
				}
			}
			details := []string{
				"package: " + pkg.ID,
				"lifecycleScriptName: " + capability.Script,
				"script: " + capability.Script,
				"command: " + capability.Command,
				"lifecycleCategory: " + classification.LifecycleCategory,
				"consumerInstallTime: " + fmt.Sprintf("%t", classification.ConsumerInstallTime),
				"execution: blocked by default",
			}
			if categoryAcknowledged {
				usedCategoryAcknowledgements[categoryAcknowledgementIndex]++
				details = append(details,
					"acknowledged: true",
					"acknowledgmentKind: lifecycle-category",
					"acknowledgedByCategory: "+categoryAcknowledgement.Category,
					"reason: "+categoryAcknowledgement.Reason,
				)
			} else {
				details = append(details,
					"acknowledged: false",
					"acknowledgmentKind: null",
				)
			}
			paths := pathsByPackage[pkg.ID]
			if len(paths) > 0 {
				details = append(details, "pulled by:")
				for _, path := range paths {
					details = append(details, "  "+strings.Join(path, " -> "))
				}
			}
			diagnostics = append(diagnostics, diag.Diagnostic{
				Code:     "TSPACK_SECURITY_LIFECYCLE_SCRIPT_PRESENT",
				Severity: diag.SeverityWarning,
				Message:  "package declares install-time lifecycle script",
				Details:  details,
			})
		}
	}
	for _, stale := range staleAcknowledgements {
		diagnostics = append(diagnostics, diag.Diagnostic{
			Code:     "TSPACK_SECURITY_ACKNOWLEDGED_CAPABILITY_STALE",
			Severity: diag.SeverityWarning,
			Message:  "acknowledged lifecycle capability command no longer matches lockfile",
			Details: []string{
				"package: " + stale.Acknowledgement.Package,
				"script: " + stale.Acknowledgement.Script,
				"acknowledged command: " + stale.Acknowledgement.Command,
				"actual command: " + stale.ActualCommand,
			},
		})
	}
	for index, acknowledgement := range categoryAcknowledgements {
		for _, script := range acknowledgement.Scripts {
			if categoryAcknowledgementScriptStale(acknowledgement, script) {
				diagnostics = append(diagnostics, diag.Diagnostic{
					Code:     "TSPACK_SECURITY_ACKNOWLEDGED_LIFECYCLE_CATEGORY_STALE",
					Severity: diag.SeverityWarning,
					Message:  "acknowledged lifecycle category includes script outside that category",
					Details: []string{
						"category: " + acknowledgement.Category,
						"script: " + script,
						"actual category: " + capmodel.ClassifyLifecycleScript(script).LifecycleCategory,
						"reason: " + acknowledgement.Reason,
					},
				})
			}
		}
		if usedCategoryAcknowledgements[index] == 0 {
			diagnostics = append(diagnostics, diag.Diagnostic{
				Code:     "TSPACK_SECURITY_ACKNOWLEDGED_LIFECYCLE_CATEGORY_UNUSED",
				Severity: diag.SeverityWarning,
				Message:  "acknowledged lifecycle category did not match any lockfile capabilities",
				Details: []string{
					"category: " + acknowledgement.Category,
					"scripts: " + strings.Join(acknowledgement.Scripts, ","),
					"reason: " + acknowledgement.Reason,
				},
			})
		}
	}
	for key, acknowledgement := range acknowledgements {
		if usedAcknowledgements[key] {
			continue
		}
		diagnostics = append(diagnostics, diag.Diagnostic{
			Code:     "TSPACK_SECURITY_ACKNOWLEDGED_CAPABILITY_UNUSED",
			Severity: diag.SeverityWarning,
			Message:  "acknowledged lifecycle capability is not present in the lockfile",
			Details: []string{
				"package: " + acknowledgement.Package,
				"script: " + acknowledgement.Script,
				"command: " + acknowledgement.Command,
				"reason: " + acknowledgement.Reason,
			},
		})
	}
	diag.SortDiagnostics(diagnostics)
	return diagnostics
}

type staleLifecycleAcknowledgement struct {
	Acknowledgement manifest.AcknowledgedCapability
	ActualCommand   string
}

func lifecycleCategoryAcknowledgements(ir *manifest.ManifestIR) []manifest.AcknowledgedLifecycleCategory {
	if ir == nil {
		return nil
	}
	return append([]manifest.AcknowledgedLifecycleCategory(nil), ir.Security.AcknowledgedLifecycleCategories...)
}

func matchingLifecycleCategoryAcknowledgement(category string, script string, acknowledgements []manifest.AcknowledgedLifecycleCategory) (manifest.AcknowledgedLifecycleCategory, int, bool) {
	for index, acknowledgement := range acknowledgements {
		if acknowledgement.Category != category {
			continue
		}
		if len(acknowledgement.Scripts) == 0 {
			return acknowledgement, index, true
		}
		for _, acknowledgedScript := range acknowledgement.Scripts {
			if acknowledgedScript == script {
				return acknowledgement, index, true
			}
		}
	}
	return manifest.AcknowledgedLifecycleCategory{}, -1, false
}

func categoryAcknowledgementScriptStale(acknowledgement manifest.AcknowledgedLifecycleCategory, script string) bool {
	classification := capmodel.ClassifyLifecycleScript(script)
	return classification.LifecycleCategory != acknowledgement.Category
}
func lifecycleAcknowledgementSet(ir *manifest.ManifestIR) map[string]manifest.AcknowledgedCapability {
	acknowledgements := map[string]manifest.AcknowledgedCapability{}
	if ir == nil {
		return acknowledgements
	}
	for _, acknowledgement := range ir.Security.AcknowledgedCapabilities {
		if acknowledgement.Kind != capmodel.LifecycleScriptKind {
			continue
		}
		key := lifecycleAcknowledgementKey(acknowledgement.Package, acknowledgement.Script, acknowledgement.Command)
		acknowledgements[key] = acknowledgement
	}
	return acknowledgements
}

func lifecycleAcknowledgementKey(packageID string, script string, command string) string {
	return packageID + "|" + capmodel.LifecycleScriptKind + "|" + script + "|" + command
}

func lifecycleStaleAcknowledgementKey(packageID string, script string) string {
	return packageID + "|" + capmodel.LifecycleScriptKind + "|" + script
}

func isLifecycleCapability(capability lockfile.Capability) bool {
	return capability.Kind == "lifecycleScript" || capability.Kind == "lifecycle-script"
}

func lifecyclePulledByPaths(lf *lockfile.Lockfile) map[string][][]string {
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

	pathsByPackage := map[string][][]string{}
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
				pathsByPackage[edge.To] = append(pathsByPackage[edge.To], nextPath)
				queue = append(queue, nextPath)
			}
		}
	}
	for packageID := range pathsByPackage {
		paths := pathsByPackage[packageID]
		sort.SliceStable(paths, func(i, j int) bool {
			return strings.Join(paths[i], " -> ") < strings.Join(paths[j], " -> ")
		})
	}
	return pathsByPackage
}
