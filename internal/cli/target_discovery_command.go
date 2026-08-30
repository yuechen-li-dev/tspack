package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/yuechen-li-dev/tspack/internal/project"
)

func runTargetInspectCommand(args []string) {
	root := "."
	kind := ""
	jsonOutput := false
	for index := 2; index < len(args); index++ {
		switch args[index] {
		case "--root":
			if index+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "TSPACK_TARGET_DISCOVERY_OPTIONS_INVALID: --root requires a value")
				exit(1)
			}
			index++
			root = args[index]
		case "--kind":
			if index+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "TSPACK_TARGET_DISCOVERY_OPTIONS_INVALID: --kind requires build or test")
				exit(1)
			}
			index++
			kind = args[index]
		case "--json":
			jsonOutput = true
		default:
			fmt.Fprintf(os.Stderr, "TSPACK_TARGET_DISCOVERY_OPTIONS_INVALID: unknown option %s\n", args[index])
			exit(1)
		}
	}
	result := project.DiscoverTargets(project.TargetDiscoveryRequest{
		Project: project.DefaultOptions(root),
		Kind:    kind,
	})
	if jsonOutput {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(result); err != nil {
			fmt.Fprintf(os.Stderr, "TSPACK_TARGET_DISCOVERY_RENDER_FAILED: %v\n", err)
			exit(1)
		}
	} else {
		renderDiscoveredTargets(result)
	}
	if hasDiagnosticErrors(result.Diagnostics) {
		if !jsonOutput {
			for _, diagnostic := range result.Diagnostics {
				fmt.Fprintf(os.Stderr, "%s: %s\n", diagnostic.Code, diagnostic.Message)
			}
		}
		exit(1)
	}
}

func renderDiscoveredTargets(result project.TargetDiscoveryResult) {
	writer := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(writer, "KIND\tPACKAGE\tROOT\tTARGET\tTOOL\tARTIFACTS/SCOPE\tPREREQUISITES")
	for _, target := range result.Targets {
		packageIdentity := target.Package
		if target.PublicationName != "" && target.PublicationName != target.Package {
			packageIdentity += " (publishes " + target.PublicationName + ")"
		}
		summary := "-"
		if target.Kind == "build" {
			summary = fmt.Sprintf("%d", len(target.Artifacts))
		} else {
			summary = fmt.Sprintf("%d source(s)", len(target.Sources))
			if target.HarnessProject != "" {
				summary += "; project=" + target.HarnessProject
			}
			if len(target.Requirements) > 0 {
				summary += fmt.Sprintf("; %d requirement(s)", len(target.Requirements))
			}
			if len(target.Fixtures) > 0 {
				summary += fmt.Sprintf("; %d fixture(s)", len(target.Fixtures))
			}
			if len(target.BuiltFixtures) > 0 {
				summary += fmt.Sprintf("; %d built fixture(s)", len(target.BuiltFixtures))
			}
		}
		prerequisites := "-"
		if len(target.Prerequisites) > 0 {
			prerequisites = strings.Join(target.Prerequisites, ",")
		}
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", target.Kind, packageIdentity, target.Root, target.Name, target.Tool, summary, prerequisites)
		if target.Kind == "build" {
			for _, artifact := range target.Artifacts {
				fmt.Fprintf(writer, "\t\t\t\t\t%s [%s/%s]\t\n", artifact.Path, artifact.Kind, artifact.Role)
			}
		} else {
			for _, requirement := range target.Requirements {
				fmt.Fprintf(writer, "\t\t\t\t\trequires %s -> %s\t\n", requirement.Identity, requirement.Producer)
			}
			for _, fixture := range target.Fixtures {
				fmt.Fprintf(writer, "\t\t\t\t\tfixture %s -> %s [%s]\t\n", fixture.Identity, fixture.RealizedPath, fixture.Producer)
			}
			for _, fixture := range target.BuiltFixtures {
				artifactNames := fixture.Artifacts
				if len(artifactNames) == 0 && fixture.Artifact != "" {
					artifactNames = []string{fixture.Artifact}
				}
				fmt.Fprintf(writer, "\t\t\t\t\tbuilt fixture %s -> %s [%s artifacts %s]\t\n", fixture.Identity, fixture.RealizedPath, fixture.ProducerTarget, strings.Join(artifactNames, ","))
			}
		}
	}
	_ = writer.Flush()
}
