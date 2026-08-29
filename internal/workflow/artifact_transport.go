package workflow

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/yuechen-li-dev/tspack/internal/project"
)

const (
	maxTransferArtifacts = 64
	maxTransferFileSize  = int64(256 << 20)
)

func (executor *Executor) runArtifactTransfer(ctx context.Context, step PlanStep) (NativeResult, error) {
	if len(step.ResolvedInputs) != 1 || step.ResolvedInputs[0].Result == nil || step.ResolvedInputs[0].Result.Build == nil {
		return NativeResult{}, errors.New("TSPACK_WORKFLOW_TRANSFER_INPUT_UNAVAILABLE: transfer requires one successful build artifact result")
	}
	artifacts := step.ResolvedInputs[0].Result.Build.Artifacts
	if len(artifacts) == 0 || len(artifacts) > maxTransferArtifacts {
		return NativeResult{}, fmt.Errorf("TSPACK_WORKFLOW_TRANSFER_LIMIT_INVALID: transfer requires 1..%d artifacts", maxTransferArtifacts)
	}
	root := executor.Root
	if root == "" {
		var err error
		root, err = os.Getwd()
		if err != nil {
			return NativeResult{}, err
		}
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return NativeResult{}, err
	}
	result := project.BuildOperationResult{Artifacts: []project.BuildArtifact{}, Targets: []project.BuildTargetResult{}}
	for index, artifact := range artifacts {
		if err := ctx.Err(); err != nil {
			return NativeResult{}, err
		}
		transported, err := transportArtifact(root, step.TransferSourceRegion, step.TransferTarget, artifact, index)
		if err != nil {
			return NativeResult{}, err
		}
		result.Artifacts = append(result.Artifacts, transported)
	}
	return NativeResult{Build: &result}, nil
}

func transportArtifact(root string, originRegion string, target string, artifact project.BuildArtifact, ordinal int) (project.BuildArtifact, error) {
	if artifact.Path == "" || filepath.IsAbs(artifact.Path) {
		return project.BuildArtifact{}, errors.New("TSPACK_WORKFLOW_TRANSFER_PATH_INVALID: artifact path must be workspace-relative")
	}
	source := filepath.Clean(filepath.Join(root, filepath.FromSlash(artifact.Path)))
	relative, err := filepath.Rel(root, source)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return project.BuildArtifact{}, errors.New("TSPACK_WORKFLOW_TRANSFER_PATH_INVALID: artifact escapes the workflow workspace")
	}
	info, err := os.Lstat(source)
	if err != nil {
		return project.BuildArtifact{}, fmt.Errorf("TSPACK_WORKFLOW_TRANSFER_READ_FAILED: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return project.BuildArtifact{}, errors.New("TSPACK_WORKFLOW_TRANSFER_PATH_INVALID: only regular non-symlink files are transportable")
	}
	if info.Size() > maxTransferFileSize {
		return project.BuildArtifact{}, fmt.Errorf("TSPACK_WORKFLOW_TRANSFER_SIZE_EXCEEDED: artifact exceeds %d bytes", maxTransferFileSize)
	}
	hash, err := hashArtifact(source)
	if err != nil {
		return project.BuildArtifact{}, err
	}
	logicalName := filepath.Base(source)
	destinationRelative := filepath.Join(".tspack", "workflow-transport", "regions", target, hash, fmt.Sprintf("%03d-%s", ordinal, logicalName))
	destination := filepath.Join(root, destinationRelative)
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return project.BuildArtifact{}, fmt.Errorf("TSPACK_WORKFLOW_TRANSFER_STAGE_FAILED: %w", err)
	}
	if err := copyArtifact(source, destination); err != nil {
		return project.BuildArtifact{}, err
	}
	transportedHash, err := hashArtifact(destination)
	if err != nil || transportedHash != hash {
		return project.BuildArtifact{}, errors.New("TSPACK_WORKFLOW_TRANSFER_INTEGRITY_FAILED: transported content does not match its identity")
	}
	artifact.Path = filepath.ToSlash(destinationRelative)
	artifact.Identity = fmt.Sprintf("artifact/%s/%s/%s", artifact.Package, artifact.Target, hash)
	artifact.ContentHash = "sha256:" + hash
	if artifact.OriginRegion == "" {
		artifact.OriginRegion = originRegion
	}
	return artifact, nil
}

func hashArtifact(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("TSPACK_WORKFLOW_TRANSFER_READ_FAILED: %w", err)
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, io.LimitReader(file, maxTransferFileSize+1)); err != nil {
		return "", fmt.Errorf("TSPACK_WORKFLOW_TRANSFER_READ_FAILED: %w", err)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func copyArtifact(source string, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("TSPACK_WORKFLOW_TRANSFER_READ_FAILED: %w", err)
	}
	defer input.Close()
	temporary, err := os.CreateTemp(filepath.Dir(destination), ".transfer-*")
	if err != nil {
		return fmt.Errorf("TSPACK_WORKFLOW_TRANSFER_STAGE_FAILED: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := io.Copy(temporary, input); err != nil {
		temporary.Close()
		return fmt.Errorf("TSPACK_WORKFLOW_TRANSFER_STAGE_FAILED: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("TSPACK_WORKFLOW_TRANSFER_STAGE_FAILED: %w", err)
	}
	if err := os.Rename(temporaryPath, destination); err != nil {
		if _, statErr := os.Stat(destination); statErr == nil {
			return nil
		}
		return fmt.Errorf("TSPACK_WORKFLOW_TRANSFER_STAGE_FAILED: %w", err)
	}
	return nil
}
