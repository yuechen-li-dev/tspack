package how

import "sort"

type Example struct {
	Label string `json:"label"`
	Text  string `json:"text"`
}

type DiagnosticHelp struct {
	Code            string    `json:"code"`
	Title           string    `json:"title"`
	Summary         string    `json:"summary"`
	Why             string    `json:"why"`
	CommonCauses    []string  `json:"commonCauses,omitempty"`
	Fixes           []string  `json:"fixes,omitempty"`
	BadExamples     []Example `json:"badExamples,omitempty"`
	GoodExamples    []Example `json:"goodExamples,omitempty"`
	RelatedDocs     []string  `json:"relatedDocs,omitempty"`
	RelatedCommands []string  `json:"relatedCommands,omitempty"`
}

var entries = []DiagnosticHelp{
	{Code: "TSPACK_IR_INVALID_RELATIVE_PATH", Title: "Invalid relative package path", Summary: "A manifest path field is not a safe package-relative path.", Why: "Manifest paths define package boundaries, publish content, and generated artifacts. Escaping package roots breaks reproducibility.", CommonCauses: []string{"Using ../ to escape package root.", "Using absolute paths.", "Leaving required library output path empty.", "Using backslashes in package paths."}, Fixes: []string{"Use package-relative paths such as src/index.ts and dist/index.d.ts.", "For app targets with no type output, set types: \"\".", "For library targets, declare a concrete types output like dist/index.d.ts."}, BadExamples: []Example{{Label: "Bad", Text: "runtime: \"../dist/index.js\"\ntypes: \"/tmp/index.d.ts\""}}, GoodExamples: []Example{{Label: "Good", Text: "runtime: \"dist/index.js\"\ntypes: \"dist/index.d.ts\""}}, RelatedDocs: []string{"docs/manifest.md", "docs/init.md"}, RelatedCommands: []string{"tspack check", "tspack init"}},
	{Code: "TSPACK_CHECK_LOCKFILE_MISSING", Title: "Lockfile missing during check", Summary: "tspack check did not find ts-lock.toml.", Why: "The lockfile captures resolved dependency state and enables reproducible builds and CI validation.", CommonCauses: []string{"Fresh repository clone without generated lockfile.", "Lockfile was deleted or not committed.", "Running check before running update in a new workspace."}, Fixes: []string{"Run tspack update to create or refresh ts-lock.toml.", "Commit the lockfile when your workflow requires deterministic CI.", "Treat this warning as actionable even if warnings-only mode exits zero."}, RelatedCommands: []string{"tspack update", "tspack check"}},
	{Code: "TSPACK_LOCK_VERSION_CONFLICT", Title: "Multiple locked versions for one package", Summary: "tspack check found multiple versions for the same package name within one source ecosystem.", Why: "Multiple locked versions can be valid, but often signal duplicated runtime dependencies or peer version drift that increases bundle size and can break singleton assumptions.", CommonCauses: []string{"Different transitive dependency ranges resolved to different versions.", "Gradual upgrades where direct and indirect dependencies are out of alignment.", "Library packages declaring singleton runtime dependencies as dependencies instead of peers."}, Fixes: []string{"Run tspack why <package> to inspect who pulls each version.", "Align dependency ranges across workspace packages and direct dependencies.", "Update lagging packages so transitive ranges converge.", "For libraries, move singleton runtime deps (for example React) to peer dependencies when appropriate.", "Accept the conflict when the versions are intentionally isolated tooling/runtime paths."}, RelatedCommands: []string{"tspack check", "tspack why", "tspack update"}},
	{
		Code:    "TSPACK_BOUNDARY_TOOL_RUNTIME_IMPORT",
		Title:   "Tool dependency imported in runtime source",
		Summary: "Runtime code imported a tool-only dependency.",
		Why:     "Tool dependencies are not part of runtime dependency closure and are not guaranteed at execution time.",
		CommonCauses: []string{
			"Importing build or test tooling from application/library runtime modules.",
			"Confusing tool and runtime dependency kinds in manifest declarations.",
		},
		Fixes: []string{
			"Move the import into build/test/tool scripts.",
			"Replace with a runtime dependency when the import is truly required at runtime.",
			"Change dependency kind deliberately after validating runtime impact.",
			"Remember that boundary from matches the importing file, not the transitive entry graph.",
		},
		RelatedDocs: []string{"docs/boundaries.md"},
	},
	{
		Code:    "TSPACK_BOUNDARY_ALLOW_ONLY_VIOLATION",
		Title:   "External runtime import is not listed in allowOnly",
		Summary: "A runtime import matched an allowOnly boundary row, but the package was not listed.",
		Why:     "allowOnly creates a strict external-package allowlist for a source scope while still allowing relative/internal imports.",
		Fixes: []string{
			"Remove or replace the external import from that source scope.",
			"Move the importing code to a scope where the package is allowed.",
			"Add the package to the matching allowOnly row only if the architecture intentionally permits it.",
			"Declare the package on the target as well; allowOnly is not a dependency declaration.",
			"Run tspack check --explain <file> to inspect matched allowOnly rows and the import decision.",
		},
		RelatedDocs: []string{"docs/boundaries.md", "docs/manifest.md"},
	},
	{
		Code:    "TSPACK_BOUNDARY_TYPE_EXPLICIT_DENY",
		Title:   "Explicit type boundary deny rule",
		Summary: "A type-only external import or re-export matched denyTypeDeps.",
		Why:     "Public and authoring type surfaces can couple consumers to packages that runtime boundaries intentionally hide.",
		Fixes: []string{
			"Move the type behind the correct public target.",
			"Remove public type leakage from the denied package.",
			"Split internal and public types so exported types do not reference internal dependencies.",
			"Adjust denyTypeDeps deliberately if the type exposure is intentional.",
			"Run tspack check --explain <file> to inspect type imports and matching type boundary rules.",
		},
		RelatedDocs: []string{"docs/boundaries.md", "docs/import-scanning.md"},
	},
	{
		Code:    "TSPACK_BOUNDARY_EXPLICIT_DENY",
		Title:   "Explicit runtime boundary deny rule",
		Summary: "A runtime import matched a deny boundary policy.",
		Why:     "Boundary rules keep package contracts enforceable and prevent accidental coupling.",
		Fixes: []string{
			"Move the import behind an allowed package boundary.",
			"Refactor shared logic into a package allowed by policy.",
			"Use a file-set pattern such as from: \"src/**\" when the restriction should apply across a source tree.",
			"If diagnostic details include transitiveFrom, inspect the seed and path details to see which reachable boundary produced the deny.",
			"Update boundary policy deliberately if architecture changed.",
			"Run tspack check --explain <file> to see matched boundary rules, target reachability, and import decisions for the importing file.",
			"Remember that boundary from matches the importing file, not the transitive entry graph; use transitiveFrom only when graph reachability is intended.",
		},
		RelatedDocs: []string{"docs/boundaries.md"},
	},
	{Code: "TSPACK_RUN_TARGET_MISSING", Title: "Run target argument is missing", Summary: "tspack run was invoked without a target name.", Why: "Run requires an explicit target to avoid ambiguous startup behavior.", Fixes: []string{"Provide a target name: tspack run <target>."}},
	{Code: "TSPACK_RUN_TARGET_NOT_FOUND", Title: "Run target not found", Summary: "The requested run target does not exist in manifest IR.", Why: "Run must map to a declared target so commands are reproducible and package-scoped.", Fixes: []string{"Use the exact target name from manifest.", "Check package scoping and update command usage."}},
	{Code: "TSPACK_INIT_FILE_EXISTS", Title: "Init refused to overwrite existing file", Summary: "tspack init detected an existing file it would overwrite.", Why: "Init is conservative to avoid destroying existing project files unintentionally.", Fixes: []string{"Re-run with --force if overwrite is intentional.", "Use an empty directory for first-time initialization."}},
	{Code: "TSPACK_BIOME_BACKEND_NOT_FOUND", Title: "Biome backend not found", Summary: "The Biome backend is not installed or not discoverable.", Why: "Formatting and linting require a working Biome backend for consistent workspace checks.", Fixes: []string{"Add @biomejs/biome as a tool dependency.", "Run tspack sync to materialize dependencies."}},
	{Code: "TSPACK_TEST_MODULE_LOAD_FAILED", Title: "Test module failed to load", Summary: "The test backend could not load a test module.", Why: "Tests must load successfully before assertions can run, otherwise failures are not trustworthy.", Fixes: []string{"Inspect backend error details and fix module initialization issues."}},
	{
		Code:    "TSPACK_SNAPSHOT_MISSING",
		Title:   "Snapshot file is missing",
		Summary: "A native xTest snapshot assertion ran without an existing golden file.",
		Why:     "Snapshot assertions are read-only by default so tests do not create source-controlled files accidentally.",
		Fixes: []string{
			"Review the actual output from the test.",
			"Run tspack test --update-snapshots to write the missing snapshot when the output is intentional.",
			"Commit the generated file under the sibling __snapshots__ directory.",
		},
		RelatedDocs:     []string{"docs/native-test-harness.md", "docs/test-command.md"},
		RelatedCommands: []string{"tspack test --update-snapshots"},
	},
	{
		Code:    "TSPACK_SNAPSHOT_MISMATCH",
		Title:   "Snapshot file differs",
		Summary: "A native xTest snapshot file exists but does not match the current output.",
		Why:     "Golden files protect complex output from accidental drift while requiring explicit review for intentional changes.",
		Fixes: []string{
			"Inspect the first differing line and expected/actual hashes in the diagnostic details.",
			"Fix the regression if the old output is still correct.",
			"Run tspack test --update-snapshots only when the new output is intentional, then review and commit the changed snapshot.",
		},
		RelatedDocs:     []string{"docs/native-test-harness.md", "docs/test-command.md"},
		RelatedCommands: []string{"tspack test --update-snapshots"},
	},
	{
		Code:    "TSPACK_SNAPSHOT_JSON_UNSUPPORTED",
		Title:   "Snapshot JSON value is unsupported",
		Summary: "expect.snapshotJson received a value that cannot be serialized deterministically.",
		Why:     "JSON golden files must be stable and explicit; silently dropping unsupported JavaScript values would hide test bugs.",
		Fixes: []string{
			"Convert the value to plain JSON data before snapshotting.",
			"Use finite numbers, strings, booleans, null, arrays, and plain objects only.",
			"Remove circular references, functions, undefined values, bigint values, symbols, and custom class instances.",
		},
		RelatedDocs: []string{"docs/native-test-harness.md"},
	},
	{Code: "TSPACK_TEST_IMPORT_NOT_FOUND", Title: "Test import not found", Summary: "A test module imports a path or package that cannot be resolved.", Why: "Unresolved imports make test execution nondeterministic and hide broken dependency contracts.", Fixes: []string{"Fix the import path or package name.", "Add the missing dependency in manifest."}},
	{Code: "TSPACK_TEST_THEORY_NO_CASES", Title: "Theory has no cases", Summary: "A native xTest Theory declared a callback body but no direct Case children.", Why: "A zero-case Theory would otherwise pass without exercising the callback, which hides missing coverage.", Fixes: []string{"Add at least one direct <Case /> child under the Theory.", "Ensure Case elements are direct Theory children, not nested inside other JSX."}},
	{Code: "TSPACK_MATERIALIZE_BIN_TARGET_MISSING", Title: "Bin target file is missing", Summary: "A package.json bin entry points at a file that does not exist.", Why: "Materialized bin shims must resolve to real executable files to keep CLI tools runnable.", Fixes: []string{"Update bin path to a real file in package.json.", "Ensure build step produces the referenced file."}},
	{Code: "TSPACK_INSPECT_PLATFORM_WEBVIEW_UNAVAILABLE", Title: "Inspect platform webview unavailable", Summary: "The inspect command could not open an embedded webview on this platform/session.", Why: "Inspect needs a UI or automation backend to render and inspect page state.", Fixes: []string{"Use another inspect backend such as cdp or browser-path options.", "Run inspect on a machine/session with webview support."}},
}

var byCode map[string]DiagnosticHelp

func init() {
	byCode = map[string]DiagnosticHelp{}
	for _, entry := range entries {
		byCode[entry.Code] = entry
	}
}

func Lookup(code string) (DiagnosticHelp, bool) {
	entry, ok := byCode[code]
	return entry, ok
}

func List() []DiagnosticHelp {
	out := append([]DiagnosticHelp(nil), entries...)
	sort.Slice(out, func(i, j int) bool {
		return out[i].Code < out[j].Code
	})
	return out
}
