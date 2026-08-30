package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	compilerir "github.com/yuechen-li-dev/tspack/internal/compiler"
	"github.com/yuechen-li-dev/tspack/internal/diag"
	"github.com/yuechen-li-dev/tspack/internal/lockfile"
	"github.com/yuechen-li-dev/tspack/internal/manifest"
	"github.com/yuechen-li-dev/tspack/internal/materialize"
	"github.com/yuechen-li-dev/tspack/internal/project"
)

// build is intentionally a narrow compiler-selection seam. Runtime process
// launching remains in run_command.go; this command only creates artifacts.
type buildCommandOptions struct {
	Root                   string
	Manifest               string
	PackageName            string
	TargetName             string
	PreserveLastSuccessful bool
}

type tsclProjectRequest struct {
	Descriptor        compilerir.Descriptor `json:"-"`
	Target            compilerir.Target     `json:"-"`
	ProjectRoot       string                `json:"projectRoot"`
	Sources           []tsclProjectSource   `json:"sources"`
	Entry             tsclProjectEntry      `json:"entry"`
	JavaScriptRuntime string                `json:"javascriptRuntime"`
	Backend           string                `json:"backend"`
	ExecutionRuntime  string                `json:"executionRuntime"`
	TargetFramework   string                `json:"targetFramework,omitempty"`
	RuntimeIdentifier string                `json:"runtimeIdentifier,omitempty"`
	JavaScriptProfile string                `json:"javascriptProfile"`
	TsXmlProfile      string                `json:"tsXmlProfile,omitempty"`
	OutputDirectory   string                `json:"outputDirectory"`
	EntryOutputPath   string                `json:"entryOutputPath"`
	BuildFingerprint  string                `json:"buildFingerprint"`
	NpmContracts      []tsclNpmContract     `json:"npmContracts"`
}

type tsclProjectSource struct {
	LogicalPath string `json:"logicalPath"`
	Path        string `json:"path"`
}

type tsclProjectEntry struct {
	Module string `json:"module"`
	Export string `json:"export"`
}

type tsclBuildResult struct {
	Success         bool   `json:"success"`
	CompilerVersion string `json:"compilerVersion"`
	Diagnostics     []struct {
		Code     string `json:"code"`
		Severity string `json:"severity"`
		Message  string `json:"message"`
		File     string `json:"file"`
		Line     int    `json:"line"`
		Column   int    `json:"column"`
	} `json:"diagnostics"`
	Outputs []struct {
		Path   string `json:"path"`
		SHA256 string `json:"sha256"`
	} `json:"outputs"`
	EntryOutputPath   string            `json:"entryOutputPath"`
	BuildFingerprint  string            `json:"buildFingerprint"`
	GraphFingerprint  string            `json:"graphFingerprint"`
	Backend           string            `json:"backend"`
	Runtime           string            `json:"runtime"`
	ArtifactKind      string            `json:"artifactKind"`
	TargetFramework   string            `json:"targetFramework"`
	RuntimeIdentifier string            `json:"runtimeIdentifier"`
	LaunchExecutable  string            `json:"launchExecutable"`
	LaunchArguments   []string          `json:"launchArguments"`
	Capabilities      []string          `json:"capabilities"`
	ToolVersions      map[string]string `json:"toolVersions"`
}

func runBuildCommand(args []string) {
	opts := parseBuildCommandOptions(args)
	workspace := openWorkspace(opts.Root)
	root := workspace.Root
	manifestPath := opts.Manifest
	if manifestPath == "" {
		manifestPath = filepath.Join(root, "manifest.tsx")
	}
	projectOptions := project.DefaultOptions(root)
	projectOptions.ManifestPath = manifestPath
	projectOptions.FrontendCLIPath = manifestFrontendCLIPath()
	request := project.BuildRequest{Project: projectOptions, PreserveLastSuccessful: opts.PreserveLastSuccessful, Executor: cliBuildTargetExecutor{}}
	if opts.PackageName != "" {
		request.Packages = []string{opts.PackageName}
	}
	if opts.TargetName != "" {
		request.Targets = []string{opts.TargetName}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	result := project.RunBuild(ctx, request)
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Code == "TSPACK_BUILD_TARGET_FAILED" {
			continue
		}
		fmt.Fprintf(os.Stderr, "%s: %s\n", diagnostic.Code, diagnostic.Message)
		for _, detail := range diagnostic.Details {
			fmt.Fprintf(os.Stderr, "  %s\n", detail)
		}
	}
	if hasDiagnosticErrors(result.Diagnostics) {
		exit(1)
	}
}

func buildTscTarget(ctx context.Context, root string, manifestPath string, ir *manifest.ManifestIR, pkg *manifest.Package, target manifest.Target) {
	packageRoot := resolvePackageRoot(root, manifestPath, ir, pkg)
	configRef := compilerConfigRef(packageRoot, target.CompilerConfig, "tsconfig.json")
	configPath := filepath.Join(packageRoot, filepath.FromSlash(configRef.Path))
	if _, err := os.Stat(configPath); err != nil {
		failBuild("TSPACK_COMPILER_CONFIG_MISSING", "tsc compiler config is missing at "+configPath)
	}
	configRef, err := projectTSCConfig(packageRoot, pkg, target, configRef)
	if err != nil {
		failBuild("TSPACK_COMPILER_CONFIG_PROJECTION_FAILED", err.Error())
	}
	compilerPath := target.CompilerPath
	if compilerPath == "" {
		compilerPath = pkg.CompilerPath
	}
	if compilerPath == "" {
		compilerPath = filepath.Join(root, "node_modules", ".bin", "tsc")
		if os.PathSeparator == '\\' {
			compilerPath += ".cmd"
		}
	} else if !filepath.IsAbs(compilerPath) {
		compilerPath = filepath.Join(packageRoot, filepath.FromSlash(compilerPath))
	}
	compilerPath = filepath.Clean(compilerPath)
	if _, err := os.Stat(compilerPath); err != nil {
		failBuild("TSPACK_COMPILER_TOOL_MISSING", "project-managed TypeScript compiler is missing at "+compilerPath)
	}
	version, err := compilerVersionAt(ctx, compilerPath, packageRoot)
	if err != nil {
		failBuild("TSPACK_COMPILER_VERSION_FAILED", err.Error())
	}
	version = strings.TrimSpace(strings.TrimPrefix(version, "Version"))
	entryPath := filepath.Join(packageRoot, filepath.FromSlash(target.Entry))
	if _, err := os.Stat(entryPath); err != nil {
		failBuild("TSPACK_COMPILER_SOURCE_MISSING", err.Error())
	}
	sources, err := collectTSCInputs(packageRoot, pkg, target)
	if err != nil {
		failBuild("TSPACK_COMPILER_SOURCE_MISSING", err.Error())
	}
	adapter := compilerir.TSCAdapter{}
	compilerTarget := compilerir.Target{
		ProjectRoot:  packageRoot,
		Package:      pkg.Name,
		Name:         target.Name,
		Language:     compilerir.LanguageIdentity{ID: target.Language},
		Compiler:     compilerir.CompilerIdentity{ID: "tsc", Version: version},
		Tool:         compilerir.ToolIdentity{Source: "npm", Name: "typescript", Version: version, Path: compilerPath},
		Config:       configRef,
		Runtime:      compilerir.RuntimeIdentity{Family: "javascript", Name: "node"},
		Outputs:      []compilerir.Output{{Kind: compilerir.OutputJavaScript, Path: target.Runtime}},
		Capabilities: adapter.DescribeCapabilities(),
	}
	for _, source := range sources {
		compilerTarget.Inputs = append(compilerTarget.Inputs, compilerir.Input{
			LogicalPath: source.LogicalPath,
			Path:        source.Path,
			Fingerprint: source.Fingerprint,
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
	invocation, err := adapter.PrepareInvocation(compilerTarget, descriptorPath)
	if err != nil {
		failBuild("TSPACK_COMPILER_TARGET_INVALID", err.Error())
	}
	command := exec.CommandContext(ctx, invocation.Executable, invocation.Arguments...)
	command.Dir = packageRoot
	output, runErr := runOwnedBuildCommand(command)
	if len(output) > 0 {
		fmt.Print(string(output))
	}
	if runErr != nil {
		failBuild("TSPACK_COMPILER_BUILD_FAILED", "tsc build failed for "+pkg.Name+":"+target.Name)
	}
	if _, err := os.Stat(filepath.Join(packageRoot, filepath.FromSlash(target.Runtime))); err != nil {
		failBuild("TSPACK_COMPILER_OUTPUT_MISSING", "tsc did not materialize declared output "+target.Runtime)
	}
	fmt.Printf("Built %s:%s with tsc %s -> %s\n", pkg.Name, target.Name, version, target.Runtime)
}

func writeCompilerDescriptor(path string, descriptor compilerir.Descriptor) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	contents, err := json.MarshalIndent(descriptor, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(contents, '\n'), 0o644)
}

func parseBuildCommandOptions(args []string) buildCommandOptions {
	opts := buildCommandOptions{Root: "."}
	for index := 1; index < len(args); index++ {
		switch args[index] {
		case "--root":
			if index+1 >= len(args) {
				failBuild("TSPACK_BUILD_INVALID_ARGS", "--root requires a value")
			}
			index++
			opts.Root = args[index]
		case "--manifest":
			if index+1 >= len(args) {
				failBuild("TSPACK_BUILD_INVALID_ARGS", "--manifest requires a value")
			}
			index++
			opts.Manifest = args[index]
		case "--package":
			if index+1 >= len(args) {
				failBuild("TSPACK_BUILD_INVALID_ARGS", "--package requires a value")
			}
			index++
			opts.PackageName = args[index]
		case "--preserve-last-successful":
			opts.PreserveLastSuccessful = true
		default:
			if strings.HasPrefix(args[index], "-") || opts.TargetName != "" {
				failBuild("TSPACK_BUILD_INVALID_ARGS", "expected at most one target name")
			}
			opts.TargetName = args[index]
		}
	}
	return opts
}

func selectBuildPackages(ir *manifest.ManifestIR, packageName string) []*manifest.Package {
	packages := []*manifest.Package{}
	for index := range ir.Packages {
		pkg := &ir.Packages[index]
		if packageName == "" || packageName == pkg.Name {
			packages = append(packages, pkg)
		}
	}
	return packages
}

func buildTsclPackage(ctx context.Context, root string, manifestPath string, ir *manifest.ManifestIR, pkg *manifest.Package, requestedTarget string, preserveLastSuccessful bool) {
	packageRoot := resolvePackageRoot(root, manifestPath, ir, pkg)
	compilerPath := ""
	if len(pkg.Targets) == 1 {
		compilerPath = pkg.Targets[0].CompilerPath
	}
	if compilerPath == "" {
		compilerPath = pkg.CompilerPath
	}
	if !filepath.IsAbs(compilerPath) {
		compilerPath = filepath.Join(packageRoot, compilerPath)
	}
	compilerPath = filepath.Clean(compilerPath)
	if _, err := os.Stat(compilerPath); err != nil {
		failBuild("TSPACK_TSCL_NOT_FOUND", fmt.Sprintf("tscl executable missing at %s", compilerPath))
	}
	compilerVersion, err := compilerVersionAt(ctx, compilerPath, packageRoot)
	if err != nil {
		failBuild("TSPACK_TSCL_VERSION_FAILED", "failed to query tscl --version: "+err.Error())
	}

	targets := selectBuildTargets(pkg, requestedTarget)
	if len(targets) == 0 {
		failBuild("TSPACK_BUILD_TARGET_NOT_FOUND", "no target matched "+requestedTarget)
	}
	for _, target := range targets {
		if !strings.HasSuffix(target.Entry, ".ts") && !strings.HasSuffix(target.Entry, ".tsx") {
			failBuild("TSPACK_TSCL_INVALID_ENTRY", "tscl target entry must be a Copeland .ts or .tsx source: "+target.Entry)
		}
		configRef := compilerConfigRef(packageRoot, target.CompilerConfig, "tsconfig.tsx")
		if configRef.Fingerprint == "missing" {
			failBuild("TSPACK_COMPILER_CONFIG_MISSING", "Copeland compiler config is missing at "+filepath.Join(packageRoot, filepath.FromSlash(configRef.Path)))
		}
		contracts := collectTsclNpmContracts(root, pkg, target)
		bindings := collectCompilerPackageBindings(root, pkg)
		request, err := newTsclProjectRequest(packageRoot, pkg.Name, compilerPath, target, compilerVersion, contracts, bindings)
		if err != nil {
			failBuild("TSPACK_TSCL_PROJECT_INVALID", err.Error())
		}
		resultPath := filepath.Join(packageRoot, ".tspack", "build-manifests", safeBuildName(pkg.Name)+"-"+safeBuildName(target.Name)+".json")
		requestPath := resultPath + ".request.json"
		if err := os.MkdirAll(filepath.Dir(resultPath), 0o755); err != nil {
			failBuild("TSPACK_BUILD_IO", err.Error())
		}
		if err := writeCompilerDescriptor(requestPath, request.Descriptor); err != nil {
			failBuild("TSPACK_BUILD_IO", err.Error())
		}
		invocation, invocationErr := (compilerir.CopelandAdapter{}).PrepareInvocation(request.Target, requestPath)
		if invocationErr != nil {
			failBuild("TSPACK_COMPILER_TARGET_INVALID", invocationErr.Error())
		}
		cmd := exec.CommandContext(ctx, invocation.Executable, invocation.Arguments...)
		cmd.Dir = packageRoot
		output, runErr := runOwnedBuildCommand(cmd)
		result, readErr := readTsclBuildResult(resultPath)
		if runErr != nil || readErr != nil || !result.Success {
			if !preserveLastSuccessful {
				removeStaleTsclEntry(packageRoot, target.Runtime)
			}
			if readErr == nil {
				printTsclDiagnostics(result)
			}
			if len(output) > 0 {
				fmt.Fprint(os.Stderr, string(output))
			}
			if readErr != nil {
				failBuild("TSPACK_TSCL_RESULT_INVALID", readErr.Error())
			}
			failBuild("TSPACK_TSCL_BUILD_FAILED", "tscl build failed for "+pkg.Name+":"+target.Name)
		}
		if result.EntryOutputPath != request.EntryOutputPath || !resultContainsOutput(result, request.EntryOutputPath) {
			if !preserveLastSuccessful {
				removeStaleTsclEntry(packageRoot, target.Runtime)
			}
			failBuild("TSPACK_TSCL_OUTPUT_MANIFEST_INVALID", "tscl result did not declare the requested entry artifact")
		}
		if _, err := os.Stat(filepath.Join(request.OutputDirectory, request.EntryOutputPath)); err != nil {
			if !preserveLastSuccessful {
				removeStaleTsclEntry(packageRoot, target.Runtime)
			}
			failBuild("TSPACK_TSCL_OUTPUT_MISSING", "tscl result declared an entry artifact that was not materialized")
		}
		if request.ExecutionRuntime == "browser" {
			materialization, materializationErr := materializeBrowserGraph(request.OutputDirectory, contracts)
			if materializationErr != nil {
				if !preserveLastSuccessful {
					removeStaleTsclEntry(packageRoot, target.Runtime)
				}
				failBuild("TSPACK_BROWSER_MATERIALIZATION_FAILED", materializationErr.Error())
			}
			if hostErr := writeBrowserHost(packageRoot, request.OutputDirectory, request.EntryOutputPath, materialization); hostErr != nil {
				if !preserveLastSuccessful {
					removeStaleTsclEntry(packageRoot, target.Runtime)
				}
				failBuild("TSPACK_BROWSER_HOST_WRITE_FAILED", hostErr.Error())
			}
		}
		toolSuffix := ""
		if dotnetSDK := result.ToolVersions["dotnetSdk"]; dotnetSDK != "" {
			toolSuffix = " dotnetSdk=" + dotnetSDK
		}
		fmt.Printf("Built %s:%s language=copeland-ts compiler=tscl@%s backend=%s runtime=%s artifact=%s%s -> %s\n", pkg.Name, target.Name, compilerVersion, result.Backend, result.Runtime, result.ArtifactKind, toolSuffix, result.EntryOutputPath)
	}
}

func selectBuildTargets(pkg *manifest.Package, requestedTarget string) []manifest.Target {
	result := []manifest.Target{}
	for _, target := range pkg.Targets {
		if requestedTarget == "" || target.Name == requestedTarget {
			result = append(result, target)
		}
	}
	return result
}

func newTsclProjectRequest(packageRoot string, packageName string, compilerPath string, target manifest.Target, compilerVersion string, npmContracts []tsclNpmContract, packageBindings []compilerir.PackageBinding) (tsclProjectRequest, error) {
	sources, err := collectTypeScriptSources(packageRoot)
	if err != nil {
		return tsclProjectRequest{}, err
	}
	outputPath := filepath.Clean(target.Runtime)
	if filepath.IsAbs(outputPath) || strings.HasPrefix(outputPath, "..") {
		return tsclProjectRequest{}, fmt.Errorf("target runtime must be a safe relative output path")
	}
	outputDirectory := filepath.Join(packageRoot, filepath.Dir(outputPath))
	entryOutputPath := filepath.Base(outputPath)
	javaScriptRuntime := target.JavaScriptRuntime
	if javaScriptRuntime == "" {
		javaScriptRuntime = "node"
	}
	backend := "javascript"
	executionRuntime := javaScriptRuntime
	outputKind := compilerir.OutputJavaScript
	capabilities := (compilerir.CopelandAdapter{}).DescribeCapabilities()
	switch target.Artifact {
	case "", "javaScript":
	case "managedExecutable":
		backend = "csharp"
		executionRuntime = "ryujit"
		outputKind = compilerir.OutputManagedExecutable
	case "nativeExecutable":
		backend = "csharp"
		executionRuntime = "nativeaot"
		outputKind = compilerir.OutputNativeExecutable
	case "wasmModule":
		backend = "csharp"
		executionRuntime = "wasm"
		outputKind = compilerir.OutputWasmModule
	default:
		return tsclProjectRequest{}, fmt.Errorf("unsupported Copeland artifact %q", target.Artifact)
	}
	targetFramework := target.TargetFramework
	if targetFramework == "" {
		targetFramework = "net10.0"
	}
	configRef := compilerConfigRef(packageRoot, target.CompilerConfig, "tsconfig.tsx")
	fingerprint := tsclBuildFingerprint(compilerVersion, target, sources, npmContracts, configRef.Fingerprint)
	request := tsclProjectRequest{
		ProjectRoot: packageRoot, Sources: sources,
		Entry:             tsclProjectEntry{Module: filepath.ToSlash(target.Entry), Export: "Main"},
		JavaScriptRuntime: javaScriptRuntime, JavaScriptProfile: "production",
		Backend: backend, ExecutionRuntime: executionRuntime,
		TargetFramework: targetFramework, RuntimeIdentifier: target.RuntimeIdentifier,
		TsXmlProfile:    target.TsXmlProfile,
		OutputDirectory: outputDirectory, EntryOutputPath: entryOutputPath,
		BuildFingerprint: fingerprint, NpmContracts: npmContracts,
	}
	payloadBytes, err := json.Marshal(request)
	if err != nil {
		return tsclProjectRequest{}, fmt.Errorf("encode Copeland compiler payload: %w", err)
	}
	compilerTarget := compilerir.Target{
		ProjectRoot:  packageRoot,
		Package:      packageName,
		Name:         target.Name,
		Language:     compilerir.LanguageIdentity{ID: "copeland-ts"},
		Compiler:     compilerir.CompilerIdentity{ID: "tscl", Version: compilerVersion},
		Tool:         compilerir.ToolIdentity{Source: "path", Name: "copeland", Version: compilerVersion, Path: compilerPath},
		Config:       configRef,
		Runtime:      compilerir.RuntimeIdentity{Family: backend, Name: executionRuntime},
		Capabilities: capabilities,
		Outputs:      []compilerir.Output{{Kind: outputKind, Path: target.Runtime}},
		Payload:      compilerir.Payload{Kind: "copeland-v1", SchemaVersion: 1, Data: payloadBytes},
	}
	for _, source := range sources {
		contents, readErr := os.ReadFile(source.Path)
		if readErr != nil {
			return tsclProjectRequest{}, readErr
		}
		sourceHash := sha256.Sum256(contents)
		compilerTarget.Inputs = append(compilerTarget.Inputs, compilerir.Input{
			LogicalPath: source.LogicalPath,
			Path:        source.Path,
			Fingerprint: hex.EncodeToString(sourceHash[:]),
		})
	}
	compilerTarget.Packages = append(compilerTarget.Packages, packageBindings...)
	if len(packageBindings) == 0 {
		for _, contract := range npmContracts {
			compilerTarget.Packages = append(compilerTarget.Packages, compilerir.PackageBinding{
				SemanticIdentity:    "npm:" + contract.PackageName,
				Version:             contract.Version,
				MaterializationPath: contract.MaterializationPath,
				MaterializationName: packageRootName(contract.PackageName),
				LocalName:           contract.PackageName,
				Role:                "runtime",
			})
		}
	}
	descriptor, err := compilerir.NewDescriptor(compilerTarget)
	if err != nil {
		return tsclProjectRequest{}, err
	}
	request.Descriptor = descriptor
	request.Target = compilerTarget
	return request, nil
}

func collectCompilerPackageBindings(root string, pkg *manifest.Package) []compilerir.PackageBinding {
	lock, _, err := lockfile.LoadFile(filepath.Join(root, "ts-lock.toml"))
	if err != nil {
		return []compilerir.PackageBinding{}
	}
	bindings := []compilerir.PackageBinding{}
	for _, dependency := range pkg.Dependencies {
		if isPackageTool(pkg, dependency) || (dependency.Source.Kind != "npm" && dependency.Source.Kind != "jsr") {
			continue
		}
		for _, locked := range lock.Packages {
			if locked.Source != dependency.Source.Kind || locked.Name != dependency.Source.Package {
				continue
			}
			materializationName := compilerMaterializationName(locked.Source, locked.Name)
			for _, edge := range lock.Edges {
				if edge.To == locked.ID && strings.HasPrefix(edge.From, pkg.Name+":") && edge.Reference != "" {
					materializationName = edge.Reference
					break
				}
			}
			bindings = append(bindings, compilerir.PackageBinding{
				SemanticIdentity:    locked.Source + ":" + locked.Name,
				Version:             locked.Version,
				MaterializationPath: filepath.Join(root, "node_modules", filepath.FromSlash(materializationName)),
				MaterializationName: materializationName,
				LocalName:           materializationName,
				Role:                "runtime",
			})
			break
		}
	}
	sort.Slice(bindings, func(i, j int) bool { return bindings[i].SemanticIdentity < bindings[j].SemanticIdentity })
	return bindings
}

func compilerMaterializationName(source string, name string) string {
	if source != "jsr" {
		return name
	}
	parts := strings.Split(strings.TrimPrefix(name, "@"), "/")
	if len(parts) == 2 {
		return "@jsr/" + parts[0] + "__" + parts[1]
	}
	return "@jsr/" + strings.TrimPrefix(name, "@")
}

func compilerConfigRef(packageRoot string, configuredPath string, defaultPath string) compilerir.ConfigRef {
	configPath := configuredPath
	if configPath == "" {
		configPath = defaultPath
	}
	absolutePath := filepath.Join(packageRoot, filepath.FromSlash(configPath))
	fingerprint := "missing"
	if contents, err := os.ReadFile(absolutePath); err == nil {
		hash := sha256.Sum256(contents)
		fingerprint = hex.EncodeToString(hash[:])
	}
	return compilerir.ConfigRef{Kind: "file", Path: configPath, Fingerprint: fingerprint}
}

func collectTypeScriptSources(packageRoot string) ([]tsclProjectSource, error) {
	sourceRoot := filepath.Join(packageRoot, "src")
	sources := []tsclProjectSource{}
	err := filepath.WalkDir(sourceRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".ts" && filepath.Ext(path) != ".tsx" {
			return nil
		}
		relative, err := filepath.Rel(packageRoot, path)
		if err != nil {
			return err
		}
		sources = append(sources, tsclProjectSource{LogicalPath: filepath.ToSlash(relative), Path: path})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read compiler source candidates: %w", err)
	}
	sort.Slice(sources, func(i, j int) bool { return sources[i].LogicalPath < sources[j].LogicalPath })
	return sources, nil
}

func collectTsclNpmContracts(root string, pkg *manifest.Package, target manifest.Target) []tsclNpmContract {
	lock, _, err := lockfile.LoadFile(filepath.Join(root, "ts-lock.toml"))
	if err != nil {
		return []tsclNpmContract{}
	}
	requestedPackages := map[string]struct{}{}
	for _, dependency := range pkg.Dependencies {
		if isPackageTool(pkg, dependency) {
			continue
		}
		if dependency.Source.Kind != "npm" {
			continue
		}
		requestedPackages[dependency.Source.Package] = struct{}{}
	}
	for _, contract := range target.NpmContracts {
		requestedPackages[contract.Package] = struct{}{}
	}

	contracts := []tsclNpmContract{}
	for name := range requestedPackages {
		packageName := packageRootName(name)
		for _, locked := range lock.Packages {
			if locked.Name != packageName {
				continue
			}
			path := filepath.Join(root, "node_modules", filepath.FromSlash(packageName))
			_, materializedErr := os.Stat(path)
			contracts = append(contracts, tsclNpmContract{
				PackageName:         name,
				Version:             locked.Version,
				MaterializationPath: path,
				Materialized:        materializedErr == nil,
				Exports:             contractsForPackage(target.NpmContracts, name),
				Components:          componentsForPackage(target.NpmContracts, name),
			})
			break
		}
	}
	sort.Slice(contracts, func(i, j int) bool { return contracts[i].PackageName < contracts[j].PackageName })
	return contracts
}

func isPackageTool(pkg *manifest.Package, dependency manifest.DependencyIntent) bool {
	for _, tool := range pkg.Tools {
		if tool == dependency.Key || tool == dependency.Source.Package {
			return true
		}
	}
	return false
}

func componentsForPackage(contracts []manifest.CopelandNpmContract, packageName string) []tsclNpmComponent {
	for _, contract := range contracts {
		if contract.Package != packageName {
			continue
		}
		components := make([]tsclNpmComponent, 0, len(contract.Components))
		for _, component := range contract.Components {
			converted := tsclNpmComponent{Name: component.Name, Properties: convertComponentProperties(component.Properties)}
			for _, member := range component.Members {
				converted.Members = append(converted.Members, tsclNpmMember{Name: member.Name, Properties: convertComponentProperties(member.Properties)})
			}
			components = append(components, converted)
		}
		return components
	}
	return nil
}

func convertComponentProperties(properties []manifest.CopelandNpmComponentProperty) []tsclNpmProperty {
	converted := make([]tsclNpmProperty, 0, len(properties))
	for _, property := range properties {
		converted = append(converted, tsclNpmProperty{Name: property.Name, Type: property.Type, Required: property.Required})
	}
	return converted
}

func contractsForPackage(contracts []manifest.CopelandNpmContract, packageName string) []tsclNpmExport {
	for _, contract := range contracts {
		if contract.Package != packageName {
			continue
		}
		exports := make([]tsclNpmExport, 0, len(contract.Exports))
		for _, exported := range contract.Exports {
			exports = append(exports, tsclNpmExport{
				Name:        exported.Name,
				Parameters:  exported.Parameters,
				Result:      exported.Result,
				RemoteError: exported.RemoteError,
				Promise:     exported.Promise,
			})
		}
		return exports
	}
	return nil
}

func compilerVersionAt(ctx context.Context, compilerPath string, directory string) (string, error) {
	cmd := exec.CommandContext(ctx, compilerPath, "--version")
	cmd.Dir = directory
	output, err := runOwnedBuildCommand(cmd)
	if err != nil {
		return "", err
	}
	version := strings.TrimSpace(string(output))
	if version == "" {
		return "", fmt.Errorf("tscl --version returned no version")
	}
	return version, nil
}

func runOwnedBuildCommand(command *exec.Cmd) ([]byte, error) {
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &output
	configureRunTargetProcess(command)
	if err := command.Start(); err != nil {
		return output.Bytes(), err
	}
	cleanup, err := attachRunTargetCleanup(command)
	if err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		return output.Bytes(), err
	}
	runErr := command.Wait()
	cleanupErr := cleanupExitedRunTargetProcessTree(command.Process.Pid, cleanup)
	if runErr == nil && cleanupErr != nil {
		runErr = cleanupErr
	}
	return output.Bytes(), runErr
}

func tsclBuildFingerprint(version string, target manifest.Target, sources []tsclProjectSource, contracts []tsclNpmContract, configFingerprint string) string {
	hash := sha256.New()
	runtime := target.JavaScriptRuntime
	if runtime == "" {
		runtime = "node"
	}
	_, _ = hash.Write([]byte("compiler=tscl\nversion=" + version + "\nconfig=" + configFingerprint + "\nruntime=" + runtime + "\nartifact=" + target.Artifact + "\ntargetFramework=" + target.TargetFramework + "\nrid=" + target.RuntimeIdentifier + "\nprofile=production\ntsxml=" + target.TsXmlProfile + "\nentry=" + target.Entry + "\noutput=" + target.Runtime + "\n"))
	for _, source := range sources {
		contents, _ := os.ReadFile(source.Path)
		_, _ = hash.Write([]byte(source.LogicalPath + "\n"))
		_, _ = hash.Write(contents)
	}
	for _, contract := range contracts {
		_, _ = hash.Write([]byte(contract.PackageName + "@" + contract.Version + "\n"))
	}
	if runtime == "browser" {
		_, _ = hash.Write([]byte("browser-materializer=" + browserTransformerIdentity() + "\n"))
		for _, contract := range contracts {
			entry, err := selectBrowserPackageEntry(contract.MaterializationPath, packageSubpath(contract.PackageName))
			if err != nil {
				_, _ = hash.Write([]byte("browser-package=" + contract.PackageName + ":unavailable\n"))
				continue
			}
			mode := "native-esm"
			if isCommonJSModule(contract.MaterializationPath, entry) {
				mode = "transformed-esm"
			}
			contents, _ := os.ReadFile(filepath.Join(contract.MaterializationPath, filepath.FromSlash(entry)))
			_, _ = hash.Write([]byte("browser-package=" + contract.PackageName + ":" + mode + ":" + entry + "\n"))
			_, _ = hash.Write(contents)
		}
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func readTsclBuildResult(path string) (tsclBuildResult, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return tsclBuildResult{}, err
	}
	var result tsclBuildResult
	if err := json.Unmarshal(contents, &result); err != nil {
		return tsclBuildResult{}, err
	}
	return result, nil
}

func resultContainsOutput(result tsclBuildResult, expectedPath string) bool {
	for _, output := range result.Outputs {
		if output.Path == expectedPath && output.SHA256 != "" {
			return true
		}
	}
	return false
}

func printTsclDiagnostics(result tsclBuildResult) {
	for _, diagnostic := range result.Diagnostics {
		location := diagnostic.File
		if diagnostic.Line > 0 {
			location += fmt.Sprintf(":%d:%d", diagnostic.Line, diagnostic.Column)
		}
		if location != "" {
			location += ": "
		}
		fmt.Fprintf(os.Stderr, "%s%s %s: %s\n", location, diagnostic.Code, diagnostic.Severity, diagnostic.Message)
	}
}

func removeStaleTsclEntry(packageRoot string, runtimePath string) {
	entryPath := filepath.Join(packageRoot, runtimePath)
	_ = os.Remove(entryPath)
}

func safeBuildName(value string) string {
	return strings.NewReplacer("/", "_", "\\", "_", "@", "").Replace(value)
}

func failBuild(code string, message string) {
	fmt.Fprintf(os.Stderr, "%s: %s\n", code, message)
	exit(1)
}

// cliBuildTargetExecutor is the temporary compiler adapter used by both the
// build command and workflow application seams while compiler implementations
// are moved out of the CLI package. It invokes compiler functions directly and
// never shells back into tspack.
type cliBuildTargetExecutor struct {
	Output io.Writer
}

func (executor cliBuildTargetExecutor) BuildTarget(ctx context.Context, request project.BuildTargetRequest) (result project.BuildTargetResult) {
	result.Package = request.Package.Name
	result.Target = request.Target.Name
	result.Compiler = request.Target.Compiler
	if err := ctx.Err(); err != nil {
		result.Diagnostics = append(result.Diagnostics, diag.Diagnostic{Code: "TSPACK_BUILD_CANCELLED", Severity: diag.SeverityError, Message: "build was cancelled", Details: []string{err.Error()}})
		return result
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			if _, ok := recovered.(exitStatus); !ok {
				panic(recovered)
			}
			result.Diagnostics = append(result.Diagnostics, diag.Diagnostic{Code: "TSPACK_BUILD_TARGET_FAILED", Severity: diag.SeverityError, Message: "build target failed", Details: []string{request.Package.Name + ":" + request.Target.Name}})
			result.Succeeded = false
		}
	}()
	root := request.Project.RootDir
	manifestPath := request.Project.ManifestPath
	if manifestPath == "" {
		manifestPath = filepath.Join(root, "manifest.tsx")
	}
	packageRoot := resolvePackageRoot(root, manifestPath, request.Manifest, request.Package)
	if err := validateCompilerSourceOwnership(packageRoot, request.Package); err != nil {
		result.Diagnostics = append(result.Diagnostics, diag.Diagnostic{Code: "TSPACK_COMPILER_SOURCE_OWNERSHIP_INVALID", Severity: diag.SeverityError, Message: err.Error()})
		return result
	}
	switch request.Target.Compiler {
	case "tsc":
		buildTscTarget(ctx, root, manifestPath, request.Manifest, request.Package, request.Target)
	case "tscl":
		buildTsclPackage(ctx, root, manifestPath, request.Manifest, request.Package, request.Target.Name, request.PreserveLastSuccessful)
	case "scriptc":
		buildScriptCTarget(ctx, root, manifestPath, request.Manifest, request.Package, request.Target)
	case "perry":
		buildPerryTarget(ctx, root, manifestPath, request.Manifest, request.Package, request.Target)
	case "rollup":
		artifacts, diagnostics := buildRollupTarget(ctx, root, manifestPath, request.Manifest, request.Package, request.Target, executor.output())
		result.Artifacts = append(result.Artifacts, artifacts...)
		result.Diagnostics = append(result.Diagnostics, diagnostics...)
		if hasDiagnosticErrors(diagnostics) {
			return result
		}
		projectionDiagnostics := projectWorkspaceBuildArtifacts(root, packageRoot, request.Package.Name, request.Target, result.Artifacts)
		result.Diagnostics = append(result.Diagnostics, projectionDiagnostics...)
		if hasDiagnosticErrors(projectionDiagnostics) {
			return result
		}
		result.Succeeded = true
		return result
	case "vite":
		artifacts, diagnostics := buildViteTarget(ctx, root, manifestPath, request.Manifest, request.Package, request.Target, executor.output())
		result.Artifacts = append(result.Artifacts, artifacts...)
		result.Diagnostics = append(result.Diagnostics, diagnostics...)
		if hasDiagnosticErrors(diagnostics) {
			return result
		}
		projectionDiagnostics := projectWorkspaceBuildArtifacts(root, packageRoot, request.Package.Name, request.Target, result.Artifacts)
		result.Diagnostics = append(result.Diagnostics, projectionDiagnostics...)
		if hasDiagnosticErrors(projectionDiagnostics) {
			return result
		}
		result.Succeeded = true
		return result
	default:
		result.Diagnostics = append(result.Diagnostics, diag.Diagnostic{Code: "TSPACK_BUILD_UNSUPPORTED_COMPILER", Severity: diag.SeverityError, Message: "unsupported compiler for target " + request.Package.Name + ":" + request.Target.Name + ": " + request.Target.Compiler})
		return result
	}
	artifactKind := request.Target.Artifact
	if artifactKind == "" {
		artifactKind = "javaScript"
	}
	result.Artifacts = append(result.Artifacts, project.BuildArtifact{Package: request.Package.Name, Target: request.Target.Name, Kind: artifactKind, Path: filepath.Join(packageRoot, filepath.FromSlash(request.Target.Runtime))})
	projectionDiagnostics := projectWorkspaceBuildArtifacts(root, packageRoot, request.Package.Name, request.Target, result.Artifacts)
	result.Diagnostics = append(result.Diagnostics, projectionDiagnostics...)
	if hasDiagnosticErrors(projectionDiagnostics) {
		return result
	}
	result.Succeeded = true
	return result
}

func projectWorkspaceBuildArtifacts(root string, packageRoot string, packageName string, target manifest.Target, artifacts []project.BuildArtifact) []diag.Diagnostic {
	declaredPatterns := []string{}
	for _, artifact := range target.Artifacts {
		if artifact.Path != "" {
			declaredPatterns = append(declaredPatterns, artifact.Path)
		}
	}
	if len(declaredPatterns) == 0 {
		declaredPatterns = append(declaredPatterns, target.Runtime, target.Types)
	}
	artifactPaths := make([]string, 0, len(artifacts))
	for _, artifact := range artifacts {
		artifactPaths = append(artifactPaths, artifact.Path)
	}
	return materialize.ProjectWorkspaceBuildArtifacts(root, packageRoot, packageName, declaredPatterns, artifactPaths)
}

func (executor cliBuildTargetExecutor) output() io.Writer {
	if executor.Output != nil {
		return executor.Output
	}
	return os.Stdout
}

func buildRollupTarget(ctx context.Context, root string, manifestPath string, ir *manifest.ManifestIR, pkg *manifest.Package, target manifest.Target, outputWriter io.Writer) ([]project.BuildArtifact, []diag.Diagnostic) {
	return buildConfiguredBundlerTarget(ctx, root, manifestPath, ir, pkg, target, outputWriter, configuredBundler{
		name:          "Rollup",
		defaultConfig: "rollup.config.js",
		executable:    "rollup",
		arguments: func(absoluteConfigPath string, _ string) []string {
			return []string{"-c", absoluteConfigPath}
		},
	})
}

func buildViteTarget(ctx context.Context, root string, manifestPath string, ir *manifest.ManifestIR, pkg *manifest.Package, target manifest.Target, outputWriter io.Writer) ([]project.BuildArtifact, []diag.Diagnostic) {
	return buildConfiguredBundlerTarget(ctx, root, manifestPath, ir, pkg, target, outputWriter, configuredBundler{
		name:          "Vite",
		defaultConfig: "vite.config.ts",
		executable:    "vite",
		arguments:     viteBuildArguments,
	})
}

func viteBuildArguments(absoluteConfigPath string, entry string) []string {
	root := filepath.ToSlash(filepath.Dir(filepath.FromSlash(entry)))
	if root == "." {
		return []string{"build", "--config", absoluteConfigPath}
	}
	return []string{"build", root, "--config", absoluteConfigPath}
}

type configuredBundler struct {
	name          string
	defaultConfig string
	executable    string
	arguments     func(absoluteConfigPath string, entry string) []string
}

func buildConfiguredBundlerTarget(ctx context.Context, root string, manifestPath string, ir *manifest.ManifestIR, pkg *manifest.Package, target manifest.Target, outputWriter io.Writer, bundler configuredBundler) ([]project.BuildArtifact, []diag.Diagnostic) {
	packageRoot := resolvePackageRoot(root, manifestPath, ir, pkg)
	configPath := target.CompilerConfig
	if configPath == "" {
		configPath = bundler.defaultConfig
	}
	absoluteConfigPath := filepath.Join(packageRoot, filepath.FromSlash(configPath))
	if _, err := os.Stat(absoluteConfigPath); err != nil {
		return nil, []diag.Diagnostic{{Code: "TSPACK_COMPILER_CONFIG_MISSING", Severity: diag.SeverityError, Message: bundler.name + " compiler config is missing", Details: []string{absoluteConfigPath}}}
	}
	entryPath := filepath.Join(packageRoot, filepath.FromSlash(target.Entry))
	if _, err := os.Stat(entryPath); err != nil {
		return nil, []diag.Diagnostic{{Code: "TSPACK_COMPILER_SOURCE_MISSING", Severity: diag.SeverityError, Message: bundler.name + " entry source is missing", Details: []string{entryPath}}}
	}
	compilerPath := target.CompilerPath
	if compilerPath == "" {
		compilerPath = filepath.Join(root, "node_modules", ".bin", bundler.executable)
		if os.PathSeparator == '\\' {
			compilerPath += ".cmd"
		}
	} else if !filepath.IsAbs(compilerPath) {
		compilerPath = filepath.Join(packageRoot, filepath.FromSlash(compilerPath))
	}
	if _, err := os.Stat(compilerPath); err != nil {
		return nil, []diag.Diagnostic{{Code: "TSPACK_COMPILER_TOOL_MISSING", Severity: diag.SeverityError, Message: "project-managed " + bundler.name + " compiler is missing", Details: []string{compilerPath}}}
	}
	declaredArtifacts := target.Artifacts
	if len(declaredArtifacts) == 0 {
		declaredArtifacts = []manifest.TargetArtifact{
			{Name: "javaScript", Kind: "javaScript", Path: target.Runtime, Role: "runtimeEntry"},
			{Name: "typeDeclarations", Kind: "typeDeclarations", Path: target.Types, Role: "typeDeclaration"},
		}
	}
	if cleanupDiagnostics := cleanDeclaredBundlerArtifacts(packageRoot, declaredArtifacts, target.Inputs, bundler.name); len(cleanupDiagnostics) > 0 {
		return nil, cleanupDiagnostics
	}
	command := exec.CommandContext(ctx, compilerPath, bundler.arguments(absoluteConfigPath, target.Entry)...)
	command.Dir = packageRoot
	output, err := runOwnedBuildCommand(command)
	if len(output) > 0 {
		_, _ = fmt.Fprint(outputWriter, string(output))
	}
	if err != nil {
		return nil, []diag.Diagnostic{{Code: "TSPACK_COMPILER_BUILD_FAILED", Severity: diag.SeverityError, Message: bundler.name + " build failed for " + pkg.Name + ":" + target.Name, Details: []string{err.Error()}}}
	}
	artifacts := []project.BuildArtifact{}
	for _, declared := range declaredArtifacts {
		if declared.Path == "" {
			continue
		}
		artifactPattern := filepath.Join(packageRoot, filepath.FromSlash(declared.Path))
		artifactPaths := []string{artifactPattern}
		if strings.ContainsAny(declared.Path, "*?[") {
			matches, globErr := filepath.Glob(artifactPattern)
			if globErr != nil {
				return nil, []diag.Diagnostic{{Code: "TSPACK_COMPILER_OUTPUT_INVALID", Severity: diag.SeverityError, Message: "declared " + bundler.name + " artifact glob is invalid", Details: []string{declared.Path, globErr.Error()}}}
			}
			artifactPaths = matches
		}
		if len(artifactPaths) == 0 {
			return nil, []diag.Diagnostic{{Code: "TSPACK_COMPILER_OUTPUT_MISSING", Severity: diag.SeverityError, Message: bundler.name + " did not materialize a declared artifact", Details: []string{declared.Path}}}
		}
		sort.Strings(artifactPaths)
		for _, artifactPath := range artifactPaths {
			contents, readErr := os.ReadFile(artifactPath)
			if readErr != nil {
				return nil, []diag.Diagnostic{{Code: "TSPACK_COMPILER_OUTPUT_MISSING", Severity: diag.SeverityError, Message: bundler.name + " did not materialize a declared artifact", Details: []string{declared.Path, readErr.Error()}}}
			}
			hash := sha256.Sum256(contents)
			identity := pkg.Name + ":" + target.Name + ":" + declared.Name
			if len(artifactPaths) > 1 {
				relativePath, relativeErr := filepath.Rel(packageRoot, artifactPath)
				if relativeErr != nil {
					return nil, []diag.Diagnostic{{Code: "TSPACK_COMPILER_OUTPUT_INVALID", Severity: diag.SeverityError, Message: "could not identify " + bundler.name + " artifact", Details: []string{artifactPath, relativeErr.Error()}}}
				}
				identity += ":" + filepath.ToSlash(relativePath)
			}
			artifacts = append(artifacts, project.BuildArtifact{
				Package:     pkg.Name,
				Target:      target.Name,
				Kind:        declared.Kind,
				Role:        declared.Role,
				Path:        artifactPath,
				Identity:    identity,
				ContentHash: hex.EncodeToString(hash[:]),
			})
		}
	}
	sort.Slice(artifacts, func(i, j int) bool {
		return artifacts[i].Identity < artifacts[j].Identity
	})
	_, _ = fmt.Fprintf(outputWriter, "Built %s:%s with %s -> %s\n", pkg.Name, target.Name, bundler.name, target.Runtime)
	return artifacts, nil
}

func cleanDeclaredBundlerArtifacts(packageRoot string, artifacts []manifest.TargetArtifact, inputs []string, compilerName string) []diag.Diagnostic {
	preservedInputs := map[string]bool{}
	for _, input := range inputs {
		if !strings.ContainsAny(input, "*?[") {
			preservedInputs[filepath.Clean(filepath.FromSlash(input))] = true
		}
	}
	for _, artifact := range artifacts {
		if artifact.Path == "" {
			continue
		}
		if preservedInputs[filepath.Clean(filepath.FromSlash(artifact.Path))] {
			continue
		}
		pattern := filepath.Join(packageRoot, filepath.FromSlash(artifact.Path))
		matches := []string{pattern}
		if strings.ContainsAny(artifact.Path, "*?[") {
			var err error
			matches, err = filepath.Glob(pattern)
			if err != nil {
				return []diag.Diagnostic{{Code: "TSPACK_COMPILER_OUTPUT_INVALID", Severity: diag.SeverityError, Message: "declared " + compilerName + " artifact glob is invalid", Details: []string{artifact.Path, err.Error()}}}
			}
		}
		for _, match := range matches {
			info, err := os.Lstat(match)
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return []diag.Diagnostic{{Code: "TSPACK_COMPILER_OUTPUT_CLEAN_FAILED", Severity: diag.SeverityError, Message: "could not inspect a declared " + compilerName + " artifact before build", Details: []string{match, err.Error()}}}
			}
			if info.IsDir() {
				return []diag.Diagnostic{{Code: "TSPACK_COMPILER_OUTPUT_CLEAN_FAILED", Severity: diag.SeverityError, Message: "declared " + compilerName + " artifacts must identify files", Details: []string{match}}}
			}
			if err := os.Remove(match); err != nil {
				return []diag.Diagnostic{{Code: "TSPACK_COMPILER_OUTPUT_CLEAN_FAILED", Severity: diag.SeverityError, Message: "could not remove a stale declared " + compilerName + " artifact", Details: []string{match, err.Error()}}}
			}
		}
	}
	return nil
}

func cleanDeclaredRollupArtifacts(packageRoot string, artifacts []manifest.TargetArtifact, inputs []string) []diag.Diagnostic {
	return cleanDeclaredBundlerArtifacts(packageRoot, artifacts, inputs, "Rollup")
}
