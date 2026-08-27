// Package manifestfrontend owns execution of the TypeScript manifest frontend.
package manifestfrontend

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/yuechen-li-dev/tspack/internal/diag"
	"github.com/yuechen-li-dev/tspack/internal/nodecmd"
)

type Result struct {
	OK          bool              `json:"ok"`
	IR          json.RawMessage   `json:"ir"`
	Diagnostics []diag.Diagnostic `json:"diagnostics"`
}

type DependencyIslandStatus string

const (
	DependencyIslandOwnedCanonical  DependencyIslandStatus = "OwnedCanonical"
	DependencyIslandOwnedRecognized DependencyIslandStatus = "OwnedRecognized"
	DependencyIslandUserDynamic     DependencyIslandStatus = "UserDynamic"
	DependencyIslandAmbiguous       DependencyIslandStatus = "Ambiguous"
	DependencyIslandUnsupported     DependencyIslandStatus = "Unsupported"
	DependencyIslandAbsent          DependencyIslandStatus = "Absent"
)

type SourceRange struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

type DependencyIslandElement struct {
	SourceRange
	FullStart int `json:"fullStart"`
}

type DependencyIsland struct {
	SourceRange
	ContentStart int                       `json:"contentStart"`
	ContentEnd   int                       `json:"contentEnd"`
	Elements     []DependencyIslandElement `json:"elements"`
}

type DependencyInsertion struct {
	Offset          int    `json:"offset"`
	Multiline       bool   `json:"multiline"`
	AttributeIndent string `json:"attributeIndent"`
	ClosingIndent   string `json:"closingIndent"`
}

type ManifestImport struct {
	ContentStart int      `json:"contentStart"`
	ContentEnd   int      `json:"contentEnd"`
	Names        []string `json:"names"`
}

type DependencySourceAnalysis struct {
	Status         DependencyIslandStatus `json:"status"`
	Authority      string                 `json:"authority,omitempty"`
	ManifestPath   string                 `json:"manifestPath"`
	PackageName    string                 `json:"packageName,omitempty"`
	Island         *DependencyIsland      `json:"island,omitempty"`
	Insertion      *DependencyInsertion   `json:"insertion,omitempty"`
	ManifestImport *ManifestImport        `json:"manifestImport,omitempty"`
	Diagnostics    []diag.Diagnostic      `json:"diagnostics"`
}

type Statistics struct {
	WorkerStarts   int
	WorkerRequests int
	OneShotStarts  int
	Cumulative     time.Duration
	Median         time.Duration
	Maximum        time.Duration
}

type request struct {
	ID           int      `json:"id"`
	Operation    string   `json:"operation,omitempty"`
	ManifestPath string   `json:"manifestPath"`
	PackageName  string   `json:"packageName,omitempty"`
	Directory    string   `json:"directory"`
	Environment  []string `json:"environment"`
}

type response struct {
	ID     int             `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  string          `json:"error,omitempty"`
}

type worker struct {
	mutex   sync.Mutex
	command *exec.Cmd
	stdin   io.WriteCloser
	stdout  *bufio.Reader
	stderr  bytes.Buffer
	nextID  int
}

var manager = struct {
	sync.Mutex
	workers   map[string]*worker
	starts    int
	requests  int
	oneshots  int
	durations []time.Duration
}{workers: map[string]*worker{}}

func Execute(frontendPath string, manifestPath string) (Result, error) {
	started := time.Now()
	result, err := executeWithWorker(frontendPath, manifestPath)
	recordDuration(time.Since(started))
	return result, err
}

func AnalyzeDependencies(frontendPath string, manifestPath string, packageName string) (DependencySourceAnalysis, error) {
	started := time.Now()
	payload, err := executeRaw(frontendPath, manifestPath, "analyze-dependencies", packageName)
	recordDuration(time.Since(started))
	if err != nil {
		return DependencySourceAnalysis{}, err
	}
	var analysis DependencySourceAnalysis
	if err := json.Unmarshal(payload, &analysis); err != nil {
		return DependencySourceAnalysis{}, fmt.Errorf("invalid dependency source analysis: %w", err)
	}
	return analysis, nil
}

func executeWithWorker(frontendPath string, manifestPath string) (Result, error) {
	payload, err := executeRaw(frontendPath, manifestPath, "", "")
	if err != nil {
		return Result{}, err
	}
	var result Result
	if err := json.Unmarshal(payload, &result); err != nil {
		return Result{}, fmt.Errorf("invalid manifest frontend result: %w", err)
	}
	return result, nil
}

func executeRaw(frontendPath string, manifestPath string, operation string, packageName string) (json.RawMessage, error) {
	nodePath, err := nodecmd.Locate()
	if err != nil {
		return nil, err
	}
	frontendContents, err := os.ReadFile(frontendPath)
	if err != nil {
		return nil, err
	}
	if !bytes.Contains(frontendContents, []byte("--stdio-worker")) {
		return executeOneShotRaw(nodePath, frontendPath, manifestPath, operation, packageName)
	}
	workerKey, err := executionKey(nodePath, frontendPath)
	if err != nil {
		return nil, err
	}

	manager.Lock()
	current := manager.workers[workerKey]
	if current == nil {
		current, err = startWorker(nodePath, frontendPath)
		if err == nil {
			manager.workers[workerKey] = current
			manager.starts++
		}
	}
	manager.Unlock()
	if err == nil {
		result, workerErr := current.execute(manifestPath, operation, packageName)
		if workerErr == nil {
			manager.Lock()
			manager.requests++
			manager.Unlock()
			return result, nil
		}
		manager.Lock()
		if manager.workers[workerKey] == current {
			delete(manager.workers, workerKey)
		}
		manager.Unlock()
		_ = current.close()
	}
	return executeOneShotRaw(nodePath, frontendPath, manifestPath, operation, packageName)
}

func executionKey(nodePath string, frontendPath string) (string, error) {
	nodeInfo, err := os.Stat(nodePath)
	if err != nil {
		return "", err
	}
	frontendContents, err := os.ReadFile(frontendPath)
	if err != nil {
		return "", err
	}
	absoluteFrontend, err := filepath.Abs(frontendPath)
	if err != nil {
		return "", err
	}
	// Size and modification time intentionally identify immutable hard-linked
	// Node fixtures without binding reuse to their per-test PATH spelling.
	nodeIdentity := fmt.Sprintf("%d:%d", nodeInfo.Size(), nodeInfo.ModTime().UnixNano())
	frontendHash := sha256.Sum256(frontendContents)
	frontendIdentity := fmt.Sprintf("%s:%x:%s", absoluteFrontend, frontendHash, typescriptIdentity(frontendPath))
	return nodeIdentity + "|" + frontendIdentity, nil
}

func typescriptIdentity(frontendPath string) string {
	frontendDirectory := filepath.Dir(frontendPath)
	candidates := []string{
		filepath.Join(frontendDirectory, "node_modules", "typescript", "package.json"),
		filepath.Join(frontendDirectory, "..", "node_modules", "typescript", "package.json"),
	}
	for _, candidate := range candidates {
		contents, err := os.ReadFile(candidate)
		if err != nil {
			continue
		}
		hash := sha256.Sum256(contents)
		return fmt.Sprintf("%x", hash)
	}
	return "typescript-package-not-adjacent"
}

func startWorker(nodePath string, frontendPath string) (*worker, error) {
	command := exec.Command(nodePath, frontendPath, "--stdio-worker")
	stdin, err := command.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, err
	}
	current := &worker{command: command, stdin: stdin, stdout: bufio.NewReader(stdout)}
	command.Stderr = &current.stderr
	if err := command.Start(); err != nil {
		_ = stdin.Close()
		return nil, err
	}
	return current, nil
}

func (current *worker) execute(manifestPath string, operation string, packageName string) (json.RawMessage, error) {
	current.mutex.Lock()
	defer current.mutex.Unlock()

	directory, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	current.nextID++
	payload, err := json.Marshal(request{
		ID:           current.nextID,
		Operation:    operation,
		ManifestPath: manifestPath,
		PackageName:  packageName,
		Directory:    directory,
		Environment:  os.Environ(),
	})
	if err != nil {
		return nil, err
	}
	if _, err := current.stdin.Write(append(payload, '\n')); err != nil {
		return nil, err
	}
	line, err := current.stdout.ReadBytes('\n')
	if err != nil {
		return nil, fmt.Errorf("manifest frontend worker stopped: %w: %s", err, strings.TrimSpace(current.stderr.String()))
	}
	var reply response
	if err := json.Unmarshal(line, &reply); err != nil {
		return nil, fmt.Errorf("invalid manifest frontend worker response: %w", err)
	}
	if reply.ID != current.nextID {
		return nil, fmt.Errorf("manifest frontend worker response id %d, want %d", reply.ID, current.nextID)
	}
	if reply.Error != "" {
		return nil, fmt.Errorf("manifest frontend worker: %s", reply.Error)
	}
	return append(json.RawMessage(nil), reply.Result...), nil
}

func executeOneShot(nodePath string, frontendPath string, manifestPath string) (Result, error) {
	payload, err := executeOneShotRaw(nodePath, frontendPath, manifestPath, "", "")
	if err != nil {
		return Result{}, err
	}
	var result Result
	if err := json.Unmarshal(payload, &result); err != nil {
		return Result{}, err
	}
	return result, nil
}

func executeOneShotRaw(nodePath string, frontendPath string, manifestPath string, operation string, packageName string) (json.RawMessage, error) {
	manager.Lock()
	manager.oneshots++
	manager.Unlock()

	arguments := []string{frontendPath, manifestPath}
	if operation == "analyze-dependencies" {
		arguments = []string{frontendPath, "--analyze-dependencies", manifestPath}
		if packageName != "" {
			arguments = append(arguments, packageName)
		}
	}
	command := exec.Command(nodePath, arguments...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	runErr := command.Run()
	validJSON := json.Valid(stdout.Bytes())
	if runErr != nil {
		if !validJSON {
			return nil, fmt.Errorf("manifest frontend failed: %w: %s", runErr, strings.TrimSpace(stderr.String()))
		}
		if operation == "analyze-dependencies" {
			return nil, fmt.Errorf("manifest frontend failed: %w: %s", runErr, strings.TrimSpace(stderr.String()))
		}
		var result Result
		if err := json.Unmarshal(stdout.Bytes(), &result); err != nil || result.OK {
			return nil, fmt.Errorf("manifest frontend failed: %w: %s", runErr, strings.TrimSpace(stderr.String()))
		}
	}
	if !validJSON {
		return nil, fmt.Errorf("invalid manifest frontend JSON")
	}
	return append(json.RawMessage(nil), stdout.Bytes()...), nil
}

func (current *worker) close() error {
	current.mutex.Lock()
	defer current.mutex.Unlock()
	_ = current.stdin.Close()
	return current.command.Wait()
}

func CloseWorkers() {
	manager.Lock()
	workers := make([]*worker, 0, len(manager.workers))
	for _, current := range manager.workers {
		workers = append(workers, current)
	}
	manager.workers = map[string]*worker{}
	manager.Unlock()
	for _, current := range workers {
		_ = current.close()
	}
}

func SnapshotStatistics() Statistics {
	manager.Lock()
	defer manager.Unlock()
	durations := append([]time.Duration(nil), manager.durations...)
	sort.Slice(durations, func(i int, j int) bool { return durations[i] < durations[j] })
	statistics := Statistics{
		WorkerStarts:   manager.starts,
		WorkerRequests: manager.requests,
		OneShotStarts:  manager.oneshots,
	}
	for _, duration := range durations {
		statistics.Cumulative += duration
		if duration > statistics.Maximum {
			statistics.Maximum = duration
		}
	}
	if len(durations) > 0 {
		statistics.Median = durations[len(durations)/2]
	}
	return statistics
}

func recordDuration(duration time.Duration) {
	manager.Lock()
	manager.durations = append(manager.durations, duration)
	manager.Unlock()
}
