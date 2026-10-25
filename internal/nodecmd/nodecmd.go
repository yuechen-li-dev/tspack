package nodecmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

const DiagnosticCode = "TSPACK_NODE_NOT_FOUND"

// ToolBinDirs follows the package-instance materializer's ownership: a target's
// declaring package supplies tools before the workspace compatibility directory.
func ToolBinDirs(workspaceRoot string, packageRoots ...string) []string {
	result := []string{}
	seen := map[string]bool{}
	for _, root := range append(append([]string(nil), packageRoots...), workspaceRoot) {
		if root == "" {
			continue
		}
		directory := filepath.Join(root, "node_modules", ".bin")
		key := filepath.Clean(directory)
		if runtime.GOOS == "windows" {
			key = strings.ToLower(key)
		}
		if !seen[key] {
			result = append(result, directory)
			seen[key] = true
		}
	}
	return result
}

type NotFoundError struct {
	Detail string
}

var locatedPaths sync.Map

func (e NotFoundError) Error() string {
	if strings.TrimSpace(e.Detail) != "" {
		return e.Detail
	}
	return "Node.js was not found on PATH."
}

func Locate() (string, error) {
	workingDirectory, _ := os.Getwd()
	cacheKey := strings.Join([]string{
		runtime.GOOS,
		workingDirectory,
		os.Getenv("PATH"),
		os.Getenv("PATHEXT"),
	}, "\x00")
	if cached, ok := locatedPaths.Load(cacheKey); ok {
		return cached.(string), nil
	}
	candidates := []string{"node"}
	if runtime.GOOS == "windows" {
		candidates = []string{"node.exe", "node.cmd", "node"}
	}
	for _, candidate := range candidates {
		path, err := exec.LookPath(candidate)
		if err == nil {
			locatedPaths.Store(cacheKey, path)
			return path, nil
		}
	}
	return "", NotFoundError{Detail: "Node.js was not found on PATH."}
}

func Command(args ...string) (*exec.Cmd, error) {
	path, err := Locate()
	if err != nil {
		return nil, err
	}
	return exec.Command(path, args...), nil
}

func IsNotFound(err error) bool {
	var notFound NotFoundError
	return errors.As(err, &notFound)
}

func MessageBody() string {
	return strings.Join(GuidanceLines(), "\n")
}

func Message() string {
	return fmt.Sprintf("%s: %s", DiagnosticCode, MessageBody())
}

func GuidanceLines() []string {
	return []string{
		"Node.js was not found on PATH.",
		"",
		"TSPack does not manage JavaScript runtime versions. Install Node.js or activate it in your shell before running this command.",
		"Recommended: use mise to manage project runtimes: https://mise.jdx.dev/",
		"Example:",
		"  mise use node@lts",
		"  mise install",
	}
}
