import {
	Audit,
	Build,
	CollectAll,
	CurrentHost,
	Finally,
	ForEach,
	GreaterThan,
	Manual,
	On,
	Pack,
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

const build = Build();
const portable = Transfer(build.artifacts, Windows());
const audit = Audit();

export default define(
	<Workspace name="workflow-m78" runtime="nodejs">
		<Workflows
			rows={[
				Workflow("M78", {
					triggers: [Manual()],
					flow: Finally(
						Sequence(
							ForEach(
								"platform",
								[CurrentHost(), Windows()],
								platform => On(platform, Test()),
								{
									mode: ParallelForEach({ concurrency: 2 }),
									failure: CollectAll(),
								},
							),
							build,
							portable,
							On(Windows(), Pack(portable.artifacts)),
							audit,
							When(
								GreaterThan(audit.failing, 0),
								Process("report blocking audit findings", {
									command: ["node", "-e", "console.log('blocking findings')"],
									capabilities: ["process", "workspaceRead"],
								}),
							),
						),
						Process("cleanup workflow transport", {
							command: ["node", "-e", "console.log('cleanup')"],
							capabilities: ["process", "workspaceRead"],
						}),
					),
				}),
			]}
		/>
		<Package name="workflow-m78" version="1.0.0" kind="app" />
	</Workspace>,
);
