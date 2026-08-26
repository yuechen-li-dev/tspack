package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/yuechen-li-dev/tspack/internal/project"
)

// lifecycleProjectPaths owns the path/configuration flags shared by project
// lifecycle commands. It is deliberately cheap: manifest evaluation remains an
// explicit application operation in internal/project.
type lifecycleProjectPaths struct {
	Options          project.Options
	manifestExplicit bool
	lockfileExplicit bool
	storeExplicit    bool
}

func newLifecycleProjectPaths() lifecycleProjectPaths {
	options := project.DefaultOptions(".")
	options.PerfWriter = os.Stderr
	return lifecycleProjectPaths{Options: options}
}

func (paths *lifecycleProjectPaths) consume(args []string, index *int) bool {
	flag := args[*index]
	switch flag {
	case "--root":
		value := lifecycleFlagValue(args, index, flag)
		paths.Options.RootDir = value
		if !paths.manifestExplicit {
			paths.Options.ManifestPath = filepath.Join(value, "manifest.tsx")
		}
		if !paths.lockfileExplicit {
			paths.Options.LockfilePath = filepath.Join(value, "ts-lock.toml")
		}
		if !paths.storeExplicit {
			paths.Options.StoreRoot = filepath.Join(value, ".tspack", "store")
		}
		return true
	case "--manifest":
		paths.Options.ManifestPath = lifecycleFlagValue(args, index, flag)
		paths.manifestExplicit = true
		return true
	case "--lockfile":
		paths.Options.LockfilePath = lifecycleFlagValue(args, index, flag)
		paths.lockfileExplicit = true
		return true
	case "--store":
		paths.Options.StoreRoot = lifecycleFlagValue(args, index, flag)
		paths.storeExplicit = true
		return true
	default:
		return false
	}
}

func lifecycleFlagValue(args []string, index *int, flag string) string {
	*index++
	if *index >= len(args) {
		fmt.Fprintf(os.Stderr, "%s requires a value\n", flag)
		os.Exit(1)
	}
	return args[*index]
}

func failUnknownLifecycleFlag(command string, flag string) {
	fmt.Fprintf(os.Stderr, "unknown %s flag: %s\n", command, flag)
	os.Exit(1)
}
