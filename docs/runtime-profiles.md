# Runtime profiles

TSPack workspaces can declare a JavaScript runtime profile on `<Workspace>`:

```tsx
<Workspace name="demo" runtime="nodejs">
  ...
</Workspace>
```

Allowed values are:

- `nodejs`
- `bun`
- `deno`

If `runtime` is omitted, TSPack defaults the workspace runtime profile to `nodejs`.

## Runtime switch demo (M42e)

The fixture family documented in [Runtime Switch Demo](runtime-switch-demo.md) demonstrates the one-line portability claim: change `<Workspace runtime="nodejs">` to `<Workspace runtime="bun">` or `<Workspace runtime="deno">` while keeping the project contract stable. The manifests differ only by the workspace runtime value, normalized IR differs only in `Workspace.Runtime`, and check/pack/why/package metadata remain stable across the three profiles.

M42e does not add workspace-runtime inheritance into RunTargets. Explicit RunTarget runtime declarations continue to win: a workspace declared as `runtime="deno"` can still have a `runtime: "bun"` RunTarget, and TSPack launches Bun for that target.

## Node.js baseline (M42b)

`nodejs` is the compatibility/default profile. These two workspace declarations are behaviorally equivalent for current supported commands:

```tsx
<Workspace name="demo">
  ...
</Workspace>

<Workspace name="demo" runtime="nodejs">
  ...
</Workspace>
```

The only intended difference is authoring source text: normalized IR and runtime reporting both say `nodejs`. Existing command behavior remains Node.js-compatible under the baseline profile. TSPack continues to use its current Node-based JavaScript bridges for manifest parsing, native xTest, artifacts, doom, inspect helpers, and other JS bridge paths until a later milestone explicitly changes those seams.

The workspace runtime profile is not package metadata. `tspack pack` and `tspack pack --verify` must not write `runtime`, `nodejs`, `bun`, or `deno` profile metadata into generated npm `package.json` files.

M42b is a regression baseline for future Bun/Deno work. It does not implement Bun or Deno execution behavior, package-manager delegation, RunTarget inheritance, native xTest bridge switching, JS bridge switching, lockfile changes, materializer changes, or npm/Bun/Deno install delegation.

## Runtime profile is not package-manager delegation

Bun and Deno bundle runtime, package, task, and test features into one tool. TSPack deliberately separates these concerns. In TSPack, runtime selects the JavaScript runtime profile; dependency resolution, lockfiles, materialization, checks, packaging, and security policy remain TSPack-owned.

The workspace runtime profile must not be confused with package manager names such as `npm`, `pnpm`, or `yarn`. Those names are rejected because they are not runtime profiles.

TSPack owns:

- dependency resolution
- `ts-lock.toml`
- sync/materialization
- check
- pack
- security lifecycle policy

M42a only records, validates, typechecks, and reports the selected runtime profile. It does not delegate installs to npm/Bun/Deno, change lockfile semantics, change materialization, switch the native xTest bridge, or make workspace runtime profile override explicit RunTarget runtime declarations.

One-line thesis: **Change runtime profile, keep project contract.**

## Bun runtime proof (M42c)

`runtime="bun"` is a supported workspace runtime profile. `tspack doctor runtime` reports the selected profile as `bun`, the profile executable as `bun`, PATH availability for that executable, `status: experimental`, `lifecycleOwner: tspack`, and `packageManagerDelegated: false`.

Bun support in M42c is intentionally constrained to runtime reporting and explicitly declared RunTargets. A RunTarget with `runtime: "bun"` is launched by TSPack as `bun` plus the declared argv payload:

```tsx
<RunTargets
  rows={[
    {
      name: "hello",
      runtime: "bun",
      cwd: "package",
      command: ["hello.js"],
      ready: { kind: "stdout-match", pattern: "ready", stream: "stdout" },
    },
  ]}
/>
```

The example above runs `bun hello.js`. TSPack does not run `bun run hello`, does not read package scripts, and does not call npm/npx as a fallback. If `bun` is missing, `tspack run` fails before child execution with `TSPACK_RUN_RUNTIME_NOT_FOUND` and a hint to install Bun or change the RunTarget runtime.

Workspace `runtime="bun"` does not automatically inherit into RunTargets in M42c. A RunTarget with `runtime: "system"` keeps system argv semantics, `runtime: "node"` keeps the existing Node/local-bin semantics, and omitted RunTarget runtime behavior is unchanged.

TSPack still owns dependency resolution, `ts-lock.toml`, materialization, checks, pack, lifecycle security policy, and package-manager semantics. M42c does not implement `bun install`, `bun add`, `bun pm`, `bun.lockb`, Bun dependency resolution, Bun materialization, Bun lifecycle ownership, native xTest runtime switching, or JavaScript bridge runtime switching.

## Deno runtime proof (M42d)

`runtime="deno"` is a supported workspace runtime profile. `tspack doctor runtime` reports the selected profile as `deno`, the profile executable as `deno`, PATH availability for that executable, `status: experimental`, `lifecycleOwner: tspack`, and `packageManagerDelegated: false`. The doctor runtime details continue to state that dependency resolution is `TSPack`, the lockfile is `ts-lock.toml`, materialization is `TSPack`, and security policy is `TSPack`.

Deno support in M42d is intentionally constrained to runtime reporting and explicitly declared RunTargets. A RunTarget with `runtime: "deno"` is launched by TSPack as `deno` plus the declared argv payload:

```tsx
<RunTargets
  rows={[
    {
      name: "hello",
      runtime: "deno",
      cwd: "package",
      command: ["run", "--allow-net=127.0.0.1:8080", "server.ts"],
      ready: { kind: "stdout-match", pattern: "ready", stream: "stdout" },
    },
  ]}
/>
```

The example above runs `deno run --allow-net=127.0.0.1:8080 server.ts`. Deno permission flags stay explicit manifest command arguments in M42d. TSPack does not infer permissions and does not add a Deno permission DSL.

TSPack does not run `deno task hello`, does not read package scripts, does not call `deno install`, `deno add`, `deno cache`, or `deno vendor`, and does not parse `deno.json`, `deno.lock`, import maps, JSR metadata, or `npm:` specifiers. If `deno` is missing, `tspack run` fails before child execution with `TSPACK_RUN_RUNTIME_NOT_FOUND` and a hint to install Deno or change the RunTarget runtime. There is no fallback to Node.js, Bun, system execution, npm, npx, or package scripts.

Workspace `runtime="deno"` does not automatically inherit into RunTargets in M42d. A RunTarget with `runtime: "system"` keeps system argv semantics, `runtime: "node"` keeps the existing Node/local-bin semantics, `runtime: "bun"` keeps the existing Bun launch adapter, and omitted RunTarget runtime behavior is unchanged.

TSPack still owns dependency resolution, `ts-lock.toml`, materialization, checks, pack, lifecycle security policy, and package-manager semantics. M42d does not implement Deno task/package/tooling delegation, native xTest runtime switching, JavaScript bridge runtime switching, or package-manager mutation.
