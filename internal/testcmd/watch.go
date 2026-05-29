package testcmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tspack/tspack/internal/diag"
)

const watchDebounce = 200 * time.Millisecond
const watchPollInterval = 500 * time.Millisecond

type FileStamp struct {
	ModTime time.Time
	Size    int64
}

type Watcher struct {
	root     string
	previous map[string]FileStamp
}

func NewWatcher(root string) (*Watcher, error) {
	state, err := ScanWatchFiles(root)
	if err != nil {
		return nil, err
	}
	watcher := &Watcher{
		root:     root,
		previous: state,
	}
	return watcher, nil
}

func (watcher *Watcher) Poll() ([]string, error) {
	current, err := ScanWatchFiles(watcher.root)
	if err != nil {
		return nil, err
	}
	changed := DiffWatchFiles(watcher.previous, current)
	watcher.previous = current
	return changed, nil
}

func ScanWatchFiles(root string) (map[string]FileStamp, error) {
	state := map[string]FileStamp{}
	cleanRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}

	err = filepath.WalkDir(cleanRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if shouldIgnoreWatchDir(entry.Name()) && path != cleanRoot {
				return filepath.SkipDir
			}
			return nil
		}
		if !isWatchFile(path) || shouldIgnoreWatchFile(entry.Name()) {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(cleanRoot, path)
		if err != nil {
			return err
		}
		key := filepath.ToSlash(relative)
		state[key] = FileStamp{
			ModTime: info.ModTime(),
			Size:    info.Size(),
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return state, nil
}

func DiffWatchFiles(previous map[string]FileStamp, current map[string]FileStamp) []string {
	changedSet := map[string]struct{}{}
	for path, previousStamp := range previous {
		currentStamp, ok := current[path]
		if !ok {
			changedSet[path] = struct{}{}
			continue
		}
		if !previousStamp.ModTime.Equal(currentStamp.ModTime) || previousStamp.Size != currentStamp.Size {
			changedSet[path] = struct{}{}
		}
	}
	for path := range current {
		if _, ok := previous[path]; !ok {
			changedSet[path] = struct{}{}
		}
	}

	changed := make([]string, 0, len(changedSet))
	for path := range changedSet {
		changed = append(changed, path)
	}
	sort.Strings(changed)
	return changed
}

func shouldIgnoreWatchDir(name string) bool {
	switch name {
	case "node_modules", ".git", ".tspack", "dist", "build", "coverage", "tspack-artifacts", "tmp", "temp":
		return true
	default:
		return false
	}
}

func isWatchFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".ts", ".tsx", ".js", ".jsx":
		return true
	default:
		return false
	}
}

func shouldIgnoreWatchFile(name string) bool {
	return strings.HasSuffix(name, "~") || strings.HasPrefix(name, ".#") || strings.HasSuffix(name, ".swp") || strings.HasSuffix(name, ".swo")
}

func runWatch(ctx context.Context, opts Options, stderr io.Writer) Result {
	if opts.List || opts.JSON {
		return Result{Diagnostics: []diag.Diagnostic{{Code: "TSPACK_TEST_WATCH_INVALID_MODE", Severity: diag.SeverityError, Message: "watch mode does not support list or JSON output"}}, ExitCode: 1}
	}

	selected := selectedBackends(opts)
	if len(selected) == 0 {
		selected = autoDetectBackends(opts.RootDir)
		if opts.Batch && containsString(selected, "xtest") {
			selected = []string{"xtest"}
		}
	}
	if len(selected) == 0 {
		return Result{Diagnostics: []diag.Diagnostic{{Code: "TSPACK_TEST_NO_BACKENDS", Severity: diag.SeverityError, Message: "no test backends discovered"}}, ExitCode: 1}
	}
	if len(selected) != 1 || selected[0] != "xtest" {
		return Result{Diagnostics: []diag.Diagnostic{{Code: "TSPACK_TEST_WATCH_UNSUPPORTED_BACKEND", Severity: diag.SeverityError, Message: "watch mode only supports the native xTest backend"}}, ExitCode: 1}
	}

	watcher, err := NewWatcher(opts.RootDir)
	if err != nil {
		return watchFailedResult(err)
	}

	fmt.Fprintf(stderr, "Watching tests under %s\n", opts.RootDir)
	fmt.Fprintln(stderr, "Press Ctrl+C to stop.")
	fmt.Fprintln(stderr)

	maxRuns := watchMaxRunsFromEnv()
	runNumber := 0
	lastExitCode := 0
	runOnce := func() Result {
		runNumber++
		fmt.Fprintf(stderr, "Run #%d\n", runNumber)
		result := Result{}
		runXTestContext(ctx, opts, &result)
		if ctx.Err() != nil {
			lastExitCode = 0
			return Result{ExitCode: 0}
		}
		if hasErrors(result.Diagnostics) || result.ExitCode != 0 {
			result.ExitCode = 1
		}
		lastExitCode = result.ExitCode
		return result
	}

	result := runOnce()
	if ctx.Err() != nil {
		return Result{ExitCode: 0}
	}
	if len(result.Diagnostics) > 0 {
		return result
	}
	if maxRuns > 0 && runNumber >= maxRuns {
		return Result{ExitCode: lastExitCode}
	}

	pollTicker := time.NewTicker(watchPollInterval)
	defer pollTicker.Stop()

	var pendingChanges []string
	var debounceTimer *time.Timer
	var debounceChannel <-chan time.Time

	stopDebounce := func() {
		if debounceTimer != nil {
			if !debounceTimer.Stop() {
				select {
				case <-debounceTimer.C:
				default:
				}
			}
		}
		debounceTimer = nil
		debounceChannel = nil
	}

	for {
		select {
		case <-ctx.Done():
			stopDebounce()
			return Result{ExitCode: 0}
		case <-pollTicker.C:
			changed, err := watcher.Poll()
			if err != nil {
				stopDebounce()
				return watchFailedResult(err)
			}
			if len(changed) == 0 {
				continue
			}
			pendingChanges = mergeChangedPaths(pendingChanges, changed)
			if debounceTimer == nil {
				debounceTimer = time.NewTimer(watchDebounce)
				debounceChannel = debounceTimer.C
				continue
			}
			if !debounceTimer.Stop() {
				select {
				case <-debounceTimer.C:
				default:
				}
			}
			debounceTimer.Reset(watchDebounce)
		case <-debounceChannel:
			debounceTimer = nil
			debounceChannel = nil
			reportChangedPaths(stderr, pendingChanges)
			pendingChanges = nil
			result := runOnce()
			if ctx.Err() != nil {
				return Result{ExitCode: 0}
			}
			if len(result.Diagnostics) > 0 {
				return result
			}
			if maxRuns > 0 && runNumber >= maxRuns {
				return Result{ExitCode: lastExitCode}
			}
		}
	}
}

func mergeChangedPaths(existing []string, next []string) []string {
	set := map[string]struct{}{}
	for _, path := range existing {
		set[path] = struct{}{}
	}
	for _, path := range next {
		set[path] = struct{}{}
	}
	merged := make([]string, 0, len(set))
	for path := range set {
		merged = append(merged, path)
	}
	sort.Strings(merged)
	return merged
}

func reportChangedPaths(stderr io.Writer, changed []string) {
	if len(changed) == 1 {
		fmt.Fprintf(stderr, "Change detected: %s\n", changed[0])
		return
	}
	fmt.Fprintf(stderr, "Change detected: %d files\n", len(changed))
}

func watchMaxRunsFromEnv() int {
	value := os.Getenv("TSPACK_TEST_WATCH_MAX_RUNS")
	if value == "" {
		return 0
	}
	maxRuns, err := strconv.Atoi(value)
	if err != nil || maxRuns < 1 {
		return 0
	}
	return maxRuns
}

func watchFailedResult(err error) Result {
	return Result{Diagnostics: []diag.Diagnostic{{Code: "TSPACK_TEST_WATCH_FAILED", Severity: diag.SeverityError, Message: "watch mode failed", Details: []string{err.Error()}}}, ExitCode: 1}
}
