package version

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	semver "github.com/Masterminds/semver/v3"
)

const RequirementFile = ".tspack-version"

type Requirement struct {
	Path    string
	Minimum string
	Current string
	TooOld  bool
}

func ReadRequirement(root string) (*Requirement, error) {
	path := filepath.Join(root, RequirementFile)
	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	minimumText := strings.TrimSpace(string(content))
	if minimumText == "" || strings.ContainsAny(minimumText, " \t\r\n") {
		return nil, fmt.Errorf("%s must contain exactly one semantic version such as v0.1.8", path)
	}
	minimum, err := semver.NewVersion(minimumText)
	if err != nil {
		return nil, fmt.Errorf("%s contains invalid semantic version %q: %w", path, minimumText, err)
	}
	current, err := semver.NewVersion(Version)
	if err != nil {
		return nil, fmt.Errorf("installed TSPack version %q is not semantic-version compatible", Version)
	}
	return &Requirement{Path: path, Minimum: "v" + minimum.String(), Current: "v" + current.String(), TooOld: current.LessThan(minimum)}, nil
}
