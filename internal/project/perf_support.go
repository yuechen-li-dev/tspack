package project

import (
	"context"
	"net/url"
	"os"

	"github.com/yuechen-li-dev/tspack/internal/lockfile"
	"github.com/yuechen-li-dev/tspack/internal/materialize"
	"github.com/yuechen-li-dev/tspack/internal/perf"
	"github.com/yuechen-li-dev/tspack/internal/resolver"
)

func ensurePerfSession(opts *Options, command string, dryRun bool) (*perf.Session, error) {
	if opts.Perf != nil {
		return opts.Perf, nil
	}
	cfg := perf.ConfigFromEnv()
	if !cfg.Enabled {
		return nil, nil
	}
	session, err := perf.NewSession(command, opts.RootDir, dryRun, cfg, opts.PerfWriter)
	if err != nil {
		return nil, err
	}
	opts.Perf = session
	return session, nil
}

func instrumentRegistryClient(client resolver.NPMRegistryClient, session *perf.Session, source string) resolver.NPMRegistryClient {
	if client == nil {
		client = resolver.NewHTTPRegistryClient("")
	}
	if session == nil {
		return client
	}
	if httpClient, ok := client.(*resolver.HTTPRegistryClient); ok {
		return &resolver.HTTPRegistryClient{
			BaseURL:              httpClient.BaseURL,
			Client:               httpClient.Client,
			Authorization:        httpClient.Authorization,
			AuthorizationEnv:     httpClient.AuthorizationEnv,
			AllowedArtifactHosts: append([]string(nil), httpClient.AllowedArtifactHosts...),
			Observe: func(kind string, requestURL string, status int, err error) {
				host := ""
				parsed, parseErr := url.Parse(requestURL)
				if parseErr == nil {
					host = parsed.Host
				}
				requestKind := kind
				if source != "" {
					requestKind = source + "." + kind
				}
				session.RecordHTTPRequest(requestKind, host, status)
				switch kind {
				case "metadata":
					session.RecordMetadataRequest()
				case "tarball":
					session.RecordTarballRequest()
				}
			},
		}
	}
	return perfRegistryClient{inner: client, session: session}
}

type perfRegistryClient struct {
	inner   resolver.NPMRegistryClient
	session *perf.Session
}

func (c perfRegistryClient) PackageMetadata(ctx context.Context, name string) (*resolver.PackageMetadata, error) {
	c.session.RecordMetadataRequest()
	return c.inner.PackageMetadata(ctx, name)
}

func (c perfRegistryClient) Tarball(ctx context.Context, tarballURL string) ([]byte, error) {
	c.session.RecordTarballRequest()
	return c.inner.Tarball(ctx, tarballURL)
}

type materializePerfObserver struct {
	session *perf.Session
}

func (o materializePerfObserver) RecordMaterializedPackage(pkg lockfile.Package) {
	o.session.RecordMaterializedPackage()
}

func (o materializePerfObserver) RecordMaterializedDirectory(path string) {
	o.session.RecordMaterializedDirectory()
}

func (o materializePerfObserver) RecordMaterializedFile(path string, size int64) {
	o.session.RecordMaterializedFile(size)
}

func (o materializePerfObserver) RecordHardlink(path string, size int64) {
	o.session.RecordHardlink(size)
}

func (o materializePerfObserver) RecordCopy(path string, size int64) {
	o.session.RecordCopy(size)
}

func (o materializePerfObserver) RecordMaterializationMarkerHit() {
	o.session.RecordMaterializationMarkerHit()
}

func (o materializePerfObserver) RecordMaterializationMarkerMiss() {
	o.session.RecordMaterializationMarkerMiss()
}

func (o materializePerfObserver) RecordMaterializationMarkerMismatch() {
	o.session.RecordMaterializationMarkerMismatch()
}

func (o materializePerfObserver) RecordMaterializationMarkerCorrupt() {
	o.session.RecordMaterializationMarkerCorrupt()
}

func (o materializePerfObserver) RecordMaterializationNoop(packages int, files int, directories int) {
	o.session.RecordMaterializationNoop(packages, files, directories)
}

func (o materializePerfObserver) RecordForcedMaterialization() {
	o.session.RecordForcedMaterialization()
}

func (o materializePerfObserver) RecordMaterializationMarkerWrite() {
	o.session.RecordMaterializationMarkerWrite()
}

func materializeStatsObserver(session *perf.Session) materialize.StatsObserver {
	if session == nil {
		return nil
	}
	return materializePerfObserver{session: session}
}

func perfWriterForCLI() *os.File {
	return os.Stderr
}
