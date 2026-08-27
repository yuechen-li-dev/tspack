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
	ManifestPath string   `json:"manifestPath"`
	Directory    string   `json:"directory"`
	Environment  []string `json:"environment"`
}

type response struct {
	ID     int    `json:"id"`
	Result Result `json:"result"`
	Error  string `json:"error,omitempty"`
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

func executeWithWorker(frontendPath string, manifestPath string) (Result, error) {
	nodePath, err := nodecmd.Locate()
	if err != nil {
		return Result{}, err
	}
	frontendContents, err := os.ReadFile(frontendPath)
	if err != nil {
		return Result{}, err
	}
	if !bytes.Contains(frontendContents, []byte("--stdio-worker")) {
		return executeOneShot(nodePath, frontendPath, manifestPath)
	}
	workerKey, err := executionKey(nodePath, frontendPath)
	if err != nil {
		return Result{}, err
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
		result, workerErr := current.execute(manifestPath)
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
	return executeOneShot(nodePath, frontendPath, manifestPath)
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

func (current *worker) execute(manifestPath string) (Result, error) {
	current.mutex.Lock()
	defer current.mutex.Unlock()

	directory, err := os.Getwd()
	if err != nil {
		return Result{}, err
	}
	current.nextID++
	payload, err := json.Marshal(request{
		ID:           current.nextID,
		ManifestPath: manifestPath,
		Directory:    directory,
		Environment:  os.Environ(),
	})
	if err != nil {
		return Result{}, err
	}
	if _, err := current.stdin.Write(append(payload, '\n')); err != nil {
		return Result{}, err
	}
	line, err := current.stdout.ReadBytes('\n')
	if err != nil {
		return Result{}, fmt.Errorf("manifest frontend worker stopped: %w: %s", err, strings.TrimSpace(current.stderr.String()))
	}
	var reply response
	if err := json.Unmarshal(line, &reply); err != nil {
		return Result{}, fmt.Errorf("invalid manifest frontend worker response: %w", err)
	}
	if reply.ID != current.nextID {
		return Result{}, fmt.Errorf("manifest frontend worker response id %d, want %d", reply.ID, current.nextID)
	}
	if reply.Error != "" {
		return Result{}, fmt.Errorf("manifest frontend worker: %s", reply.Error)
	}
	return reply.Result, nil
}

func executeOneShot(nodePath string, frontendPath string, manifestPath string) (Result, error) {
	manager.Lock()
	manager.oneshots++
	manager.Unlock()

	command := exec.Command(nodePath, frontendPath, manifestPath)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	runErr := command.Run()
	var result Result
	parseErr := json.Unmarshal(stdout.Bytes(), &result)
	if runErr != nil && (parseErr != nil || result.OK) {
		return Result{}, fmt.Errorf("manifest frontend failed: %w: %s", runErr, strings.TrimSpace(stderr.String()))
	}
	if parseErr != nil {
		return Result{}, fmt.Errorf("invalid manifest frontend JSON: %w", parseErr)
	}
	return result, nil
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
