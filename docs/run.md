# tspack run

`tspack run` launches declared runtime targets from the manifest.

## Usage
- `tspack run`
- `tspack run <target>`
- `tspack run --package <package-name> [target]`
- `tspack run --list`
- `tspack run --list --json`
- `tspack run --package <package-name> --list [--json]`
- `tspack run [--package <package-name>] [target] --root <path> [--manifest <path>] --ready-timeout <seconds> [--once]`

## Listing targets

`tspack run --list` prints declared run targets without starting any process. Targets are grouped by package and show runtime, command, URL, effective cwd policy/path, and readiness policy.

`tspack run --list --json` writes a machine-readable payload to stdout only:

```json
{
  "command": "run",
  "mode": "list",
  "root": ".",
  "package": null,
  "targets": [
    {
      "id": "@prisma-ui/demo:dev",
      "package": "@prisma-ui/demo",
      "name": "dev",
      "runtime": "system",
      "command": ["node", "packages/demo/server.js"],
      "url": "http://127.0.0.1:5173",
      "cwd": "workspace",
      "cwdPath": "/repo",
      "ready": { "kind": "http", "path": "/" }
    }
  ],
  "diagnostics": []
}
```

Use `--package <package-name>` with `--list` to show only one package's targets. `--list` rejects positional targets and `--once` because listing never starts a process.

## Target identity and selection

Run target identity is package-qualified as `<package-name>:<target-name>`, for example `@prisma-ui/demo:dev`. Users select targets with `--package <package-name>` plus the target name, or by target name alone when the name is unambiguous.

Selection rules without `--package`:
1. explicit `<target>` if provided and exactly one package declares that target name
2. the single global `dev` target if exactly one package declares `dev`
3. the only target if exactly one target exists globally
4. otherwise fail with `TSPACK_RUN_TARGET_AMBIGUOUS`

Selection rules with `--package <package-name>`:
1. explicit `<target>` inside that package if provided
2. that package's `dev` target if present
3. that package's only target if exactly one target exists in the package
4. otherwise fail with a package-scoped diagnostic

If no run targets exist, fails with `TSPACK_RUN_TARGET_MISSING`. If `--package` names an unknown package, fails with `TSPACK_RUN_PACKAGE_NOT_FOUND` and includes known packages. If a target is missing inside a selected package, fails with `TSPACK_RUN_TARGET_NOT_FOUND` and includes that package's known targets.

Duplicate target names across packages are allowed. For example, if both `@prisma-ui/demo` and `@prisma-ui/docs` declare `dev`, `tspack run dev` fails with `TSPACK_RUN_TARGET_AMBIGUOUS`, lists `@prisma-ui/demo:dev` and `@prisma-ui/docs:dev`, and hints to use `--package <name>`.

## Manifest syntax
Use `<RunTargets rows={[...]} />` under a package.

Each row supports:
- `name`
- `runtime`: `system` or `node` (M22)
- `command`: argv array (not shell string)
- `url`: base URL for status/readiness
- `cwd`: optional `"workspace"` or `"package"`; omitted means `"workspace"` for compatibility
- `ready`: optional readiness policy. Supported kinds are HTTP, TCP, and literal stdout/stderr substring matching.


## Working directory policy

RunTargets support an explicit `cwd` field:

- `cwd: "workspace"` runs the command from the workspace/project root. This is also the behavior when `cwd` is omitted, preserving existing manifests.
- `cwd: "package"` runs the command from the declaring package root, which is the natural choice for package-local dev servers.

TSPack does not rewrite command argv paths. Relative paths are resolved by the child process working directory.

Workspace-root command example:

```tsx
<RunTargets rows={[{
  name: "dev",
  runtime: "system",
  cwd: "workspace",
  command: ["node", "packages/demo/server.js"],
  url: "http://127.0.0.1:5173",
}]} />
```

Package-root command example:

```tsx
<RunTargets rows={[{
  name: "dev",
  runtime: "system",
  cwd: "package",
  command: ["node", "server.js"],
  url: "http://127.0.0.1:5173",
}]} />
```

Status output includes the effective cwd, for example `Cwd: workspace (/repo)` or `Cwd: package (/repo/packages/demo)`. `run --list`, `run --list --json`, and `doctor run` also report the effective cwd.

## Runtime notes
- `system`: built-in runtime support that executes the declared argv directly. `tspack doctor run` reports this runtime as available without looking for a binary named `system`.
- `node`: execute argv directly and prefer local `node_modules/.bin` before PATH.
- Future reserved backends: `bun`, `deno` (not implemented yet and not launched by `tspack run`).

Runtimes are process launch backends only. TSPack still owns manifest/lock/package/test lifecycle.

## Streams

`tspack run` writes TSPack-owned status/progress lines (`Starting`, `Package`, `Runtime`, `Command`, `Waiting for`, `Ready`) to **stderr**. The `Starting` line uses the package-qualified target ID, such as `@prisma-ui/demo:dev`. The child process stdout passes through to stdout, and the child process stderr passes through to stderr. This keeps stdout usable for scripts that consume child output.

## Manifest selection

By default, `tspack run` loads `<root>/manifest.tsx`. Pass `--manifest <path>` to load an explicit manifest path; this composes with `--root` but does not change command cwd semantics. Commands execute from the selected RunTarget cwd policy. `--package` selects the declaring package; it does not rewrite command argv paths.

## Readiness

Readiness is process readiness only. It is not a long-running healthcheck, lifecycle script, shell interpolation hook, WebSocket probe, or package-manager behavior.

All readiness kinds use `--ready-timeout` (default 30s). `--once` exits after readiness succeeds and terminates the child process.

### HTTP readiness

```json
{ "kind": "http", "path": "/" }
```

HTTP readiness preserves the original behavior: TSPack polls `url + ready.path` until it receives a `200-399` response. HTTP readiness requires the RunTarget `url`.

Status output:

```text
Waiting for: http http://127.0.0.1:5173/
Ready: http://127.0.0.1:5173/
```

### TCP readiness

```json
{ "kind": "tcp", "port": 5432 }
```

```json
{ "kind": "tcp", "host": "127.0.0.1", "port": 6379 }
```

TCP readiness attempts to connect to `host:port` until the connection succeeds or the timeout expires. The default host is `127.0.0.1`. The port must be an integer from `1` through `65535`. A successful connection is closed immediately. TCP readiness does not require `url`, although `url` may still be present for documentation or follow-up tooling.

Status output:

```text
Waiting for: tcp 127.0.0.1:5432
Ready: tcp 127.0.0.1:5432
```

### stdout-match readiness

```json
{
  "kind": "stdout-match",
  "pattern": "Local:",
  "stream": "stdout"
}
```

`stdout-match` readiness succeeds when the configured literal substring appears in the selected child output stream. It is not a regular expression. `stream` may be `stdout`, `stderr`, or `both`; omitted means `both`.

TSPack observes the selected stream while preserving passthrough: child stdout still goes to stdout, child stderr still goes to stderr, and TSPack status remains on stderr. Matching uses a small rolling buffer, so a literal pattern split across output chunks can still be detected.

Status output:

```text
Waiting for: stdout-match "Local:" on stdout
Ready: matched "Local:"
URL: http://127.0.0.1:5173
```

The `URL:` line is printed when the RunTarget also declares `url`.

## Inspect integration (M23)
- `tspack inspect dev`
- `tspack inspect --run dev`

`inspect` can start a declared run target, wait for the declared readiness kind, inspect its URL, then terminate the process.

## Not supported
- npm scripts (`npm run`)
- `npx`
- package.json script inference
- shell-string command execution
- npm script inference for inspect/run

## Mutation contract
`tspack run` does not mutate:
- `manifest.tsx` / `package.manifest.tsx`
- `ts-lock.toml`
- dependencies / `node_modules`

## Local tool-bin behavior

For `node` runtime launches, local compatibility tool resolution depends on strict materialized `node_modules/.bin` entries generated from root-visible dependencies only. TSPack does not infer npm scripts and does not expose transitive-only bins at root.

