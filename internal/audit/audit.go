package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/yuechen-li-dev/tspack/internal/lockfile"
)

const DefaultEndpoint = "https://api.osv.dev"

type Severity string

const (
	SeverityUnknown  Severity = "unknown"
	SeverityLow      Severity = "low"
	SeverityModerate Severity = "moderate"
	SeverityHigh     Severity = "high"
	SeverityCritical Severity = "critical"
)

type Query struct {
	Name      string
	Version   string
	PageToken string
}

type QueryResult struct {
	IDs           []string
	NextPageToken string
}

type Reference struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

type Score struct {
	Type  string `json:"type"`
	Score string `json:"score"`
}

type Event struct {
	Fixed        string `json:"fixed"`
	LastAffected string `json:"last_affected"`
}

type Range struct {
	Events []Event `json:"events"`
}

type Affected struct {
	Package struct {
		Ecosystem string `json:"ecosystem"`
		Name      string `json:"name"`
	} `json:"package"`
	Ranges            []Range        `json:"ranges"`
	Severity          []Score        `json:"severity"`
	EcosystemSpecific map[string]any `json:"ecosystem_specific"`
}

type Vulnerability struct {
	ID               string         `json:"id"`
	Aliases          []string       `json:"aliases"`
	Summary          string         `json:"summary"`
	Severity         []Score        `json:"severity"`
	Affected         []Affected     `json:"affected"`
	References       []Reference    `json:"references"`
	DatabaseSpecific map[string]any `json:"database_specific"`
}

type Client interface {
	QueryBatch(context.Context, []Query) ([]QueryResult, error)
	Get(context.Context, string) (Vulnerability, error)
}

type HTTPClient struct {
	Endpoint string
	Client   *http.Client
}

func (c *HTTPClient) QueryBatch(ctx context.Context, queries []Query) ([]QueryResult, error) {
	type osvQuery struct {
		Version string `json:"version"`
		Package struct {
			Name      string `json:"name"`
			Ecosystem string `json:"ecosystem"`
		} `json:"package"`
		PageToken string `json:"page_token,omitempty"`
	}
	type osvResult struct {
		Vulns []struct {
			ID string `json:"id"`
		} `json:"vulns"`
		NextPageToken string `json:"next_page_token"`
	}
	payload := struct {
		Queries []osvQuery `json:"queries"`
	}{Queries: make([]osvQuery, len(queries))}
	for index, query := range queries {
		payload.Queries[index].Version = query.Version
		payload.Queries[index].Package.Name = query.Name
		payload.Queries[index].Package.Ecosystem = "npm"
		payload.Queries[index].PageToken = query.PageToken
	}
	var response struct {
		Results []osvResult `json:"results"`
	}
	if err := c.request(ctx, http.MethodPost, "/v1/querybatch", payload, &response); err != nil {
		return nil, err
	}
	if len(response.Results) != len(queries) {
		return nil, fmt.Errorf("OSV batch returned %d results for %d queries", len(response.Results), len(queries))
	}
	results := make([]QueryResult, len(response.Results))
	for index, result := range response.Results {
		results[index].NextPageToken = result.NextPageToken
		for _, vulnerability := range result.Vulns {
			results[index].IDs = append(results[index].IDs, vulnerability.ID)
		}
	}
	return results, nil
}

func (c *HTTPClient) Get(ctx context.Context, id string) (Vulnerability, error) {
	var vulnerability Vulnerability
	err := c.request(ctx, http.MethodGet, "/v1/vulns/"+url.PathEscape(id), nil, &vulnerability)
	return vulnerability, err
}

func (c *HTTPClient) request(ctx context.Context, method, requestPath string, payload any, target any) error {
	endpoint := strings.TrimRight(c.Endpoint, "/")
	if endpoint == "" {
		endpoint = DefaultEndpoint
	}
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint+requestPath, body)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "tspack-audit")
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	client := c.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("OSV request failed: %w", err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 32<<20))
	if err != nil {
		return fmt.Errorf("read OSV response: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("OSV request returned HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(responseBody)))
	}
	if err := json.Unmarshal(responseBody, target); err != nil {
		return fmt.Errorf("decode OSV response: %w", err)
	}
	return nil
}

type Finding struct {
	ID         string      `json:"id"`
	Aliases    []string    `json:"aliases,omitempty"`
	Summary    string      `json:"summary"`
	Severity   Severity    `json:"severity"`
	Package    string      `json:"package"`
	Version    string      `json:"version"`
	Fixed      []string    `json:"fixedVersions,omitempty"`
	References []Reference `json:"references,omitempty"`
	Paths      [][]string  `json:"paths,omitempty"`
}

type Report struct {
	Packages int       `json:"packages"`
	Findings []Finding `json:"findings"`
}

func Scan(ctx context.Context, lf *lockfile.Lockfile, client Client) (Report, error) {
	packages := npmPackages(lf)
	report := Report{Packages: len(packages), Findings: []Finding{}}
	if len(packages) == 0 {
		return report, nil
	}
	queries := make([]Query, len(packages))
	for index, pkg := range packages {
		queries[index] = Query{Name: pkg.Name, Version: pkg.Version}
	}
	idsByPackage := make([]map[string]bool, len(packages))
	for index := range idsByPackage {
		idsByPackage[index] = map[string]bool{}
	}
	for len(queries) > 0 {
		results, err := client.QueryBatch(ctx, queries)
		if err != nil {
			return report, err
		}
		nextQueries := []Query{}
		for resultIndex, result := range results {
			originalIndex := packageIndex(packages, queries[resultIndex].Name, queries[resultIndex].Version)
			for _, id := range result.IDs {
				idsByPackage[originalIndex][id] = true
			}
			if result.NextPageToken != "" {
				next := queries[resultIndex]
				next.PageToken = result.NextPageToken
				nextQueries = append(nextQueries, next)
			}
		}
		queries = nextQueries
	}
	records := map[string]Vulnerability{}
	for _, ids := range idsByPackage {
		for id := range ids {
			if _, ok := records[id]; ok {
				continue
			}
			record, err := client.Get(ctx, id)
			if err != nil {
				return report, fmt.Errorf("fetch OSV record %s: %w", id, err)
			}
			records[id] = record
		}
	}
	paths := packagePaths(lf)
	for index, pkg := range packages {
		ids := make([]string, 0, len(idsByPackage[index]))
		for id := range idsByPackage[index] {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			record := records[id]
			report.Findings = append(report.Findings, Finding{ID: record.ID, Aliases: sortedStrings(record.Aliases), Summary: record.Summary, Severity: classifySeverity(record, pkg.Name), Package: pkg.Name, Version: pkg.Version, Fixed: fixedVersions(record, pkg.Name), References: advisoryReferences(record.References), Paths: paths[pkg.ID]})
		}
	}
	sort.SliceStable(report.Findings, func(i, j int) bool {
		if severityRank(report.Findings[i].Severity) != severityRank(report.Findings[j].Severity) {
			return severityRank(report.Findings[i].Severity) > severityRank(report.Findings[j].Severity)
		}
		if report.Findings[i].Package != report.Findings[j].Package {
			return report.Findings[i].Package < report.Findings[j].Package
		}
		return report.Findings[i].ID < report.Findings[j].ID
	})
	return report, nil
}

func ParseThreshold(value string) (Severity, error) {
	switch strings.ToLower(value) {
	case "", "any":
		return SeverityUnknown, nil
	case "low":
		return SeverityLow, nil
	case "moderate":
		return SeverityModerate, nil
	case "high":
		return SeverityHigh, nil
	case "critical":
		return SeverityCritical, nil
	default:
		return "", fmt.Errorf("audit level must be one of any, low, moderate, high, or critical")
	}
}

func FailsThreshold(finding Severity, threshold Severity) bool {
	if threshold == SeverityUnknown {
		return true
	}
	if finding == SeverityUnknown {
		return false
	}
	return severityRank(finding) >= severityRank(threshold)
}

func npmPackages(lf *lockfile.Lockfile) []lockfile.Package {
	packages := []lockfile.Package{}
	seen := map[string]bool{}
	for _, pkg := range lf.Packages {
		if pkg.Source != "npm" || pkg.Name == "" || pkg.Version == "" || seen[pkg.ID] {
			continue
		}
		seen[pkg.ID] = true
		packages = append(packages, pkg)
	}
	sort.SliceStable(packages, func(i, j int) bool { return packages[i].ID < packages[j].ID })
	return packages
}

func packageIndex(packages []lockfile.Package, name, version string) int {
	for index, pkg := range packages {
		if pkg.Name == name && pkg.Version == version {
			return index
		}
	}
	return 0
}

func packagePaths(lf *lockfile.Lockfile) map[string][][]string {
	children := map[string][]string{}
	for _, edge := range lf.Edges {
		children[edge.From] = append(children[edge.From], edge.To)
	}
	for from := range children {
		sort.Strings(children[from])
	}
	paths := map[string][][]string{}
	roots := []string{}
	for from := range children {
		if strings.Contains(from, ":target:") || strings.HasSuffix(from, ":tool") {
			roots = append(roots, from)
		}
	}
	sort.Strings(roots)
	for _, root := range roots {
		queue := [][]string{{root}}
		seen := map[string]bool{root: true}
		for len(queue) > 0 {
			currentPath := queue[0]
			queue = queue[1:]
			for _, child := range children[currentPath[len(currentPath)-1]] {
				if seen[child] {
					continue
				}
				seen[child] = true
				next := append(append([]string{}, currentPath...), child)
				paths[child] = append(paths[child], next)
				queue = append(queue, next)
			}
		}
	}
	return paths
}

func classifySeverity(record Vulnerability, packageName string) Severity {
	for _, affected := range record.Affected {
		if affected.Package.Name == packageName {
			if severity := textualSeverity(affected.EcosystemSpecific); severity != SeverityUnknown {
				return severity
			}
		}
	}
	return textualSeverity(record.DatabaseSpecific)
}

func textualSeverity(values map[string]any) Severity {
	value, _ := values["severity"].(string)
	switch strings.ToLower(value) {
	case "low":
		return SeverityLow
	case "moderate", "medium":
		return SeverityModerate
	case "high":
		return SeverityHigh
	case "critical":
		return SeverityCritical
	default:
		return SeverityUnknown
	}
}

func severityRank(severity Severity) int {
	switch severity {
	case SeverityLow:
		return 1
	case SeverityModerate:
		return 2
	case SeverityHigh:
		return 3
	case SeverityCritical:
		return 4
	default:
		return 0
	}
}

func fixedVersions(record Vulnerability, packageName string) []string {
	values := []string{}
	for _, affected := range record.Affected {
		if affected.Package.Name != packageName {
			continue
		}
		for _, affectedRange := range affected.Ranges {
			for _, event := range affectedRange.Events {
				if event.Fixed != "" {
					values = append(values, event.Fixed)
				}
			}
		}
	}
	return sortedStrings(values)
}

func advisoryReferences(references []Reference) []Reference {
	values := []Reference{}
	for _, reference := range references {
		if reference.URL != "" && (reference.Type == "ADVISORY" || reference.Type == "WEB" || reference.Type == "FIX") {
			values = append(values, reference)
		}
	}
	sort.SliceStable(values, func(i, j int) bool { return values[i].URL < values[j].URL })
	return values
}

func sortedStrings(values []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, value := range values {
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}
