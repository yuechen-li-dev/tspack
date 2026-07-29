package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strings"
)

type browserMaterialization struct {
	SchemaVersion int                      `json:"schemaVersion"`
	Imports       []browserImport          `json:"imports"`
	Packages      []browserPackageArtifact `json:"packages"`
}

type browserImport struct {
	Specifier string `json:"specifier"`
	URL       string `json:"url"`
}

type browserPackageArtifact struct {
	Specifier   string `json:"specifier"`
	PackageName string `json:"packageName"`
	Version     string `json:"version"`
	Mode        string `json:"mode"`
	SourceEntry string `json:"sourceEntry"`
	Output      string `json:"output"`
	SHA256      string `json:"sha256"`
	Tool        string `json:"tool,omitempty"`
}

type browserPackageJSON struct {
	Browser json.RawMessage `json:"browser"`
	Module  string          `json:"module"`
	Main    string          `json:"main"`
	Exports json.RawMessage `json:"exports"`
}

func materializeBrowserGraph(outputDirectory string, contracts []tsclNpmContract) (browserMaterialization, error) {
	result := browserMaterialization{SchemaVersion: 1, Imports: []browserImport{}, Packages: []browserPackageArtifact{}}
	for _, contract := range contracts {
		if !contract.Materialized {
			return browserMaterialization{}, fmt.Errorf("browser package %q was resolved but is not materialized", contract.PackageName)
		}

		sourceDirectory := contract.MaterializationPath
		entry, err := selectBrowserPackageEntry(sourceDirectory, packageSubpath(contract.PackageName))
		if err != nil {
			return browserMaterialization{}, fmt.Errorf("select browser entry for %q: %w", contract.PackageName, err)
		}

		packageDirectoryName := packageOutputDirectory(contract.PackageName)
		packageDirectory := filepath.Join(outputDirectory, "packages", packageDirectoryName)
		mode := "native-esm"
		output := filepath.ToSlash(filepath.Join("packages", packageDirectoryName, entry))
		tool := ""
		if isCommonJSModule(sourceDirectory, entry) || shouldBundleBrowserPackage(contract.PackageName) {
			mode = "transformed-esm"
			output = filepath.ToSlash(filepath.Join("packages", packageDirectoryName, "entry.mjs"))
			tool, err = transformBrowserPackage(sourceDirectory, entry, filepath.Join(outputDirectory, filepath.FromSlash(output)), contract.PackageName)
			if err != nil {
				return browserMaterialization{}, fmt.Errorf("transform browser package %q: %w", contract.PackageName, err)
			}
		} else if err := copyBrowserPackage(sourceDirectory, packageDirectory); err != nil {
			return browserMaterialization{}, fmt.Errorf("materialize browser package %q: %w", contract.PackageName, err)
		}

		outputPath := filepath.Join(outputDirectory, filepath.FromSlash(output))
		hash, err := browserArtifactHash(outputPath)
		if err != nil {
			return browserMaterialization{}, fmt.Errorf("hash browser package %q: %w", contract.PackageName, err)
		}
		entryPath := "./" + output
		result.Imports = append(result.Imports, browserImport{
			Specifier: contract.PackageName,
			URL:       entryPath,
		})
		result.Packages = append(result.Packages, browserPackageArtifact{
			Specifier:   contract.PackageName,
			PackageName: packageRootName(contract.PackageName),
			Version:     contract.Version,
			Mode:        mode,
			SourceEntry: entry,
			Output:      output,
			SHA256:      hash,
			Tool:        tool,
		})
	}

	hostDirectory := filepath.Join(outputDirectory, "packages", "copeland-browser-v1")
	if err := os.MkdirAll(hostDirectory, 0o755); err != nil {
		return browserMaterialization{}, err
	}
	if err := os.WriteFile(filepath.Join(hostDirectory, "index.js"), []byte(browserHostModule), 0o644); err != nil {
		return browserMaterialization{}, err
	}
	result.Imports = append(result.Imports, browserImport{
		Specifier: "@copeland/browser-v1",
		URL:       "./packages/copeland-browser-v1/index.js",
	})
	result.Packages = append(result.Packages, browserPackageArtifact{
		Specifier:   "@copeland/browser-v1",
		PackageName: "@copeland/browser-v1",
		Version:     "1",
		Mode:        "generated-host",
		SourceEntry: "index.js",
		Output:      "packages/copeland-browser-v1/index.js",
		SHA256:      mustBrowserArtifactHash(filepath.Join(hostDirectory, "index.js")),
	})

	sort.Slice(result.Imports, func(left int, right int) bool {
		return result.Imports[left].Specifier < result.Imports[right].Specifier
	})
	sort.Slice(result.Packages, func(left int, right int) bool {
		return result.Packages[left].Specifier < result.Packages[right].Specifier
	})
	return result, nil
}

func shouldBundleBrowserPackage(specifier string) bool {
	return packageRootName(specifier) == "@base-ui-components/react"
}

func selectBrowserPackageEntry(directory string, subpath string) (string, error) {
	bytes, err := os.ReadFile(filepath.Join(directory, "package.json"))
	if err != nil {
		return "", err
	}

	var packageJSON browserPackageJSON
	if err := json.Unmarshal(bytes, &packageJSON); err != nil {
		return "", err
	}
	if len(packageJSON.Exports) > 0 {
		entry, found, err := selectBrowserExport(packageJSON.Exports, exportKey(subpath))
		if err != nil {
			return "", err
		}
		if found {
			return validateBrowserEntry(entry)
		}
	}

	browser := browserString(packageJSON.Browser)
	for _, candidate := range []string{browser, packageJSON.Module, packageJSON.Main} {
		if candidate != "" {
			return validateBrowserEntry(candidate)
		}
	}
	return "", fmt.Errorf("package has no browser, import, default, module, or main entry")
}

func browserString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		// A browser replacement map needs module-level specifier rewriting. M1
		// selects the exports map first and deliberately does not pretend that a
		// replacement object is a root entrypoint.
		return ""
	}
	return value
}

func selectBrowserExport(raw json.RawMessage, key string) (string, bool, error) {
	var direct string
	if err := json.Unmarshal(raw, &direct); err == nil {
		return direct, true, nil
	}

	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return "", false, fmt.Errorf("exports must be a string or object")
	}
	if selected, ok := object[key]; ok {
		return selectBrowserExport(selected, "")
	}
	for _, condition := range []string{"browser", "import", "default"} {
		if candidate, ok := object[condition]; ok {
			entry, found, err := selectBrowserExport(candidate, "")
			if err != nil {
				return "", false, err
			}
			if found {
				return entry, true, nil
			}
		}
	}
	if _, requireOnly := object["require"]; requireOnly {
		return "", false, fmt.Errorf("package is CommonJS-only for the root export")
	}
	return "", false, nil
}

func validateBrowserEntry(entry string) (string, error) {
	clean := filepath.ToSlash(filepath.Clean(entry))
	clean = strings.TrimPrefix(clean, "./")
	if clean == "." || clean == "" || strings.HasPrefix(clean, "../") || filepath.IsAbs(clean) {
		return "", fmt.Errorf("browser entry %q is not a safe package-relative path", entry)
	}
	return clean, nil
}

func packageRootName(specifier string) string {
	parts := strings.Split(specifier, "/")
	if strings.HasPrefix(specifier, "@") && len(parts) >= 2 {
		return strings.Join(parts[:2], "/")
	}
	return parts[0]
}

func packageSubpath(specifier string) string {
	root := packageRootName(specifier)
	return strings.TrimPrefix(specifier, root)
}

func exportKey(subpath string) string {
	if subpath == "" {
		return "."
	}
	return "." + subpath
}

func isCommonJSModule(directory string, entry string) bool {
	contents, err := os.ReadFile(filepath.Join(directory, filepath.FromSlash(entry)))
	if err != nil {
		return false
	}
	moduleText := string(contents)
	return strings.Contains(moduleText, "require(") ||
		strings.Contains(moduleText, "module.exports") ||
		strings.Contains(moduleText, "exports.")
}

func transformBrowserPackage(sourceDirectory string, entry string, outputPath string, specifier string) (string, error) {
	esbuildPath, err := bundledEsbuildPath()
	if err != nil {
		return "", err
	}
	transformerIdentity := browserTransformerIdentity()
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return "", err
	}

	bundleOutputPath := outputPath
	if browserInteropWrapper(specifier) != "" {
		bundleOutputPath = strings.TrimSuffix(outputPath, filepath.Ext(outputPath)) + ".bundle.mjs"
	}

	arguments := []string{
		filepath.Join(sourceDirectory, filepath.FromSlash(entry)),
		"--bundle",
		"--format=esm",
		"--platform=browser",
		"--target=es2020",
		"--define:process.env.NODE_ENV=\"production\"",
		"--outfile=" + bundleOutputPath,
		"--log-level=warning",
		"--external:react",
		"--external:react/*",
		"--external:react-dom",
		"--external:react-dom/*",
	}
	if specifier == "react-dom/client" {
		arguments = append(arguments,
			"--external:react",
			"--external:react-dom",
			`--banner:js=import __copelandReact from "react"; import __copelandReactDom from "react-dom"; var require = name => { if (name === "react") return __copelandReact; if (name === "react-dom") return __copelandReactDom; throw new Error("Copeland React browser realization cannot require " + name + "."); };`)
	} else if specifier == "react-dom" {
		arguments = append(arguments,
			"--external:react",
			`--banner:js=import __copelandReact from "react"; var require = name => { if (name === "react") return __copelandReact; throw new Error("Copeland React browser realization cannot require " + name + "."); };`)
	} else if shouldBundleBrowserPackage(specifier) {
		arguments = append(arguments,
			`--banner:js=import __copelandReact from "react"; import __copelandReactDom from "react-dom"; var require = name => { if (name === "react") return __copelandReact; if (name === "react-dom") return __copelandReactDom; throw new Error("Copeland third-party React realization cannot require " + name + "."); };`)
	}
	command := exec.Command(esbuildPath, arguments...)
	output, err := command.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if strings.Contains(message, "Could not resolve \"fs\"") {
			return "", fmt.Errorf("selected graph requires Node built-in \"fs\"")
		}
		if message == "" {
			message = err.Error()
		}
		return "", fmt.Errorf("esbuild browser transformation failed: %s", message)
	}
	if wrapper := browserInteropWrapper(specifier); wrapper != "" {
		contents := fmt.Sprintf(wrapper, filepath.ToSlash(filepath.Base(bundleOutputPath)))
		if err := os.WriteFile(outputPath, []byte(contents), 0o644); err != nil {
			return "", err
		}
	}
	return transformerIdentity, nil
}

// browserInteropWrapper makes the two bounded React runtime contracts explicit
// after esbuild converts their CommonJS entrypoints. It is intentionally not a
// general CommonJS export reflector.
func browserInteropWrapper(specifier string) string {
	switch specifier {
	case "react":
		return `import React from "./%s";
export default React;
export const Activity = React.Activity;
export const Children = React.Children;
export const Component = React.Component;
export const Fragment = React.Fragment;
export const Profiler = React.Profiler;
export const PureComponent = React.PureComponent;
export const StrictMode = React.StrictMode;
export const Suspense = React.Suspense;
export const __CLIENT_INTERNALS_DO_NOT_USE_OR_WARN_USERS_THEY_CANNOT_UPGRADE = React.__CLIENT_INTERNALS_DO_NOT_USE_OR_WARN_USERS_THEY_CANNOT_UPGRADE;
export const __COMPILER_RUNTIME = React.__COMPILER_RUNTIME;
export const cache = React.cache;
export const cacheSignal = React.cacheSignal;
export const cloneElement = React.cloneElement;
export const createContext = React.createContext;
export const createElement = React.createElement;
export const createRef = React.createRef;
export const forwardRef = React.forwardRef;
export const isValidElement = React.isValidElement;
export const lazy = React.lazy;
export const memo = React.memo;
export const startTransition = React.startTransition;
export const unstable_useCacheRefresh = React.unstable_useCacheRefresh;
export const use = React.use;
export const useActionState = React.useActionState;
export const useCallback = React.useCallback;
export const useContext = React.useContext;
export const useDebugValue = React.useDebugValue;
export const useDeferredValue = React.useDeferredValue;
export const useEffect = React.useEffect;
export const useEffectEvent = React.useEffectEvent;
export const useId = React.useId;
export const useImperativeHandle = React.useImperativeHandle;
export const useInsertionEffect = React.useInsertionEffect;
export const useLayoutEffect = React.useLayoutEffect;
export const useMemo = React.useMemo;
export const useOptimistic = React.useOptimistic;
export const useReducer = React.useReducer;
export const useRef = React.useRef;
export const useState = React.useState;
export const useSyncExternalStore = React.useSyncExternalStore;
export const useTransition = React.useTransition;
export const version = React.version;
`
	case "react-dom/client":
		return `import ReactDomClient from "./%s";
export default ReactDomClient;
export const createRoot = ReactDomClient.createRoot;
`
	case "react-dom":
		return `import ReactDom from "./%s";
export default ReactDom;
export const createPortal = ReactDom.createPortal;
export const flushSync = ReactDom.flushSync;
export const preconnect = ReactDom.preconnect;
export const prefetchDNS = ReactDom.prefetchDNS;
export const preinit = ReactDom.preinit;
export const preinitModule = ReactDom.preinitModule;
export const preload = ReactDom.preload;
export const requestFormReset = ReactDom.requestFormReset;
export const version = ReactDom.version;
`
	case "react/jsx-runtime":
		return `import ReactJsxRuntime from "./%s";
export default ReactJsxRuntime;
export const Fragment = ReactJsxRuntime.Fragment;
export const jsx = ReactJsxRuntime.jsx;
export const jsxs = ReactJsxRuntime.jsxs;
`
	default:
		return ""
	}
}

func browserTransformerIdentity() string {
	esbuildPath, err := bundledEsbuildPath()
	if err != nil {
		return "esbuild:unavailable"
	}
	versionOutput, err := exec.Command(esbuildPath, "--version").Output()
	if err != nil {
		return "esbuild:version-unavailable"
	}
	return "esbuild@" + strings.TrimSpace(string(versionOutput))
}

func bundledEsbuildPath() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("locate bundled esbuild: source location unavailable")
	}
	repositoryRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	fileName := "esbuild"
	if runtime.GOOS == "windows" {
		fileName += ".cmd"
	}
	path := filepath.Join(repositoryRoot, "manifest-frontend", "node_modules", ".bin", fileName)
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("bundled esbuild is unavailable at %s", path)
	}
	return path, nil
}

func browserArtifactHash(path string) (string, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(contents)
	return hex.EncodeToString(hash[:]), nil
}

func mustBrowserArtifactHash(path string) string {
	hash, err := browserArtifactHash(path)
	if err != nil {
		panic(err)
	}
	return hash
}

func copyBrowserPackage(sourceDirectory string, destinationDirectory string) error {
	return filepath.WalkDir(sourceDirectory, func(sourcePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(sourceDirectory, sourcePath)
		if err != nil {
			return err
		}
		if relative == "." {
			return os.MkdirAll(destinationDirectory, 0o755)
		}
		if entry.IsDir() {
			return os.MkdirAll(filepath.Join(destinationDirectory, relative), 0o755)
		}
		if !entry.Type().IsRegular() {
			return nil
		}

		destinationPath := filepath.Join(destinationDirectory, relative)
		if err := os.MkdirAll(filepath.Dir(destinationPath), 0o755); err != nil {
			return err
		}
		source, err := os.Open(sourcePath)
		if err != nil {
			return err
		}
		defer source.Close()
		destination, err := os.Create(destinationPath)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(destination, source)
		closeErr := destination.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
}

func packageOutputDirectory(packageName string) string {
	return strings.NewReplacer("@", "scope-", "/", "-").Replace(packageName)
}

func writeBrowserHost(packageRoot string, outputDirectory string, entryOutputPath string, materialization browserMaterialization) error {
	imports := make(map[string]string, len(materialization.Imports))
	for _, item := range materialization.Imports {
		imports[item.Specifier] = item.URL
	}
	importMap, err := json.Marshal(struct {
		Imports map[string]string `json:"imports"`
	}{Imports: imports})
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outputDirectory, "import-map.json"), append(importMap, '\n'), 0o644); err != nil {
		return err
	}
	materializationManifest, err := json.MarshalIndent(materialization, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outputDirectory, "browser-materialization.json"), append(materializationManifest, '\n'), 0o644); err != nil {
		return err
	}
	preflightScript := ""
	if hasBrowserImport(materialization, "react") && hasBrowserImport(materialization, "react-dom/client") {
		preflightPath := filepath.Join(outputDirectory, "react-preflight.js")
		if err := os.WriteFile(preflightPath, []byte(reactPreflightModule), 0o644); err != nil {
			return err
		}
		preflightScript = "<script type=\"module\" src=\"./react-preflight.js\"></script>\n"
	}

	template, readErr := os.ReadFile(filepath.Join(packageRoot, "index.html"))
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return readErr
	}
	if len(template) == 0 {
		template = []byte("<!doctype html>\n<html lang=\"en\">\n<head><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width, initial-scale=1\"><title>Copeland browser M1</title></head>\n<body>\n<main><h1>Copeland browser M1</h1><p id=\"status\">Loading…</p><button id=\"increment\" type=\"button\">Increment</button></main>\n</body>\n</html>\n")
	}
	bootstrap := "<script type=\"importmap\">" + string(importMap) + "</script>\n" +
		preflightScript +
		"<script type=\"module\" src=\"./" + entryOutputPath + "\"></script>\n"
	html := strings.Replace(string(template), "</body>", bootstrap+"</body>", 1)
	if html == string(template) {
		html += bootstrap
	}
	for _, asset := range browserStaticAssets(template) {
		sourceAsset := filepath.Join(packageRoot, asset)
		destinationAsset := filepath.Join(outputDirectory, asset)
		if _, statErr := os.Stat(sourceAsset); statErr == nil {
			if copyErr := copyBrowserFile(sourceAsset, destinationAsset); copyErr != nil {
				return copyErr
			}
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return statErr
		}
	}
	return os.WriteFile(filepath.Join(outputDirectory, "index.html"), []byte(html), 0o644)
}

var browserLinkHrefPattern = regexp.MustCompile(`(?is)<link\b[^>]*\bhref\s*=\s*["']([^"']+)["']`)

// browserStaticAssets finds safe, local stylesheet-style assets linked by the
// authored browser shell. The browser materializer owns copying these assets;
// applications do not need a second static-file host or a bespoke post-build
// copy script for generated CSS.
func browserStaticAssets(template []byte) []string {
	assets := map[string]struct{}{"styles.css": {}}
	for _, match := range browserLinkHrefPattern.FindAllSubmatch(template, -1) {
		href := string(match[1])
		if separator := strings.IndexAny(href, "?#"); separator >= 0 {
			href = href[:separator]
		}
		if href == "" || strings.Contains(href, ":") || strings.HasPrefix(href, "/") {
			continue
		}

		relative := filepath.Clean(filepath.FromSlash(href))
		if relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			continue
		}
		assets[relative] = struct{}{}
	}

	result := make([]string, 0, len(assets))
	for asset := range assets {
		result = append(result, asset)
	}
	sort.Strings(result)
	return result
}

func copyBrowserFile(sourcePath string, destinationPath string) error {
	if err := os.MkdirAll(filepath.Dir(destinationPath), 0o755); err != nil {
		return err
	}
	contents, err := os.ReadFile(sourcePath)
	if err != nil {
		return err
	}
	return os.WriteFile(destinationPath, contents, 0o644)
}

func hasBrowserImport(materialization browserMaterialization, specifier string) bool {
	return slices.ContainsFunc(materialization.Imports, func(item browserImport) bool {
		return item.Specifier == specifier
	})
}

func browserMaterializationFingerprint(materialization browserMaterialization) string {
	hash := sha256.New()
	for _, item := range materialization.Imports {
		_, _ = hash.Write([]byte(item.Specifier + "=" + item.URL + "\n"))
	}
	return hex.EncodeToString(hash.Sum(nil))
}

const browserHostModule = `export function setText(id, text) {
  const element = document.getElementById(id);
  if (element === null) throw new Error("Copeland browser host could not find text element '" + id + "'.");
  element.textContent = text;
}

export function onClick(id, callback) {
  const button = document.getElementById(id);
  if (!(button instanceof HTMLButtonElement)) throw new Error("Copeland browser host expected button '" + id + "'.");
  button.addEventListener("click", callback);
}

export function dispatch(initialState, reduce, render) {
  let currentState = initialState;
  render(currentState);
  return event => {
    const nextState = reduce(currentState, event);
    if (nextState !== currentState) {
      currentState = nextState;
      render(currentState);
    }
  };
}

export function getMountElement(id) {
  const element = document.getElementById(id);
  if (element === null) throw new Error("Copeland React host could not find mount element '" + id + "'.");
  return element;
}

export function dispatchReact(initialState, reduce, render) {
  let currentState = initialState;
  const send = event => {
    currentState = reduce(currentState, event);
    render(currentState, send);
  };
  render(currentState, send);
  return send;
}

export function copyText(text, onSuccess, onFailure) {
  if (navigator.clipboard && typeof navigator.clipboard.writeText === "function") {
    navigator.clipboard.writeText(text).then(onSuccess, () => copyTextWithDocument(text, onSuccess, onFailure));
    return;
  }

  copyTextWithDocument(text, onSuccess, onFailure);
}

function copyTextWithDocument(text, onSuccess, onFailure) {
  const element = document.createElement("textarea");
  element.value = text;
  element.setAttribute("readonly", "");
  element.style.position = "fixed";
  element.style.opacity = "0";
  document.body.appendChild(element);
  element.select();

  const copied = document.execCommand("copy");
  element.remove();
  if (copied) onSuccess();
  else onFailure();
}

export function getViewportWidth() {
  return window.innerWidth;
}

export function subscribeViewport(onChange) {
  window.addEventListener("resize", onChange, { passive: true });
  window.addEventListener("orientationchange", onChange, { passive: true });

  return () => {
    window.removeEventListener("resize", onChange);
    window.removeEventListener("orientationchange", onChange);
  };
}
`

const reactPreflightModule = `import * as React from "react";
import { createRoot } from "react-dom/client";

if (typeof React.createElement !== "function" || typeof createRoot !== "function") {
  throw new Error("Copeland React package preflight found missing browser exports.");
}

const preflightContainer = document.createElement("div");
const preflightRoot = createRoot(preflightContainer);
preflightRoot.unmount();
window.__copelandReactPreflight = { createElement: React.createElement, createRoot };
document.documentElement.dataset.copelandReactPreflight = "ready";
`
