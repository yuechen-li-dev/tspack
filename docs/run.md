# tspack run

`tspack run` launches declared runtime targets from the manifest.

RunTargets are manifest-declared runtime targets. `tspack run` is intentionally not `npm run` and does not fall back to `package.json` scripts.

RunTarget lifecycle semantics are explicit:
- targets with `ready` are server targets
- targets without `ready` are finite commands
- `--once` proves a server target becomes ready, then stops that process tree
- `--once` is harmless on finite targets because they already complete by process exit

## Usage
- `tspack run`
- `tspack run <target>`
- `tspack run --package <package-name> [target]`
- `tspack run --list`
- `tspack run --list --json`
- `tspack run --package <package-name> --list [--json]`
- `tspack run [--package <package-name>] [target] --root <path> [--manifest <path>] --ready-timeout <seconds> [--env KEY=VALUE]... [--once]`

## Listing targets

`tspack run --list` prints declared run targets without starting any process. Targets are grouped by package and show the package kind, runtime, command, URL, effective cwd policy/path, and readiness policy.

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
      "packageKind": "service",
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

Use `--package <package-name>` with `--list` to show only one package's targets. `--list` rejects positional targets, `--once`, and `--env` because listing never starts a process and has no child execution environment.

Targets without `ready` are listed as `ready: none (finite target)`.

Service packages are a natural home for RunTargets that combine `Env(...)`
environment contracts and `Service(...)` TCP/HTTP preflight requirements. The
`service` package kind is only classification in M59c; it does not start
dependent services or change RunTarget execution.

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
- `runtime`: `system`, `node`, `bun`, or `deno`
- `command`: argv array (not shell string)
- `url`: optional base URL for status/readiness and follow-up tooling
- `cwd`: optional `"workspace"` or `"package"`; omitted means `"workspace"` for compatibility
- `ready`: optional readiness policy. Supported kinds are HTTP, TCP, and literal stdout/stderr substring matching. A declared `ready` makes the target a server target; omitted `ready` makes it a finite command.

### Bun RunTarget runtime

A RunTarget with `runtime: "bun"` treats `command` as the Bun argv payload. TSPack prefixes the executable, so `command: ["server.js"]` runs as:

```text
bun server.js
```

This is runtime execution only. TSPack does not call `bun run`, does not read `package.json` scripts, does not run lifecycle hooks, and does not delegate package-manager operations to Bun. `runtime: "system"` continues to execute the declared argv directly, and workspace `runtime="bun"` is inherited by RunTargets that omit `runtime`; explicit RunTarget runtime still wins.

If Bun is not on `PATH`, `tspack run` fails before execution with `TSPACK_RUN_RUNTIME_NOT_FOUND`, including `runtime: bun`, `executable: bun`, the target name, and a hint to install Bun or change the RunTarget runtime. TSPack does not fall back to Node.js, system execution, npm, or npx.


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

### Runtime profile vs. RunTarget runtime

The workspace runtime profile selects the project runtime identity, for example `<Workspace runtime="bun">`. It does not delegate package resolution, lockfile ownership, sync/materialization, check, pack, or lifecycle policy to npm, Bun, or Deno. TSPack still owns those policies.

RunTarget `runtime` is the process-launch backend for a single target. Runtime resolution is explicit target runtime > workspace runtime profile > default `nodejs`. An explicit RunTarget runtime preserves previous behavior and wins over the workspace runtime profile. A RunTarget without `runtime` inherits the workspace runtime profile, so a workspace with `runtime="bun"` launches that target through Bun and a workspace with `runtime="deno"` launches it through Deno. If a target should remain Node/system under a Bun/Deno workspace profile, set `runtime` explicitly on the RunTarget.

### Runtime inheritance examples

```tsx
<Workspace runtime="bun">
  <Package name="app" version="1.0.0" kind="app">
    <RunTargets rows={[{ name: "dev", command: ["server.js"] }]} />
  </Package>
</Workspace>
```

The `dev` target omits `runtime`, so it resolves to `bun (workspace)` and executes as `bun server.js`.

```tsx
<Workspace runtime="bun">
  <Package name="app" version="1.0.0" kind="app">
    <RunTargets rows={[{ name: "dev-node", runtime: "node", command: ["node", "server.js"] }]} />
  </Package>
</Workspace>
```

The `dev-node` target resolves to `node (explicit)`, so Bun does not override it. With no workspace runtime and no target runtime, the target resolves to `nodejs (default)`.

### RunTarget runtime values

- `node` / `nodejs`: uses Node.js as the runtime identity for local tool execution. After `tspack sync`, TSPack prepends the project materialized tool bin at `node_modules/.bin` before host `PATH` and resolves the first command token from that project bin when it is a bare command name. On Windows this includes `.cmd` shims such as `vite.cmd` and `tsc.cmd`. It does **not** prepend `node` to path-containing script files. For JavaScript files, use `command: ["node", "packages/demo/server.js"]` or make the file executable with a shebang.
- `bun`: invokes `bun` with the declared argv payload. For example, `command: ["server.js"]` runs as `bun server.js`. TSPack does not run `bun run`, read package scripts, use npm, or use npx as a fallback.
- `deno`: invokes `deno` with the declared argv payload. For example, `command: ["run", "--allow-net=127.0.0.1:8080", "server.ts"]` runs as `deno run --allow-net=127.0.0.1:8080 server.ts`.
- `system`: built-in runtime support that executes the declared argv directly as a system command. It does not use node-local tool resolution. For JavaScript files, include `node`, `bun`, or `deno` explicitly if needed. `tspack doctor run` reports this runtime as available without looking for a binary named `system`.

Good JavaScript file command with Node:

```tsx
runtime: "node",
command: ["node", "packages/demo/server.js"],
```

Good local tool command with Node:

```tsx
runtime: "node",
command: ["vite", "--host", "127.0.0.1"],
// bare vite can resolve from local tools/.bin
```

Good system command with an explicit JavaScript runtime:

```tsx
runtime: "system",
command: ["node", "packages/demo/server.js"],
```

Potentially surprising Node command:

```tsx
runtime: "node",
command: ["packages/demo/server.js"],
// runtime "node" does not prepend node; the file must be executable with a shebang or this may fail with permission denied
```

Deno permission flags are explicit command arguments for now. TSPack does not infer permissions, define a Deno permission DSL, run `deno task`, read Deno package scripts, parse `deno.json` or `deno.lock`, generate import maps, or call `deno install`, `deno add`, `deno cache`, or `deno vendor`.

If `deno` is missing for an explicit `runtime: "deno"` RunTarget, `tspack run` fails before execution with `TSPACK_RUN_RUNTIME_NOT_FOUND`, including `runtime: deno`, `executable: deno`, the target name, and a hint to install Deno or change the RunTarget runtime. TSPack does not fall back to Node.js, Bun, system execution, npm, npx, or package scripts.

Runtimes are process launch backends only. TSPack still owns manifest/lock/package/test lifecycle.

## Environment overlays

`tspack run` accepts repeatable explicit child-process environment overlays:

```sh
tspack run dev --env PORT=3001
tspack run --package @acme/app dev --env PORT=3001 --env NODE_ENV=development
tspack run dev --once --env PORT=3001
```

Each `--env` value must be `KEY=VALUE`. Keys must match `[A-Za-z_][A-Za-z0-9_]*`; empty keys, keys starting with a digit, and assignments without `=` are rejected with `TSPACK_RUN_INVALID_ENV`. Values may be empty (`--env FOO=`), and values may contain additional equals signs (`--env FOO=bar=baz`). Duplicate keys are deterministic: the later flag wins, and status output lists the key once.

TSPack starts from `os.Environ()`, overlays the final `--env` key/value pairs for the child process, and passes that environment to the spawned command. It does not mutate the parent process environment, write values to the manifest/lock/store, load dotenv files, read env files, define manifest-level env declarations, expand variables, strip quotes, or perform shell interpolation. Values are literal after the user's shell has already produced argv. For example, TSPack does not expand `--env API_URL=$URL` or interpolate `--env PATH=$PATH:/x`; any expansion was done by the shell before TSPack received the argument.

Status output prints only keys, never values:

```text
Env: PORT, NODE_ENV
```

If no overlays are provided, the `Env:` line is omitted. `tspack run --list` rejects `--env` because list mode does not execute a child process. `tspack doctor run` does not inspect CLI `--env` overlays.

Readiness URLs and readiness configuration are not interpolated. If a server reads `PORT` from the child environment, the manifest `url` or TCP readiness port must already match the value supplied by `--env`; TSPack does not rewrite readiness URLs dynamically.

`inspect --run` shares the RunTarget startup path and also accepts `--env KEY=VALUE` for the temporary child process while keeping inspect JSON on stdout clean.

## Streams

`tspack run` writes TSPack-owned status/progress lines (`Starting`, `Package`, `Runtime`, `Command`, `Waiting for`, `Ready`) to **stderr**. The `Starting` line uses the package-qualified target ID, such as `@prisma-ui/demo:dev`. The child process stdout passes through to stdout, and the child process stderr passes through to stderr. This keeps stdout usable for scripts that consume child output.

## Manifest selection

By default, `tspack run` loads `<root>/manifest.tsx`. Pass `--manifest <path>` to load an explicit manifest path; this composes with `--root` but does not change command cwd semantics. Commands execute from the selected RunTarget cwd policy. `--package` selects the declaring package; it does not rewrite command argv paths.

## Lifecycle and readiness

Readiness is process readiness only. It is not a long-running healthcheck, lifecycle script, shell interpolation hook, WebSocket probe, or package-manager behavior.

Finite targets do not wait for readiness. They succeed when the launched command exits with code `0`, fail on non-zero exit, and are expected to leave no attached child or grandchild processes behind.

Server targets declare `ready`. All readiness kinds use `--ready-timeout` (default 30s). `--once` exits after readiness succeeds and then stops the launched process tree. After `--once`, the server is not expected to remain available for manual probing.

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

For `node` runtime launches, local compatibility tool resolution depends on strict materialized `node_modules/.bin` entries generated from root-visible dependencies only. `tspack sync` owns those shims, and `tspack run` prepends that project tool bin before the host `PATH`. TSPack does not infer npm scripts and does not expose transitive-only bins at root.

On Windows, if `tspack sync` reports `TSPACK_MATERIALIZE_FILE_LOCKED`, stop the relevant dev server or `node`/`esbuild`/`vite` process and rerun `tspack sync`. Use `tspack sync --clean` after stopping file holders when the materialized tree is stale. Global tool installs are not the intended fix.

## Related docs

See `docs/claude-fooding-phase5.md` for the Phase 5 RunTarget remediation closeout.

## RunTarget environment contracts

RunTargets may declare an `env` contract in `manifest.tsx`. The ergonomic helper form is:

```tsx
RunTargets rows={[{
  name: "dev",
  command: ["tsx", "server.ts"],
  env: [
    Env("DATABASE_URL", { required: true, secret: true, description: "Postgres connection string" }),
    Env("JWT_SECRET", { required: true, secret: true }),
    Env("PORT", { default: "3000", description: "HTTP port" }),
    Env("NODE_ENV", { default: "development" }),
  ],
}]} />
```

At execution time, `tspack run` starts from the host environment plus any explicit `--env KEY=VALUE` overlays. For each declared variable, host or overlay values win; otherwise a `default` is injected; otherwise `required: true` fails before the process starts with `TSPACK_RUN_ENV_MISSING`. Optional variables without defaults may remain missing.

`secret: true` redacts defaults and values from diagnostics, JSON, and target listings. Secret names are still shown so operators know what to set. TSPack does not load `.env` files in M59a, does not prompt, does not mutate the parent shell, and does not scrub undeclared variables yet; undeclared host environment variables are still inherited. A later strict mode may add allowlisting/scrubbing.

## RunTarget service requirements

RunTargets can declare local or external services with `requires: [Service(...)]`. M59b supports TCP checks and HTTP/HTTPS health checks only; TSPack does not start Docker Compose, service containers, Testcontainers, or any other service lifecycle.

```tsx
RunTargets({
  rows: [
    {
      name: "dev",
      runtime: "system",
      command: ["tsx", "src/server.ts"],
      url: "http://127.0.0.1:3000",
      requires: [
        Service("postgres", { tcp: "127.0.0.1:5432", description: "Local Postgres database" }),
        Service("api", { http: "http://127.0.0.1:8080/health", expectStatus: 200, optional: true }),
      ],
    },
  ],
})
```

Each service must provide exactly one of `tcp` (`host:port`) or `http` (`http://` or `https://`). HTTP checks default to status `200`; service checks default to `timeoutMs: 1000`. Required service failures stop `tspack run` before the command starts with `TSPACK_RUN_SERVICE_UNAVAILABLE`. Optional service failures are printed as warnings and the target continues. `tspack check` validates declaration shape but does not require services to be running.

## Readiness URL env interpolation

HTTP server RunTargets may use simple environment placeholders in the declared `url`, for example `http://127.0.0.1:${PORT}` with `ready: { kind: "http", path: "/health" }`. TSPack resolves host environment, applies repeatable `--env KEY=VALUE` overlays, injects RunTarget `Env(...)` defaults for missing values, validates required env, and then interpolates `${NAME}` placeholders before starting the process and polling readiness.

Only portable env names are supported in readiness placeholders: `${PORT}`, `${HOST}`, and other names matching `/^[A-Za-z_][A-Za-z0-9_]*$/`. TSPack does not evaluate expressions, run commands, or interpolate arbitrary template syntax. A missing placeholder variable fails before process start with `TSPACK_RUN_READY_ENV_MISSING`; malformed placeholder syntax fails with `TSPACK_MANIFEST_READY_INVALID`; placeholders that reference `Env(..., { secret: true })` fail with `TSPACK_RUN_READY_ENV_SECRET` because readiness URLs can appear in logs and diagnostics.

Static readiness URLs continue to work unchanged. `tspack run --list` shows the manifest template string rather than interpolating against the current host environment.

## Preflight-only mode

`tspack run <target> --preflight-only` validates the contract needed before a RunTarget starts and then exits without launching the command. It resolves the target, applies env defaults and `--env` overlays, validates required `Env(...)`, interpolates HTTP readiness URLs enough to catch missing, secret, or malformed placeholders, and runs external `Service(...)` preflights. Required service failures still fail with `TSPACK_RUN_SERVICE_UNAVAILABLE`; optional service failures warn and exit successfully.

`--preflight-only` does not check the target's own readiness URL because the process was not started. It proves that declared env and external dependencies are available; it does not prove that the target can boot.

## Service preflights vs target readiness

`Service(...)` describes external dependencies such as Postgres, Redis, or an upstream HTTP API. These checks happen before TSPack starts the RunTarget command. RunTarget `ready` describes the target's own health signal after it starts, such as `GET /health` on the service itself. Do not model a service's own health endpoint as `Service(...)`; use `ready` for that self-health check.
