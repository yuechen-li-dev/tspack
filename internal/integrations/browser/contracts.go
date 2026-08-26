// Package browser owns the browser-specific materialization adapter used by
// tscl builds. It consumes compiler contracts and writes browser runtime assets;
// generic project lifecycle packages do not depend on browser semantics.
package browser

type NpmContract struct {
	PackageName         string         `json:"packageName"`
	Version             string         `json:"version"`
	MaterializationPath string         `json:"materializationPath"`
	Materialized        bool           `json:"materialized"`
	Exports             []NpmExport    `json:"exports,omitempty"`
	Components          []NpmComponent `json:"components,omitempty"`
}

type NpmExport struct {
	Name        string   `json:"name"`
	Parameters  []string `json:"parameters"`
	Result      string   `json:"result"`
	RemoteError string   `json:"remoteError,omitempty"`
	Promise     bool     `json:"promise,omitempty"`
}

type NpmComponent struct {
	Name       string        `json:"name"`
	Properties []NpmProperty `json:"properties,omitempty"`
	Members    []NpmMember   `json:"members,omitempty"`
}

type NpmMember struct {
	Name       string        `json:"name"`
	Properties []NpmProperty `json:"properties,omitempty"`
}

type NpmProperty struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Required bool   `json:"required,omitempty"`
}
