package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	compilerir "github.com/yuechen-li-dev/tspack/internal/compiler"
	"github.com/yuechen-li-dev/tspack/internal/lockfile"
	"github.com/yuechen-li-dev/tspack/internal/manifest"
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
	ir := workspace.LoadManifest(manifestPath)
	packages := selectBuildPackages(ir, opts.PackageName)
	if len(packages) == 0 {
		failBuild("TSPACK_BUILD_PACKAGE_NOT_FOUND", "no package matched the requested build selection")
	}
	for _, pkg := range packages {
		packageRoot := resolvePackageRoot(root, manifestPath, ir, pkg)
		if err := validateCompilerSourceOwnership(packageRoot, pkg); err != nil {
			failBuild("TSPACK_COMPILER_SOURCE_OWNERSHIP_INVALID", err.Error())
		}
		targets, err := orderBuildTargets(pkg, opts.TargetName)
		if err != nil || len(targets) == 0 {
			message := "no target matched " + opts.TargetName
			if err != nil {
				message = err.Error()
			}
			failBuild("TSPACK_BUILD_TARGET_NOT_FOUND", message)
		}
		for _, target := range targets {
			switch target.Compiler {
			case "tsc":
				buildTscTarget(root, manifestPath, ir, pkg, target)
			case "tscl":
				selectedPackage := *pkg
				selectedPackage.Targets = []manifest.Target{target}
				buildTsclPackage(root, manifestPath, ir, &selectedPackage, "", opts.PreserveLastSuccessful)
			case "scriptc":
				buildScriptCTarget(root, manifestPath, ir, pkg, target)
			case "perry":
				buildPerryTarget(root, manifestPath, ir, pkg, target)
			default:
				failBuild("TSPACK_BUILD_UNSUPPORTED_COMPILER", "unsupported compiler for target "+pkg.Name+":"+target.Name+": "+target.Compiler)
			}
		}
	}
}

func buildTscTarget(root string, manifestPath string, ir *manifest.ManifestIR, pkg *manifest.Package, target manifest.Target) {
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
	version, err := compilerVersionAt(compilerPath, packageRoot)
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
	command := exec.Command(invocation.Executable, invocation.Arguments...)
	command.Dir = packageRoot
	output, runErr := command.CombinedOutput()
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

func buildTsclPackage(root string, manifestPath string, ir *manifest.ManifestIR, pkg *manifest.Package, requestedTarget string, preserveLastSuccessful bool) {
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
	compilerVersion, err := compilerVersionAt(compilerPath, packageRoot)
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
		cmd := exec.Command(invocation.Executable, invocation.Arguments...)
		cmd.Dir = packageRoot
		output, runErr := cmd.CombinedOutput()
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

func compilerVersionAt(compilerPath string, directory string) (string, error) {
	cmd := exec.Command(compilerPath, "--version")
	cmd.Dir = directory
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	version := strings.TrimSpace(string(output))
	if version == "" {
		return "", fmt.Errorf("tscl --version returned no version")
	}
	return version, nil
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
