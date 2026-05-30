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
