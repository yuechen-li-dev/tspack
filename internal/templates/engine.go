package templates

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"github.com/yuechen-li-dev/tspack/internal/concepts"
)

//go:embed builtin/**
var builtinFS embed.FS

const MetadataFile = "tspack-template.toml"

const (
	SourceKindBuiltin = "built-in"
	SourceKindLocal   = "local"
)

var conceptNameRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9-]*(\.[A-Za-z][A-Za-z0-9-]*)*$`)
var variableNameRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)
var placeholderRe = regexp.MustCompile(`\{\{([A-Za-z][A-Za-z0-9_]*)\}\}`)

type TemplateSource struct {
	Kind string
	Root string
	Path string
}

type RawTemplate struct {
	Format      int                 `toml:"format"`
	Name        string              `toml:"name"`
	Description string              `toml:"description"`
	Kind        string              `toml:"kind"`
	Concepts    []string            `toml:"concepts"`
	Variables   map[string]Variable `toml:"variables"`
	Files       []File              `toml:"files"`
	Source      TemplateSource
	sourceFS    fs.FS
}

// Template is the normalized semantic template model used by listing and planning.
// Future overlays and composition should normalize into this layer before planning.
type Template struct {
	Format      int
	Name        string
	Description string
	Kind        string
	Concepts    []string
	Variables   map[string]Variable
	Files       []File
	Source      TemplateSource
	source      fs.FS
	root        string
}

type Variable struct {
	Description string   `toml:"description"`
	Default     string   `toml:"default"`
	Allowed     []string `toml:"allowed"`
}

type File struct {
	From string `toml:"from"`
	To   string `toml:"to"`
}

type ApplyOptions struct {
	Destination string
	Values      map[string]string
	Force       bool
	DryRun      bool
}

type PlanOptions struct {
	Destination string
	Values      map[string]string
	Force       bool
}

type TemplatePlan struct {
	TemplateName string
	TemplateKind string
	Concepts     []string
	Values       map[string]string
	Files        []PlannedFile
}

type PlannedFile struct {
	Path            string
	SourcePath      string
	DestinationPath string
	Rendered        bool
	content         []byte
}

func BuiltinNames() []string {
	entries, err := fs.ReadDir(builtinFS, "builtin")
	if err != nil {
		return nil
	}
	names := []string{}
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	return names
}

func LoadRawBuiltin(name string) (*RawTemplate, error) {
	if name == "" {
		name = "static"
	}
	root := filepath.ToSlash(filepath.Join("builtin", name))
	return loadRawFromFS(builtinFS, root, TemplateSource{Kind: SourceKindBuiltin, Root: root, Path: name})
}

func LoadBuiltin(name string) (*Template, error) {
	raw, err := LoadRawBuiltin(name)
	if err != nil {
		return nil, err
	}
	return Normalize(raw)
}

func LoadRawLocal(path string) (*RawTemplate, error) {
	clean, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("TSPACK_TEMPLATE_NOT_FOUND: %w", err)
	}
	info, err := os.Stat(filepath.Join(clean, MetadataFile))
	if err != nil {
		return nil, fmt.Errorf("TSPACK_TEMPLATE_NOT_FOUND: %s", path)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("TSPACK_TEMPLATE_NOT_FOUND: %s", filepath.Join(path, MetadataFile))
	}
	return loadRawFromFS(os.DirFS(clean), ".", TemplateSource{Kind: SourceKindLocal, Root: ".", Path: clean})
}

func LoadLocal(path string) (*Template, error) {
	raw, err := LoadRawLocal(path)
	if err != nil {
		return nil, err
	}
	return Normalize(raw)
}

func Load(nameOrPath string) (*Template, error) {
	if looksLikePath(nameOrPath) {
		return LoadLocal(nameOrPath)
	}
	return LoadBuiltin(nameOrPath)
}

func ListBuiltins() ([]*Template, error) {
	out := []*Template{}
	for _, name := range BuiltinNames() {
		tmpl, err := LoadBuiltin(name)
		if err != nil {
			return nil, err
		}
		out = append(out, tmpl)
	}
	return out, nil
}

func looksLikePath(value string) bool {
	return strings.HasPrefix(value, ".") || strings.HasPrefix(value, string(filepath.Separator)) || strings.ContainsAny(value, `/\`)
}

func loadRawFromFS(source fs.FS, root string, sourceInfo TemplateSource) (*RawTemplate, error) {
	metadataPath := pathJoin(root, MetadataFile)
	data, err := fs.ReadFile(source, metadataPath)
	if err != nil {
		return nil, fmt.Errorf("TSPACK_TEMPLATE_NOT_FOUND: %s", metadataPath)
	}
	var raw RawTemplate
	if err := toml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("TSPACK_TEMPLATE_INVALID: %w", err)
	}
	raw.Source = sourceInfo
	raw.sourceFS = source
	return &raw, nil
}

func Normalize(raw *RawTemplate) (*Template, error) {
	if raw == nil {
		return nil, errors.New("TSPACK_TEMPLATE_INVALID: template is nil")
	}
	tmpl := &Template{
		Format:      raw.Format,
		Name:        raw.Name,
		Description: raw.Description,
		Kind:        raw.Kind,
		Concepts:    append([]string{}, raw.Concepts...),
		Variables:   copyVariables(raw.Variables),
		Files:       append([]File{}, raw.Files...),
		Source:      raw.Source,
		source:      raw.sourceFS,
		root:        raw.Source.Root,
	}
	if err := tmpl.Validate(); err != nil {
		return nil, err
	}
	return tmpl, nil
}

func (t *Template) Validate() error {
	if t.Format != 1 || t.Name == "" || t.Description == "" || t.Kind == "" || len(t.Concepts) == 0 || len(t.Files) == 0 {
		return errors.New("TSPACK_TEMPLATE_INVALID: format, name, description, kind, concepts, and files are required")
	}
	for _, concept := range t.Concepts {
		if !conceptNameRe.MatchString(concept) {
			return fmt.Errorf("TSPACK_TEMPLATE_INVALID: invalid concept %q", concept)
		}
	}
	for name, variable := range t.Variables {
		if name == "" || !variableNameRe.MatchString(name) {
			return fmt.Errorf("TSPACK_TEMPLATE_INVALID: invalid variable name %q", name)
		}
		if len(variable.Allowed) > 0 && variable.Default != "" && !contains(variable.Allowed, variable.Default) {
			return fmt.Errorf("TSPACK_TEMPLATE_VARIABLE_INVALID: default for %s is not allowed", name)
		}
	}
	seenDestinations := map[string]bool{}
	for _, file := range t.Files {
		if err := validateTemplatePath(file.From); err != nil {
			return fmt.Errorf("TSPACK_TEMPLATE_PATH_INVALID: from %q", file.From)
		}
		if err := validateTemplatePath(file.To); err != nil {
			return fmt.Errorf("TSPACK_TEMPLATE_PATH_INVALID: to %q", file.To)
		}
		if seenDestinations[file.To] {
			return fmt.Errorf("TSPACK_TEMPLATE_PATH_INVALID: duplicate destination %q", file.To)
		}
		seenDestinations[file.To] = true
		if _, err := fs.Stat(t.source, pathJoin(t.root, file.From)); err != nil {
			return fmt.Errorf("TSPACK_TEMPLATE_NOT_FOUND: %s", file.From)
		}
	}
	return nil
}

func (t *Template) ResolveValues(overrides map[string]string) (map[string]string, error) {
	values := map[string]string{}
	for name, variable := range t.Variables {
		value := variable.Default
		if overrides != nil && overrides[name] != "" {
			value = overrides[name]
		}
		if value == "" {
			return nil, fmt.Errorf("TSPACK_TEMPLATE_VARIABLE_MISSING: %s", name)
		}
		if len(variable.Allowed) > 0 && !contains(variable.Allowed, value) {
			return nil, fmt.Errorf("TSPACK_TEMPLATE_VARIABLE_INVALID: %s must be one of %s", name, strings.Join(variable.Allowed, ", "))
		}
		values[name] = value
	}
	for name, value := range overrides {
		if value != "" {
			values[name] = value
		}
	}
	return values, nil
}

func (t *Template) Plan(opts PlanOptions) (*TemplatePlan, error) {
	dest, err := filepath.Abs(opts.Destination)
	if err != nil {
		return nil, fmt.Errorf("TSPACK_TEMPLATE_WRITE_FAILED: %w", err)
	}
	plan := &TemplatePlan{
		TemplateName: t.Name,
		TemplateKind: t.Kind,
		Concepts:     append([]string{}, t.Concepts...),
		Values:       copyStringMap(opts.Values),
		Files:        []PlannedFile{},
	}
	for _, file := range t.Files {
		target, err := safeJoin(dest, file.To)
		if err != nil {
			return nil, err
		}
		if existsAsDir(target) {
			return nil, fmt.Errorf("TSPACK_TEMPLATE_WRITE_FAILED: %s is a directory", file.To)
		}
		if !opts.Force && exists(target) {
			return nil, fmt.Errorf("TSPACK_TEMPLATE_FILE_EXISTS: %s (use --force to overwrite)", file.To)
		}
		data, err := fs.ReadFile(t.source, pathJoin(t.root, file.From))
		if err != nil {
			return nil, fmt.Errorf("TSPACK_TEMPLATE_NOT_FOUND: %s", file.From)
		}
		rendered := strings.HasSuffix(file.From, ".tmpl")
		if rendered {
			content, err := render(string(data), opts.Values)
			if err != nil {
				return nil, err
			}
			data = []byte(content)
		}
		if t.usesStaticConceptManifest(file) {
			content, err := t.renderStaticConceptManifestFile(opts.Values)
			if err != nil {
				return nil, err
			}
			data = []byte(content)
			rendered = true
		}
		plan.Files = append(plan.Files, PlannedFile{Path: file.To, SourcePath: file.From, DestinationPath: target, Rendered: rendered, content: data})
	}
	return plan, nil
}

func (t *Template) usesStaticConceptManifest(file File) bool {
	return t.Source.Kind == SourceKindBuiltin && t.Name == "static" && file.To == "manifest.tsx"
}

func (t *Template) renderStaticConceptManifestFile(values map[string]string) (string, error) {
	conceptIR, err := concepts.BuildConceptIR(t.Concepts, t.Kind, concepts.Builtins)
	if err != nil {
		return "", formatTemplateConceptCompositionError(t.Name, err)
	}
	return renderStaticConceptManifest(values, conceptIR)
}

func formatTemplateConceptCompositionError(templateName string, err error) error {
	if conflict, ok := err.(concepts.Conflict); ok {
		return fmt.Errorf("TSPACK_TEMPLATE_CONCEPT_COMPOSITION_FAILED: template %q concept %q conflict at %s: %s", templateName, conflict.ConceptB, conflict.Path, conflict.Reason)
	}
	return fmt.Errorf("TSPACK_TEMPLATE_CONCEPT_COMPOSITION_FAILED: template %q: %w", templateName, err)
}

func (t *Template) Apply(opts ApplyOptions) ([]PlannedFile, error) {
	plan, err := t.Plan(PlanOptions{Destination: opts.Destination, Values: opts.Values, Force: opts.Force})
	if err != nil {
		return nil, err
	}
	if opts.DryRun {
		return plan.Files, nil
	}
	if err := ApplyPlan(plan); err != nil {
		return nil, err
	}
	return plan.Files, nil
}

func ApplyPlan(plan *TemplatePlan) error {
	for _, file := range plan.Files {
		if err := os.MkdirAll(filepath.Dir(file.DestinationPath), 0o755); err != nil {
			return fmt.Errorf("TSPACK_TEMPLATE_WRITE_FAILED: %w", err)
		}
		if err := os.WriteFile(file.DestinationPath, file.content, 0o644); err != nil {
			return fmt.Errorf("TSPACK_TEMPLATE_WRITE_FAILED: %w", err)
		}
	}
	return nil
}

func render(input string, values map[string]string) (string, error) {
	missing := ""
	output := placeholderRe.ReplaceAllStringFunc(input, func(token string) string {
		name := placeholderRe.FindStringSubmatch(token)[1]
		value, ok := values[name]
		if !ok {
			missing = name
			return token
		}
		return value
	})
	if missing != "" {
		return "", fmt.Errorf("TSPACK_TEMPLATE_UNKNOWN_VARIABLE: %s", missing)
	}
	return output, nil
}

func validateTemplatePath(value string) error {
	if value == "" || filepath.IsAbs(value) || strings.Contains(value, "\\") {
		return errors.New("invalid path")
	}
	clean := filepath.Clean(value)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.Contains(clean, "/../") {
		return errors.New("invalid path")
	}
	if regexp.MustCompile(`^[A-Za-z]:`).MatchString(value) {
		return errors.New("invalid path")
	}
	return nil
}

func safeJoin(root string, rel string) (string, error) {
	if err := validateTemplatePath(rel); err != nil {
		return "", fmt.Errorf("TSPACK_TEMPLATE_PATH_INVALID: %s", rel)
	}
	target := filepath.Join(root, filepath.FromSlash(rel))
	if target != root && !strings.HasPrefix(target, root+string(filepath.Separator)) {
		return "", fmt.Errorf("TSPACK_TEMPLATE_PATH_INVALID: %s", rel)
	}
	return target, nil
}

func copyVariables(input map[string]Variable) map[string]Variable {
	out := map[string]Variable{}
	for name, variable := range input {
		variable.Allowed = append([]string{}, variable.Allowed...)
		out[name] = variable
	}
	return out
}

func copyStringMap(input map[string]string) map[string]string {
	out := map[string]string{}
	for name, value := range input {
		out[name] = value
	}
	return out
}

func pathJoin(parts ...string) string { return filepath.ToSlash(filepath.Join(parts...)) }
func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
func exists(path string) bool      { _, err := os.Stat(path); return err == nil }
func existsAsDir(path string) bool { info, err := os.Stat(path); return err == nil && info.IsDir() }
