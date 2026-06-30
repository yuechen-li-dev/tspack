package nodecmd

import (
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

const DiagnosticCode = "TSPACK_NODE_NOT_FOUND"

type NotFoundError struct {
	Detail string
}

func (e NotFoundError) Error() string {
	if strings.TrimSpace(e.Detail) != "" {
		return e.Detail
	}
	return "Node.js was not found on PATH."
}

func Locate() (string, error) {
	candidates := []string{"node"}
	if runtime.GOOS == "windows" {
		candidates = []string{"node.exe", "node.cmd", "node"}
	}
	for _, candidate := range candidates {
		path, err := exec.LookPath(candidate)
		if err == nil {
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
