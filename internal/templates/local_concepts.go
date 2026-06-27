package templates

import (
	"bytes"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"github.com/yuechen-li-dev/tspack/internal/concepts"
)

type LocalConceptRef struct {
	Name string `toml:"name"`
	Path string `toml:"path"`
}

type localConceptFile struct {
	Format          int                            `toml:"format"`
	Name            string                         `toml:"name"`
	Description     string                         `toml:"description"`
	Provides        []string                       `toml:"provides"`
	Expects         []string                       `toml:"expects"`
	ExpectsAnyOf    [][]string                     `toml:"expectsAnyOf"`
	Conflicts       []string                       `toml:"conflicts"`
	CompatibleKinds []string                       `toml:"compatibleKinds"`
	Files           []localConceptFileContribution `toml:"files"`
	Dependencies    []localConceptDependency       `toml:"dependencies"`
	Tools           []localConceptDependency       `toml:"tools"`
	Peers           []localConceptDependency       `toml:"peers"`
	RunTargets      []localConceptRunTarget        `toml:"runTargets"`
}

type localConceptFileContribution struct {
	Destination string `toml:"destination"`
	Source      string `toml:"source"`
	Render      bool   `toml:"render"`
}

type localConceptDependency struct {
	Key    string `toml:"key"`
	Source string `toml:"source"`
	Range  string `toml:"range"`
}

type localConceptRunTarget struct {
	Name    string `toml:"name"`
	Command string `toml:"command"`
	Cwd     string `toml:"cwd"`
}

type localConceptLoadedFile struct {
	Destination string
	Source      string
	Render      bool
	SourceRoot  string
}

type localConceptSet struct {
	Registry  concepts.Registry
	Fragments []concepts.Fragment
	Files     []localConceptLoadedFile
}

func loadLocalConceptSet(t *Template) (*localConceptSet, error) {
	if len(t.LocalConcepts) == 0 {
		return &localConceptSet{Registry: concepts.Builtins}, nil
	}
	fragments := []concepts.Fragment{}
	files := []localConceptLoadedFile{}
	seen := map[string]bool{}
	stack := map[string]bool{}
	for _, name := range t.Concepts {
		stack[name] = true
	}
	for _, ref := range t.LocalConcepts {
		if ref.Name == "" || !conceptNameRe.MatchString(ref.Name) {
			return nil, fmt.Errorf("TSPACK_TEMPLATE_CONCEPT_INVALID: local concept name %q is invalid", ref.Name)
		}
		if seen[ref.Name] {
			return nil, fmt.Errorf("TSPACK_TEMPLATE_CONCEPT_INVALID: duplicate local concept %q", ref.Name)
		}
		seen[ref.Name] = true
		if !stack[ref.Name] {
			return nil, fmt.Errorf("TSPACK_TEMPLATE_CONCEPT_INVALID: local concept %q is declared but not listed in template concepts", ref.Name)
		}
		if _, ok := concepts.Lookup(ref.Name); ok {
			return nil, fmt.Errorf("TSPACK_TEMPLATE_CONCEPT_INVALID: local concept %q shadows a built-in concept", ref.Name)
		}
		if err := validateLocalConceptRefPath(ref.Path); err != nil {
			return nil, fmt.Errorf("TSPACK_TEMPLATE_CONCEPT_PATH_INVALID: local concept %q path %q: %v", ref.Name, ref.Path, err)
		}
		fragment, loadedFiles, err := loadOneLocalConcept(t.source, t.root, ref)
		if err != nil {
			return nil, err
		}
		fragments = append(fragments, fragment)
		files = append(files, loadedFiles...)
	}
	registryFragments := []concepts.Fragment{}
	for _, name := range concepts.BuiltinNames() {
		fragment, _ := concepts.Lookup(name)
		registryFragments = append(registryFragments, fragment)
	}
	registryFragments = append(registryFragments, fragments...)
	return &localConceptSet{Registry: concepts.NewRegistry(registryFragments), Fragments: fragments, Files: files}, nil
}

func loadOneLocalConcept(source fs.FS, templateRoot string, ref LocalConceptRef) (concepts.Fragment, []localConceptLoadedFile, error) {
	data, err := fs.ReadFile(source, pathJoin(templateRoot, ref.Path))
	if err != nil {
		return concepts.Fragment{}, nil, fmt.Errorf("TSPACK_TEMPLATE_CONCEPT_LOAD_FAILED: concept %q file %q: %w", ref.Name, ref.Path, err)
	}
	var raw localConceptFile
	if err := toml.NewDecoder(bytes.NewReader(data)).DisallowUnknownFields().Decode(&raw); err != nil {
		return concepts.Fragment{}, nil, fmt.Errorf("TSPACK_TEMPLATE_CONCEPT_INVALID: concept %q file %q: %w", ref.Name, ref.Path, err)
	}
	if raw.Format != 1 {
		return concepts.Fragment{}, nil, fmt.Errorf("TSPACK_TEMPLATE_CONCEPT_INVALID: concept %q file %q has unsupported format %d", ref.Name, ref.Path, raw.Format)
	}
	if raw.Name != ref.Name {
		return concepts.Fragment{}, nil, fmt.Errorf("TSPACK_TEMPLATE_CONCEPT_INVALID: concept %q file %q declares name %q", ref.Name, ref.Path, raw.Name)
	}
	for _, name := range append(append(append([]string{}, raw.Provides...), raw.Expects...), raw.Conflicts...) {
		if !conceptNameRe.MatchString(name) {
			return concepts.Fragment{}, nil, fmt.Errorf("TSPACK_TEMPLATE_CONCEPT_INVALID: concept %q has invalid concept reference %q", ref.Name, name)
		}
	}
	for _, group := range raw.ExpectsAnyOf {
		if len(group) == 0 {
			return concepts.Fragment{}, nil, fmt.Errorf("TSPACK_TEMPLATE_CONCEPT_INVALID: concept %q has empty expectsAnyOf group", ref.Name)
		}
		for _, name := range group {
			if !conceptNameRe.MatchString(name) {
				return concepts.Fragment{}, nil, fmt.Errorf("TSPACK_TEMPLATE_CONCEPT_INVALID: concept %q has invalid concept reference %q", ref.Name, name)
			}
		}
	}
	fragment := concepts.Fragment{Name: raw.Name, Description: raw.Description, Provides: raw.Provides, Requires: raw.Expects, RequiresAnyOf: raw.ExpectsAnyOf, Conflicts: raw.Conflicts, CompatibleKinds: raw.CompatibleKinds}
	for _, dep := range raw.Dependencies {
		if err := validateLocalConceptDependency(ref.Name, dep); err != nil {
			return concepts.Fragment{}, nil, err
		}
		fragment.Manifest.Dependencies = append(fragment.Manifest.Dependencies, lowerDep(dep))
	}
	for _, dep := range raw.Tools {
		if err := validateLocalConceptDependency(ref.Name, dep); err != nil {
			return concepts.Fragment{}, nil, err
		}
		fragment.Manifest.Tools = append(fragment.Manifest.Tools, lowerDep(dep))
	}
	for _, dep := range raw.Peers {
		if err := validateLocalConceptDependency(ref.Name, dep); err != nil {
			return concepts.Fragment{}, nil, err
		}
		fragment.Manifest.Peers = append(fragment.Manifest.Peers, lowerDep(dep))
	}
	for _, target := range raw.RunTargets {
		if target.Name == "" || target.Command == "" {
			return concepts.Fragment{}, nil, fmt.Errorf("TSPACK_TEMPLATE_CONCEPT_INVALID: concept %q runTarget requires name and command", ref.Name)
		}
		fragment.Manifest.RunTargets = append(fragment.Manifest.RunTargets, concepts.RunTargetContribution{Name: target.Name, Command: target.Command, Cwd: target.Cwd})
	}
	conceptDir := filepath.ToSlash(filepath.Dir(ref.Path))
	if conceptDir == "." {
		conceptDir = ""
	}
	loadedFiles := []localConceptLoadedFile{}
	for _, file := range raw.Files {
		if err := validateTemplatePath(file.Destination); err != nil {
			return concepts.Fragment{}, nil, fmt.Errorf("TSPACK_TEMPLATE_CONCEPT_PATH_INVALID: concept %q destination %q", ref.Name, file.Destination)
		}
		if err := validateTemplatePath(file.Source); err != nil {
			return concepts.Fragment{}, nil, fmt.Errorf("TSPACK_TEMPLATE_CONCEPT_PATH_INVALID: concept %q source %q", ref.Name, file.Source)
		}
		sourcePath := pathJoin(conceptDir, file.Source)
		if _, err := fs.Stat(source, pathJoin(templateRoot, sourcePath)); err != nil {
			return concepts.Fragment{}, nil, fmt.Errorf("TSPACK_TEMPLATE_CONCEPT_LOAD_FAILED: concept %q source %q: %w", ref.Name, file.Source, err)
		}
		fragment.Files = append(fragment.Files, concepts.FileContribution{Path: file.Destination, Content: sourcePath, Rendered: file.Render})
		loadedFiles = append(loadedFiles, localConceptLoadedFile{Destination: file.Destination, Source: sourcePath, Render: file.Render, SourceRoot: conceptDir})
	}
	return fragment, loadedFiles, nil
}

func lowerDep(dep localConceptDependency) concepts.DependencyContribution {
	return concepts.DependencyContribution{Name: dep.Key, Source: dep.Source, Range: dep.Range}
}

func validateLocalConceptRefPath(value string) error {
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return fmt.Errorf("remote URLs are not supported")
	}
	return validateTemplatePath(value)
}

func validateLocalConceptDependency(conceptName string, dep localConceptDependency) error {
	if dep.Key == "" || dep.Source == "" || dep.Range == "" {
		return fmt.Errorf("TSPACK_TEMPLATE_CONCEPT_INVALID: concept %q dependency requires key, source, and range", conceptName)
	}
	if dep.Source != "npm" {
		return fmt.Errorf("TSPACK_TEMPLATE_CONCEPT_INVALID: concept %q dependency %q has unsupported source %q", conceptName, dep.Key, dep.Source)
	}
	return nil
}
