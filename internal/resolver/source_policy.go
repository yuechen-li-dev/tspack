package resolver

import (
	"fmt"
	"net/url"
	"strings"
)

const (
	DefaultNPMEndpoint = "https://registry.npmjs.org"
	DefaultJSREndpoint = "https://npm.jsr.io"
)

// SourcePolicy controls whether a semantic registry source may be used and
// which concrete endpoints may supply its metadata and artifacts. Endpoints do
// not participate in package identity.
type SourcePolicy struct {
	Origin               string
	AllowedSources       map[string]bool
	Endpoints            map[string][]RegistryEndpoint
	Offline              bool
	RequireIntegrity     bool
	RequireAuditCoverage bool
	RecordProvenance     bool
}

type RegistryEndpoint struct {
	URL                  string
	TokenEnv             string
	FallbackOnNotFound   bool
	AllowedArtifactHosts []string
}

func DefaultSourcePolicy() SourcePolicy {
	return SourcePolicy{
		Origin:         "TSPack defaults",
		AllowedSources: map[string]bool{SourceNPM: true, SourceJSR: true},
		Endpoints: map[string][]RegistryEndpoint{
			SourceNPM: {{URL: DefaultNPMEndpoint}},
			SourceJSR: {{URL: DefaultJSREndpoint}},
		},
	}
}

func (policy SourcePolicy) Allows(source string) bool {
	if policy.AllowedSources == nil {
		return source == SourceNPM || source == SourceJSR
	}
	return policy.AllowedSources[source]
}

func (policy SourcePolicy) EndpointChain(source string) []RegistryEndpoint {
	if endpoints := policy.Endpoints[source]; len(endpoints) > 0 {
		return append([]RegistryEndpoint(nil), endpoints...)
	}
	defaults := DefaultSourcePolicy()
	return append([]RegistryEndpoint(nil), defaults.Endpoints[source]...)
}

func (policy SourcePolicy) Validate() error {
	for source, endpoints := range policy.Endpoints {
		if source != SourceNPM && source != SourceJSR {
			return fmt.Errorf("unsupported registry source %q", source)
		}
		if len(endpoints) == 0 {
			return fmt.Errorf("source %s must declare at least one endpoint", source)
		}
		seen := map[string]bool{}
		for _, endpoint := range endpoints {
			parsed, err := url.Parse(endpoint.URL)
			if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" || parsed.User != nil {
				return fmt.Errorf("source %s has invalid endpoint %q", source, RedactURL(endpoint.URL))
			}
			if URLHasSensitiveQuery(parsed) {
				return fmt.Errorf("source %s endpoint must not embed credentials in query parameters: %q", source, RedactURL(endpoint.URL))
			}
			normalized := strings.TrimRight(endpoint.URL, "/")
			if seen[normalized] {
				return fmt.Errorf("source %s repeats endpoint %q", source, RedactURL(endpoint.URL))
			}
			seen[normalized] = true
		}
	}
	return nil
}

func URLHasSensitiveQuery(parsed *url.URL) bool {
	if parsed == nil {
		return false
	}
	for key := range parsed.Query() {
		lower := strings.ToLower(key)
		if strings.Contains(lower, "token") || strings.Contains(lower, "secret") || strings.Contains(lower, "auth") || strings.Contains(lower, "key") {
			return true
		}
	}
	return false
}

func (policy SourcePolicy) Backends(observe func(source string, kind string, requestURL string, status int, err error)) (BackendRegistry, error) {
	if err := policy.Validate(); err != nil {
		return nil, err
	}
	backends := BackendRegistry{}
	for _, source := range []string{SourceNPM, SourceJSR} {
		if !policy.Allows(source) {
			continue
		}
		clients := make([]endpointClient, 0, len(policy.EndpointChain(source)))
		for _, endpoint := range policy.EndpointChain(source) {
			client := NewHTTPRegistryClient(endpoint.URL)
			client.AllowedArtifactHosts = append([]string(nil), endpoint.AllowedArtifactHosts...)
			client.Observe = func(kind string, requestURL string, status int, err error) {
				if observe != nil {
					observe(source, kind, requestURL, status, err)
				}
			}
			if endpoint.TokenEnv != "" {
				client.AuthorizationEnv = endpoint.TokenEnv
			}
			clients = append(clients, endpointClient{endpoint: endpoint, client: client})
		}
		backend := NewFallbackRegistryBackend(source, clients, policy.RecordProvenance)
		backends[source] = backend
	}
	return backends, nil
}

func RedactURL(value string) string {
	parsed, err := url.Parse(value)
	if err != nil {
		return "<redacted-url>"
	}
	parsed.User = nil
	query := parsed.Query()
	for key := range query {
		lower := strings.ToLower(key)
		if strings.Contains(lower, "token") || strings.Contains(lower, "secret") || strings.Contains(lower, "auth") || strings.Contains(lower, "key") {
			query.Set(key, "REDACTED")
		}
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
