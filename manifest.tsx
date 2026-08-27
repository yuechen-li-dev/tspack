// biome-ignore assist/source/organizeImports: Manifest imports are organized by TSPack and should not be auto-organized by IDEs or formatters.
import {
	CompatFiles,
	JsonFile,
	Package,
	RunTargets,
	Security,
	TsConfig,
	Tools,
	UpdatePolicy,
	VSCode,
	Workspace,
	define,
	defineDeps,
	dep,
	npm,
	tool,
} from "tspack/manifest";

const deps = defineDeps({
	manifestFrontendTypescript: tool(npm("typescript", "^5.9.0"), { key: "typescript" }),
	manifestFrontendVitest: tool(npm("vitest", "^3.2.6"), { key: "vitest" }),
	manifestFrontendPlaywright: tool(npm("playwright", "^1.54.0"), { key: "playwright" }),
	biome: tool(npm("@biomejs/biome", "^2.5.1"), { key: "@biomejs/biome" }),
	vscodeTypescript: tool(npm("typescript", "^5.9.0"), { key: "vscode-typescript" }),
	vscodeVitest: tool(npm("vitest", "^3.2.6"), { key: "vscode-vitest" }),
	vscodeNodeTypes: tool(npm("@types/node", "^22.10.2"), { key: "@types/node" }),
	vscodeApiTypes: tool(npm("@types/vscode", "^1.92.0"), { key: "@types/vscode" }),
});

export default define(
	<Workspace name="tspack" runtime="nodejs">
		<CompatFiles>
			<JsonFile
				path="tsconfig.tspack.json"
				value={TsConfig.manifestEditor({
					include: [
						"manifest.tsx",
						"examples/compat-json-basic/manifest.tsx",
						"examples/incremental-existing-react/manifest.tsx",
						"examples/incremental-existing-monorepo/manifest.tsx",
						"examples/incremental-existing-monorepo/packages/ui/package.manifest.tsx",
						"examples/nestjs-service/manifest.tsx",
						"examples/runtime-switch-notes/manifest.tsx",
						"examples/runtime-switch-notes/tests/inspect.xtest.tsx",
						"examples/runtime-switch-notes/tests/runtime-switch.xtest.tsx",
						"examples/update-policy-notes/manifest.tsx",
						".tspack/types/**/*.d.ts",
					],
					exclude: [
						"node_modules/**",
						"dist/**",
						".tspack/store/**",
						"tspack-artifacts/**",
						"fixtures/**",
						"testdata/**",
						"manifest-frontend/**/fixtures/**",
						"manifest-frontend/**/testdata/**",
					],
				})}
			/>
			<JsonFile
				path=".vscode/settings.json"
				value={VSCode.settings({
					typescriptTsdk: "manifest-frontend/node_modules/typescript/lib",
				})}
			/>
			<JsonFile path=".vscode/extensions.json" value={VSCode.extensions()} />
		</CompatFiles>
		<Security
			acknowledgedLifecycleCategories={[
				{
					category: "maintainer-publish",
					scripts: ["prepare", "prepublishOnly"],
					reason:
						"Maintainer-side lifecycle scripts are blocked by TSPack and do not execute during consumer install/update.",
				},
			]}
		/>
		<UpdatePolicy
			rows={[
				{
					name: "typescript",
					kind: "tool",
					strategy: "rolling",
					level: "minor",
					reason:
						"Compiler updates may roll within the current major after the manifest frontend and extension typechecks pass.",
				},
				{
					name: "vitest",
					kind: "tool",
					strategy: "rolling",
					level: "minor",
					reason:
						"Test runner updates may roll within the current major after the Go and TypeScript test matrix passes.",
				},
				{
					name: "playwright",
					kind: "tool",
					strategy: "manual",
					reason:
						"Browser and inspect behavior is environment-sensitive and reviewed manually post-0.1.0.",
				},
				{
					name: "@types/node",
					kind: "tool",
					strategy: "rolling",
					level: "minor",
					reason: "Node type updates are tooling-only and can roll with TypeScript validation.",
				},
				{
					name: "@biomejs/biome",
					kind: "tool",
					strategy: "manual",
					reason:
						"Formatter backend updates stay manual because native/lifecycle behavior should be reviewed with security gates.",
				},
				{
					name: "@types/vscode",
					kind: "tool",
					strategy: "manual",
					reason: "VS Code API type updates are coordinated with extension compatibility testing.",
				},
				{
					name: "react",
					kind: "dep",
					strategy: "manual",
					reason:
						"Example runtime framework updates stay manual and do not drive root self-host policy mutation.",
				},
			]}
		/>

		<Package name="@tspack/cli" version="0.1.0" license="MIT" kind="app">
			<RunTargets
				rows={[
					{
						name: "go-test",
						runtime: "system",
						command: ["sh", "-c", "go test ./cmd/... ./internal/... ./tools/... && echo TSPACK_READY"],
						url: "",
						cwd: "workspace",
						ready: { kind: "stdout-match", pattern: "TSPACK_READY", stream: "stdout" },
					},
					{
						name: "release-build",
						runtime: "system",
						command: ["sh", "-c", "./scripts/build-release.sh && echo TSPACK_READY"],
						url: "",
						cwd: "workspace",
						ready: { kind: "stdout-match", pattern: "TSPACK_READY", stream: "stdout" },
					},
					{
						name: "check-self",
						runtime: "system",
						command: ["sh", "-c", "go run ./cmd/tspack check --root . && echo TSPACK_READY"],
						url: "",
						cwd: "workspace",
						ready: { kind: "stdout-match", pattern: "TSPACK_READY", stream: "stdout" },
					},
					{
						name: "check-format-self",
						runtime: "system",
						command: [
							"sh",
							"-c",
							"go run ./cmd/tspack check --format --root . && echo TSPACK_READY",
						],
						url: "",
						cwd: "workspace",
						ready: { kind: "stdout-match", pattern: "TSPACK_READY", stream: "stdout" },
					},
					{
						name: "doctor-security-self",
						runtime: "system",
						command: [
							"sh",
							"-c",
							"go run ./cmd/tspack doctor security --root . && echo TSPACK_READY",
						],
						url: "",
						cwd: "workspace",
						ready: { kind: "stdout-match", pattern: "TSPACK_READY", stream: "stdout" },
					},
					{
						name: "policy-plan-self",
						runtime: "system",
						command: [
							"sh",
							"-c",
							"go run ./cmd/tspack update --policy --dry-run --root . && echo TSPACK_READY",
						],
						url: "",
						cwd: "workspace",
						ready: { kind: "stdout-match", pattern: "TSPACK_READY", stream: "stdout" },
					},
					{
						name: "package-release-linux-amd64",
						runtime: "system",
						command: [
							"sh",
							"-c",
							"./scripts/package-release.sh --goos linux --goarch amd64 --version v0.0.0-selfhost --out-dir dist/selfhost-test && echo TSPACK_READY",
						],
						url: "",
						cwd: "workspace",
						ready: { kind: "stdout-match", pattern: "TSPACK_READY", stream: "stdout" },
					},
				]}
			/>
		</Package>

		<Package
			name="@tspack/manifest-frontend"
			version="0.0.0"
			license="MIT"
			kind="app"
			dependencies={{
				values: [
					deps.manifestFrontendTypescript,
					deps.manifestFrontendVitest,
					deps.manifestFrontendPlaywright,
					deps.biome,
				],
			}}
		>
			<Tools
				values={[
					deps.manifestFrontendTypescript,
					deps.manifestFrontendVitest,
					deps.manifestFrontendPlaywright,
					deps.biome,
				]}
			/>
			<RunTargets
				rows={[
					{
						name: "frontend-build",
						runtime: "system",
						command: ["sh", "-c", "cd manifest-frontend && npm run build && echo TSPACK_READY"],
						url: "",
						cwd: "workspace",
						ready: { kind: "stdout-match", pattern: "TSPACK_READY", stream: "stdout" },
					},
					{
						name: "frontend-typecheck",
						runtime: "system",
						command: [
							"sh",
							"-c",
							"cd manifest-frontend && npm run typecheck:manifest-api && echo TSPACK_READY",
						],
						url: "",
						cwd: "workspace",
						ready: { kind: "stdout-match", pattern: "TSPACK_READY", stream: "stdout" },
					},
					{
						name: "frontend-test",
						runtime: "system",
						command: ["sh", "-c", "cd manifest-frontend && npm test && echo TSPACK_READY"],
						url: "",
						cwd: "workspace",
						ready: { kind: "stdout-match", pattern: "TSPACK_READY", stream: "stdout" },
					},
				]}
			/>
		</Package>

		<Package
			name="@tspack/vscode-extension"
			version="0.0.1"
			license="MIT"
			kind="app"
			dependencies={{
				values: [
					deps.vscodeTypescript,
					deps.vscodeVitest,
					deps.vscodeNodeTypes,
					deps.vscodeApiTypes,
				],
			}}
		>
			<Tools
				values={[
					deps.vscodeTypescript,
					deps.vscodeVitest,
					deps.vscodeNodeTypes,
					deps.vscodeApiTypes,
				]}
			/>
			<RunTargets
				rows={[
					{
						name: "extension-test",
						runtime: "system",
						command: ["sh", "-c", "cd extensions/tspack-vscode && npm test && echo TSPACK_READY"],
						url: "",
						cwd: "workspace",
						ready: { kind: "stdout-match", pattern: "TSPACK_READY", stream: "stdout" },
					},
					{
						name: "extension-compile",
						runtime: "system",
						command: [
							"sh",
							"-c",
							"cd extensions/tspack-vscode && npm run compile && echo TSPACK_READY",
						],
						url: "",
						cwd: "workspace",
						ready: { kind: "stdout-match", pattern: "TSPACK_READY", stream: "stdout" },
					},
				]}
			/>
		</Package>

		<Package
			name="@tspack/examples-runtime-switch-notes"
			version="0.1.0"
			license="MIT"
			kind="app"
			dependencies={{ values: [] }}
		/>
		<Package
			name="@tspack/examples-update-policy-notes"
			version="0.1.0"
			license="MIT"
			kind="app"
			dependencies={{ values: [dep(npm("react", "^18.3.0"))] }}
		/>
	</Workspace>,
);
