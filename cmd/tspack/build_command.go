package main

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

	"github.com/yuechen-li-dev/tspack/internal/lockfile"
	"github.com/yuechen-li-dev/tspack/internal/manifest"
)

// build is intentionally a narrow compiler-selection seam. Runtime process
// launching remains in run_command.go; this command only creates artifacts.
type buildCommandOptions struct {
	Root        string
	Manifest    string
	PackageName string
	TargetName  string
}

type tsclProjectRequest struct {
	ProjectRoot       string              `json:"projectRoot"`
	Sources           []tsclProjectSource `json:"sources"`
	Entry             tsclProjectEntry    `json:"entry"`
	JavaScriptRuntime string              `json:"javascriptRuntime"`
	JavaScriptProfile string              `json:"javascriptProfile"`
	TsXmlProfile      string              `json:"tsXmlProfile,omitempty"`
	OutputDirectory   string              `json:"outputDirectory"`
	EntryOutputPath   string              `json:"entryOutputPath"`
	BuildFingerprint  string              `json:"buildFingerprint"`
	NpmContracts      []tsclNpmContract   `json:"npmContracts"`
}

type tsclProjectSource struct {
	LogicalPath string `json:"logicalPath"`
	Path        string `json:"path"`
}

type tsclProjectEntry struct {
	Module string `json:"module"`
	Export string `json:"export"`
}

type tsclNpmContract struct {
	PackageName         string             `json:"packageName"`
	Version             string             `json:"version"`
	MaterializationPath string             `json:"materializationPath"`
	Materialized        bool               `json:"materialized"`
	Exports             []tsclNpmExport    `json:"exports,omitempty"`
	Components          []tsclNpmComponent `json:"components,omitempty"`
}

type tsclNpmExport struct {
	Name        string   `json:"name"`
	Parameters  []string `json:"parameters"`
	Result      string   `json:"result"`
	RemoteError string   `json:"remoteError,omitempty"`
	Promise     bool     `json:"promise,omitempty"`
}

type tsclNpmComponent struct {
	Name       string            `json:"name"`
	Properties []tsclNpmProperty `json:"properties,omitempty"`
	Members    []tsclNpmMember   `json:"members,omitempty"`
}

type tsclNpmMember struct {
	Name       string            `json:"name"`
	Properties []tsclNpmProperty `json:"properties,omitempty"`
}

type tsclNpmProperty struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Required bool   `json:"required,omitempty"`
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
	EntryOutputPath  string `json:"entryOutputPath"`
	BuildFingerprint string `json:"buildFingerprint"`
}

func runBuildCommand(args []string) {
	opts := parseBuildCommandOptions(args)
	root := resolveWorkspaceRoot(opts.Root)
	manifestPath := opts.Manifest
	if manifestPath == "" {
		manifestPath = filepath.Join(root, "manifest.tsx")
	}
	ir := loadManifestPathForRun(root, manifestPath)
	packages := selectBuildPackages(ir, opts.PackageName)
	if len(packages) == 0 {
		failBuild("TSPACK_BUILD_PACKAGE_NOT_FOUND", "no package matched the requested build selection")
	}
	for _, pkg := range packages {
		if pkg.Compiler == "tsc" {
			failBuild("TSPACK_BUILD_TSC_UNCHANGED", "tspack build currently owns the tscl integration path only; existing tsc projects keep their declared RunTarget commands")
		}
		if pkg.Compiler != "tscl" {
			failBuild("TSPACK_BUILD_UNSUPPORTED_COMPILER", "unsupported compiler for package "+pkg.Name+": "+pkg.Compiler)
		}
		buildTsclPackage(root, manifestPath, ir, pkg, opts.TargetName)
	}
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

func buildTsclPackage(root string, manifestPath string, ir *manifest.ManifestIR, pkg *manifest.Package, requestedTarget string) {
	packageRoot := resolvePackageRoot(root, manifestPath, ir, pkg)
	compilerPath := pkg.CompilerPath
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
		contracts := collectTsclNpmContracts(root, pkg, target)
		request, err := newTsclProjectRequest(packageRoot, target, compilerVersion, contracts)
		if err != nil {
			failBuild("TSPACK_TSCL_PROJECT_INVALID", err.Error())
		}
		resultPath := filepath.Join(packageRoot, ".tspack", "build-manifests", safeBuildName(pkg.Name)+"-"+safeBuildName(target.Name)+".json")
		requestPath := resultPath + ".request.json"
		if err := os.MkdirAll(filepath.Dir(resultPath), 0o755); err != nil {
			failBuild("TSPACK_BUILD_IO", err.Error())
		}
		requestBytes, _ := json.MarshalIndent(request, "", "  ")
		if err := os.WriteFile(requestPath, append(requestBytes, '\n'), 0o644); err != nil {
			failBuild("TSPACK_BUILD_IO", err.Error())
		}
		cmd := exec.Command(compilerPath, "build", "--project", requestPath, "--result", resultPath)
		cmd.Dir = packageRoot
		output, runErr := cmd.CombinedOutput()
		result, readErr := readTsclBuildResult(resultPath)
		if runErr != nil || readErr != nil || !result.Success {
			removeStaleTsclEntry(packageRoot, target.Runtime)
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
			removeStaleTsclEntry(packageRoot, target.Runtime)
			failBuild("TSPACK_TSCL_OUTPUT_MANIFEST_INVALID", "tscl result did not declare the requested entry artifact")
		}
		if _, err := os.Stat(filepath.Join(request.OutputDirectory, request.EntryOutputPath)); err != nil {
			removeStaleTsclEntry(packageRoot, target.Runtime)
			failBuild("TSPACK_TSCL_OUTPUT_MISSING", "tscl result declared an entry artifact that was not materialized")
		}
		if request.JavaScriptRuntime == "browser" {
			materialization, materializationErr := materializeBrowserGraph(request.OutputDirectory, contracts)
			if materializationErr != nil {
				removeStaleTsclEntry(packageRoot, target.Runtime)
				failBuild("TSPACK_BROWSER_MATERIALIZATION_FAILED", materializationErr.Error())
			}
			if hostErr := writeBrowserHost(packageRoot, request.OutputDirectory, request.EntryOutputPath, materialization); hostErr != nil {
				removeStaleTsclEntry(packageRoot, target.Runtime)
				failBuild("TSPACK_BROWSER_HOST_WRITE_FAILED", hostErr.Error())
			}
		}
		fmt.Printf("Built %s:%s with tscl %s -> %s\n", pkg.Name, target.Name, compilerVersion, result.EntryOutputPath)
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

func newTsclProjectRequest(packageRoot string, target manifest.Target, compilerVersion string, npmContracts []tsclNpmContract) (tsclProjectRequest, error) {
	sources, err := collectCopelandSources(packageRoot)
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
	fingerprint := tsclBuildFingerprint(compilerVersion, target, sources, npmContracts)
	return tsclProjectRequest{
		ProjectRoot: packageRoot, Sources: sources,
		Entry:             tsclProjectEntry{Module: filepath.ToSlash(target.Entry), Export: "Main"},
		JavaScriptRuntime: javaScriptRuntime, JavaScriptProfile: "production",
		TsXmlProfile:    target.TsXmlProfile,
		OutputDirectory: outputDirectory, EntryOutputPath: entryOutputPath,
		BuildFingerprint: fingerprint, NpmContracts: npmContracts,
	}, nil
}

func collectCopelandSources(packageRoot string) ([]tsclProjectSource, error) {
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
		return nil, fmt.Errorf("read Copeland source roots: %w", err)
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

func tsclBuildFingerprint(version string, target manifest.Target, sources []tsclProjectSource, contracts []tsclNpmContract) string {
	hash := sha256.New()
	runtime := target.JavaScriptRuntime
	if runtime == "" {
		runtime = "node"
	}
	_, _ = hash.Write([]byte("compiler=tscl\nversion=" + version + "\nruntime=" + runtime + "\nprofile=production\ntsxml=" + target.TsXmlProfile + "\nentry=" + target.Entry + "\noutput=" + target.Runtime + "\n"))
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
	os.Exit(1)
}
