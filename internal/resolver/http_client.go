package resolver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

type HTTPRegistryClient struct {
	BaseURL              string
	Client               *http.Client
	Observe              func(kind string, requestURL string, status int, err error)
	Authorization        string
	AuthorizationEnv     string
	AllowedArtifactHosts []string
}

var (
	defaultRegistryHTTPClientOnce sync.Once
	defaultRegistryHTTPClient     *http.Client
)

func NewHTTPRegistryClient(baseURL string) *HTTPRegistryClient {
	if baseURL == "" {
		baseURL = "https://registry.npmjs.org"
	}
	return &HTTPRegistryClient{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Client:  sharedRegistryHTTPClient(),
	}
}

func sharedRegistryHTTPClient() *http.Client {
	defaultRegistryHTTPClientOnce.Do(func() {
		defaultRegistryHTTPClient = &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				Proxy:                 http.ProxyFromEnvironment,
				MaxIdleConns:          128,
				MaxIdleConnsPerHost:   64,
				MaxConnsPerHost:       0,
				IdleConnTimeout:       90 * time.Second,
				TLSHandshakeTimeout:   10 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
				ForceAttemptHTTP2:     true,
			},
		}
	})
	return defaultRegistryHTTPClient
}

func (c *HTTPRegistryClient) PackageMetadata(ctx context.Context, name string) (*PackageMetadata, error) {
	u, err := c.packageURL(name)
	if err != nil {
		return nil, err
	}
	body, status, err := c.get(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("%w", err)
	}
	if status == http.StatusNotFound {
		return nil, fmt.Errorf("package %s not found", name)
	}
	if status < 200 || status > 299 {
		return nil, fmt.Errorf("metadata request failed: status=%d package=%s", status, name)
	}
	var out PackageMetadata
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("metadata decode failed for %s: %w", name, err)
	}
	return &out, nil
}

func (c *HTTPRegistryClient) Tarball(ctx context.Context, tarballURL string) ([]byte, error) {
	if err := c.validateArtifactURL(tarballURL); err != nil {
		return nil, err
	}
	body, status, err := c.get(ctx, tarballURL)
	if err != nil {
		return nil, err
	}
	if status < 200 || status > 299 {
		return nil, fmt.Errorf("tarball request failed: status=%d", status)
	}
	return body, nil
}

func (c *HTTPRegistryClient) packageURL(name string) (string, error) {
	u, err := url.Parse(c.BaseURL)
	if err != nil {
		return "", fmt.Errorf("invalid registry url: %w", err)
	}

	escapedBasePath := u.EscapedPath()
	escapedPackageName := url.PathEscape(name)
	u.Path = appendDecodedPathSegment(u.Path, name)
	u.RawPath = appendEscapedPathSegment(escapedBasePath, escapedPackageName)
	return u.String(), nil
}

func appendDecodedPathSegment(basePath, segment string) string {
	basePath = strings.TrimRight(basePath, "/")
	if basePath == "" {
		return "/" + segment
	}
	return basePath + "/" + segment
}

func appendEscapedPathSegment(basePath, escapedSegment string) string {
	basePath = strings.TrimRight(basePath, "/")
	if basePath == "" {
		return "/" + escapedSegment
	}
	return basePath + "/" + escapedSegment
}

func (c *HTTPRegistryClient) get(ctx context.Context, u string) ([]byte, int, error) {
	client := c.Client
	if client == nil {
		client = sharedRegistryHTTPClient()
	}
	client = c.redirectSafeClient(client)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", "tspack")
	if err := c.applyAuthorization(req, u); err != nil {
		return nil, 0, err
	}
	if httpKind(u) == "tarball" {
		req.Header.Set("Accept", "application/octet-stream")
	} else {
		req.Header.Set("Accept", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		if c.Observe != nil {
			c.Observe(httpKind(u), u, 0, err)
		}
		return nil, 0, fmt.Errorf("registry request failed: %s", redactError(err))
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		delay := retryAfterDelay(resp.Header.Get("Retry-After"))
		if delay >= 0 {
			if c.Observe != nil {
				c.Observe(httpKind(u), u, resp.StatusCode, nil)
			}
			resp.Body.Close()
			timer := time.NewTimer(delay)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return nil, 0, ctx.Err()
			case <-timer.C:
				return c.getWithoutRetry(ctx, u)
			}
		}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		if c.Observe != nil {
			c.Observe(httpKind(u), u, resp.StatusCode, err)
		}
		return nil, resp.StatusCode, err
	}
	if c.Observe != nil {
		c.Observe(httpKind(u), u, resp.StatusCode, nil)
	}
	return body, resp.StatusCode, nil
}

func (c *HTTPRegistryClient) getWithoutRetry(ctx context.Context, u string) ([]byte, int, error) {
	client := c.Client
	if client == nil {
		client = sharedRegistryHTTPClient()
	}
	client = c.redirectSafeClient(client)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", "tspack")
	if err := c.applyAuthorization(req, u); err != nil {
		return nil, 0, err
	}
	if httpKind(u) == "tarball" {
		req.Header.Set("Accept", "application/octet-stream")
	} else {
		req.Header.Set("Accept", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		if c.Observe != nil {
			c.Observe(httpKind(u), u, 0, err)
		}
		return nil, 0, fmt.Errorf("registry request failed: %s", redactError(err))
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(resp.Body)
	if c.Observe != nil {
		c.Observe(httpKind(u), u, resp.StatusCode, readErr)
	}
	return body, resp.StatusCode, readErr
}

func (c *HTTPRegistryClient) shouldAuthorize(requestURL string) bool {
	base, baseErr := url.Parse(c.BaseURL)
	request, requestErr := url.Parse(requestURL)
	if baseErr != nil || requestErr != nil {
		return false
	}
	return strings.EqualFold(base.Scheme, request.Scheme) && strings.EqualFold(base.Host, request.Host)
}

func (c *HTTPRegistryClient) redirectSafeClient(client *http.Client) *http.Client {
	if client == nil || (c.Authorization == "" && c.AuthorizationEnv == "") {
		return client
	}
	clone := *client
	previousCheck := client.CheckRedirect
	clone.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if !c.shouldAuthorize(request.URL.String()) {
			request.Header.Del("Authorization")
		}
		if previousCheck != nil {
			return previousCheck(request, via)
		}
		if len(via) >= 10 {
			return fmt.Errorf("stopped after 10 redirects")
		}
		return nil
	}
	return &clone
}

func (c *HTTPRegistryClient) applyAuthorization(request *http.Request, requestURL string) error {
	if !c.shouldAuthorize(requestURL) {
		return nil
	}
	value := c.Authorization
	if c.AuthorizationEnv != "" {
		secret := os.Getenv(c.AuthorizationEnv)
		if secret == "" {
			return fmt.Errorf("TSPACK_REGISTRY_AUTH_FAILED: credential environment variable %s is not set", c.AuthorizationEnv)
		}
		value = "Bearer " + secret
	}
	if value != "" {
		request.Header.Set("Authorization", value)
	}
	return nil
}

func retryAfterDelay(value string) time.Duration {
	if strings.TrimSpace(value) == "" {
		return -1
	}
	seconds, err := time.ParseDuration(strings.TrimSpace(value) + "s")
	if err != nil || seconds < 0 {
		return -1
	}
	if seconds > time.Second {
		return time.Second
	}
	return seconds
}

func (c *HTTPRegistryClient) validateArtifactURL(value string) error {
	if len(c.AllowedArtifactHosts) == 0 {
		return nil
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return fmt.Errorf("TSPACK_REGISTRY_ENDPOINT_DENIED: invalid artifact URL")
	}
	host := strings.ToLower(parsed.Hostname())
	for _, allowed := range c.AllowedArtifactHosts {
		if host == strings.ToLower(strings.TrimSpace(allowed)) {
			return nil
		}
	}
	return fmt.Errorf("TSPACK_REGISTRY_ENDPOINT_DENIED: artifact host %s is not allowed", host)
}

func httpKind(u string) string {
	parsed, err := url.Parse(u)
	if err != nil {
		return "unknown"
	}
	if strings.HasSuffix(parsed.Path, ".tgz") || strings.HasSuffix(parsed.Path, ".tar.gz") {
		return "tarball"
	}
	return "metadata"
}
