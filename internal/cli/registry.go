package cli

// commandHandlers is the single registration point for executable commands.
// Help topics remain presentation data in help.go; handlers own parsing and
// application orchestration for their command.
var commandHandlers = map[string]func([]string){
	"add":              runAddCommand,
	"adopt":            runAdoptCommand,
	"artifact":         runArtifactCommand,
	"audit":            runAuditCommand,
	"bench":            runBenchCommand,
	"build":            runBuildCommand,
	"check":            runCheckCommand,
	"compat":           runCompatCommand,
	"doctor":           runDoctorCommand,
	"doom":             runDoomCommand,
	"format":           runFormatCommand,
	"how":              runHowCommand,
	"init":             runInitCommand,
	"inspect":          runInspectCommand,
	"lint":             runLintCommand,
	"materialize-tree": runMaterializeTreeCommand,
	"migrate":          runMigrateCommand,
	"npm":              runNpmCommand,
	"outdated":         runOutdatedCommand,
	"pack":             runPackCommand,
	"run":              runRunCommand,
	"scenario":         runScenarioCommand,
	"skyrim":           runSkyrimFixtureCommand,
	"sync":             runSyncCommand,
	"test":             runTestCommand,
	"update":           runUpdateCommand,
	"why":              runWhyCommand,
}
