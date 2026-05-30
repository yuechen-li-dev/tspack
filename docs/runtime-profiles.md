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
