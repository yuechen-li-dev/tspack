// Package compiler defines the stable boundary between resolved TSPack project
// truth and compiler-owned language semantics.
package compiler

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

const DescriptorSchemaVersion = 1

type LanguageIdentity struct {
	ID string
}

type CompilerIdentity struct {
	ID      string
	Version string
}

type ToolIdentity struct {
	Source  string
	Name    string
	Version string
	Path    string
}

type ConfigRef struct {
	Kind        string
	Path        string
	Fingerprint string
}

type RuntimeIdentity struct {
	Family string
	Name   string
}

type Capability string

const (
	CapabilityParse                        Capability = "parse"
	CapabilityTypeCheck                    Capability = "typeCheck"
	CapabilityEmitJavaScript               Capability = "emitJavaScript"
	CapabilityEmitManaged                  Capability = "emitManaged"
	CapabilityRunCLR                       Capability = "runClr"
	CapabilityEmitDeclarations             Capability = "emitDeclarations"
	CapabilityEmitNative                   Capability = "emitNative"
	CapabilityEmitObject                   Capability = "emitObject"
	CapabilityEmitWasm                     Capability = "emitWasm"
	CapabilityStaticCoverage               Capability = "staticCoverage"
	CapabilityDynamicFallback              Capability = "dynamicFallback"
	CapabilityIncremental                  Capability = "incremental"
	CapabilityProjectReferences            Capability = "projectReferences"
	CapabilitySourceMaps                   Capability = "sourceMaps"
	CapabilityCompilerOwnedSourcePartition Capability = "compilerOwnedSourcePartition"
	CapabilityCompilerOwnedConfig          Capability = "compilerOwnedConfig"
)

type OutputKind string

const (
	OutputJavaScript        OutputKind = "javaScript"
	OutputManagedExecutable OutputKind = "managedExecutable"
	OutputDeclarations      OutputKind = "declarations"
	OutputNativeExecutable  OutputKind = "nativeExecutable"
	OutputNativeObject      OutputKind = "nativeObject"
	OutputWasmModule        OutputKind = "wasmModule"
	OutputLibrary           OutputKind = "library"
	OutputCompilerMetadata  OutputKind = "compilerMetadata"
)

type Input struct {
	LogicalPath string
	Path        string
	Fingerprint string
}

type Output struct {
	Kind OutputKind
	Path string
}

// PackageBinding is resolved compiler-visible truth. SemanticIdentity remains
// distinct from the Node-compatible materialization name used on disk.
type PackageBinding struct {
	SemanticIdentity    string
	Version             string
	MaterializationPath string
	MaterializationName string
	LocalName           string
	Role                string
	TypeSurfaces        []string
}

type Payload struct {
	Kind          string
	SchemaVersion int
	Data          json.RawMessage
}

// Target is TSPack's in-memory compiler IR. It contains orchestration facts,
// never compiler flags or language-specific semantic structures.
type Target struct {
	ProjectRoot  string
	Package      string
	Name         string
	Language     LanguageIdentity
	Compiler     CompilerIdentity
	Tool         ToolIdentity
	Config       ConfigRef
	Inputs       []Input
	Packages     []PackageBinding
	Runtime      RuntimeIdentity
	Outputs      []Output
	Capabilities []Capability
	Payload      Payload
}

// Adapter is deliberately small. Adapters translate a target into an
// invocation, but never resolve packages or interpret a compiler type system.
type Adapter interface {
	CompilerID() string
	DescribeCapabilities() []Capability
	ValidateTarget(Target) error
	PrepareInvocation(Target, string) (Invocation, error)
}

type Invocation struct {
	Executable  string
	Arguments   []string
	Directory   string
	Environment map[string]string
}

type Descriptor struct {
	SchemaVersion   int                 `json:"schemaVersion"`
	ProjectRoot     string              `json:"projectRoot"`
	Target          DescriptorTarget    `json:"target"`
	Language        DescriptorIdentity  `json:"language"`
	Compiler        DescriptorCompiler  `json:"compiler"`
	Tool            DescriptorTool      `json:"tool"`
	CompilerConfig  DescriptorConfig    `json:"compilerConfig"`
	Sources         []DescriptorSource  `json:"sources"`
	Packages        []DescriptorPackage `json:"packages"`
	Runtime         DescriptorRuntime   `json:"runtime"`
	Outputs         []DescriptorOutput  `json:"outputs"`
	Capabilities    []string            `json:"capabilities"`
	CompilerPayload *DescriptorPayload  `json:"compilerPayload,omitempty"`
}

type DescriptorTarget struct {
	Package string `json:"package"`
	Name    string `json:"name"`
}

type DescriptorIdentity struct {
	ID string `json:"id"`
}

type DescriptorCompiler struct {
	ID      string `json:"id"`
	Version string `json:"version"`
}

type DescriptorTool struct {
	Source  string `json:"source"`
	Name    string `json:"name"`
	Version string `json:"version"`
	Path    string `json:"path"`
}

type DescriptorConfig struct {
	Kind        string `json:"kind"`
	Path        string `json:"path"`
	Fingerprint string `json:"fingerprint"`
}

type DescriptorSource struct {
	LogicalPath string `json:"logicalPath"`
	Path        string `json:"path"`
	Fingerprint string `json:"fingerprint"`
}

type DescriptorPackage struct {
	SemanticIdentity    string   `json:"semanticIdentity"`
	Version             string   `json:"version"`
	MaterializationPath string   `json:"materializationPath"`
	MaterializationName string   `json:"materializationName"`
	LocalName           string   `json:"localName"`
	Role                string   `json:"role"`
	TypeSurfaces        []string `json:"typeSurfaces,omitempty"`
}

type DescriptorRuntime struct {
	Family string `json:"family"`
	Name   string `json:"name"`
}

type DescriptorOutput struct {
	Kind string `json:"kind"`
	Path string `json:"path"`
}

type DescriptorPayload struct {
	Kind          string          `json:"kind"`
	SchemaVersion int             `json:"schemaVersion"`
	Data          json.RawMessage `json:"data"`
}

func NewDescriptor(target Target) (Descriptor, error) {
	if err := ValidateTarget(target); err != nil {
		return Descriptor{}, err
	}
	descriptor := Descriptor{
		SchemaVersion:  DescriptorSchemaVersion,
		ProjectRoot:    target.ProjectRoot,
		Target:         DescriptorTarget{Package: target.Package, Name: target.Name},
		Language:       DescriptorIdentity{ID: target.Language.ID},
		Compiler:       DescriptorCompiler{ID: target.Compiler.ID, Version: target.Compiler.Version},
		Tool:           DescriptorTool{Source: target.Tool.Source, Name: target.Tool.Name, Version: target.Tool.Version, Path: target.Tool.Path},
		CompilerConfig: DescriptorConfig{Kind: target.Config.Kind, Path: target.Config.Path, Fingerprint: target.Config.Fingerprint},
		Runtime:        DescriptorRuntime{Family: target.Runtime.Family, Name: target.Runtime.Name},
		Sources:        []DescriptorSource{},
		Packages:       []DescriptorPackage{},
		Outputs:        []DescriptorOutput{},
		Capabilities:   []string{},
	}
	for _, input := range target.Inputs {
		descriptor.Sources = append(descriptor.Sources, DescriptorSource(input))
	}
	for _, binding := range target.Packages {
		descriptor.Packages = append(descriptor.Packages, DescriptorPackage{
			SemanticIdentity:    binding.SemanticIdentity,
			Version:             binding.Version,
			MaterializationPath: binding.MaterializationPath,
			MaterializationName: binding.MaterializationName,
			LocalName:           binding.LocalName,
			Role:                binding.Role,
			TypeSurfaces:        append([]string(nil), binding.TypeSurfaces...),
		})
	}
	for _, output := range target.Outputs {
		descriptor.Outputs = append(descriptor.Outputs, DescriptorOutput{Kind: string(output.Kind), Path: output.Path})
	}
	for _, capability := range target.Capabilities {
		descriptor.Capabilities = append(descriptor.Capabilities, string(capability))
	}
	if target.Payload.Kind != "" {
		descriptor.CompilerPayload = &DescriptorPayload{Kind: target.Payload.Kind, SchemaVersion: target.Payload.SchemaVersion, Data: target.Payload.Data}
	}
	sort.Slice(descriptor.Sources, func(i, j int) bool { return descriptor.Sources[i].LogicalPath < descriptor.Sources[j].LogicalPath })
	sort.Slice(descriptor.Packages, func(i, j int) bool {
		return descriptor.Packages[i].SemanticIdentity < descriptor.Packages[j].SemanticIdentity
	})
	sort.Strings(descriptor.Capabilities)
	return descriptor, nil
}

func ValidateTarget(target Target) error {
	missing := []string{}
	for name, value := range map[string]string{
		"projectRoot":      target.ProjectRoot,
		"target.package":   target.Package,
		"target.name":      target.Name,
		"language.id":      target.Language.ID,
		"compiler.id":      target.Compiler.ID,
		"compiler.version": target.Compiler.Version,
		"tool.name":        target.Tool.Name,
	} {
		if strings.TrimSpace(value) == "" {
			missing = append(missing, name)
		}
	}
	if len(target.Inputs) == 0 {
		missing = append(missing, "sources")
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("compiler target is missing required fields: %s", strings.Join(missing, ", "))
	}
	if target.Payload.Kind != "" && target.Payload.SchemaVersion < 1 {
		return fmt.Errorf("compiler payload %q requires a positive schema version", target.Payload.Kind)
	}
	return nil
}
