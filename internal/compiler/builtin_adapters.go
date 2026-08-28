package compiler

import (
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
