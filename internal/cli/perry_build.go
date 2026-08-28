package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	compilerir "github.com/yuechen-li-dev/tspack/internal/compiler"
	"github.com/yuechen-li-dev/tspack/internal/manifest"
)

type perryConfig struct {
	SchemaVersion  int      `json:"schemaVersion"`
	Target         string   `json:"target,omitempty"`
	OutputType     string   `json:"outputType,omitempty"`
	FastMath       bool     `json:"fastMath,omitempty"`
	FPContract     string   `json:"fpContract,omitempty"`
	TypeCheck      bool     `json:"typeCheck,omitempty"`
	NoAutoOptimize bool     `json:"noAutoOptimize,omitempty"`
	NoCodegen      bool     `json:"noCodegen,omitempty"`
	Features       []string `json:"features,omitempty"`
}

func buildPerryTarget(ctx context.Context, root string, manifestPath string, ir *manifest.ManifestIR, pkg *manifest.Package, target manifest.Target) {
	packageRoot := resolvePackageRoot(root, manifestPath, ir, pkg)
	configPath := target.CompilerConfig
	if configPath == "" {
		configPath = "perry.json"
	}
	configRef := compilerConfigRef(packageRoot, configPath, "perry.json")
	if configRef.Fingerprint == "missing" {
		failBuild("TSPACK_COMPILER_CONFIG_MISSING", "Perry compiler config is missing at "+filepath.Join(packageRoot, filepath.FromSlash(configRef.Path)))
	}
	config, err := loadPerryConfig(filepath.Join(packageRoot, filepath.FromSlash(configRef.Path)))
	if err != nil {
		failBuild("TSPACK_PERRY_CONFIG_INVALID", err.Error())
	}
	compilerPath := perryToolPath(root, packageRoot, pkg, target)
	if _, err := os.Stat(compilerPath); err != nil {
		failBuild("TSPACK_COMPILER_TOOL_MISSING", "project-managed Perry compiler is missing at "+compilerPath)
	}
	version, err := compilerVersionAt(ctx, compilerPath, packageRoot)
	if err != nil {
		failBuild("TSPACK_COMPILER_VERSION_FAILED", err.Error())
	}
	version = normalizePerryVersion(version)
	runtimeDirectory, err := discoverPerryRuntimeDirectory(ctx, compilerPath, packageRoot)
	if err != nil {
		failBuild("TSPACK_COMPILER_RUNTIME_MISSING", err.Error())
	}

	inputs, err := collectOwnedInputs(packageRoot, target.Inputs)
	if err != nil {
		failBuild("TSPACK_COMPILER_SOURCE_MISSING", err.Error())
	}
	entryPath := filepath.Clean(filepath.Join(packageRoot, filepath.FromSlash(target.Entry)))
	if !containsInputPath(inputs, entryPath) {
		failBuild("TSPACK_COMPILER_ENTRY_NOT_OWNED", "Perry entry is not owned by target inputs: "+target.Entry)
	}
	if target.Artifact != "nativeExecutable" {
		failBuild("TSPACK_COMPILER_ARTIFACT_UNSUPPORTED", "Perry M71b supports nativeExecutable artifacts")
	}
	outputPath := filepath.Clean(filepath.Join(packageRoot, filepath.FromSlash(target.Runtime)))
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		failBuild("TSPACK_BUILD_IO", err.Error())
	}
	stagingPath := outputPath + ".tspack-staging"
	_ = os.Remove(stagingPath)

	payloadBytes, err := json.Marshal(compilerir.PerryPayloadV1{
		Entry:          filepath.ToSlash(target.Entry),
		Output:         stagingPath,
		Target:         config.Target,
		OutputType:     config.OutputType,
		FastMath:       config.FastMath,
		FPContract:     config.FPContract,
		TypeCheck:      config.TypeCheck,
		NoAutoOptimize: config.NoAutoOptimize,
		NoCodegen:      config.NoCodegen,
		Features:       append([]string(nil), config.Features...),
	})
	if err != nil {
		failBuild("TSPACK_PERRY_CONFIG_INVALID", err.Error())
	}
	adapter := compilerir.PerryAdapter{RuntimeDirectory: runtimeDirectory}
	compilerTarget := compilerir.Target{
		ProjectRoot:  packageRoot,
		Package:      pkg.Name,
		Name:         target.Name,
		Language:     compilerir.LanguageIdentity{ID: "perry-ts"},
		Compiler:     compilerir.CompilerIdentity{ID: "perry", Version: version},
		Tool:         compilerir.ToolIdentity{Source: perryToolSource(pkg, target), Name: "@perryts/perry", Version: version, Path: compilerPath},
		Config:       configRef,
		Runtime:      compilerir.RuntimeIdentity{Family: "native", Name: perryTargetPlatform(config)},
		Outputs:      []compilerir.Output{{Kind: compilerir.OutputNativeExecutable, Path: target.Runtime}},
		Capabilities: adapter.DescribeCapabilities(),
		Payload:      compilerir.Payload{Kind: "perry-v1", SchemaVersion: 1, Data: payloadBytes},
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
		fmt.Printf("Cached %s:%s with perry %s -> %s\n", pkg.Name, target.Name, version, target.Runtime)
		return
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
		failBuild("TSPACK_COMPILER_BUILD_FAILED", "Perry build failed for "+pkg.Name+":"+target.Name)
	}
	if _, err := os.Stat(stagingPath); err != nil {
		failBuild("TSPACK_COMPILER_OUTPUT_MISSING", "Perry did not materialize declared output "+target.Runtime)
	}
	if err := replaceArtifactAtomically(stagingPath, outputPath); err != nil {
		failBuild("TSPACK_BUILD_IO", err.Error())
	}
	cacheBytes, _ := json.MarshalIndent(compilerCacheRecord{
		SchemaVersion: 1,
		Fingerprint:   fingerprint,
		Compiler:      version,
		Platform:      perryTargetPlatform(config),
	}, "", "  ")
	if err := writeFileAtomically(cachePath, append(cacheBytes, '\n'), 0o644); err != nil {
		failBuild("TSPACK_BUILD_IO", err.Error())
	}
	fmt.Printf("Built %s:%s with perry %s -> %s\n", pkg.Name, target.Name, version, target.Runtime)
}

func loadPerryConfig(path string) (perryConfig, error) {
	file, err := os.Open(path)
	if err != nil {
		return perryConfig{}, err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var config perryConfig
	if err := decoder.Decode(&config); err != nil {
		return perryConfig{}, fmt.Errorf("decode %s: %w", path, err)
	}
	if config.SchemaVersion != 1 {
		return perryConfig{}, fmt.Errorf("Perry config schemaVersion must be 1")
	}
	if config.OutputType == "" {
		config.OutputType = "executable"
	}
	if config.OutputType != "executable" {
		return perryConfig{}, fmt.Errorf("Perry M71b executable adapter requires outputType executable")
	}
	if config.FPContract != "" && config.FPContract != "off" && config.FPContract != "on" && config.FPContract != "fast" {
		return perryConfig{}, fmt.Errorf("Perry fpContract must be off, on, or fast")
	}
	for _, feature := range config.Features {
		if strings.TrimSpace(feature) == "" || strings.Contains(feature, ",") {
			return perryConfig{}, fmt.Errorf("Perry features must be non-empty names without commas")
		}
	}
	return config, nil
}

func perryToolPath(root string, packageRoot string, pkg *manifest.Package, target manifest.Target) string {
	configured := target.CompilerPath
	if configured == "" && pkg.Compiler == "perry" {
		configured = pkg.CompilerPath
	}
	if configured != "" {
		if filepath.IsAbs(configured) {
			return filepath.Clean(configured)
		}
		return filepath.Clean(filepath.Join(packageRoot, filepath.FromSlash(configured)))
	}
	path := filepath.Join(root, "node_modules", ".bin", "perry")
	if os.PathSeparator == '\\' {
		path += ".cmd"
	}
	return path
}

func perryToolSource(pkg *manifest.Package, target manifest.Target) string {
	if target.CompilerPath != "" || (pkg.Compiler == "perry" && pkg.CompilerPath != "") {
		return "path"
	}
	return "npm"
}

func normalizePerryVersion(value string) string {
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(value), "perry"))
}

func perryTargetPlatform(config perryConfig) string {
	if config.Target != "" {
		return config.Target
	}
	return runtime.GOOS + "-" + runtime.GOARCH
}

func discoverPerryRuntimeDirectory(ctx context.Context, compilerPath string, packageRoot string) (string, error) {
	invocation := compilerir.Invocation{
		Executable: compilerPath,
		Arguments:  []string{"doctor"},
		Directory:  packageRoot,
	}
	output, err := runCompilerInvocation(ctx, invocation)
	if err != nil {
		return "", fmt.Errorf("Perry doctor could not materialize its packaged runtime: %s", strings.TrimSpace(string(output)))
	}
	for _, line := range strings.Split(string(output), "\n") {
		const marker = "runtime library:"
		index := strings.Index(line, marker)
		if index < 0 {
			continue
		}
		libraryPath := strings.TrimSpace(line[index+len(marker):])
		if _, statErr := os.Stat(libraryPath); statErr != nil {
			return "", fmt.Errorf("Perry doctor reported a missing runtime library at %s", libraryPath)
		}
		return filepath.Dir(libraryPath), nil
	}
	return "", fmt.Errorf("Perry doctor did not report a runtime library path")
}
