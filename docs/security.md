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
- `tspack check` warns with `TSPACK_SECURITY_LIFECYCLE_SCRIPT_PRESENT` when lockfile packages include lifecycle capabilities.
- `tspack update` may fetch npm tarballs and populate the store, but it never executes package code or lifecycle scripts.
- `tspack update --dry-run` may fetch registry metadata for version resolution but does not fetch/store tarballs or materialize `node_modules`.
- `tspack sync` materializes files only and never executes scripts.

## Current policy status

- Lifecycle scripts are blocked by design (never executed).
- No script allowlist execution exists in v1.
- No capability approval execution flow exists in v1.

## Non-goals for M14

- vulnerability scanning
- license scanning
- native build support
- binary download support
- script execution

- `tspack outdated` fetches registry metadata only; it does not fetch tarballs or execute scripts.

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

`update` refreshes capability metadata while resolving packages. `sync` and materialization read this metadata but never execute lifecycle scripts and must not mutate the lockfile just to add missing metadata. `check` warns with `TSPACK_SECURITY_LIFECYCLE_SCRIPT_PRESENT`, and `why` shows lifecycle capabilities next to the matching lock package.

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

`behaviorFixture` points to a source-controlled xTest behavior fixture, and `behaviorReport` points to an optional JSON report captured from a previous explicit behavior probe. They are evidence references only: `check`, `doctor security`, `why`, `update`, `sync`, and materialization do not execute fixtures or lifecycle scripts and do not generate or update reports. Paths must be safe project-relative paths; fixture paths must end in `.xtest.ts` or `.xtest.tsx`, and report paths must end in `.json`. Missing fixture/report references are warnings, invalid report JSON is a warning, and omitted evidence remains allowed. Future policy may require evidence, but M37e does not.


## Lifecycle security audit view (M37d)

`tspack doctor security` is the read-only audit summary for lifecycle capabilities and policy status. It does not execute lifecycle scripts, run lifecycle probes, mutate the manifest or lockfile, generate policy, contact registries, run `npm audit`, scan malware databases, approve rebuilds, or create jailed builds.

The security doctor view summarizes lifecycle capabilities recorded in `ts-lock.toml`, including total, acknowledged, unacknowledged, stale, unused, package, and behavior-evidence counts. Per-capability rows show the lock package ID, script, command, `execution: blocked`, acknowledgment status and reason, behavior fixture/report paths and statuses when present, and pulled-by paths when the lock graph has enough edge information. Exact acknowledgments are `ok`; unacknowledged capabilities, stale command drift, and unused acknowledgments are warnings. Warning-only security doctor output exits `0`; error-level findings exit nonzero for the scoped command.

A missing lockfile is reported as a warning because doctor cannot audit locked lifecycle capabilities until `tspack update` records package metadata. In that state, manifest acknowledgments are not treated as unused because there is no lock graph to evaluate. When the lockfile exists and no lifecycle capabilities are recorded, the summary is `ok` with zero counts.

Relationship between security tools:

- `tspack check` is the enforcement-style diagnostics path (`TSPACK_SECURITY_LIFECYCLE_SCRIPT_PRESENT`, stale acknowledgments, unused acknowledgments).
- `tspack doctor security` is the read-only summary/report view for lifecycle security posture.
- `tspack why` and `tspack why --reverse` are reachability and investigation views for packages and lock graph paths.
- `lifecycle.runScript(...)` in native xTest is an explicit behavior probe for tests; doctor does not run probes.
