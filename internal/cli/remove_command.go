package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/yuechen-li-dev/tspack/internal/authoring"
	"github.com/yuechen-li-dev/tspack/internal/diag"
	"github.com/yuechen-li-dev/tspack/internal/project"
)

type removeCommandOptions struct {
	Paths         lifecycleProjectPaths
	PackageSpec   string
	TargetPackage string
	Optional      *bool
	Kind          authoring.DependencyKind
	Source        string
	DryRun        bool
	JSON          bool
}

type removeJSONReport struct {
	Command                   string                        `json:"command"`
	OK                        bool                          `json:"ok"`
	DryRun                    bool                          `json:"dryRun"`
	Package                   string                        `json:"package,omitempty"`
	Source                    string                        `json:"source,omitempty"`
	Kind                      string                        `json:"kind,omitempty"`
	Optional                  bool                          `json:"optional"`
	RemovedConstraint         string                        `json:"removedConstraint,omitempty"`
	Constraint                string                        `json:"constraint,omitempty"`
	TargetPackage             string                        `json:"targetPackage,omitempty"`
	ManifestPath              string                        `json:"manifestPath,omitempty"`
	DeclarationRemoved        bool                          `json:"declarationRemoved"`
	ManifestChanged           bool                          `json:"manifestChanged"`
	LockChanged               bool                          `json:"lockChanged"`
	StillDeclared             bool                          `json:"stillDeclared"`
	StillRequired             bool                          `json:"stillRequired"`
	StillResolved             bool                          `json:"stillResolved"`
	ResolvedStatusKnown       bool                          `json:"resolvedStatusKnown"`
	LockPackageRemoved        bool                          `json:"lockPackageRemoved"`
	NoEditableDeclaration     bool                          `json:"noEditableDeclaration"`
	NewlyEffectiveDeclaration *removeJSONDeclaration        `json:"newlyEffective,omitempty"`
	Provenance                []removeJSONDeclaration       `json:"provenance"`
	Performance               dependencyEditJSONPerformance `json:"performance"`
	Diagnostics               []diag.Diagnostic             `json:"diagnostics"`
}

type dependencyEditJSONPerformance struct {
	ManifestLoadMs           float64        `json:"manifestLoadMs"`
	MetadataSelectionMs      float64        `json:"metadataSelectionMs"`
	SemanticEditMs           float64        `json:"semanticEditMs"`
	ProjectionMs             float64        `json:"projectionMs"`
	UpdateMs                 float64        `json:"updateMs"`
	TotalMs                  float64        `json:"totalMs"`
	RegistryMetadataRequests int            `json:"registryMetadataRequests"`
	RegistryTarballRequests  int            `json:"registryTarballRequests"`
	RegistryRequests         map[string]int `json:"registryRequests,omitempty"`
}

type removeJSONDeclaration struct {
	Package      string `json:"package"`
	Source       string `json:"source"`
	Kind         string `json:"kind"`
	Optional     bool   `json:"optional"`
	Constraint   string `json:"constraint,omitempty"`
	OriginKind   string `json:"originKind"`
	OriginName   string `json:"originName,omitempty"`
	ManifestPath string `json:"manifestPath,omitempty"`
}

func runRemoveCommand(args []string) {
	options := parseRemoveCommandOptions(args)
	options.Paths.discoverDependencyRoot()
	workingDirectory, _ := os.Getwd()
	result := project.RunRemoveDependency(project.RemoveDependencyRequest{
		Project:          options.Paths.Options,
		PackageSpec:      options.PackageSpec,
		TargetPackage:    options.TargetPackage,
		WorkingDirectory: workingDirectory,
		Kind:             options.Kind,
		Source:           options.Source,
		Optional:         options.Optional,
		DryRun:           options.DryRun,
	})
	if options.JSON {
		renderRemoveJSON(result)
	} else {
		renderRemoveHuman(result)
	}
	exitForDiagnostics(result.Diagnostics)
}

func parseRemoveCommandOptions(args []string) removeCommandOptions {
	options := removeCommandOptions{Paths: newLifecycleProjectPaths()}
	for index := 1; index < len(args); index++ {
		if options.Paths.consume(args, &index) {
			continue
		}
		switch args[index] {
		case "--optional":
			optional := true
			options.Optional = &optional
		case "--dev":
			options.Kind = authoring.DependencyTest
		case "--tool":
			options.Kind = authoring.DependencyTool
		case "--kind":
			options.Kind = authoring.DependencyKind(lifecycleFlagValue(args, &index, "--kind"))
		case "--source":
			options.Source = lifecycleFlagValue(args, &index, "--source")
		case "--dry-run":
			options.DryRun = true
		case "--json":
			options.JSON = true
		case "--package":
			options.TargetPackage = lifecycleFlagValue(args, &index, "--package")
		default:
			if strings.HasPrefix(args[index], "-") {
				failUnknownLifecycleFlag("remove", args[index])
			}
			if options.PackageSpec != "" {
				fmt.Fprintln(os.Stderr, "remove accepts exactly one package selector")
				exit(1)
			}
			options.PackageSpec = args[index]
		}
	}
	if options.PackageSpec == "" {
		fmt.Fprintln(os.Stderr, "usage: tspack remove <package> [--source npm|jsr] [--kind dep|peer|tool|test] [--optional] [--package <name|path>] [--dry-run] [--json]")
		exit(1)
	}
	return options
}

func renderRemoveHuman(result project.RemoveDependencyResult) {
	renderHumanDiagnostics(os.Stderr, result.Diagnostics, checkRenderOptions{})
	if hasErrors(result.Diagnostics) {
		return
	}
	if result.NoEditableDeclaration {
		fmt.Printf("%s has no editable explicit declaration in %s.\n", result.Package, result.TargetPackage)
		if len(result.RemainingDeclarations) > 0 {
			fmt.Println()
			fmt.Println("Current source:")
			for _, declaration := range result.RemainingDeclarations {
				fmt.Printf("  %s -> %s %s\n", removeOriginLabel(declaration), declaration.Identity.Name, declaration.Constraint)
			}
		}
		fmt.Println()
		fmt.Println("No changes.")
		return
	}

	verb := "Removed"
	if result.DryRun {
		verb = "Would remove"
	}
	fmt.Printf("%s explicit %s %s.\n", verb, dependencyEditIdentity(result.Source, result.Package), result.RemovedConstraint)
	if result.StillRequired {
		fmt.Println()
		fmt.Printf("%s remains required:\n", result.Package)
		for _, declaration := range result.RemainingDeclarations {
			fmt.Printf("  %s -> %s %s\n", removeOriginLabel(declaration), declaration.Identity.Name, declaration.Constraint)
		}
	} else {
		fmt.Println()
		fmt.Printf("%s is no longer a direct dependency of %s.\n", result.Package, result.TargetPackage)
	}
	if result.ResolvedStatusKnown && result.StillResolved && !result.StillRequired {
		fmt.Printf("%s remains in the resolved graph through another dependency or package.\n", result.Package)
	}

	fmt.Println()
	if result.DryRun {
		fmt.Println("Would update:")
	} else {
		fmt.Println("Updated:")
	}
	if result.ManifestChanged {
		fmt.Printf("  %s\n", result.ManifestPath)
	}
	if result.LockChanged {
		fmt.Println("  ts-lock.toml")
	}
	if result.DryRun {
		fmt.Println()
		fmt.Println("No files were written.")
	}
}

func renderRemoveJSON(result project.RemoveDependencyResult) {
	report := removeJSONReport{
		Command:                   "remove",
		OK:                        !hasErrors(result.Diagnostics),
		DryRun:                    result.DryRun,
		Package:                   result.Package,
		Source:                    result.Source,
		Kind:                      string(result.Kind),
		Optional:                  result.Optional,
		RemovedConstraint:         result.RemovedConstraint,
		Constraint:                result.RemovedConstraint,
		TargetPackage:             result.TargetPackage,
		ManifestPath:              result.ManifestPath,
		DeclarationRemoved:        result.DeclarationRemoved,
		ManifestChanged:           result.ManifestChanged,
		LockChanged:               result.LockChanged,
		StillDeclared:             result.StillDeclared,
		StillRequired:             result.StillRequired,
		StillResolved:             result.StillResolved,
		ResolvedStatusKnown:       result.ResolvedStatusKnown,
		LockPackageRemoved:        result.LockPackageRemoved,
		NoEditableDeclaration:     result.NoEditableDeclaration,
		NewlyEffectiveDeclaration: nil,
		Provenance:                []removeJSONDeclaration{},
		Performance: dependencyEditJSONPerformance{
			ManifestLoadMs:           durationMilliseconds(result.Performance.ManifestLoad),
			SemanticEditMs:           durationMilliseconds(result.Performance.SemanticRemoval),
			ProjectionMs:             durationMilliseconds(result.Performance.Projection),
			UpdateMs:                 durationMilliseconds(result.Performance.Update),
			TotalMs:                  durationMilliseconds(result.Performance.Total),
			RegistryMetadataRequests: result.Performance.RegistryMetadataRequests,
			RegistryTarballRequests:  result.Performance.RegistryTarballRequests,
			RegistryRequests:         result.Performance.RegistryRequests,
		},
		Diagnostics: append([]diag.Diagnostic{}, result.Diagnostics...),
	}
	if result.NewlyEffectiveDeclaration != nil {
		declaration := removeJSONDeclarationValue(*result.NewlyEffectiveDeclaration)
		report.NewlyEffectiveDeclaration = &declaration
	}
	for _, declaration := range result.RemainingDeclarations {
		report.Provenance = append(report.Provenance, removeJSONDeclarationValue(declaration))
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		fmt.Fprintf(os.Stderr, "TSPACK_REMOVE_JSON_ENCODE_FAILED: %v\n", err)
		exit(1)
	}
}

func dependencyEditIdentity(source string, packageName string) string {
	if source == "" {
		return packageName
	}
	return source + ":" + packageName
}

func dependencyEditPerformanceJSON(performance project.AddDependencyPerformance) dependencyEditJSONPerformance {
	return dependencyEditJSONPerformance{
		ManifestLoadMs:           durationMilliseconds(performance.ManifestLoad),
		MetadataSelectionMs:      durationMilliseconds(performance.MetadataSelection),
		SemanticEditMs:           durationMilliseconds(performance.SemanticEdit),
		ProjectionMs:             durationMilliseconds(performance.Projection),
		UpdateMs:                 durationMilliseconds(performance.Update),
		TotalMs:                  durationMilliseconds(performance.Total),
		RegistryMetadataRequests: performance.RegistryMetadataRequests,
		RegistryTarballRequests:  performance.RegistryTarballRequests,
		RegistryRequests:         performance.RegistryRequests,
	}
}

func durationMilliseconds(duration time.Duration) float64 {
	return float64(duration) / float64(time.Millisecond)
}

func removeJSONDeclarationValue(declaration authoring.DependencyDeclaration) removeJSONDeclaration {
	return removeJSONDeclaration{
		Package:      declaration.Identity.Name,
		Source:       declaration.Identity.Source,
		Kind:         string(declaration.Kind),
		Optional:     declaration.Optional,
		Constraint:   declaration.Constraint,
		OriginKind:   string(declaration.Origin.Kind),
		OriginName:   declaration.Origin.Name,
		ManifestPath: declaration.Origin.SourcePath,
	}
}

func removeOriginLabel(declaration authoring.DependencyDeclaration) string {
	if declaration.Origin.Name != "" {
		return declaration.Origin.Name
	}
	return string(declaration.Origin.Kind)
}
