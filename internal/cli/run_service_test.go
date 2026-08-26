package cli

import (
	"bytes"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/yuechen-li-dev/tspack/internal/manifest"
)

func TestRunTargetServiceTCPPreflightAvailableAndUnavailable(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err == nil {
			_ = conn.Close()
		}
	}()

	available := manifest.RunTargetRequirement{Name: "tcp", TCP: listener.Addr().String(), TimeoutMs: 1000}
	if err := checkRunTargetService(available); err != nil {
		t.Fatalf("available TCP service failed preflight: %v", err)
	}
	<-done

	closed, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	unavailableEndpoint := closed.Addr().String()
	_ = closed.Close()

	target := manifest.RunTarget{Name: "dev", Requires: []manifest.RunTargetRequirement{{Name: "missing", TCP: unavailableEndpoint, TimeoutMs: 100}}}
	if runErr := preflightRunTargetServices(target, &bytes.Buffer{}); runErr == nil || runErr.code != "TSPACK_RUN_SERVICE_UNAVAILABLE" {
		t.Fatalf("expected unavailable required service diagnostic, got %#v", runErr)
	}
}

func TestRunTargetServiceHTTPPreflightAndOptionalWarning(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	available := manifest.RunTargetRequirement{Name: "api", HTTP: server.URL + "/health", ExpectStatus: http.StatusNoContent, TimeoutMs: 1000}
	if err := checkRunTargetService(available); err != nil {
		t.Fatalf("available HTTP service failed preflight: %v", err)
	}

	mismatch := manifest.RunTargetRequirement{Name: "api", HTTP: server.URL + "/health", ExpectStatus: http.StatusOK, TimeoutMs: 1000, Optional: true}
	stderr := &bytes.Buffer{}
	target := manifest.RunTarget{Name: "dev", Requires: []manifest.RunTargetRequirement{mismatch}}
	if runErr := preflightRunTargetServices(target, stderr); runErr != nil {
		t.Fatalf("optional service mismatch should not block run: %#v", runErr)
	}
	if !bytes.Contains(stderr.Bytes(), []byte("Warning:")) {
		t.Fatalf("optional service mismatch should warn, got %q", stderr.String())
	}
}
