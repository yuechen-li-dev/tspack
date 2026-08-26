package cli

import (
	"os"
	"strings"

	"github.com/yuechen-li-dev/tspack/internal/project"
)

func runSyncCommand(args []string) {
	paths := newLifecycleProjectPaths()
	clean := false
	force := false
	jsonOutput := false
	for index := 1; index < len(args); index++ {
		if paths.consume(args, &index) {
			continue
		}
		switch args[index] {
		case "--clean":
			clean = true
		case "--force":
			force = true
		case "--json":
			jsonOutput = true
		default:
			if strings.HasPrefix(args[index], "-") {
				failUnknownLifecycleFlag("sync", args[index])
			}
			failUnknownLifecycleFlag("sync", args[index])
		}
	}
	if !jsonOutput {
		paths.Options.Progress = project.Progress{Enabled: true, Writer: os.Stderr}
	}
	result := project.RunSync(project.SyncRequest{Project: paths.Options, Clean: clean, Force: force})
	renderHumanDiagnostics(os.Stderr, result.Diagnostics, checkRenderOptions{})
	exitForDiagnostics(result.Diagnostics)
}
