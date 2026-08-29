import {
	Build,
	CollectAll,
	CurrentHost,
	Finally,
	ForEach,
	GreaterThan,
	Manual,
	MatchResult,
	NotEmpty,
	On,
	Package,
	ParallelForEach,
	Process,
	Sequence,
	Test,
	Transfer,
	When,
	Windows,
	Workflow,
	Workflows,
	Workspace,
	define,
} from "tspack/manifest";

const runs = ForEach(
	"platform",
	[CurrentHost(), Windows()],
	platform => On(platform, Test()),
	{
		mode: ParallelForEach({ concurrency: 2 }),
		failure: CollectAll(),
	},
);
const first = runs[0];
const build = Build();

export default define(
	<Workspace name="workflow-m79" runtime="nodejs">
		<Workflows
			rows={[
				Workflow("M79", {
					triggers: [Manual()],
					flow: Finally(
						Sequence(
							runs,
							MatchResult(first, {
								succeeded: result =>
									When(
										GreaterThan(result.failed, 0),
										Process("record first failed tests", {
											command: ["node", "-e", "console.log('first failed tests')"],
											capabilities: ["process", "workspaceRead"],
										}),
									),
								failed: () => Process("record first failure", { command: ["node", "--version"], capabilities: ["process", "workspaceRead"] }),
								cancelled: () => Process("record first cancellation", { command: ["node", "--version"], capabilities: ["process", "workspaceRead"] }),
								timedOut: () => Process("record first timeout", { command: ["node", "--version"], capabilities: ["process", "workspaceRead"] }),
							}),
							ForEach("run", runs, run =>
								MatchResult(run, {
									succeeded: result =>
										When(
											GreaterThan(result.failed, 0),
											Process("record failed tests", { command: ["node", "--version"], capabilities: ["process", "workspaceRead"] }),
											Process("record successful tests", { command: ["node", "--version"], capabilities: ["process", "workspaceRead"] }),
										),
									failed: () => Process("record failed iteration", { command: ["node", "--version"], capabilities: ["process", "workspaceRead"] }),
									cancelled: () => Process("record cancelled iteration", { command: ["node", "--version"], capabilities: ["process", "workspaceRead"] }),
									timedOut: () => Process("record timed out iteration", { command: ["node", "--version"], capabilities: ["process", "workspaceRead"] }),
								}),
							),
							ForEach("platform-config", [CurrentHost(), Windows()], platform =>
								ForEach("configuration", ["debug", "release"], configuration =>
									On(
										platform,
										Process("nested build", {
											command: ["node", "-e", "console.log('nested build')"],
											capabilities: ["process", "workspaceRead"],
										}),
									),
								),
							),
							build,
							MatchResult(build, {
								succeeded: result => When(
									NotEmpty(result.artifacts),
									Transfer(result.artifacts, Windows()),
								),
								failed: () => Process("record build failure", { command: ["node", "--version"], capabilities: ["process", "workspaceRead"] }),
								cancelled: () => Process("record build cancellation", { command: ["node", "--version"], capabilities: ["process", "workspaceRead"] }),
								timedOut: () => Process("record build timeout", { command: ["node", "--version"], capabilities: ["process", "workspaceRead"] }),
							}),
						),
						Process("cleanup M79 fixture", {
							command: ["node", "-e", "console.log('cleanup')"],
							capabilities: ["process", "workspaceRead"],
						}),
					),
				}),
			]}
		/>
		<Package name="workflow-m79" version="1.0.0" kind="app" />
	</Workspace>,
);
