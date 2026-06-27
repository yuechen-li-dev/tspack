package main

import "fmt"

func printDefaultHelp() {
	fmt.Println("TSPack — TypeScript-first project/package manager")
	fmt.Println()
	fmt.Println("TSPack helps you declare a TypeScript project contract, resolve dependencies,")
	fmt.Println("materialize a lockfile, validate reality, run project targets, and package artifacts.")
	fmt.Println()
	fmt.Println("Common workflows:")
	fmt.Println()
	fmt.Println("Create a project:")
	fmt.Println("  tspack init --template react --name my-app")
	fmt.Println("  cd my-app")
	fmt.Println("  tspack update")
	fmt.Println("  tspack sync")
	fmt.Println("  tspack run dev")
	fmt.Println()
	fmt.Println("Validate a project:")
	fmt.Println("  tspack check")
	fmt.Println("  tspack check --format")
	fmt.Println("  tspack why react")
	fmt.Println()
	fmt.Println("Prepare a package:")
	fmt.Println("  tspack pack --dry-run --package @scope/pkg")
	fmt.Println("  tspack pack --verify --package @scope/pkg")
	fmt.Println()
	fmt.Println("Core commands:")
	fmt.Println("  init       Create a project from an inert template")
	fmt.Println("  update     Resolve dependencies into ts-lock.toml")
	fmt.Println("  sync       Materialize locked dependencies and hydrate missing local store artifacts")
	fmt.Println("  check      Validate manifest, lockfile, security policy, and formatting")
	fmt.Println("  run        Run declared RunTargets")
	fmt.Println("  why        Explain why a dependency exists")
	fmt.Println("  pack       Create or verify package artifacts")
	fmt.Println("  doctor     Diagnose project/runtime/security/inspect setup")
	fmt.Println("  outdated   Show available dependency updates")
	fmt.Println("  format     Run formatter over project files")
	fmt.Println("  lint       Run linter over project files")
	fmt.Println("  inspect    Inspect browser/app/runtime targets (experimental)")
	fmt.Println()
	fmt.Println("Reference shortcuts:")
	fmt.Println("  tspack check [--root .] [--format] [--show-conflicts] [--show-lifecycle]")
	fmt.Println("  tspack run [target] [--root .]")
	fmt.Println("  tspack inspect <url> [experimental]")
	fmt.Println("  tspack format [paths...] [--root .] [--check]")
	fmt.Println("  tspack lint [paths...] [--root .] [--fix]")
	fmt.Println("  tspack doctor [format|run|runtime|inspect|security]")
	fmt.Println("  tspack test [--root .]")
	fmt.Println("  tspack artifact [--root .]")
	fmt.Println("  tspack bench [--root .]")
	fmt.Println("  tspack doom [--root .]")
	fmt.Println()
	fmt.Println("Advanced flags:")
	fmt.Println("  --show-conflicts   Show individual version conflict diagnostics instead of summary")
	fmt.Println("  --show-lifecycle   Show individual lifecycle script diagnostics instead of summary")
	fmt.Println()
	fmt.Println("Learn more:")
	fmt.Println("  tspack help workflow")
	fmt.Println("  tspack help concepts")
	fmt.Println("  tspack help commands")
	fmt.Println("  tspack help init")
	fmt.Println("  tspack help all")
	fmt.Println("  tspack how <diagnostic-code>")
	fmt.Println("  tspack --version")
	fmt.Println()
	fmt.Println("Need help with an error?")
	fmt.Println("  tspack how TSPACK_SECURITY_LIFECYCLE_SCRIPT_PRESENT")
	fmt.Println("  tspack how TSPACK_FORMAT_BACKEND_MISSING")
}

func printCommandsHelp() {
	fmt.Println("TSPack commands")
	fmt.Println()
	fmt.Println("Project setup:")
	fmt.Println("  init        Create a new project from a template")
	fmt.Println("  migrate     Convert package.json/package-lock evidence into manifest suggestions")
	fmt.Println()
	fmt.Println("Dependency lifecycle:")
	fmt.Println("  update      Resolve dependencies and update ts-lock.toml")
	fmt.Println("  sync        Materialize dependencies from ts-lock.toml")
	fmt.Println("  outdated    Show available updates")
	fmt.Println("  why         Explain dependency graph paths and decisions")
	fmt.Println()
	fmt.Println("Validation and diagnostics:")
	fmt.Println("  check       Validate project contract, lockfile, security policy, and optionally format")
	fmt.Println("  doctor      Diagnose project/runtime/security/inspect setup")
	fmt.Println("  how         Explain diagnostic codes")
	fmt.Println("  format      Run formatter over project files")
	fmt.Println()
	fmt.Println("Execution and testing:")
	fmt.Println("  run         Run declared RunTargets")
	fmt.Println("  test        Run xTest/Vitest-compatible project tests")
	fmt.Println("  inspect     Inspect browser/app/runtime targets")
	fmt.Println("  list        List project packages/apps")
	fmt.Println()
	fmt.Println("Packaging:")
	fmt.Println("  pack        Create or verify package artifacts")
	fmt.Println("  artifact    Create/list/filter artifact records")
	fmt.Println()
	fmt.Println("Maintenance / info:")
	fmt.Println("  help        Show help")
	fmt.Println("  --version   Show version")
}

func printWorkflowHelp() {
	fmt.Println("TSPack project lifecycle:")
	fmt.Println()
	fmt.Println("init")
	fmt.Println("  Creates manifest.tsx and starter project files.")
	fmt.Println("update")
	fmt.Println("  Resolves declared dependencies and writes ts-lock.toml.")
	fmt.Println("sync")
	fmt.Println("  Materializes dependencies from ts-lock.toml into the local project/store layout.")
	fmt.Println("  When a required local store artifact is missing, sync hydrates it from the locked source without changing dependency resolution.")
	fmt.Println("check")
	fmt.Println("  Validates that the manifest, lockfile, dependency graph, security policy,")
	fmt.Println("  and generated/tooling files still agree.")
	fmt.Println("run")
	fmt.Println("  Executes named RunTargets declared in manifest.tsx.")
	fmt.Println("pack")
	fmt.Println("  Builds/verifies package artifacts from the manifest contract.")
	fmt.Println()
	fmt.Println("Typical app:")
	fmt.Println("  tspack init --template react --name my-app")
	fmt.Println("  tspack update")
	fmt.Println("  tspack sync")
	fmt.Println("  tspack check")
	fmt.Println("  tspack run dev")
	fmt.Println()
	fmt.Println("Typical library:")
	fmt.Println("  tspack init --template react-library --name ui-kit --package @local/ui-kit")
	fmt.Println("  tspack update")
	fmt.Println("  tspack sync")
	fmt.Println("  tspack run typecheck")
	fmt.Println("  tspack run build")
	fmt.Println("  tspack run build-types")
	fmt.Println("  tspack pack --verify --package @local/ui-kit")
}

func printConceptsHelp() {
	fmt.Println("TSPack concepts")
	fmt.Println()
	fmt.Println("Manifest:")
	fmt.Println("  manifest.tsx is the project contract. It declares packages, dependencies,")
	fmt.Println("  targets, RunTargets, security policy, and update policy.")
	fmt.Println("Lockfile:")
	fmt.Println("  ts-lock.toml records resolved dependency reality.")
	fmt.Println("Update:")
	fmt.Println("  tspack update changes resolution state and lockfile state.")
	fmt.Println("Sync:")
	fmt.Println("  tspack sync materializes what the lockfile says should exist.")
	fmt.Println("Check:")
	fmt.Println("  tspack check validates that project reality matches the manifest/lockfile.")
	fmt.Println("Templates:")
	fmt.Println("  tspack init templates are inert scaffolds. They do not run commands,")
	fmt.Println("  install dependencies, or fetch remote code.")
	fmt.Println("Concepts:")
	fmt.Println("  Template concepts describe generated project shape, such as react.app,")
	fmt.Println("  react.library, vite.app, package.exports, or tspack.pack.")
	fmt.Println("Security:")
	fmt.Println("  Lifecycle scripts are detected and audited. Execution remains blocked by default.")
	fmt.Println("RunTargets:")
	fmt.Println("  Named commands declared in the manifest and executed through tspack run.")
	fmt.Println("Diagnostics:")
	fmt.Println("  Use tspack how <code> to explain TSPack diagnostic codes.")
}

func printHelpTopic(topic string) bool {
	switch topic {
	case "", "help":
		printDefaultHelp()
	case "commands":
		printCommandsHelp()
	case "workflow":
		printWorkflowHelp()
	case "concepts":
		printConceptsHelp()
	case "all", "legacy", "flags":
		printLegacyHelp()
	case "init", "update", "sync", "check", "run", "why", "outdated", "pack", "doctor", "test", "inspect", "migrate", "format", "how":
		printCommandHelp(topic)
	default:
		return false
	}
	return true
}

func printCommandHelp(command string) {
	pages := map[string][]string{
		"init":     {"tspack init", "Creates a TSPack project from an inert template.", "Use when starting a project or applying a local scaffold.", "Examples:", "  tspack init --template static --name hello-static", "  tspack init --template react --name my-app", "  tspack init --template react-library --name ui-kit --package @local/ui-kit", "  tspack init --template ./my-template --name custom", "  tspack init --list-templates", "Templates:", "  static          TypeScript + Vite static browser app", "  react           React + Vite + TypeScript app", "  react-library   React + Vite + TypeScript component library", "Safety:", "  Templates do not run commands, install dependencies, or fetch remote code.", "Important flags:", "  --root <path>        Destination root", "  --template <name>    Built-in template or local template directory", "  --name <name>        Project name", "  --package <name>     Package name for library templates", "  --force              Overwrite existing files", "Related:", "  update          Resolve generated dependencies", "  sync            Materialize generated dependencies", "  check           Validate generated project"},
		"update":   {"tspack update", "Resolves declared dependencies and writes ts-lock.toml.", "Use after editing manifest.tsx dependencies or when checking available policy-approved changes.", "Examples:", "  tspack update", "  tspack update react --dry-run", "Important flags:", "  --root <path>   Project root", "  --dry-run       Preview lockfile changes", "  --json          Emit machine-readable output", "  --quiet         Reduce output", "Related:", "  sync            Materialize the lockfile", "  outdated        Show available updates", "  check           Validate the result"},
		"sync":     {"tspack sync", "Materializes dependencies from ts-lock.toml.", "Use after:", "  tspack update", "Examples:", "  tspack sync", "  tspack sync --clean", "Behavior:", "  sync materializes project tool shims in node_modules/.bin for declared tool dependencies.", "  On fresh machines, sync hydrates missing local store artifacts from the locked source without changing versions or rewriting ts-lock.toml.", "  On Windows, sync retries transient locked-file replacement failures before reporting a diagnostic.", "Important flags:", "  --root <path>   Project root", "  --clean         Rebuild materialized dependency layout", "Related:", "  update          Resolve dependencies into ts-lock.toml", "  check           Validate the current project"},
		"check":    {"tspack check", "Validates manifest, lockfile, dependency graph, security policy, and optional formatting.", "Use before running, packaging, or committing project contract changes.", "Examples:", "  tspack check", "  tspack check --format", "  tspack check --show-conflicts", "  tspack how <code>", "Important flags:", "  --root <path>       Project root", "  --format            Run read-only format validation", "  --show-conflicts    Show individual version conflict diagnostics", "  --show-lifecycle    Show individual lifecycle script diagnostics", "  --json              Emit JSON diagnostics", "Related:", "  how             Explain diagnostic codes", "  format          Apply formatting", "  doctor          Diagnose local setup"},
		"run":      {"tspack run", "Runs named RunTargets declared in manifest.tsx.", "Use for dev servers, builds, typechecks, and other project commands owned by the manifest.", "Examples:", "  tspack run dev", "  tspack run build", "  tspack run --list", "Behavior:", "  RunTargets are manifest-declared runtime targets, not package.json scripts.", "  Targets with ready checks are server targets; targets without ready checks are finite commands.", "  --once proves a server target becomes ready, then stops its process tree.", "  For runtime node/nodejs targets, run prepends the project tool bin from sync before host PATH.", "  Env contracts validate required variables before execution, inject defaults, and redact secrets.", "Important flags:", "  --root <path>              Project root", "  --package <name>           Select package", "  --env KEY=VALUE            Add environment variables", "  --once                     Stop after readiness", "  --ready-timeout <seconds>  Readiness timeout", "Related:", "  inspect         Inspect a running target", "  check           Validate targets before running"},
		"why":      {"tspack why", "Explains why a dependency exists and how it is reached.", "Use when auditing dependency graph decisions or investigating lockfile entries.", "Examples:", "  tspack why react", "  tspack why --reverse vite", "Important flags:", "  --root <path>      Project root", "  --package <name>   Start from one package", "  --reverse          Show reverse paths", "  --json             Emit JSON output", "Related:", "  outdated        Compare available versions", "  update          Change resolution state"},
		"outdated": {"tspack outdated", "Shows available dependency updates.", "Use before update planning or policy review.", "Examples:", "  tspack outdated", "  tspack outdated --json", "Important flags:", "  --root <path>   Project root", "  --json          Emit JSON output", "Related:", "  update          Apply resolution changes", "  why             Explain current dependencies"},
		"pack":     {"tspack pack", "Creates or verifies package artifacts from the manifest contract.", "Use before publishing or release validation.", "Examples:", "  tspack pack --dry-run --package @scope/pkg", "  tspack pack --verify --package @scope/pkg", "Important flags:", "  --root <path>      Project root", "  --out <dir>        Output directory", "  --package <name>   Package to pack", "  --dry-run          Preview artifact plan", "  --verify           Verify package contents", "Related:", "  artifact        Inspect artifact records", "  check           Validate package contract"},
		"doctor":   {"tspack doctor", "Diagnoses project, runtime, security, formatting, and inspect setup.", "Use when local tooling or environment behavior looks wrong.", "Examples:", "  tspack doctor", "  tspack doctor inspect", "  tspack doctor security --json", "Important flags:", "  --root <path>   Project root", "  --json          Emit JSON output", "Related:", "  check           Validate project state", "  how             Explain diagnostic codes"},
		"test":     {"tspack test", "Runs xTest/Vitest-compatible project tests.", "Use for project tests declared in the TSPack test flow.", "Examples:", "  tspack test", "  tspack test --list", "  tspack test --filter login", "Important flags:", "  --root <path>          Project root", "  --list                 List tests", "  --filter <text>        Filter tests", "  --update-snapshots     Update snapshots", "Related:", "  check           Validate before testing", "  run             Run project targets"},
		"inspect":  {"tspack inspect", "Inspects browser/app/runtime targets (experimental).", "Use to inspect a URL or a RunTarget-backed app with browser backends.", "Usage:", "  tspack inspect <url> [experimental] [--run target] [--browser backend]", "Examples:", "  tspack inspect https://example.test", "  tspack inspect --run dev --url http://localhost:5173", "Important flags:", "  --run <target>        Start a RunTarget before inspecting", "  --browser <backend>   Select browser backend", "  --selector <css>      Inspect a selector", "  --json                Emit JSON output", "Related:", "  run             Start targets", "  doctor inspect  Diagnose inspect setup"},
		"migrate":  {"tspack migrate", "Converts package.json/package-lock evidence into manifest suggestions.", "Use when adopting TSPack in an existing JavaScript or TypeScript project.", "Examples:", "  tspack migrate --check", "  tspack migrate --write", "Important flags:", "  --root <path>          Project root", "  --package-json <path>  package.json evidence", "  --package-lock <path>  package-lock evidence", "  --scan-source          Include source import evidence", "  --write                Write suggestions", "Related:", "  init            Start new projects", "  check           Validate migrated contracts"},
		"format":   {"tspack format", "Runs formatter over project files.", "Use to apply or check the formatting policy used by tspack check --format.", "Examples:", "  tspack format", "  tspack format --check", "Important flags:", "  --root <path>   Project root", "  --check         Check without writing", "Related:", "  check --format  Include formatting in validation", "  doctor format   Diagnose formatter backend"},
		"how":      {"tspack how", "Explains TSPack diagnostic codes.", "Use when check, doctor, update, or another command reports a TSPack_* code.", "Examples:", "  tspack how TSPACK_SECURITY_LIFECYCLE_SCRIPT_PRESENT", "  tspack how TSPACK_FORMAT_BACKEND_MISSING", "  tspack how --list", "Important flags:", "  --json          Emit JSON output", "Related:", "  check           Produces diagnostics", "  doctor          Produces environment diagnostics"},
	}
	for _, line := range pages[command] {
		fmt.Println(line)
	}
}
