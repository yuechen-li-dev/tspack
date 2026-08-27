# Security and capability policy (M14)

## Core principle: fetch is not execute

TSPack fetches package metadata/content and records deterministic lockfile truth.
It does not execute package lifecycle scripts.

Allowed behavior:
- fetch metadata
- fetch tarballs
- read `package.json`
- inspect lifecycle scripts
- record package capabilities in `ts-lock.toml`
- materialize files into `node_modules`

Forbidden behavior:
- running `preinstall/install/postinstall`
- running `prepare/prepack/postpack/prepublish`
- invoking shell/node/powershell to execute dependency package code
- native addon compilation as install side effects
- binary download side effects from lifecycle scripts

## Capability model

Lockfile package entries include:

- `capability.kind`
- `capability.script`
- `capability.command`

Lifecycle script capability records use:

- `kind = "lifecycleScript"`
- `script = "<script-name>"`
- `command = "<raw package.json script command>"`

Detected lifecycle script names:

- `preinstall`
- `install`
- `postinstall`
- `prepublish`
- `prepare`
- `prepack`
- `postpack`

Capabilities are sorted/deduplicated deterministically and round-trip through lockfile parse/marshal.

## Visibility

- `tspack update` produces lockfile changes when capabilities change.
- `tspack check` warns with `TSPACK_SECURITY_LIFECYCLE_SCRIPT_PRESENT` when lockfile packages include lifecycle capabilities. Human check output summarizes multiple lifecycle-present warnings by default because execution is blocked by policy; use `tspack check --show-lifecycle` for every script and pull-chain detail. `tspack check --json` remains full-detail, and serious security diagnostics stay visible in default human output.
- `tspack adopt --security` is a read-only observed npm metadata report for existing package.json/package-lock projects. It labels whether lifecycle script data came from `package.json`, `package-lock.json`, or installed package metadata and does not treat the result as manifest policy truth.
- `tspack update` may fetch npm or JSR registry artifacts and populate the store, but it never executes package code or lifecycle scripts.
- `tspack update --dry-run` may fetch registry metadata for version resolution but does not fetch/store tarballs or materialize `node_modules`.
- `tspack sync` materializes files only and never executes scripts.

`tspack add` follows the same boundary. It contacts only the explicitly selected
registry (`npm` by default or `jsr` with `--source jsr`), performs no fallback
search, invokes neither npm nor Deno, and then delegates artifact verification
and store capture to the normal update path. Unknown sources fail before
registry access.

JSR compatibility artifacts do not receive npm lifecycle capability semantics.
JSR packages report no install-script capabilities. If a JSR package depends on
an npm package, that npm package retains its normal lifecycle capability data
and blocked-by-default behavior.

M70c includes a controlled compatibility-registry case whose metadata and
tarball both declare `preinstall` and `postinstall`. Resolution records no JSR
lifecycle capabilities, and normal update/sync/materialization still executes
nothing. Compatibility packaging therefore cannot accidentally grant npm
install-script semantics. This does not suppress npm capabilities on npm
transitives reached from a JSR parent.

Native audit coverage remains source-explicit. OSV currently has an npm
ecosystem but no JSR ecosystem identifier, so mixed-source audit reports mark
JSR packages `unsupported-ecosystem` instead of silently presenting them as
clean.
The audit model classifies JSR as an unsupported ecosystem and other unmapped
sources as coverage unknown. JSR's npm-compatible name is not submitted as npm
vulnerability identity. Linked repositories or registry provenance are useful
review evidence but do not prove that an npm advisory applies to a JSR package.

The content store may deduplicate identical bytes, but its metadata retains a
deterministic set of every source-qualified package provenance for that hash.
The lockfile remains authoritative graph provenance; content deduplication does
not collapse npm and JSR identity.

## Current policy status

- Lifecycle scripts are blocked by design (never executed).
- No script allowlist execution exists in v1.
- No capability approval execution flow exists in v1.

## Observed npm lifecycle visibility

`tspack adopt --security` exists for incremental adoption before a project has been migrated into a TSPack manifest. It surfaces observed npm lifecycle scripts so users can review hidden install-time behavior in an existing npm-style project.

Important boundaries:

- this is not a CVE scanner
- this is not `npm audit`
- this does not execute package code
- this does not imply a package is malicious
- this does not create TSPack policy decisions automatically

The report focuses on lifecycle visibility and context:

- root package lifecycle scripts are reported separately
- dependency lifecycle scripts are labeled as direct or transitive
- optional/dev/peer context is shown when known
- why chains are shown when they can be derived from observed lock metadata

When `package-lock.json` lacks script metadata, the report says so explicitly. If installed `node_modules` metadata is available, TSPack may inspect `package.json` files read-only and label those findings as `installed-package-json`.

## Historical non-goals for M14

- vulnerability scanning
- license scanning
- native build support
- binary download support
- script execution

- `tspack outdated` fetches registry metadata only; it does not fetch tarballs or execute scripts.

Vulnerability scanning was outside the original M14 lifecycle-capability milestone. It is now provided separately by the read-only `tspack audit` command; see `docs/audit.md`. Lifecycle capability policy and known-vulnerability data remain distinct signals.


## Phase 7 lifecycle security closeout

Phase 7 closes the lifecycle-script policy loop around the same core rule: package-manager operations do not grant ambient execution. Lifecycle capabilities are blocked by default during `update`, `sync`, and materialization. Acknowledgments are exact warning-suppression metadata only; they do not approve rebuilds, npm-install compatibility behavior, or any script execution. Behavior evidence links (`behaviorFixture` and `behaviorReport`) are read-only metadata that `check`, `doctor security`, and `why` may validate or display without running probes automatically.

The explicit behavior probe remains native xTest `lifecycle.runScript(...)` / `runLifecycleScript`, which is useful for controlled JavaScript lifecycle fixtures and reports denied network, environment, child-process, and filesystem behavior. That helper is a behavior test harness, not normal package-manager execution and not a kernel sandbox. `tspack doctor security` is the summary/reporting view for lifecycle posture; it reads manifest policy and lockfile metadata without executing scripts.

OS-level jails are deferred. They are not required for TSPack v1's default security model because TSPack does not execute lifecycle scripts by default. If lifecycle execution is ever added, it should sit behind a swappable backend seam instead of becoming an implicit package-manager behavior. See `docs/claude-fooding-phase7.md` for the full closeout, release-gate stance, and non-goals.

## Run environment overlays

`tspack run --env KEY=VALUE` passes explicit values to the child process environment after inheriting the parent environment. These values are process inputs, not secret-manager entries: TSPack does not redact child output, store secrets, load dotenv files, or provide approval semantics for environment variables.

TSPack status output prints only environment keys, such as `Env: PORT, NODE_ENV`, and never prints overlay values itself. The child process can still print any environment value to stdout or stderr, and those streams pass through unchanged.

TSPack does not perform shell expansion, variable interpolation, or quote stripping for `--env`; the value is whatever argv contains after the user's shell has run.


## Pack artifact verification

`tspack pack --verify` is a non-executing structural check of the produced npm tarball. It opens the generated archive, parses `package/package.json`, validates metadata and referenced file paths, and checks peer dependency metadata without running package code, lifecycle hooks, package scripts, `npm install`, publish flows, or network registry checks.

## Lifecycle script capabilities (M37a)

TSPack detects npm lifecycle scripts as package capabilities. Supported lifecycle names are `preinstall`, `install`, `postinstall`, `prepack`, `prepare`, `postpack`, `prepublish`, `prepublishOnly`, and `postpublish`. Commands are copied as raw strings from `package.json`; TSPack does not parse or execute them.

A capability is recorded on the lockfile package entry, for example:

```toml
[[package.capability]]
kind = "lifecycleScript"
script = "postinstall"
command = "node install.js"
```

`update` refreshes capability metadata while resolving packages. `sync` and materialization read this metadata but never execute lifecycle scripts and must not mutate the lockfile just to add missing metadata. `check` warns with `TSPACK_SECURITY_LIFECYCLE_SCRIPT_PRESENT`, and `why` shows lifecycle capabilities next to the matching lock package. When many packages declare lifecycle scripts, default human `check` output prints a concise summary and points to `--show-lifecycle`; `doctor security` remains the policy-posture command.

This is a supply-chain visibility feature: in traditional npm installs, lifecycle scripts can run during installation and may access CI, cloud, npm, or environment credentials. TSPack keeps fetching and syncing separate from execution. Future lifecycle testing, approval policy, and jailed execution are deferred.

`update --dry-run` includes lifecycle capability changes as ordinary package changes when lock metadata differs. A dedicated lifecycle capability diff view is deferred.

## Lifecycle behavior harness (M37b)

Package name trust is not enough: a historically trusted package can be compromised and ship a new lifecycle script. M37b adds an explicit lifecycle behavior probe for JavaScript/Node lifecycle scripts so tests can ask what a script tries to do instead of trusting the package authority or name.

The harness is separate from normal package-manager operations. `tspack update`, `tspack sync`, and materialization still do not execute lifecycle scripts. The probe runs only when test code explicitly calls the native xTest helper.

The M37b probe runs controlled commands of the form `node install.js` or `node ./install.js` with optional script argv. It rejects arbitrary shell lifecycle strings such as `sh -c ...`, `npm run ...`, and `node install.js && curl ...` with `TSPACK_LIFECYCLE_UNSUPPORTED_COMMAND`.

Default probe policy denies common risky behavior:

- network access through common Node `http`, `https`, `net`, `tls`, and `dns` APIs;
- child process creation through `child_process.spawn`, `exec`, `execFile`, and `fork`;
- reads of common secret-like environment keys such as `NPM_TOKEN`, `NODE_AUTH_TOKEN`, `GITHUB_TOKEN`, AWS, Vault, SSH, Google, and Azure credential keys;
- reads and writes outside the package directory and probe temp directory.

Security honesty: the guard is Node preload instrumentation, not a kernel security boundary. It is useful for tests and behavior detection, but it is not an OS jail, network namespace, container sandbox, malware scanner, or vulnerability database. Future hardened execution may use OS-level sandboxes or jails; M37b deliberately does not implement them.

Known MVP limitations: the guard intercepts common Node APIs, not every possible filesystem or native-code path; it supports controlled Node script commands only; it does not implement approval policy, package-manager mutation behavior, `npm install` compatibility, dotenv loading, or public install/rebuild execution.

## Lifecycle capability acknowledgments

TSPack records npm lifecycle scripts as package capabilities and blocks lifecycle execution during `update`, `sync`, and materialization. A project may acknowledge a known lifecycle capability in the manifest to suppress the default `TSPACK_SECURITY_LIFECYCLE_SCRIPT_PRESENT` warning:

```tsx
<Security
  acknowledgedCapabilities={[
    {
      package: "npm:esbuild@0.24.0",
      kind: "lifecycleScript",
      script: "postinstall",
      command: "node install.js",
      reason: "Known lifecycle capability; execution remains blocked by TSPack.",
    },
  ]}
/>
```

Acknowledgment is not execution permission. It does not cause TSPack to run package scripts, does not approve rebuilds, and does not enable npm-install compatibility behavior. The package ID, script name, and raw command must match the lockfile capability exactly. If the package changes the command, `tspack check` reports command drift with `TSPACK_SECURITY_ACKNOWLEDGED_CAPABILITY_STALE` and still reports the actual unacknowledged lifecycle capability. If an acknowledgment no longer matches any lockfile capability, `tspack check` reports `TSPACK_SECURITY_ACKNOWLEDGED_CAPABILITY_UNUSED`.

Acknowledgments may also link to behavior evidence metadata:

```tsx
<Security
  acknowledgedCapabilities={[
    {
      package: "npm:esbuild@0.24.0",
      kind: "lifecycleScript",
      script: "postinstall",
      command: "node install.js",
      reason: "Known lifecycle capability; execution remains blocked by TSPack.",
      behaviorFixture: "security/esbuild-postinstall.valid.xtest.tsx",
      behaviorReport: "security/esbuild-postinstall.report.json",
    },
  ]}
/>
```

`behaviorFixture` points to a source-controlled xTest behavior fixture, and `behaviorReport` points to an optional JSON report captured from a previous explicit behavior probe. They are evidence references only: `check`, `doctor security`, `why`, `update`, `sync`, and materialization do not execute fixtures or lifecycle scripts and do not generate or update reports. Paths must be safe project-relative paths; fixture paths must end in `.xtest.ts` or `.xtest.tsx`, and report paths must end in `.json`. Missing fixture/report references are warnings, including `TSPACK_SECURITY_BEHAVIOR_FIXTURE_MISSING` for missing fixtures; invalid report JSON is a warning, and omitted evidence remains allowed. Future policy may require evidence, but M37e does not.

Behavior fixtures depend on the native xTest globals documented in `docs/native-test-harness.md`. For lifecycle evidence, use `runLifecycleScript` directly in the fixture:

```tsx
export default (
  <Suite name="esbuild postinstall behavior">
    <Fact name="postinstall stays inside guard policy">{async () => {
      const report = await runLifecycleScript({
        packageDir: "/absolute/path/to/fixture/package",
        command: "node install.js",
      });

      assert.equal(report.exitCode, 0, "postinstall exits successfully");
      assert.equal(report.violations.length, 0, "postinstall stays inside policy");
    }}</Fact>
  </Suite>
);
```

`tspack doctor security` validates referenced fixture/report paths and reports their present or missing status, but it does not execute the fixture.


## Lifecycle security audit view (M37d)

`tspack doctor security` is the read-only audit summary for lifecycle capabilities and policy status. It does not execute lifecycle scripts, run lifecycle probes, mutate the manifest or lockfile, generate policy, contact registries, run `npm audit`, scan malware databases, approve rebuilds, or create jailed builds.

The security doctor view summarizes lifecycle capabilities recorded in `ts-lock.toml`, including total, acknowledged, unacknowledged, stale, unused, package, and behavior-evidence counts. Per-capability rows show the lock package ID, script, command, `execution: blocked`, acknowledgment status and reason, behavior fixture/report paths and statuses when present, and pulled-by paths when the lock graph has enough edge information. Exact acknowledgments are `ok`; unacknowledged capabilities, stale command drift, and unused acknowledgments are warnings. Warning-only security doctor output exits `0`; error-level findings exit nonzero for the scoped command.

A missing lockfile is reported as a warning because doctor cannot audit locked lifecycle capabilities until `tspack update` records package metadata. In that state, manifest acknowledgments are not treated as unused because there is no lock graph to evaluate. When the lockfile exists and no lifecycle capabilities are recorded, the summary is `ok` with zero counts.

Relationship between security tools:

- `tspack check` is the enforcement-style diagnostics path (`TSPACK_SECURITY_LIFECYCLE_SCRIPT_PRESENT`, stale acknowledgments, unused acknowledgments).
- `tspack doctor security` is the read-only summary/report view for lifecycle security posture.
- `tspack why` and `tspack why --reverse` are reachability and investigation views for packages and lock graph paths.
- `lifecycle.runScript(...)` in native xTest is an explicit behavior probe for tests; doctor does not run probes.


## Lifecycle script categories

TSPack classifies recorded lifecycle capabilities by operational relevance while continuing to block all lifecycle execution by default. Consumer install-time scripts are `preinstall`, `install`, and `postinstall`; these are the highest-relevance hooks for dependency consumers because npm-style installers can run them during dependency installation. Maintainer publish-time scripts are `prepublishOnly`, `prepublish`, `prepare`, `prepack`, `postpack`, `publish`, and `postpublish`; these are generally maintainer workflow hooks and are operationally less urgent for consumers, but TSPack still records them as blocked capabilities. Any detected lifecycle script outside those known sets is categorized as `other` rather than being silently treated as install-time.

Lifecycle diagnostics include `lifecycleScriptName`, `lifecycleCategory`, and `consumerInstallTime`. Human `tspack check` summarizes lifecycle warnings by category and `tspack check --show-lifecycle` reveals every script, command, category, execution posture, and pull chain. `tspack check --json` keeps individual diagnostics with classification fields. `tspack doctor security` reports lifecycle category counts and shows the same classification for acknowledged and unacknowledged capabilities.

### Lifecycle category acknowledgments

Projects may acknowledge a lifecycle category without permitting execution. This reduces repeated default `tspack check` lifecycle noise for operationally lower-risk categories while keeping the capabilities visible in `tspack check --json`, `tspack check --show-lifecycle`, `tspack doctor security`, and `tspack why` audit surfaces.

```tsx
<Security
  acknowledgedLifecycleCategories={[
    {
      category: "maintainer-publish",
      reason: "Maintainer-side package scripts are blocked by TSPack and do not execute during consumer install/update.",
    },
  ]}
/>
```

A category acknowledgment may be constrained to reviewed script names:

```tsx
<Security
  acknowledgedLifecycleCategories={[
    {
      category: "maintainer-publish",
      scripts: ["prepublishOnly", "prepare", "prepack"],
      reason: "Reviewed maintainer-side lifecycle scripts; execution remains blocked by TSPack.",
    },
  ]}
/>
```

Exact `acknowledgedCapabilities` entries remain stronger package/script/command evidence and take precedence when both exact and category acknowledgments match. A `maintainer-publish` category acknowledgment does not suppress `preinstall`, `install`, or `postinstall`; consumer-install category acknowledgments must be explicit and should be treated as higher-risk review metadata. Category acknowledgments do not allow lifecycle execution, rebuilds, package-manager compatibility behavior, or resolver changes.


## UpdatePolicy security gates

`tspack update --policy --dry-run` evaluates allowed policy-plan candidates against the same lifecycle capability and acknowledgment model used by `tspack check` and `tspack doctor security`. The planner does not execute lifecycle scripts, behavior fixtures, or package code, and it does not fetch external vulnerability feeds. It only uses package metadata already resolved for outdated/update planning and the workspace security policy.

Exact `Security.acknowledgedCapabilities` rows pass matching candidate lifecycle capabilities when package, script, and command match the candidate package ID. `Security.acknowledgedLifecycleCategories` rows can pass matching lifecycle categories when no exact acknowledgment is present. Stale or mismatched acknowledgments do not pass a candidate.

An unacknowledged consumer-install lifecycle script (`preinstall`, `install`, or `postinstall`) blocks policy-plan readiness. An unacknowledged maintainer-publish lifecycle script such as `prepare` or `prepublishOnly` requires review. Passing a policy-plan gate is not execution permission: lifecycle execution remains blocked by default. Behavior fixture/report paths remain evidence for review surfaces and do not run during policy planning.

## Observed npm lifecycle capability warnings (M62e)

`tspack adopt --security` reports lifecycle scripts in existing npm projects as observed capabilities. An install-time lifecycle script means npm may execute package code during install or materialization phases such as `preinstall`, `install`, `postinstall`, or `prepare`. This is common in parts of the npm ecosystem, including some platform-selector or binary-helper packages, but presence does not mean a package is malicious.

The report distinguishes root scripts from dependency scripts and labels direct, transitive, optional, dev, and peer context when local `package.json`, `package-lock.json`, or installed package metadata makes that visible. Optional findings should be read carefully: optional packages are often platform-specific, but TSPack does not infer native behavior or trust from the package name alone.

The observed npm report is read-only and intentionally narrower than `npm audit`: it does not fetch vulnerability feeds, execute package code, run lifecycle hooks, or decide manifest security policy. Its job is to show install-time and pack/publish behavior with source metadata and why chains so users can make informed adoption decisions without alarmist labels.
