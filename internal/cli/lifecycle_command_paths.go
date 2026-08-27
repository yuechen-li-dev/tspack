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
	rootExplicit     bool
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
		paths.rootExplicit = true
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

// discoverDependencyRoot gives dependency editing its package-directory UX
// without changing root semantics for unrelated lifecycle commands.
func (paths *lifecycleProjectPaths) discoverDependencyRoot() {
	if paths.rootExplicit || paths.manifestExplicit {
		return
	}
	workingDirectory, err := os.Getwd()
	if err != nil {
		return
	}
	for directory := workingDirectory; ; directory = filepath.Dir(directory) {
		manifestPath := filepath.Join(directory, "manifest.tsx")
		if _, err := os.Stat(manifestPath); err == nil {
			paths.Options.RootDir = directory
			paths.Options.ManifestPath = manifestPath
			if !paths.lockfileExplicit {
				paths.Options.LockfilePath = filepath.Join(directory, "ts-lock.toml")
			}
			if !paths.storeExplicit {
				paths.Options.StoreRoot = filepath.Join(directory, ".tspack", "store")
			}
			return
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return
		}
	}
}

func lifecycleFlagValue(args []string, index *int, flag string) string {
	*index++
	if *index >= len(args) {
		fmt.Fprintf(os.Stderr, "%s requires a value\n", flag)
		exit(1)
	}
	return args[*index]
}

func failUnknownLifecycleFlag(command string, flag string) {
	fmt.Fprintf(os.Stderr, "unknown %s flag: %s\n", command, flag)
	exit(1)
}
