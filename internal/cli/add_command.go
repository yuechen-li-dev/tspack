package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/yuechen-li-dev/tspack/internal/authoring"
	"github.com/yuechen-li-dev/tspack/internal/diag"
	"github.com/yuechen-li-dev/tspack/internal/project"
)

type addCommandOptions struct {
	Paths         lifecycleProjectPaths
	PackageSpec   string
	TargetPackage string
	Kind          authoring.DependencyKind
	Optional      bool
	DryRun        bool
	JSON          bool
}

type addJSONReport struct {
	Command             string               `json:"command"`
	OK                  bool                 `json:"ok"`
	DryRun              bool                 `json:"dryRun"`
	Package             string               `json:"package,omitempty"`
	Source              string               `json:"source,omitempty"`
	Kind                string               `json:"kind,omitempty"`
	Optional            bool                 `json:"optional"`
	RequestedConstraint string               `json:"requestedConstraint,omitempty"`
	SelectedVersion     string               `json:"selectedVersion,omitempty"`
	WrittenConstraint   string               `json:"writtenConstraint,omitempty"`
	TargetPackage       string               `json:"targetPackage,omitempty"`
	ManifestPath        string               `json:"manifestPath,omitempty"`
	ManifestChanged     bool                 `json:"manifestChanged"`
	LockChanged         bool                 `json:"lockChanged"`
	AlreadyPresent      bool                 `json:"alreadyPresent"`
	Shadowed            []addJSONDeclaration `json:"shadowed"`
	Diagnostics         []diag.Diagnostic    `json:"diagnostics"`
}

type addJSONDeclaration struct {
	Package    string `json:"package"`
	Source     string `json:"source"`
	Kind       string `json:"kind"`
	Constraint string `json:"constraint,omitempty"`
	Origin     string `json:"origin"`
}

func runAddCommand(args []string) {
	options := parseAddCommandOptions(args)
	result := project.RunAddDependency(project.AddDependencyRequest{
		Project:       options.Paths.Options,
		PackageSpec:   options.PackageSpec,
		Kind:          options.Kind,
		TargetPackage: options.TargetPackage,
		Optional:      options.Optional,
		DryRun:        options.DryRun,
	})
	if options.JSON {
		renderAddJSON(result)
	} else {
		renderAddHuman(result)
	}
	exitForDiagnostics(result.Diagnostics)
}

func parseAddCommandOptions(args []string) addCommandOptions {
	options := addCommandOptions{
		Paths: newLifecycleProjectPaths(),
		Kind:  authoring.DependencyRuntime,
	}
	for index := 1; index < len(args); index++ {
		if options.Paths.consume(args, &index) {
			continue
		}
		switch args[index] {
		case "--optional":
			options.Optional = true
		case "--dry-run":
			options.DryRun = true
		case "--json":
			options.JSON = true
		case "--package":
			options.TargetPackage = lifecycleFlagValue(args, &index, "--package")
		default:
			if strings.HasPrefix(args[index], "-") {
				failUnknownLifecycleFlag("add", args[index])
			}
			if options.PackageSpec != "" {
				fmt.Fprintln(os.Stderr, "add accepts exactly one package specification")
				exit(1)
			}
			options.PackageSpec = args[index]
		}
	}
	if options.PackageSpec == "" {
		fmt.Fprintln(os.Stderr, "usage: tspack add <package> [--optional] [--package <name>] [--dry-run] [--json]")
		exit(1)
	}
	return options
}

func renderAddHuman(result project.AddDependencyResult) {
	renderHumanDiagnostics(os.Stderr, result.Diagnostics, checkRenderOptions{})
	if hasErrors(result.Diagnostics) {
		return
	}
	if result.AlreadyPresent {
		fmt.Printf("%s is already declared as %s.\n", result.Package, result.WrittenConstraint)
		fmt.Println("No changes.")
		return
	}
	verb := "Added"
	if result.DryRun {
		verb = "Would add"
	}
	fmt.Printf("%s %s %s\n", verb, result.Package, result.WrittenConstraint)
	fmt.Println()
	fmt.Printf("  package: %s\n", result.Package)
	fmt.Printf("  source: %s\n", result.Source)
	fmt.Printf("  kind: %s\n", result.Kind)
	if result.Optional {
		fmt.Println("  optional: true")
	}
	if result.SelectedVersion != "" {
		fmt.Printf("  resolved: %s\n", result.SelectedVersion)
	}
	if len(result.ShadowedDeclarations) > 0 {
		fmt.Println()
		fmt.Println("Overrides:")
		for _, declaration := range result.ShadowedDeclarations {
			fmt.Printf("  %s -> %s %s\n", addOriginLabel(declaration), declaration.Identity.Name, declaration.Constraint)
		}
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

func renderAddJSON(result project.AddDependencyResult) {
	report := addJSONReport{
		Command:             "add",
		OK:                  !hasErrors(result.Diagnostics),
		DryRun:              result.DryRun,
		Package:             result.Package,
		Source:              result.Source,
		Kind:                string(result.Kind),
		Optional:            result.Optional,
		RequestedConstraint: result.RequestedConstraint,
		SelectedVersion:     result.SelectedVersion,
		WrittenConstraint:   result.WrittenConstraint,
		TargetPackage:       result.TargetPackage,
		ManifestPath:        result.ManifestPath,
		ManifestChanged:     result.ManifestChanged,
		LockChanged:         result.LockChanged,
		AlreadyPresent:      result.AlreadyPresent,
		Shadowed:            []addJSONDeclaration{},
		Diagnostics:         append([]diag.Diagnostic{}, result.Diagnostics...),
	}
	for _, declaration := range result.ShadowedDeclarations {
		report.Shadowed = append(report.Shadowed, addJSONDeclaration{
			Package:    declaration.Identity.Name,
			Source:     declaration.Identity.Source,
			Kind:       string(declaration.Kind),
			Constraint: declaration.Constraint,
			Origin:     addOriginLabel(declaration),
		})
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		fmt.Fprintf(os.Stderr, "TSPACK_ADD_JSON_ENCODE_FAILED: %v\n", err)
		exit(1)
	}
}

func addOriginLabel(declaration authoring.DependencyDeclaration) string {
	if declaration.Origin.Name != "" {
		return declaration.Origin.Name
	}
	return string(declaration.Origin.Kind)
}
