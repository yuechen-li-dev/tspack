package resolver

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/yuechen-li-dev/tspack/internal/capability"
	"github.com/yuechen-li-dev/tspack/internal/lockfile"
	"github.com/yuechen-li-dev/tspack/internal/packageidentity"
)

const (
	SourceNPM = packageidentity.SourceNPM
	SourceJSR = packageidentity.SourceJSR
)

// RegistryBackend adapts one registry into TSPack's source-qualified package
// model. Resolver and store code consume these normalized values rather than
// registry response fields such as npm's dist.tarball.
type RegistryBackend interface {
	Source() string
	Metadata(context.Context, string) (*RegistryPackageMetadata, error)
	FetchArtifact(context.Context, ArtifactDescriptor) ([]byte, error)
	Host() string
}

type RegistryPackageMetadata struct {
	Identity PackageIdentity
	Versions map[string]RegistryPackageVersion
}

type PackageIdentity struct {
	Source string
	Name   string
}

func (identity PackageIdentity) ID(version string) string {
	return identity.Source + ":" + identity.Name + "@" + version
}

type RegistryPackageVersion struct {
	Identity            PackageIdentity
	Version             string
	Dependencies        []DependencyRequirement
	PeerRequirements    []DependencyRequirement
	Artifact            ArtifactDescriptor
	ArtifactPackageName string
	Capabilities        []lockfile.Capability
}

type DependencyRequirement struct {
	Identity   PackageIdentity
	Reference  string
	Constraint string
	Kind       string
	Optional   bool
}

type ArtifactDescriptor struct {
	Kind      string
	URL       string
	Integrity string
}

type BackendRegistry map[string]RegistryBackend

func (registry BackendRegistry) Backend(source string) (RegistryBackend, bool) {
	backend, ok := registry[source]
	return backend, ok
}

type npmBackend struct {
	client NPMRegistryClient
	host   string
}

func NewNPMBackend(client NPMRegistryClient) RegistryBackend {
	if client == nil {
		client = NewHTTPRegistryClient("")
	}
	return &npmBackend{client: client, host: registryClientHost(client)}
}

func (backend *npmBackend) Source() string {
	return SourceNPM
}

func (backend *npmBackend) Host() string {
	return backend.host
}

func (backend *npmBackend) Metadata(ctx context.Context, name string) (*RegistryPackageMetadata, error) {
	raw, err := backend.client.PackageMetadata(ctx, name)
	if err != nil {
		return nil, err
	}

	identity := PackageIdentity{Source: SourceNPM, Name: name}
	metadata := &RegistryPackageMetadata{
		Identity: identity,
		Versions: make(map[string]RegistryPackageVersion, len(raw.Versions)),
	}
	for version, rawVersion := range raw.Versions {
		dependencies, normalizeErr := normalizeNPMDependencies(rawVersion.Dependencies, false, nil)
		if normalizeErr != nil {
			return nil, fmt.Errorf("normalize dependencies for npm:%s@%s: %w", name, version, normalizeErr)
		}
		optionalDependencies, normalizeErr := normalizeNPMDependencies(rawVersion.OptionalDependencies, true, nil)
		if normalizeErr != nil {
			return nil, fmt.Errorf("normalize optional dependencies for npm:%s@%s: %w", name, version, normalizeErr)
		}
		dependencies = append(dependencies, optionalDependencies...)
		peerRequirements, normalizeErr := normalizeNPMDependencies(rawVersion.PeerDependencies, false, rawVersion.PeerDependenciesMeta)
		if normalizeErr != nil {
			return nil, fmt.Errorf("normalize peer dependencies for npm:%s@%s: %w", name, version, normalizeErr)
		}
		for index := range peerRequirements {
			peerRequirements[index].Kind = "peer"
		}
		metadata.Versions[version] = RegistryPackageVersion{
			Identity:            identity,
			Version:             rawVersion.Version,
			Dependencies:        dependencies,
			PeerRequirements:    peerRequirements,
			Artifact:            ArtifactDescriptor{Kind: "tarball", URL: rawVersion.Dist.Tarball, Integrity: rawVersion.Dist.Integrity},
			ArtifactPackageName: rawVersion.Name,
			Capabilities:        capability.FromPackageJSONScripts(rawVersion.Scripts),
		}
	}
	return metadata, nil
}

func (backend *npmBackend) FetchArtifact(ctx context.Context, artifact ArtifactDescriptor) ([]byte, error) {
	return backend.client.Tarball(ctx, artifact.URL)
}

func normalizeNPMDependencies(dependencies map[string]string, optional bool, peerMeta map[string]struct {
	Optional bool `json:"optional"`
}) ([]DependencyRequirement, error) {
	out := make([]DependencyRequirement, 0, len(dependencies))
	for _, dependency := range sortedDeps(dependencies) {
		identity := PackageIdentity{Source: SourceNPM, Name: dependency.name}
		constraint := dependency.rng
		if strings.HasPrefix(constraint, "npm:") {
			aliasIdentity, aliasConstraint, err := parseNPMAlias(constraint)
			if err != nil {
				return nil, err
			}
			identity = aliasIdentity
			constraint = aliasConstraint
		}
		dependencyOptional := optional
		if metadata, ok := peerMeta[dependency.name]; ok && metadata.Optional {
			dependencyOptional = true
		}
		out = append(out, DependencyRequirement{
			Identity:   identity,
			Reference:  dependency.name,
			Constraint: constraint,
			Kind:       "runtime",
			Optional:   dependencyOptional,
		})
	}
	return out, nil
}

func parseNPMAlias(value string) (PackageIdentity, string, error) {
	spec := strings.TrimPrefix(value, "npm:")
	name := spec
	constraint := "*"
	separator := strings.LastIndex(spec, "@")
	if separator > 0 {
		name = spec[:separator]
		constraint = spec[separator+1:]
	}
	if strings.TrimSpace(name) == "" || strings.TrimSpace(constraint) == "" {
		return PackageIdentity{}, "", fmt.Errorf("invalid npm alias %q", value)
	}
	if strings.HasPrefix(name, "@") && !strings.Contains(name, "/") {
		return PackageIdentity{}, "", fmt.Errorf("invalid scoped npm alias %q", value)
	}
	return PackageIdentity{Source: SourceNPM, Name: name}, constraint, nil
}

type jsrBackend struct {
	client NPMRegistryClient
	host   string
}

// NewJSRBackend uses JSR's documented npm compatibility registry directly.
// This produces Node/TypeScript-compatible immutable tarballs without invoking
// Deno, npm, npx, or another package manager.
func NewJSRBackend(client NPMRegistryClient) RegistryBackend {
	if client == nil {
		client = NewHTTPRegistryClient("https://npm.jsr.io")
	}
	return &jsrBackend{client: client, host: registryClientHost(client)}
}

func (backend *jsrBackend) Source() string {
	return SourceJSR
}

func (backend *jsrBackend) Host() string {
	return backend.host
}

func (backend *jsrBackend) Metadata(ctx context.Context, name string) (*RegistryPackageMetadata, error) {
	compatibilityName, err := jsrCompatibilityName(name)
	if err != nil {
		return nil, err
	}
	raw, err := backend.client.PackageMetadata(ctx, compatibilityName)
	if err != nil {
		return nil, err
	}

	identity := PackageIdentity{Source: SourceJSR, Name: name}
	metadata := &RegistryPackageMetadata{
		Identity: identity,
		Versions: make(map[string]RegistryPackageVersion, len(raw.Versions)),
	}
	for version, rawVersion := range raw.Versions {
		dependencies, normalizeErr := normalizeJSRDependencies(rawVersion.Dependencies)
		if normalizeErr != nil {
			return nil, fmt.Errorf("normalize dependencies for jsr:%s@%s: %w", name, version, normalizeErr)
		}
		optionalDependencies, normalizeErr := normalizeJSRDependencies(rawVersion.OptionalDependencies)
		if normalizeErr != nil {
			return nil, fmt.Errorf("normalize optional dependencies for jsr:%s@%s: %w", name, version, normalizeErr)
		}
		for index := range optionalDependencies {
			optionalDependencies[index].Optional = true
		}
		dependencies = append(dependencies, optionalDependencies...)
		peerRequirements, normalizeErr := normalizeJSRDependencies(rawVersion.PeerDependencies)
		if normalizeErr != nil {
			return nil, fmt.Errorf("normalize peer dependencies for jsr:%s@%s: %w", name, version, normalizeErr)
		}
		for index := range peerRequirements {
			peerRequirements[index].Kind = "peer"
			if metadata, ok := rawVersion.PeerDependenciesMeta[peerRequirements[index].Reference]; ok {
				peerRequirements[index].Optional = metadata.Optional
			}
		}
		metadata.Versions[version] = RegistryPackageVersion{
			Identity:            identity,
			Version:             rawVersion.Version,
			Dependencies:        dependencies,
			PeerRequirements:    peerRequirements,
			Artifact:            ArtifactDescriptor{Kind: "tarball", URL: rawVersion.Dist.Tarball, Integrity: rawVersion.Dist.Integrity},
			ArtifactPackageName: rawVersion.Name,
		}
	}
	return metadata, nil
}

func (backend *jsrBackend) FetchArtifact(ctx context.Context, artifact ArtifactDescriptor) ([]byte, error) {
	return backend.client.Tarball(ctx, artifact.URL)
}

func jsrCompatibilityName(name string) (string, error) {
	return packageidentity.JSRCompatibilityName(name)
}

func normalizeJSRDependencies(dependencies map[string]string) ([]DependencyRequirement, error) {
	out := make([]DependencyRequirement, 0, len(dependencies))
	for _, dependency := range sortedDeps(dependencies) {
		identity := PackageIdentity{Source: SourceNPM, Name: dependency.name}
		constraint := dependency.rng
		if strings.HasPrefix(dependency.name, "@jsr/") {
			logicalName, err := logicalJSRName(dependency.name)
			if err != nil {
				return nil, err
			}
			identity = PackageIdentity{Source: SourceJSR, Name: logicalName}
		} else if strings.HasPrefix(constraint, "npm:") {
			aliasIdentity, aliasConstraint, err := parseNPMAlias(constraint)
			if err != nil {
				return nil, err
			}
			identity = aliasIdentity
			constraint = aliasConstraint
		}
		out = append(out, DependencyRequirement{
			Identity:   identity,
			Reference:  dependency.name,
			Constraint: constraint,
			Kind:       "runtime",
		})
	}
	return out, nil
}

func logicalJSRName(compatibilityName string) (string, error) {
	return packageidentity.LogicalJSRName(compatibilityName)
}

func registryClientHost(client NPMRegistryClient) string {
	httpClient, ok := client.(*HTTPRegistryClient)
	if !ok {
		return ""
	}
	parsed, err := url.Parse(httpClient.BaseURL)
	if err != nil {
		return ""
	}
	return parsed.Host
}
