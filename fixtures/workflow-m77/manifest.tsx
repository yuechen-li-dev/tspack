import {
	Audit,
	Branch,
	Build,
	Finally,
	ForEach,
	Manual,
	MatchResult,
	Package,
	Pack,
	Parallel,
	Process,
	Sequence,
	Test,
	Workflow,
	Workflows,
	Workspace,
	define,
} from "tspack/manifest";

const build = Build();

export default define(
	<Workspace name="workflow-m77" runtime="nodejs">
		<Workflows
			rows={[
				Workflow("CI", {
					triggers: [Manual()],
					flow: Finally(
						Sequence(
							Process("prepare workspace", {
								command: ["node", "-e", "console.log('prepare')"],
								capabilities: ["process", "workspaceRead"],
							}),
							Parallel(
								Branch("test", Test()),
								Branch("build", build),
							),
							MatchResult(build, {
								succeeded: result => Pack(result.artifacts),
								failed: () =>
									Process("report build failure", {
										command: ["node", "-e", "console.log('build failed')"],
										capabilities: ["process", "workspaceRead"],
									}),
								cancelled: () =>
									Process("report build cancellation", {
										command: ["node", "-e", "console.log('build cancelled')"],
										capabilities: ["process", "workspaceRead"],
									}),
								timedOut: () =>
									Process("report build timeout", {
										command: ["node", "-e", "console.log('build timed out')"],
										capabilities: ["process", "workspaceRead"],
									}),
							}),
							ForEach("suite", ["unit", "integration"], suite =>
								Test({ filter: suite }),
							),
							Audit(),
						),
						Process("cleanup workspace", {
							command: ["node", "-e", "console.log('cleanup')"],
							capabilities: ["process", "workspaceRead"],
						}),
					),
				}),
			]}
		/>
		<Package name="workflow-m77" version="1.0.0" kind="app" />
	</Workspace>,
);
