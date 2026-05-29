# tspack run (M22)

`tspack run` launches declared runtime targets from the manifest.

## Usage
- `tspack run`
- `tspack run <target>`
- `tspack run [target] --root <path> [--manifest <path>] --ready-timeout <seconds> [--once]`

## Target selection
1. explicit `<target>` if provided
2. `dev` target if present
3. only target if exactly one exists
4. otherwise fail (`TSPACK_RUN_TARGET_AMBIGUOUS`)

If no run targets exist, fails with `TSPACK_RUN_TARGET_MISSING`.

## Manifest syntax
Use `<RunTargets rows={[...]} />` under a package.

Each row supports:
- `name`
- `runtime`: `system` or `node` (M22)
- `command`: argv array (not shell string)
- `url`: base URL for status/readiness
- `ready`: optional `{ kind: "http", path: "/" }`

## Runtime notes
- `system`: built-in runtime support that executes the declared argv directly. `tspack doctor run` reports this runtime as available without looking for a binary named `system`.
- `node`: execute argv directly and prefer local `node_modules/.bin` before PATH.
- Future reserved backends: `bun`, `deno` (not implemented yet and not launched by `tspack run`).

Runtimes are process launch backends only. TSPack still owns manifest/lock/package/test lifecycle.

## Streams

`tspack run` writes TSPack-owned status/progress lines (`Starting`, `Runtime`, `Command`, `Waiting for`, `Ready`) to **stderr**. The child process stdout passes through to stdout, and the child process stderr passes through to stderr. This keeps stdout usable for scripts that consume child output.

## Manifest selection

By default, `tspack run` loads `<root>/manifest.tsx`. Pass `--manifest <path>` to load an explicit manifest path; this composes with `--root` but does not change command cwd semantics. Commands still execute from the workspace root in M35a.

## Readiness
M22 readiness is HTTP polling:
- success on status `200-399`
- timeout via `--ready-timeout` (default 30s)
- `--once` exits after ready and terminates child process

## Inspect integration (M23)
- `tspack inspect dev`
- `tspack inspect --run dev`

`inspect` can start a declared run target, wait for HTTP readiness, inspect its URL, then terminate the process.

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

