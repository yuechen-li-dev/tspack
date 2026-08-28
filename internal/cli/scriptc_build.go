package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"

	compilerir "github.com/yuechen-li-dev/tspack/internal/compiler"
	"github.com/yuechen-li-dev/tspack/internal/manifest"
)

type scriptCConfig struct {
	SchemaVersion int      `json:"schemaVersion"`
	Dynamic       bool     `json:"dynamic,omitempty"`
	Backend       string   `json:"backend,omitempty"`
	Optimization  string   `json:"optimization,omitempty"`
	NPMStatic     []string `json:"npmStatic,omitempty"`
	CC            string   `json:"cc,omitempty"`
	Target        string   `json:"target,omitempty"`
}

type compilerCacheRecord struct {
	SchemaVersion int    `json:"schemaVersion"`
	Fingerprint   string `json:"fingerprint"`
	Compiler      string `json:"compiler"`
	Platform      string `json:"platform"`
}

func buildScriptCTarget(ctx context.Context, root string, manifestPath string, ir *manifest.ManifestIR, pkg *manifest.Package, target manifest.Target) {
	packageRoot := resolvePackageRoot(root, manifestPath, ir, pkg)
	configPath := target.CompilerConfig
	if configPath == "" {
		configPath = "scriptc.json"
	}
	configRef := compilerConfigRef(packageRoot, configPath, "scriptc.json")
	if configRef.Fingerprint == "missing" {
		failBuild("TSPACK_COMPILER_CONFIG_MISSING", "ScriptC compiler config is missing at "+filepath.Join(packageRoot, filepath.FromSlash(configRef.Path)))
	}
	config, err := loadScriptCConfig(filepath.Join(packageRoot, filepath.FromSlash(configRef.Path)))
	if err != nil {
		failBuild("TSPACK_SCRIPTC_CONFIG_INVALID", err.Error())
	}
	compilerPath := scriptCToolPath(root, packageRoot, pkg, target)
	if _, err := os.Stat(compilerPath); err != nil {
		failBuild("TSPACK_COMPILER_TOOL_MISSING", "project-managed ScriptC compiler is missing at "+compilerPath)
	}
	version, err := compilerVersionAt(ctx, compilerPath, packageRoot)
	if err != nil {
		failBuild("TSPACK_COMPILER_VERSION_FAILED", err.Error())
	}
	version = normalizeCompilerVersion(version)

	inputs, err := collectOwnedInputs(packageRoot, target.Inputs)
	if err != nil {
		failBuild("TSPACK_COMPILER_SOURCE_MISSING", err.Error())
	}
	entryPath := filepath.Clean(filepath.Join(packageRoot, filepath.FromSlash(target.Entry)))
	if !containsInputPath(inputs, entryPath) {
		failBuild("TSPACK_COMPILER_ENTRY_NOT_OWNED", "ScriptC entry is not owned by target inputs: "+target.Entry)
	}
	if target.Artifact != "nativeExecutable" {
		failBuild("TSPACK_COMPILER_ARTIFACT_UNSUPPORTED", "ScriptC M71a supports nativeExecutable artifacts")
	}
	outputPath := filepath.Clean(filepath.Join(packageRoot, filepath.FromSlash(target.Runtime)))
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		failBuild("TSPACK_BUILD_IO", err.Error())
	}
	stagingPath := outputPath + ".tspack-staging"
	_ = os.Remove(stagingPath)

	payloadValue := compilerir.ScriptCPayloadV1{
		Entry:        filepath.ToSlash(target.Entry),
		Output:       stagingPath,
		Dynamic:      config.Dynamic,
		Backend:      config.Backend,
		Optimization: config.Optimization,
		NPMStatic:    append([]string(nil), config.NPMStatic...),
		CC:           config.CC,
		Target:       config.Target,
	}
	payloadBytes, err := json.Marshal(payloadValue)
	if err != nil {
		failBuild("TSPACK_SCRIPTC_CONFIG_INVALID", err.Error())
	}
	adapter := compilerir.ScriptCAdapter{}
	compilerTarget := compilerir.Target{
		ProjectRoot: packageRoot,
		Package:     pkg.Name,
		Name:        target.Name,
		Language:    compilerir.LanguageIdentity{ID: "scriptc"},
		Compiler:    compilerir.CompilerIdentity{ID: "scriptc", Version: version},
		Tool:        compilerir.ToolIdentity{Source: scriptCToolSource(pkg, target), Name: "scriptc", Version: version, Path: compilerPath},
		Config:      configRef,
		Runtime:     compilerir.RuntimeIdentity{Family: "native", Name: scriptCTargetPlatform(config)},
		Outputs: []compilerir.Output{
			{Kind: compilerir.OutputNativeExecutable, Path: target.Runtime},
			{Kind: compilerir.OutputCompilerMetadata, Path: scriptCCoverageRelativePath(pkg.Name, target.Name)},
		},
		Capabilities: adapter.DescribeCapabilities(),
		Payload:      compilerir.Payload{Kind: "scriptc-v1", SchemaVersion: 1, Data: payloadBytes},
		Packages:     collectScriptCPackageBindings(root, pkg, target),
	}
	for _, input := range inputs {
		compilerTarget.Inputs = append(compilerTarget.Inputs, compilerir.Input{
			LogicalPath: input.LogicalPath,
			Path:        input.Path,
			Fingerprint: input.Fingerprint,
		})
	}
	descriptor, err := compilerir.NewDescriptor(compilerTarget)
	if err != nil {
		failBuild("TSPACK_COMPILER_TARGET_INVALID", err.Error())
	}
	descriptorPath := filepath.Join(packageRoot, ".tspack", "build-manifests", safeBuildName(pkg.Name)+"-"+safeBuildName(target.Name)+".compiler-target.json")
	if err := writeCompilerDescriptor(descriptorPath, descriptor); err != nil {
		failBuild("TSPACK_BUILD_IO", err.Error())
	}

	fingerprint := compilerTargetFingerprint(compilerTarget)
	cachePath := filepath.Join(packageRoot, ".tspack", "compiler-cache", safeBuildName(pkg.Name)+"-"+safeBuildName(target.Name)+".json")
	if compilerCacheHit(cachePath, outputPath, fingerprint) {
		fmt.Printf("Cached %s:%s with scriptc %s -> %s\n", pkg.Name, target.Name, version, target.Runtime)
		return
	}

	coverageInvocation, err := adapter.PrepareCoverageInvocation(compilerTarget)
	if err != nil {
		failBuild("TSPACK_COMPILER_TARGET_INVALID", err.Error())
	}
	coverageOutput, coverageErr := runCompilerInvocation(ctx, coverageInvocation)
	coveragePath := filepath.Join(packageRoot, filepath.FromSlash(scriptCCoverageRelativePath(pkg.Name, target.Name)))
	if err := writeFileAtomically(coveragePath, coverageOutput, 0o644); err != nil {
		failBuild("TSPACK_BUILD_IO", err.Error())
	}
	if coverageErr != nil {
		if len(coverageOutput) > 0 {
			fmt.Fprint(os.Stderr, string(coverageOutput))
		}
		if !config.Dynamic {
			failBuild("TSPACK_COMPILER_STATIC_COVERAGE_REQUIRED", "ScriptC hot-path target is not fully static; set dynamic: true explicitly only if an engine-backed target is intended")
		}
		failBuild("TSPACK_SCRIPTC_COVERAGE_FAILED", "ScriptC coverage failed for "+pkg.Name+":"+target.Name)
	}
	if !config.Dynamic && scriptCCoverageRequiresDynamic(coverageOutput) {
		fmt.Fprint(os.Stderr, string(coverageOutput))
		failBuild("TSPACK_COMPILER_STATIC_COVERAGE_REQUIRED", "ScriptC hot-path target reports dynamic-only sites; set dynamic: true explicitly only if an engine-backed target is intended")
	}

	invocation, err := adapter.PrepareInvocation(compilerTarget, descriptorPath)
	if err != nil {
		failBuild("TSPACK_COMPILER_TARGET_INVALID", err.Error())
	}
	buildOutput, buildErr := runCompilerInvocation(ctx, invocation)
	if len(buildOutput) > 0 {
		fmt.Print(string(buildOutput))
	}
	if buildErr != nil {
		_ = os.Remove(stagingPath)
		failBuild("TSPACK_COMPILER_BUILD_FAILED", "ScriptC build failed for "+pkg.Name+":"+target.Name)
	}
	if _, err := os.Stat(stagingPath); err != nil {
		failBuild("TSPACK_COMPILER_OUTPUT_MISSING", "ScriptC did not materialize declared output "+target.Runtime)
	}
	if err := replaceArtifactAtomically(stagingPath, outputPath); err != nil {
		failBuild("TSPACK_BUILD_IO", err.Error())
	}
	cache := compilerCacheRecord{SchemaVersion: 1, Fingerprint: fingerprint, Compiler: version, Platform: runtime.GOOS + "-" + runtime.GOARCH}
	cacheBytes, _ := json.MarshalIndent(cache, "", "  ")
	if err := writeFileAtomically(cachePath, append(cacheBytes, '\n'), 0o644); err != nil {
		failBuild("TSPACK_BUILD_IO", err.Error())
	}
	fmt.Printf("Built %s:%s with scriptc %s -> %s\n", pkg.Name, target.Name, version, target.Runtime)
}

func loadScriptCConfig(path string) (scriptCConfig, error) {
	file, err := os.Open(path)
	if err != nil {
		return scriptCConfig{}, err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var config scriptCConfig
	if err := decoder.Decode(&config); err != nil {
		return scriptCConfig{}, fmt.Errorf("decode %s: %w", path, err)
	}
	if config.SchemaVersion != 1 {
		return scriptCConfig{}, fmt.Errorf("ScriptC config schemaVersion must be 1")
	}
	if config.Backend != "" && config.Backend != "llvm" && config.Backend != "c" {
		return scriptCConfig{}, fmt.Errorf("ScriptC config backend must be llvm or c")
	}
	if config.Optimization != "" && config.Optimization != "release" && config.Optimization != "dev" {
		return scriptCConfig{}, fmt.Errorf("ScriptC config optimization must be release or dev")
	}
	if config.CC != "" && config.CC != "clang" && config.CC != "zigcc" {
		return scriptCConfig{}, fmt.Errorf("ScriptC config cc must be clang or zigcc")
	}
	if config.Target != "" && config.CC != "zigcc" {
		return scriptCConfig{}, fmt.Errorf("ScriptC config target requires cc zigcc")
	}
	for _, packageName := range config.NPMStatic {
		if strings.TrimSpace(packageName) == "" || packageName == "auto" {
			return scriptCConfig{}, fmt.Errorf("ScriptC config npmStatic must name explicit target-visible packages; auto is not deterministic enough for M71a")
		}
	}
	return config, nil
}

func scriptCToolPath(root string, packageRoot string, pkg *manifest.Package, target manifest.Target) string {
	configured := target.CompilerPath
	if configured == "" && pkg.Compiler == "scriptc" {
		configured = pkg.CompilerPath
	}
	if configured != "" {
		if filepath.IsAbs(configured) {
			return filepath.Clean(configured)
		}
		return filepath.Clean(filepath.Join(packageRoot, filepath.FromSlash(configured)))
	}
	path := filepath.Join(root, "node_modules", ".bin", "scriptc")
	if os.PathSeparator == '\\' {
		path += ".cmd"
	}
	return path
}

func scriptCToolSource(pkg *manifest.Package, target manifest.Target) string {
	if target.CompilerPath != "" || (pkg.Compiler == "scriptc" && pkg.CompilerPath != "") {
		return "path"
	}
	return "npm"
}

func normalizeCompilerVersion(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "scriptc")
	value = strings.TrimPrefix(value, "Version")
	return strings.TrimSpace(value)
}

func scriptCCoverageRequiresDynamic(output []byte) bool {
	report := string(output)
	return strings.Contains(report, "runs with --dynamic")
}

func runCompilerInvocation(ctx context.Context, invocation compilerir.Invocation) ([]byte, error) {
	command := exec.CommandContext(ctx, invocation.Executable, invocation.Arguments...)
	command.Dir = invocation.Directory
	command.Env = os.Environ()
	keys := make([]string, 0, len(invocation.Environment))
	for key := range invocation.Environment {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		command.Env = append(command.Env, key+"="+invocation.Environment[key])
	}
	return runOwnedBuildCommand(command)
}

func scriptCTargetPlatform(config scriptCConfig) string {
	if config.Target != "" {
		return config.Target
	}
	return runtime.GOOS + "-" + runtime.GOARCH
}

func collectScriptCPackageBindings(root string, pkg *manifest.Package, target manifest.Target) []compilerir.PackageBinding {
	all := collectCompilerPackageBindings(root, pkg)
	visible := map[string]bool{}
	for _, dependency := range target.Deps {
		visible[dependency] = true
	}
	bindings := []compilerir.PackageBinding{}
	for _, binding := range all {
		if visible[binding.LocalName] || visible[binding.MaterializationName] || visible[strings.TrimPrefix(binding.SemanticIdentity, "npm:")] {
			bindings = append(bindings, binding)
		}
	}
	return bindings
}

type ownedInput struct {
	LogicalPath string
	Path        string
	Fingerprint string
}

func collectOwnedInputs(packageRoot string, patterns []string) ([]ownedInput, error) {
	inputs := []ownedInput{}
	err := filepath.WalkDir(packageRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			name := entry.Name()
			if path != packageRoot && (name == ".git" || name == ".tspack" || name == "node_modules" || name == "dist") {
				return filepath.SkipDir
			}
			return nil
		}
		logical, err := filepath.Rel(packageRoot, path)
		if err != nil {
			return err
		}
		logical = filepath.ToSlash(logical)
		if !matchesAnyCompilerInput(logical, patterns) {
			return nil
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		hash := sha256.Sum256(contents)
		inputs = append(inputs, ownedInput{LogicalPath: logical, Path: path, Fingerprint: hex.EncodeToString(hash[:])})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(inputs, func(i, j int) bool { return inputs[i].LogicalPath < inputs[j].LogicalPath })
	if len(inputs) == 0 {
		return nil, fmt.Errorf("compiler input patterns matched no files: %s", strings.Join(patterns, ", "))
	}
	return inputs, nil
}

func matchesAnyCompilerInput(logicalPath string, patterns []string) bool {
	for _, pattern := range patterns {
		if compilerGlobMatches(filepath.ToSlash(pattern), logicalPath) {
			return true
		}
	}
	return false
}

func compilerGlobMatches(pattern string, value string) bool {
	var expression strings.Builder
	expression.WriteString("^")
	for index := 0; index < len(pattern); index++ {
		switch pattern[index] {
		case '*':
			if index+1 < len(pattern) && pattern[index+1] == '*' {
				index++
				if index+1 < len(pattern) && pattern[index+1] == '/' {
					index++
					expression.WriteString("(?:.*/)?")
				} else {
					expression.WriteString(".*")
				}
			} else {
				expression.WriteString("[^/]*")
			}
		case '?':
			expression.WriteString("[^/]")
		default:
			expression.WriteString(regexp.QuoteMeta(string(pattern[index])))
		}
	}
	expression.WriteString("$")
	matched, _ := regexp.MatchString(expression.String(), filepath.ToSlash(value))
	return matched
}

func containsInputPath(inputs []ownedInput, path string) bool {
	for _, input := range inputs {
		if samePath(input.Path, path) {
			return true
		}
	}
	return false
}

func samePath(left string, right string) bool {
	if os.PathSeparator == '\\' {
		return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
	}
	return filepath.Clean(left) == filepath.Clean(right)
}

func compilerTargetFingerprint(target compilerir.Target) string {
	descriptor, _ := compilerir.NewDescriptor(target)
	contents, _ := json.Marshal(descriptor)
	hash := sha256.Sum256(append(contents, []byte("\x00"+runtime.GOOS+"/"+runtime.GOARCH)...))
	return hex.EncodeToString(hash[:])
}

func compilerCacheHit(cachePath string, outputPath string, fingerprint string) bool {
	if _, err := os.Stat(outputPath); err != nil {
		return false
	}
	contents, err := os.ReadFile(cachePath)
	if err != nil {
		return false
	}
	var cache compilerCacheRecord
	return json.Unmarshal(contents, &cache) == nil && cache.SchemaVersion == 1 && cache.Fingerprint == fingerprint
}

func scriptCCoverageRelativePath(packageName string, targetName string) string {
	return filepath.ToSlash(filepath.Join(".tspack", "compiler-metadata", safeBuildName(packageName)+"-"+safeBuildName(targetName)+".coverage.txt"))
}

func writeFileAtomically(path string, contents []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, contents, mode); err != nil {
		return err
	}
	return replaceArtifactAtomically(temporary, path)
}

func replaceArtifactAtomically(stagingPath string, outputPath string) error {
	backupPath := outputPath + ".previous"
	_ = os.Remove(backupPath)
	if _, err := os.Stat(outputPath); err == nil {
		if err := os.Rename(outputPath, backupPath); err != nil {
			return err
		}
	}
	if err := os.Rename(stagingPath, outputPath); err != nil {
		_ = os.Rename(backupPath, outputPath)
		return err
	}
	_ = os.Remove(backupPath)
	return nil
}

func validateCompilerSourceOwnership(packageRoot string, pkg *manifest.Package) error {
	owners := map[string]string{}
	for _, target := range pkg.Targets {
		if len(target.Inputs) == 0 {
			continue
		}
		inputs, err := collectOwnedInputs(packageRoot, target.Inputs)
		if err != nil {
			return fmt.Errorf("target %s: %w", target.Name, err)
		}
		for _, input := range inputs {
			if owner, exists := owners[input.LogicalPath]; exists && owner != target.Name {
				return fmt.Errorf("TSPACK_COMPILER_SOURCE_OVERLAP: %s is owned by both %s and %s", input.LogicalPath, owner, target.Name)
			}
			owners[input.LogicalPath] = target.Name
		}
	}
	defaultOwners := []manifest.Target{}
	for _, target := range pkg.Targets {
		if len(target.Inputs) == 0 && target.Compiler == "tsc" {
			defaultOwners = append(defaultOwners, target)
		}
	}
	if len(defaultOwners) == 1 {
		sources, err := collectTypeScriptSources(packageRoot)
		if err != nil {
			return err
		}
		for _, source := range sources {
			if owners[source.LogicalPath] == "" {
				owners[source.LogicalPath] = defaultOwners[0].Name
			}
		}
	}
	return validateCrossCompilerImports(packageRoot, pkg, owners)
}

var compilerBoundaryImportPattern = regexp.MustCompile(`(?m)(?:from\s*|import\s*\(\s*|require\s*\(\s*)["'](\.{1,2}/[^"']+)["']`)

func validateCrossCompilerImports(packageRoot string, pkg *manifest.Package, owners map[string]string) error {
	compilerByTarget := map[string]string{}
	for _, target := range pkg.Targets {
		compilerByTarget[target.Name] = target.Compiler
	}
	for logicalPath, owner := range owners {
		contents, err := os.ReadFile(filepath.Join(packageRoot, filepath.FromSlash(logicalPath)))
		if err != nil {
			return err
		}
		matches := compilerBoundaryImportPattern.FindAllSubmatch(contents, -1)
		for _, match := range matches {
			resolved := resolveRelativeTypeScriptImport(logicalPath, string(match[1]), owners)
			otherOwner := owners[resolved]
			if otherOwner != "" && otherOwner != owner && compilerByTarget[otherOwner] != compilerByTarget[owner] {
				return fmt.Errorf("TSPACK_COMPILER_CROSS_TARGET_SOURCE_IMPORT: %s imports %s owned by %s; consume the declared artifact boundary instead", logicalPath, resolved, otherOwner)
			}
		}
	}
	return nil
}

func resolveRelativeTypeScriptImport(importer string, specifier string, owners map[string]string) string {
	base := filepath.ToSlash(filepath.Clean(filepath.Join(filepath.Dir(importer), filepath.FromSlash(specifier))))
	candidates := []string{base, base + ".ts", base + ".tsx", strings.TrimSuffix(base, ".js") + ".ts", strings.TrimSuffix(base, ".js") + ".tsx", base + "/index.ts", base + "/index.tsx"}
	for _, candidate := range candidates {
		candidate = filepath.ToSlash(candidate)
		if owners[candidate] != "" {
			return candidate
		}
	}
	return filepath.ToSlash(base)
}

func orderBuildTargets(pkg *manifest.Package, requestedTarget string) ([]manifest.Target, error) {
	byName := map[string]manifest.Target{}
	for _, target := range pkg.Targets {
		byName[target.Name] = target
	}
	selected := map[string]bool{}
	var selectTarget func(string) error
	selectTarget = func(name string) error {
		if selected[name] {
			return nil
		}
		target, ok := byName[name]
		if !ok {
			return fmt.Errorf("unknown compiler target dependency %s", name)
		}
		selected[name] = true
		for _, dependency := range target.DependsOn {
			if err := selectTarget(dependency); err != nil {
				return err
			}
		}
		return nil
	}
	if requestedTarget == "" {
		for _, target := range pkg.Targets {
			if err := selectTarget(target.Name); err != nil {
				return nil, err
			}
		}
	} else if err := selectTarget(requestedTarget); err != nil {
		return nil, err
	}
	state := map[string]int{}
	ordered := []manifest.Target{}
	var visit func(string) error
	visit = func(name string) error {
		if state[name] == 2 || !selected[name] {
			return nil
		}
		if state[name] == 1 {
			return fmt.Errorf("TSPACK_COMPILER_TARGET_DEPENDENCY_CYCLE: cycle includes %s", name)
		}
		state[name] = 1
		target := byName[name]
		for _, dependency := range target.DependsOn {
			if err := visit(dependency); err != nil {
				return err
			}
		}
		state[name] = 2
		ordered = append(ordered, target)
		return nil
	}
	for _, target := range pkg.Targets {
		if err := visit(target.Name); err != nil {
			return nil, err
		}
	}
	return ordered, nil
}

func projectTSCConfig(packageRoot string, pkg *manifest.Package, target manifest.Target, original compilerir.ConfigRef) (compilerir.ConfigRef, error) {
	excludes := []string{}
	for _, other := range pkg.Targets {
		if other.Name == target.Name || other.Compiler == "tsc" {
			continue
		}
		excludes = append(excludes, other.Inputs...)
	}
	if len(excludes) == 0 {
		return original, nil
	}
	sort.Strings(excludes)
	projectionPath := filepath.Join(packageRoot, ".tspack", "compiler-configs", safeBuildName(pkg.Name)+"-"+safeBuildName(target.Name)+".tsconfig.json")
	relativePackageRoot, err := filepath.Rel(filepath.Dir(projectionPath), packageRoot)
	if err != nil {
		return compilerir.ConfigRef{}, err
	}
	projectedExcludes := make([]string, 0, len(excludes))
	for _, exclude := range excludes {
		projectedExcludes = append(projectedExcludes, filepath.ToSlash(filepath.Join(relativePackageRoot, filepath.FromSlash(exclude))))
	}
	relativeBase, err := filepath.Rel(filepath.Dir(projectionPath), filepath.Join(packageRoot, filepath.FromSlash(original.Path)))
	if err != nil {
		return compilerir.ConfigRef{}, err
	}
	document := map[string]any{"extends": filepath.ToSlash(relativeBase), "exclude": projectedExcludes}
	contents, _ := json.MarshalIndent(document, "", "  ")
	if err := writeFileAtomically(projectionPath, append(contents, '\n'), 0o644); err != nil {
		return compilerir.ConfigRef{}, err
	}
	hash := sha256.Sum256(append([]byte(original.Fingerprint+"\x00"), contents...))
	relativeProjection, _ := filepath.Rel(packageRoot, projectionPath)
	return compilerir.ConfigRef{Kind: "file", Path: filepath.ToSlash(relativeProjection), Fingerprint: hex.EncodeToString(hash[:])}, nil
}

func collectTSCInputs(packageRoot string, pkg *manifest.Package, target manifest.Target) ([]ownedInput, error) {
	if len(target.Inputs) > 0 {
		return collectOwnedInputs(packageRoot, target.Inputs)
	}
	sources, err := collectTypeScriptSources(packageRoot)
	if err != nil {
		return nil, err
	}
	excludedPatterns := []string{}
	for _, other := range pkg.Targets {
		if other.Name != target.Name && other.Compiler != "tsc" {
			excludedPatterns = append(excludedPatterns, other.Inputs...)
		}
	}
	inputs := []ownedInput{}
	for _, source := range sources {
		if matchesAnyCompilerInput(source.LogicalPath, excludedPatterns) {
			continue
		}
		contents, err := os.ReadFile(source.Path)
		if err != nil {
			return nil, err
		}
		hash := sha256.Sum256(contents)
		inputs = append(inputs, ownedInput{LogicalPath: source.LogicalPath, Path: source.Path, Fingerprint: hex.EncodeToString(hash[:])})
	}
	return inputs, nil
}
