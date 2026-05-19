package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type doctorScope string

const (
	doctorScopeAll     doctorScope = "all"
	doctorScopeFormat  doctorScope = "format"
	doctorScopeRun     doctorScope = "run"
	doctorScopeInspect doctorScope = "inspect"
)

type DoctorReport struct {
	Root     string          `json:"root"`
	Sections []DoctorSection `json:"sections"`
	Summary  DoctorSummary   `json:"summary"`
}
type DoctorSection struct {
	Name   string        `json:"name"`
	Checks []DoctorCheck `json:"checks"`
}
type DoctorCheck struct {
	Name           string         `json:"name"`
	Status         string         `json:"status"`
	Message        string         `json:"message"`
	Details        map[string]any `json:"details,omitempty"`
	Recommendation string         `json:"recommendation,omitempty"`
}
type DoctorSummary struct {
	Ok       int `json:"ok"`
	Warnings int `json:"warnings"`
	Errors   int `json:"errors"`
}

func runDoctorCommand(args []string) {
	scope := doctorScopeAll
	root := "."
	jsonOut := false
	for i := 1; i < len(args); i++ {
		a := args[i]
		switch a {
		case "format", "run", "inspect":
			scope = doctorScope(a)
		case "--root":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "TSPACK_DOCTOR_ROOT_INVALID: --root requires a value")
				os.Exit(1)
			}
			i++
			root = args[i]
		case "--json":
			jsonOut = true
		default:
			if strings.HasPrefix(a, "-") {
				fmt.Fprintf(os.Stderr, "TSPACK_DOCTOR_INVALID_SCOPE: unknown flag %s\n", a)
				os.Exit(1)
			}
			fmt.Fprintf(os.Stderr, "TSPACK_DOCTOR_INVALID_SCOPE: unknown scope %s\n", a)
			os.Exit(1)
		}
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "TSPACK_DOCTOR_ROOT_INVALID: %v\n", err)
		os.Exit(1)
	}
	if st, err := os.Stat(abs); err != nil || !st.IsDir() {
		fmt.Fprintln(os.Stderr, "TSPACK_DOCTOR_ROOT_INVALID: root directory does not exist")
		os.Exit(1)
	}

	report := DoctorReport{Root: abs}
	report.Sections = append(report.Sections, doctorProject(abs).toSection("Project"))
	if scope == doctorScopeAll || scope == doctorScopeFormat {
		report.Sections = append(report.Sections, doctorFormat(abs).toSection("Format/Lint"))
	}
	if scope == doctorScopeAll || scope == doctorScopeRun {
		report.Sections = append(report.Sections, doctorRun(abs).toSection("Run"))
	}
	if scope == doctorScopeAll || scope == doctorScopeInspect {
		report.Sections = append(report.Sections, doctorInspect(abs).toSection("Inspect (experimental)"))
	}
	for _, s := range report.Sections {
		for _, c := range s.Checks {
			if c.Status == "ok" {
				report.Summary.Ok++
			}
			if c.Status == "warning" {
				report.Summary.Warnings++
			}
			if c.Status == "error" {
				report.Summary.Errors++
			}
		}
	}
	if jsonOut {
		b, _ := json.MarshalIndent(report, "", "  ")
		os.Stdout.Write(append(b, '\n'))
	} else {
		printDoctorText(report)
	}
	if (scope == doctorScopeFormat || scope == doctorScopeRun) && report.Summary.Errors > 0 {
		os.Exit(1)
	}
}

type doctorBuilder struct{ checks []DoctorCheck }

func (d doctorBuilder) toSection(name string) DoctorSection {
	return DoctorSection{Name: name, Checks: d.checks}
}

func doctorProject(root string) doctorBuilder {
	d := doctorBuilder{}
	manifestPath := filepath.Join(root, "manifest.tsx")
	if _, err := os.Stat(manifestPath); err == nil {
		d.checks = append(d.checks, DoctorCheck{Name: "manifest", Status: "ok", Message: "manifest.tsx found"})
	} else {
		d.checks = append(d.checks, DoctorCheck{Name: "manifest", Status: "error", Message: "manifest.tsx missing", Recommendation: "Create manifest.tsx at project root."})
	}
	if _, err := os.Stat(filepath.Join(root, "ts-lock.toml")); err == nil {
		d.checks = append(d.checks, DoctorCheck{Name: "lockfile", Status: "ok", Message: "ts-lock.toml found"})
	} else {
		d.checks = append(d.checks, DoctorCheck{Name: "lockfile", Status: "warning", Message: "ts-lock.toml missing"})
	}
	if _, err := os.Stat(filepath.Join(root, "node_modules")); err == nil {
		d.checks = append(d.checks, DoctorCheck{Name: "node_modules", Status: "ok", Message: "node_modules present"})
	} else {
		d.checks = append(d.checks, DoctorCheck{Name: "node_modules", Status: "warning", Message: "node_modules missing"})
	}
	return d
}

func doctorFormat(root string) doctorBuilder {
	d := doctorBuilder{}
	local := filepath.Join(root, "node_modules", ".bin", "biome")
	if _, err := os.Stat(local); err == nil {
		d.checks = append(d.checks, DoctorCheck{Name: "biome", Status: "ok", Message: "biome found", Details: map[string]any{"path": local, "source": "local"}})
	} else if p, err := exec.LookPath("biome"); err == nil {
		d.checks = append(d.checks, DoctorCheck{Name: "biome", Status: "ok", Message: "biome found", Details: map[string]any{"path": p, "source": "path"}})
	} else {
		d.checks = append(d.checks, DoctorCheck{Name: "biome", Status: "error", Message: "biome backend missing", Recommendation: "Install biome for format/lint support."})
	}
	if _, err := os.Stat(filepath.Join(root, "biome.json")); err == nil {
		d.checks = append(d.checks, DoctorCheck{Name: "config", Status: "ok", Message: "biome.json found"})
	} else if _, err := os.Stat(filepath.Join(root, "biome.jsonc")); err == nil {
		d.checks = append(d.checks, DoctorCheck{Name: "config", Status: "ok", Message: "biome.jsonc found"})
	} else {
		d.checks = append(d.checks, DoctorCheck{Name: "config", Status: "warning", Message: "biome config not found; TSPack defaults will be used"})
	}
	return d
}

func doctorRun(root string) doctorBuilder {
	d := doctorBuilder{}
	if p, err := exec.LookPath("node"); err == nil {
		d.checks = append(d.checks, DoctorCheck{Name: "node", Status: "ok", Message: "node found", Details: map[string]any{"path": p}})
	} else {
		d.checks = append(d.checks, DoctorCheck{Name: "node", Status: "warning", Message: "node not found"})
	}
	if _, err := exec.LookPath("bun"); err == nil {
		d.checks = append(d.checks, DoctorCheck{Name: "bun", Status: "ok", Message: "bun found"})
	} else {
		d.checks = append(d.checks, DoctorCheck{Name: "bun", Status: "warning", Message: "bun not found"})
	}
	if _, err := exec.LookPath("deno"); err == nil {
		d.checks = append(d.checks, DoctorCheck{Name: "deno", Status: "ok", Message: "deno found"})
	} else {
		d.checks = append(d.checks, DoctorCheck{Name: "deno", Status: "warning", Message: "deno not found"})
	}
	if _, err := os.Stat(filepath.Join(root, "manifest.tsx")); err != nil {
		d.checks = append(d.checks, DoctorCheck{Name: "runTargets", Status: "error", Message: "manifest.tsx missing"})
		return d
	}
	ir := loadManifestForRun(root)
	count := 0
	for _, pkg := range ir.Packages {
		for _, rt := range pkg.RunTargets {
			count++
			d.checks = append(d.checks, DoctorCheck{Name: "runTarget:" + rt.Name, Status: "ok", Message: fmt.Sprintf("%s runtime=%s url=%s", rt.Name, rt.Runtime, rt.URL)})
		}
	}
	if count == 0 {
		d.checks = append(d.checks, DoctorCheck{Name: "runTargets", Status: "error", Message: "no run targets declared"})
	}
	return d
}

func doctorInspect(root string) doctorBuilder {
	_ = root
	d := doctorBuilder{}
	d.checks = append(d.checks, DoctorCheck{Name: "inspect", Status: "warning", Message: "inspect is experimental"})
	display := os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != ""
	if display {
		d.checks = append(d.checks, DoctorCheck{Name: "platform-webview", Status: "ok", Message: "display session detected", Details: map[string]any{"candidate": "webkitgtk"}})
	} else {
		d.checks = append(d.checks, DoctorCheck{Name: "platform-webview", Status: "warning", Message: "display session missing (DISPLAY/WAYLAND_DISPLAY)", Details: map[string]any{"candidate": "webkitgtk"}})
	}
	d.checks = append(d.checks, DoctorCheck{Name: "cdp", Status: "not_applicable", Message: "available when explicit --cdp endpoint is provided"})
	d.checks = append(d.checks, DoctorCheck{Name: "host-path", Status: "not_applicable", Message: "explicit host path required"})
	return d
}

func printDoctorText(report DoctorReport) {
	fmt.Println("TSPack Doctor")
	fmt.Println()
	fmt.Printf("Project Root\n  root: %s\n\n", report.Root)
	for _, s := range report.Sections {
		fmt.Println(s.Name)
		for _, c := range s.Checks {
			fmt.Printf("  %s: %s\n", c.Name, c.Message)
		}
		fmt.Println()
	}
	fmt.Println("Summary")
	fmt.Printf("  ok: %d\n  warnings: %d\n  errors: %d\n", report.Summary.Ok, report.Summary.Warnings, report.Summary.Errors)
}
