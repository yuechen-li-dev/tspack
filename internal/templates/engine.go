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
)

//go:embed builtin/**
var builtinFS embed.FS

const MetadataFile = "tspack-template.toml"

var conceptNameRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9-]*(\.[A-Za-z][A-Za-z0-9-]*)*$`)
var placeholderRe = regexp.MustCompile(`\{\{([A-Za-z][A-Za-z0-9_]*)\}\}`)

type Template struct {
	Format      int                 `toml:"format"`
	Name        string              `toml:"name"`
	Description string              `toml:"description"`
	Kind        string              `toml:"kind"`
	Concepts    []string            `toml:"concepts"`
	Variables   map[string]Variable `toml:"variables"`
	Files       []File              `toml:"files"`
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

type PlannedFile struct {
	Path string
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

func LoadBuiltin(name string) (*Template, error) {
	if name == "" {
		name = "static"
	}
	root := filepath.ToSlash(filepath.Join("builtin", name))
	return loadFromFS(builtinFS, root)
}

func LoadLocal(path string) (*Template, error) {
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
	return loadFromFS(os.DirFS(clean), ".")
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

func loadFromFS(source fs.FS, root string) (*Template, error) {
	metadataPath := pathJoin(root, MetadataFile)
	data, err := fs.ReadFile(source, metadataPath)
	if err != nil {
		return nil, fmt.Errorf("TSPACK_TEMPLATE_NOT_FOUND: %s", metadataPath)
	}
	var tmpl Template
	if err := toml.Unmarshal(data, &tmpl); err != nil {
		return nil, fmt.Errorf("TSPACK_TEMPLATE_INVALID: %w", err)
	}
	tmpl.source = source
	tmpl.root = root
	if err := tmpl.Validate(); err != nil {
		return nil, err
	}
	return &tmpl, nil
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
		if name == "" || !regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`).MatchString(name) {
			return fmt.Errorf("TSPACK_TEMPLATE_INVALID: invalid variable name %q", name)
		}
		if len(variable.Allowed) > 0 && variable.Default != "" && !contains(variable.Allowed, variable.Default) {
			return fmt.Errorf("TSPACK_TEMPLATE_VARIABLE_INVALID: default for %s is not allowed", name)
		}
	}
	for _, file := range t.Files {
		if err := validateTemplatePath(file.From); err != nil {
			return fmt.Errorf("TSPACK_TEMPLATE_PATH_INVALID: from %q", file.From)
		}
		if err := validateTemplatePath(file.To); err != nil {
			return fmt.Errorf("TSPACK_TEMPLATE_PATH_INVALID: to %q", file.To)
		}
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

func (t *Template) Apply(opts ApplyOptions) ([]PlannedFile, error) {
	dest, err := filepath.Abs(opts.Destination)
	if err != nil {
		return nil, fmt.Errorf("TSPACK_TEMPLATE_WRITE_FAILED: %w", err)
	}
	planned := []PlannedFile{}
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
		planned = append(planned, PlannedFile{Path: file.To})
	}
	if opts.DryRun {
		return planned, nil
	}
	for _, file := range t.Files {
		data, err := fs.ReadFile(t.source, pathJoin(t.root, file.From))
		if err != nil {
			return nil, fmt.Errorf("TSPACK_TEMPLATE_NOT_FOUND: %s", file.From)
		}
		content := string(data)
		if strings.HasSuffix(file.From, ".tmpl") {
			content, err = render(content, opts.Values)
			if err != nil {
				return nil, err
			}
		}
		target, err := safeJoin(dest, file.To)
		if err != nil {
			return nil, err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return nil, fmt.Errorf("TSPACK_TEMPLATE_WRITE_FAILED: %w", err)
		}
		if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
			return nil, fmt.Errorf("TSPACK_TEMPLATE_WRITE_FAILED: %w", err)
		}
	}
	return planned, nil
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
