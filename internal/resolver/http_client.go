package resolver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type HTTPRegistryClient struct {
	BaseURL string
	Client  *http.Client
}

func NewHTTPRegistryClient(baseURL string) *HTTPRegistryClient {
	if baseURL == "" {
		baseURL = "https://registry.npmjs.org"
	}
	return &HTTPRegistryClient{BaseURL: strings.TrimRight(baseURL, "/"), Client: &http.Client{Timeout: 30 * time.Second}}
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
		client = &http.Client{Timeout: 30 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, 0, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return body, resp.StatusCode, nil
}
