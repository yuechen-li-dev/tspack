package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestAppRunInjectsOutputAndReturnsStatus(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	app := &App{
		Stdin:  strings.NewReader(""),
		Stdout: &stdout,
		Stderr: &stderr,
	}

	code := app.Run(context.Background(), []string{"definitely-not-a-command"})

	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "unknown command: definitely-not-a-command") {
		t.Fatalf("stderr missing unknown-command diagnostic: %q", stderr.String())
	}
	if !strings.Contains(stdout.String(), "TSPack") {
		t.Fatalf("stdout missing help output: %q", stdout.String())
	}
}

func TestAppRunRecoversExpectedHandlerExit(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	app := &App{
		Stdin:  strings.NewReader(""),
		Stdout: &stdout,
		Stderr: &stderr,
	}

	code := app.Run(context.Background(), []string{"check", "--explain"})

	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "TSPACK_CHECK_EXPLAIN_FILE_REQUIRED") {
		t.Fatalf("stderr missing parser diagnostic: %q", stderr.String())
	}
}
