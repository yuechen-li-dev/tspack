// Package packageidentity defines the boundary between TSPack package truth
// and runtime-specific compatibility spelling.
package packageidentity

import (
	"fmt"
	"strings"
)

const (
	SourceNPM       = "npm"
	SourceJSR       = "jsr"
	SourceNPMCompat = "npm-compat"
	RuntimeNode     = "node"
)

// PackageIdentity is TSPack's semantic package identity. It is the identity
// used by authoring, dependency edges, lock packages, and provenance reports.
type PackageIdentity struct {
	Source string `json:"source"`
	Name   string `json:"name"`
}

func (identity PackageIdentity) Key() string {
	return identity.Source + ":" + identity.Name
}

// MaterializationIdentity describes the package-tree name selected at a
// backend/runtime boundary. It must never be used as authoring truth.
type MaterializationIdentity struct {
	Source string `json:"source"`
	Name   string `json:"name"`
}

// ImportIdentity describes the spelling consumed by a specific runtime or
// compiler-compatible resolver.
type ImportIdentity struct {
	Runtime   string `json:"runtime"`
	Specifier string `json:"specifier"`
}

// PackageUsage is a presentation-neutral explanation of how one semantic
// package is materialized and imported by a runtime.
type PackageUsage struct {
	Semantic       PackageIdentity         `json:"semantic"`
	MaterializedAs MaterializationIdentity `json:"materializedAs"`
	Import         ImportIdentity          `json:"import"`
	Notes          []string                `json:"notes,omitempty"`
}

// NodeUsage returns stable Node/TypeScript package-tree and import spelling.
func NodeUsage(identity PackageIdentity) (PackageUsage, error) {
	usage := PackageUsage{
		Semantic: identity,
		MaterializedAs: MaterializationIdentity{
			Source: identity.Source,
			Name:   identity.Name,
		},
		Import: ImportIdentity{
			Runtime:   RuntimeNode,
			Specifier: identity.Name,
		},
	}

	switch identity.Source {
	case SourceNPM:
		if strings.TrimSpace(identity.Name) == "" {
			return PackageUsage{}, fmt.Errorf("npm package name is empty")
		}
		return usage, nil
	case SourceJSR:
		compatibilityName, err := JSRCompatibilityName(identity.Name)
		if err != nil {
			return PackageUsage{}, err
		}
		usage.MaterializedAs = MaterializationIdentity{
			Source: SourceNPMCompat,
			Name:   compatibilityName,
		}
		usage.Import.Specifier = compatibilityName
		usage.Notes = []string{
			"JSR semantic identity is preserved; Node and TypeScript consume the JSR npm-compatibility package name.",
		}
		return usage, nil
	default:
		return PackageUsage{}, fmt.Errorf("package source %q has no Node registry compatibility mapping", identity.Source)
	}
}

// JSRCompatibilityName maps a semantic JSR name to the documented npm
// compatibility spelling. Double underscores are rejected because they make
// the reverse mapping ambiguous.
func JSRCompatibilityName(name string) (string, error) {
	scope, pkg, err := splitJSRName(name)
	if err != nil {
		return "", err
	}
	if strings.Contains(scope, "__") || strings.Contains(pkg, "__") {
		return "", fmt.Errorf("JSR package name %q cannot be represented unambiguously by the npm compatibility registry", name)
	}
	return "@jsr/" + scope + "__" + pkg, nil
}

// LogicalJSRName maps a compatibility dependency key back to semantic JSR
// identity and rejects malformed or ambiguous spellings.
func LogicalJSRName(compatibilityName string) (string, error) {
	if !strings.HasPrefix(compatibilityName, "@jsr/") {
		return "", fmt.Errorf("unsupported JSR compatibility dependency name %q", compatibilityName)
	}
	value := strings.TrimPrefix(compatibilityName, "@jsr/")
	if strings.Count(value, "__") != 1 {
		return "", fmt.Errorf("unsupported or ambiguous JSR compatibility dependency name %q", compatibilityName)
	}
	parts := strings.SplitN(value, "__", 2)
	logicalName := "@" + parts[0] + "/" + parts[1]
	if _, _, err := splitJSRName(logicalName); err != nil {
		return "", fmt.Errorf("unsupported JSR compatibility dependency name %q: %w", compatibilityName, err)
	}
	return logicalName, nil
}

func splitJSRName(name string) (string, string, error) {
	if !strings.HasPrefix(name, "@") {
		return "", "", fmt.Errorf("JSR package name %q must use @scope/package form", name)
	}
	parts := strings.Split(strings.TrimPrefix(name, "@"), "/")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", fmt.Errorf("JSR package name %q must use @scope/package form", name)
	}
	if strings.ContainsAny(parts[0], "\\:@") || strings.ContainsAny(parts[1], "\\:@") {
		return "", "", fmt.Errorf("JSR package name %q contains unsupported characters", name)
	}
	return parts[0], parts[1], nil
}
