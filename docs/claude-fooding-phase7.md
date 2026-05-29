# Claude-Fooding Phase 7 Security / Policy Closeout

Claude-fooding Phase 7 focused on supply-chain security hardening for npm lifecycle scripts and executable package capabilities. The closeout state is **Success**: lifecycle behavior is visible, default package-manager paths remain non-executing, acknowledgments and evidence are explicit metadata, and release-gate smoke coverage documents the expected policy posture.

## Threat model

Traditional npm-compatible package managers may execute lifecycle scripts such as `preinstall`, `install`, and `postinstall` during install. That creates an ambient execution path:

```text
package install -> lifecycle hook -> arbitrary code execution with user/CI environment
```

Recent supply-chain attacks have abused typosquatted packages or compromised maintainer accounts to run install-time code that can read CI/CD, cloud, npm, SSH, or other environment secrets. Authority-based trust is brittle: a famous package name, long package history, or familiar maintainer account does not guarantee safe current behavior.

TSPack's answer is behavior visibility and default non-execution. Package identity is metadata, not behavior trust. A package can be fetched, locked, explained, checked, and materialized without being allowed to execute lifecycle code.

## Security thesis

TSPack's Phase 7 security thesis is:

- fetch is not execute;
- update is not execute;
- sync/materialization is not execute;
- lifecycle scripts are recorded capabilities, not ambient rights;
- `check`, `doctor security`, and `why` make executable capabilities visible;
- acknowledgments suppress known warning noise but do not grant execution permission;
- behavior fixtures and behavior reports provide evidence metadata but do not grant execution permission;
- the native xTest lifecycle harness lets users test script behavior explicitly through `lifecycle.runScript(...)` / `runLifecycleScript`, separate from normal package-manager operations.

## Remediation summary

| Finding / need | Milestone | Fix | Status |
| --- | --- | --- | --- |
| Lifecycle scripts were hidden package behavior. | M37a | Detect lifecycle scripts from npm metadata/package manifests, record `lifecycleScript` lockfile capabilities, warn through `check`, and surface capabilities through `why`, JSON why, and reverse why. | Done |
| Script behavior needed explicit test evidence without normal install execution. | M37b | Add native xTest `lifecycle.runScript(...)` / `runLifecycleScript` helper with scrubbed environment, temp HOME/TMP, package cwd, Node preload behavior guard, and structured violations/output. | Done |
| Known capabilities needed review metadata without granting execution. | M37c | Add exact `<Security acknowledgedCapabilities={...} />` entries for package/kind/script/command/reason, stale-command warnings, unused-acknowledgment warnings, and acknowledged visibility in why surfaces. | Done |
| Security posture needed a report command. | M37d | Add `tspack doctor security` and `--json` lifecycle summaries for no capabilities, unacknowledged, acknowledged, stale, unused, missing lock, pulled-by paths, and non-execution posture. | Done |
| Acknowledgments needed links to behavior evidence. | M37e | Add read-only `behaviorFixture` and `behaviorReport` evidence references, validate paths/statuses, and surface evidence in `check`, `doctor security`, and `why` without automatic execution. | Done |

## Current lifecycle capability model

TSPack records executable package behavior as lockfile capability metadata. A lifecycle capability records the script name and raw command:

```toml
[[package.capability]]
kind = "lifecycleScript"
script = "postinstall"
command = "node install.js"
```

The current user-facing model is:

- `tspack update` records lifecycle capability metadata while resolving packages and preparing store artifacts, but does not execute the command.
- `tspack check` reports unacknowledged lifecycle capabilities with `TSPACK_SECURITY_LIFECYCLE_SCRIPT_PRESENT`.
- `tspack why`, `tspack why --json`, and `tspack why --reverse` show lifecycle capabilities, acknowledgment status, and evidence metadata where relevant.
- `tspack doctor security` summarizes lifecycle status, including no capability, unacknowledged, acknowledged, stale, unused, and missing-lock states.
- `<Security acknowledgedCapabilities={...} />` is exact manifest policy for warning suppression only.
- `behaviorFixture` and `behaviorReport` are read-only evidence links attached to acknowledgments.
- Native xTest `lifecycle.runScript(...)` is the explicit behavior-probe harness for controlled Node lifecycle scripts.

## Current security workflow

A typical lifecycle-security review starts with non-executing package-manager commands:

```sh
tspack update
tspack check
tspack doctor security
tspack doctor security --json
tspack why npm:<pkg>@<version>
tspack why --reverse <pkg>
tspack how TSPACK_SECURITY_LIFECYCLE_SCRIPT_PRESENT
tspack how TSPACK_LIFECYCLE_NETWORK_DENIED
```

When behavior evidence is useful, add an explicit xTest fixture. A valid fixture should assert that the controlled script exits successfully and reports no guard violations:

```tsx
import { Suite, Fact, Project, assert, lifecycle } from "@tspack/manifest-frontend/native-test";

export default (
  <Suite name="lifecycle behavior">
    <Fact name="postinstall stays inside package directory">
      <Project from="fixtures/lifecycle-safe" />
      {async ({ project }) => {
        const result = await lifecycle.runScript({
          packageDir: project.path,
          command: "node install.js",
        });

        assert.equal(result.exitCode, 0, "safe fixture should exit successfully");
        assert.equal(result.violations.length, 0, "safe fixture should not hit lifecycle guard violations");
      }}
    </Fact>
  </Suite>
);
```

Invalid fixtures should assert concrete denied behavior, such as `network`, `env`, `childProcess`, `fsRead`, or `fsWrite` violations. These tests are behavior probes only. They do not convert acknowledgments or evidence into package-manager execution permission.

## OS jail stance

OS-level jailing is intentionally deferred. TSPack's current model avoids ambient lifecycle execution entirely. It is not required for TSPack v1's default security model because `update`, `sync`, and materialization do not execute lifecycle scripts.

Cross-platform sandboxing is high-complexity and must fail closed. If lifecycle execution is ever added, TSPack should expose a swappable execution backend seam rather than baking platform-specific sandboxing into the core package-manager contract.

Potential future backends include:

- no-execution backend, default;
- Node behavior guard backend, probe/test only;
- Linux jail backend;
- macOS sandbox backend;
- Windows restricted execution backend, if practical.

TSPack's current concern is behavior visibility, explicit evidence, and non-ambient execution.

## Explicit non-goals / deferred work

Phase 7 does not implement:

- lifecycle execution during `update`, `sync`, or materialization;
- npm install compatibility scripts;
- approval or allow-run policy;
- an OS jail backend in v1;
- vulnerability database integration;
- malware scanning;
- automatic behavior fixture execution;
- automatic policy generation;
- trust-by-package-name whitelists;
- secret-manager semantics.
