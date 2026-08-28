import {
	Check,
	CurrentHost,
	Job,
	Manual,
	Package,
	Process,
	PullRequest,
	Secret,
	Workflow,
	WorkflowEnv,
	Workflows,
	Workspace,
	define,
} from "tspack/manifest";

export default define(
	<Workspace name="workflow-m74" runtime="nodejs">
		<Workflows
			rows={[
				Workflow("CI", {
					triggers: [Manual(), PullRequest({ branches: ["main"], paths: ["src/**"] })],
					jobs: [
						Job("validate", {
							runsOn: CurrentHost(),
							steps: [Check()],
						}),
						Job("external", {
							needs: ["validate"],
							runsOn: CurrentHost(),
							steps: [
								Process("verify environment contract", {
									command: [
										"node",
										"-e",
										"if (!process.env.M74_TOKEN) process.exit(2); console.log('fixture-ok')",
									],
									cwd: "workspace",
									env: [WorkflowEnv("M74_TOKEN", Secret("M74_TOKEN"))],
									capabilities: ["process", "environment", "secrets", "workspaceRead"],
								}),
							],
						}),
					],
				}),
			]}
		/>
		<Package name="workflow-fixture" version="1.0.0" kind="app" />
	</Workspace>,
);
