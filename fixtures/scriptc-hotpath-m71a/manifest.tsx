import {
	Package,
	RunTargets,
	Targets,
	Tools,
	Workspace,
	define,
	defineDeps,
	npm,
	tool,
} from "tspack/manifest";

const deps = defineDeps({
	scriptc: tool(npm("scriptc", "0.0.35"), { key: "scriptc" }),
	typescript: tool(npm("typescript", "5.9.3"), { key: "typescript" }),
	nodeTypes: tool(npm("@types/node", "24.3.0"), { key: "@types/node" }),
});

export default define(
	<Workspace name="scriptc-hotpath-m71a" runtime="nodejs">
		<Package
			name="scriptc-hotpath-m71a"
			version="1.0.0"
			kind="app"
			dependencies={{ values: [deps.scriptc, deps.typescript, deps.nodeTypes] }}
		>
			<Tools values={[deps.scriptc, deps.typescript, deps.nodeTypes]} />
			<Targets
				rows={[
					{
						name: "app",
						language: "typescript",
						compiler: "tsc",
						compilerConfig: "tsconfig.json",
						inputs: ["src/app/**"],
						dependsOn: ["hotpath"],
						export: ".",
						entry: "src/app/main.ts",
						runtime: "dist/app/main.js",
						types: "",
						deps: [],
						peers: [],
					},
					{
						name: "hotpath",
						language: "scriptc",
						compiler: "scriptc",
						compilerConfig: "scriptc.json",
						inputs: ["src/hot/**"],
						artifact: "nativeExecutable",
						export: "./hotpath",
						entry: "src/hot/compute.ts",
						runtime: "dist/hotpath.exe",
						types: "",
						deps: [],
						peers: [],
					},
				]}
			/>
			<RunTargets
				rows={[
					{
						name: "start",
						runtime: "node",
						cwd: "package",
						command: ["node", "dist/app/main.js"],
						url: "",
					},
				]}
			/>
		</Package>
	</Workspace>,
);
