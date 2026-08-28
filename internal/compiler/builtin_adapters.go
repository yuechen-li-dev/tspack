package compiler

import (
	"encoding/json"
	"fmt"
	"strings"
)

type TSCAdapter struct{}

func (TSCAdapter) CompilerID() string { return "tsc" }

func (TSCAdapter) DescribeCapabilities() []Capability {
	return []Capability{
		CapabilityParse,
		CapabilityTypeCheck,
		CapabilityEmitJavaScript,
		CapabilityEmitDeclarations,
		CapabilityIncremental,
		CapabilityProjectReferences,
		CapabilitySourceMaps,
		CapabilityCompilerOwnedConfig,
	}
}

func (adapter TSCAdapter) ValidateTarget(target Target) error {
	if target.Compiler.ID != adapter.CompilerID() || target.Language.ID != "typescript" {
		return fmt.Errorf("tsc adapter requires language typescript and compiler tsc")
	}
	if target.Config.Path == "" {
		return fmt.Errorf("tsc adapter requires a compiler-owned config reference")
	}
	return ValidateTarget(target)
}

func (adapter TSCAdapter) PrepareInvocation(target Target, _ string) (Invocation, error) {
	if err := adapter.ValidateTarget(target); err != nil {
		return Invocation{}, err
	}
	return Invocation{
		Executable: target.Tool.Path,
		Arguments:  []string{"--project", target.Config.Path},
		Directory:  target.ProjectRoot,
	}, nil
}

type CopelandAdapter struct{}

func (CopelandAdapter) CompilerID() string { return "tscl" }

func (CopelandAdapter) DescribeCapabilities() []Capability {
	return []Capability{
		CapabilityParse,
		CapabilityTypeCheck,
		CapabilityEmitJavaScript,
		CapabilityCompilerOwnedConfig,
		CapabilityCompilerOwnedSourcePartition,
	}
}

// ScriptCPayloadV1 is the bounded adapter payload for ScriptC's executable
// lane. ScriptC-specific flags stay here rather than becoming generic target
// semantics.
type ScriptCPayloadV1 struct {
	Entry        string   `json:"entry"`
	Output       string   `json:"output"`
	Dynamic      bool     `json:"dynamic,omitempty"`
	Backend      string   `json:"backend,omitempty"`
	Optimization string   `json:"optimization,omitempty"`
	NPMStatic    []string `json:"npmStatic,omitempty"`
	CC           string   `json:"cc,omitempty"`
	Target       string   `json:"target,omitempty"`
}

type ScriptCAdapter struct{}

func (ScriptCAdapter) CompilerID() string { return "scriptc" }

func (ScriptCAdapter) DescribeCapabilities() []Capability {
	return []Capability{
		CapabilityParse,
		CapabilityTypeCheck,
		CapabilityEmitNative,
		CapabilityCompilerOwnedConfig,
		CapabilityStaticCoverage,
		CapabilityDynamicFallback,
	}
}

// PerryPayloadV1 contains Perry-owned native compilation choices. Keeping
// these flags in a versioned payload preserves the language-neutral target IR.
type PerryPayloadV1 struct {
	Entry          string   `json:"entry"`
	Output         string   `json:"output"`
	Target         string   `json:"target,omitempty"`
	OutputType     string   `json:"outputType,omitempty"`
	FastMath       bool     `json:"fastMath,omitempty"`
	FPContract     string   `json:"fpContract,omitempty"`
	TypeCheck      bool     `json:"typeCheck,omitempty"`
	NoAutoOptimize bool     `json:"noAutoOptimize,omitempty"`
	NoCodegen      bool     `json:"noCodegen,omitempty"`
	Features       []string `json:"features,omitempty"`
}

type PerryAdapter struct {
	RuntimeDirectory string
}

func (PerryAdapter) CompilerID() string { return "perry" }

func (PerryAdapter) DescribeCapabilities() []Capability {
	return []Capability{
		CapabilityParse,
		CapabilityTypeCheck,
		CapabilityEmitNative,
		CapabilityCompilerOwnedConfig,
		CapabilityCompilerOwnedSourcePartition,
	}
}

func (adapter PerryAdapter) ValidateTarget(target Target) error {
	if target.Compiler.ID != adapter.CompilerID() || target.Language.ID != "perry-ts" {
		return fmt.Errorf("Perry adapter requires language perry-ts and compiler perry")
	}
	if target.Payload.Kind != "perry-v1" || target.Payload.SchemaVersion != 1 {
		return fmt.Errorf("Perry adapter requires perry-v1 payload schema 1")
	}
	nativeExecutables := 0
	for _, output := range target.Outputs {
		if output.Kind != OutputNativeExecutable {
			return fmt.Errorf("Perry M71b adapter supports nativeExecutable output")
		}
		nativeExecutables++
	}
	if nativeExecutables != 1 {
		return fmt.Errorf("Perry M71b adapter requires one nativeExecutable output")
	}
	_, err := decodePerryPayload(target.Payload)
	if err != nil {
		return err
	}
	return ValidateTarget(target)
}

func (adapter PerryAdapter) PrepareInvocation(target Target, _ string) (Invocation, error) {
	if err := adapter.ValidateTarget(target); err != nil {
		return Invocation{}, err
	}
	payload, err := decodePerryPayload(target.Payload)
	if err != nil {
		return Invocation{}, err
	}
	arguments := []string{"compile", payload.Entry, "--output", payload.Output, "--no-color"}
	if payload.Target != "" {
		arguments = append(arguments, "--target", payload.Target)
	}
	if payload.OutputType != "" {
		arguments = append(arguments, "--output-type", payload.OutputType)
	}
	if payload.FastMath {
		arguments = append(arguments, "--fast-math")
	}
	if payload.FPContract != "" {
		arguments = append(arguments, "--fp-contract", payload.FPContract)
	}
	if payload.TypeCheck {
		arguments = append(arguments, "--type-check")
	}
	if payload.NoAutoOptimize {
		arguments = append(arguments, "--no-auto-optimize")
	}
	if payload.NoCodegen {
		arguments = append(arguments, "--no-codegen")
	}
	if len(payload.Features) > 0 {
		arguments = append(arguments, "--features", strings.Join(payload.Features, ","))
	}
	return Invocation{
		Executable:  target.Tool.Path,
		Arguments:   arguments,
		Directory:   target.ProjectRoot,
		Environment: perryEnvironment(adapter.RuntimeDirectory),
	}, nil
}

func perryEnvironment(runtimeDirectory string) map[string]string {
	if strings.TrimSpace(runtimeDirectory) == "" {
		return nil
	}
	return map[string]string{"PERRY_RUNTIME_DIR": runtimeDirectory}
}

func decodePerryPayload(payload Payload) (PerryPayloadV1, error) {
	var decoded PerryPayloadV1
	if err := json.Unmarshal(payload.Data, &decoded); err != nil {
		return PerryPayloadV1{}, fmt.Errorf("decode Perry payload: %w", err)
	}
	if strings.TrimSpace(decoded.Entry) == "" || strings.TrimSpace(decoded.Output) == "" {
		return PerryPayloadV1{}, fmt.Errorf("Perry payload requires entry and output")
	}
	if decoded.OutputType != "" && decoded.OutputType != "executable" {
		return PerryPayloadV1{}, fmt.Errorf("Perry M71b executable adapter requires outputType executable")
	}
	if decoded.FPContract != "" && decoded.FPContract != "off" && decoded.FPContract != "on" && decoded.FPContract != "fast" {
		return PerryPayloadV1{}, fmt.Errorf("Perry fpContract must be off, on, or fast")
	}
	for _, feature := range decoded.Features {
		if strings.TrimSpace(feature) == "" || strings.Contains(feature, ",") {
			return PerryPayloadV1{}, fmt.Errorf("Perry features must be non-empty names without commas")
		}
	}
	return decoded, nil
}

func (adapter ScriptCAdapter) ValidateTarget(target Target) error {
	if target.Compiler.ID != adapter.CompilerID() || target.Language.ID != "scriptc" {
		return fmt.Errorf("ScriptC adapter requires language scriptc and compiler scriptc")
	}
	if target.Payload.Kind != "scriptc-v1" || target.Payload.SchemaVersion != 1 {
		return fmt.Errorf("ScriptC adapter requires scriptc-v1 payload schema 1")
	}
	nativeExecutables := 0
	for _, output := range target.Outputs {
		switch output.Kind {
		case OutputNativeExecutable:
			nativeExecutables++
		case OutputCompilerMetadata:
		default:
			return fmt.Errorf("ScriptC M71a adapter supports nativeExecutable and compilerMetadata outputs")
		}
	}
	if nativeExecutables != 1 {
		return fmt.Errorf("ScriptC M71a adapter requires one nativeExecutable output")
	}
	_, err := decodeScriptCPayload(target.Payload)
	if err != nil {
		return err
	}
	return ValidateTarget(target)
}

func (adapter ScriptCAdapter) PrepareInvocation(target Target, _ string) (Invocation, error) {
	if err := adapter.ValidateTarget(target); err != nil {
		return Invocation{}, err
	}
	payload, err := decodeScriptCPayload(target.Payload)
	if err != nil {
		return Invocation{}, err
	}
	arguments := []string{"build", payload.Entry, "--out", payload.Output, "--no-keep-c"}
	arguments = appendScriptCOptions(arguments, payload)
	return Invocation{
		Executable:  target.Tool.Path,
		Arguments:   arguments,
		Directory:   target.ProjectRoot,
		Environment: scriptCEnvironment(payload),
	}, nil
}

func (adapter ScriptCAdapter) PrepareCoverageInvocation(target Target) (Invocation, error) {
	if err := adapter.ValidateTarget(target); err != nil {
		return Invocation{}, err
	}
	payload, err := decodeScriptCPayload(target.Payload)
	if err != nil {
		return Invocation{}, err
	}
	arguments := appendScriptCOptions([]string{"coverage", payload.Entry}, payload)
	return Invocation{
		Executable:  target.Tool.Path,
		Arguments:   arguments,
		Directory:   target.ProjectRoot,
		Environment: scriptCEnvironment(payload),
	}, nil
}

func decodeScriptCPayload(payload Payload) (ScriptCPayloadV1, error) {
	var decoded ScriptCPayloadV1
	if err := json.Unmarshal(payload.Data, &decoded); err != nil {
		return ScriptCPayloadV1{}, fmt.Errorf("decode ScriptC payload: %w", err)
	}
	if strings.TrimSpace(decoded.Entry) == "" || strings.TrimSpace(decoded.Output) == "" {
		return ScriptCPayloadV1{}, fmt.Errorf("ScriptC payload requires entry and output")
	}
	if decoded.Backend != "" && decoded.Backend != "llvm" && decoded.Backend != "c" {
		return ScriptCPayloadV1{}, fmt.Errorf("ScriptC backend must be llvm or c")
	}
	if decoded.Optimization != "" && decoded.Optimization != "release" && decoded.Optimization != "dev" {
		return ScriptCPayloadV1{}, fmt.Errorf("ScriptC optimization must be release or dev")
	}
	if decoded.CC != "" && decoded.CC != "clang" && decoded.CC != "zigcc" {
		return ScriptCPayloadV1{}, fmt.Errorf("ScriptC cc must be clang or zigcc")
	}
	if decoded.Target != "" && decoded.CC != "zigcc" {
		return ScriptCPayloadV1{}, fmt.Errorf("ScriptC cross target requires cc zigcc")
	}
	return decoded, nil
}

func scriptCEnvironment(payload ScriptCPayloadV1) map[string]string {
	environment := map[string]string{}
	if payload.CC != "" {
		environment["SCRIPTC_CC"] = payload.CC
	}
	if payload.Target != "" {
		environment["SCRIPTC_TARGET"] = payload.Target
	}
	return environment
}

func appendScriptCOptions(arguments []string, payload ScriptCPayloadV1) []string {
	if payload.Dynamic {
		arguments = append(arguments, "--dynamic")
	}
	if payload.Backend != "" {
		arguments = append(arguments, "--backend", payload.Backend)
	}
	if payload.Optimization != "" {
		arguments = append(arguments, "--optimization", payload.Optimization)
	}
	for _, packageName := range payload.NPMStatic {
		arguments = append(arguments, "--npm-static", packageName)
	}
	return arguments
}

func (adapter CopelandAdapter) ValidateTarget(target Target) error {
	if target.Compiler.ID != adapter.CompilerID() || target.Language.ID != "copeland-ts" {
		return fmt.Errorf("Copeland adapter requires language copeland-ts and compiler tscl")
	}
	if target.Payload.Kind != "copeland-v1" || target.Payload.SchemaVersion != 1 {
		return fmt.Errorf("Copeland adapter requires copeland-v1 payload schema 1")
	}
	return ValidateTarget(target)
}

func (adapter CopelandAdapter) PrepareInvocation(target Target, descriptorPath string) (Invocation, error) {
	if err := adapter.ValidateTarget(target); err != nil {
		return Invocation{}, err
	}
	resultPath := strings.TrimSuffix(descriptorPath, ".request.json")
	return Invocation{
		Executable: target.Tool.Path,
		Arguments:  []string{"build", "--project", descriptorPath, "--result", resultPath},
		Directory:  target.ProjectRoot,
	}, nil
}
