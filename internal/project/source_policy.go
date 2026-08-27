package project

import (
	"context"
	"fmt"
	"net/url"

	"github.com/yuechen-li-dev/tspack/internal/manifest"
	"github.com/yuechen-li-dev/tspack/internal/perf"
	"github.com/yuechen-li-dev/tspack/internal/resolver"
)

type registryBackendMetadataClient struct {
	backend resolver.RegistryBackend
}

func (client registryBackendMetadataClient) PackageMetadata(ctx context.Context, name string) (*resolver.PackageMetadata, error) {
	metadata, err := client.backend.Metadata(ctx, name)
	if err != nil {
		return nil, err
	}
	out := &resolver.PackageMetadata{Name: name, Versions: map[string]resolver.PackageVersion{}}
	for version, registryVersion := range metadata.Versions {
		scripts := map[string]string{}
		for _, packageCapability := range registryVersion.Capabilities {
			if packageCapability.Kind == "lifecycleScript" && packageCapability.Script != "" {
				scripts[packageCapability.Script] = packageCapability.Command
			}
		}
		out.Versions[version] = resolver.PackageVersion{Name: name, Version: registryVersion.Version, Scripts: scripts}
	}
	return out, nil
}

func (client registryBackendMetadataClient) Tarball(context.Context, string) ([]byte, error) {
	return nil, fmt.Errorf("metadata-only registry client does not fetch artifacts")
}

func resolverSourcePolicy(ir *manifest.ManifestIR) resolver.SourcePolicy {
	policy := resolver.DefaultSourcePolicy()
	if ir == nil {
		return policy
	}
	declared := ir.RegistryPolicy
	policy.Origin = "manifest RegistryPolicy"
	policy.Offline = declared.Offline
	policy.RequireIntegrity = declared.RequireIntegrity
	policy.RequireAuditCoverage = declared.RequireAuditCoverage
	policy.RecordProvenance = hasDeclaredSourcePolicy(ir)
	if declared.AllowedSources != nil {
		policy.AllowedSources = map[string]bool{}
		for _, source := range declared.AllowedSources {
			policy.AllowedSources[source] = true
		}
	}
	for _, source := range declared.Sources {
		endpoints := make([]resolver.RegistryEndpoint, 0, len(source.Endpoints))
		for _, endpoint := range source.Endpoints {
			endpoints = append(endpoints, resolver.RegistryEndpoint{
				URL:                  endpoint.URL,
				TokenEnv:             endpoint.TokenEnv,
				FallbackOnNotFound:   endpoint.FallbackOnNotFound,
				AllowedArtifactHosts: append([]string(nil), endpoint.AllowedArtifactHosts...),
			})
		}
		policy.Endpoints[source.Kind] = endpoints
	}
	return policy
}

func hasDeclaredSourcePolicy(ir *manifest.ManifestIR) bool {
	if ir == nil {
		return false
	}
	policy := ir.RegistryPolicy
	return policy.AllowedSources != nil || policy.Offline || policy.RequireIntegrity || policy.RequireAuditCoverage || len(policy.Sources) > 0
}

func sourcePolicyBackends(policy resolver.SourcePolicy, session *perf.Session) (resolver.BackendRegistry, error) {
	return policy.Backends(func(source string, kind string, requestURL string, status int, err error) {
		if session == nil {
			return
		}
		host := ""
		if parsed, parseErr := url.Parse(requestURL); parseErr == nil {
			host = parsed.Host
		}
		session.RecordHTTPRequest(source+"."+kind, host, status)
		switch kind {
		case "metadata":
			session.RecordMetadataRequest()
		case "tarball":
			session.RecordTarballRequest()
		}
	})
}
