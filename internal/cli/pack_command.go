package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/yuechen-li-dev/tspack/internal/project"
)

func runPackCommand(args []string) {
	paths := newLifecycleProjectPaths()
	options := project.PackOptions{}
	for index := 1; index < len(args); index++ {
		if paths.consume(args, &index) {
			continue
		}
		switch args[index] {
		case "--out":
			options.OutputDir = lifecycleFlagValue(args, &index, "--out")
		case "--package":
			options.PackageName = lifecycleFlagValue(args, &index, "--package")
		case "--dry-run":
			options.DryRun = true
		case "--verify":
			options.Verify = true
		default:
			if strings.HasPrefix(args[index], "-") {
				failUnknownLifecycleFlag("pack", args[index])
			}
			failUnknownLifecycleFlag("pack", args[index])
		}
	}
	operation := project.RunPack(project.PackRequest{Project: paths.Options, Options: options})
	renderHumanDiagnostics(os.Stderr, operation.Diagnostics, checkRenderOptions{})
	if operation.Pack != nil {
		for _, artifact := range operation.Pack.Artifacts {
			fmt.Printf("packed %s@%s -> %s (%s)\n", artifact.PackageName, artifact.Version, artifact.Path, artifact.Hash)
			if artifact.Verified {
				fmt.Printf("Verified package artifact: %s\n", artifact.Path)
			}
		}
		if options.DryRun {
			for _, file := range operation.Pack.Preview {
				fmt.Printf("%s %s <- %s\n", file.PackageName, file.ArchivePath, file.SourcePath)
			}
		}
	}
	if hasErrors(operation.Diagnostics) {
		fmt.Fprintln(os.Stderr, "pack failed; no artifacts were written")
		exit(1)
	}
}
