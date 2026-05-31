# Runtime Switch Demo

## Claim

Change runtime profile, keep project contract.

The M42e demo fixture family proves that the same project can be represented as Node.js, Bun, or Deno by changing only the `<Workspace runtime="...">` value. TSPack still owns dependency resolution, `ts-lock.toml`, materialization, checks, pack, lifecycle policy, and package-manager semantics.

## One-line switch

The demo fixtures are:

- `fixtures/valid/runtime-switch-nodejs`
- `fixtures/valid/runtime-switch-bun`
- `fixtures/valid/runtime-switch-deno`

Their manifests are identical except for this one line:

```tsx
<Workspace name="runtime-switch" runtime="nodejs">
```

```tsx
<Workspace name="runtime-switch" runtime="bun">
```

```tsx
<Workspace name="runtime-switch" runtime="deno">
```

## What changes

- `tspack doctor runtime` reports the selected profile as `nodejs`, `bun`, or `deno`.
- The selected profile's executable availability is reported for the chosen runtime.
- An explicitly declared RunTarget that uses `runtime: "bun"` or `runtime: "deno"` launches that runtime with the declared argv payload.

## What does not change

- dependency resolution
- `ts-lock.toml` semantics
- materialization
- `tspack check`
- `tspack pack` and generated package metadata
- `tspack why`
- lifecycle security policy
- format/lint behavior
- native xTest bridge behavior unless a future milestone says otherwise
- JavaScript bridge behavior unless a future milestone says otherwise

The workspace runtime profile is not npm package metadata. It must not appear in generated `package.json` output.

## Why this is not package-manager switching

Bun and Deno bundle runtime, package, task, and test features. TSPack deliberately separates those concerns. Runtime profile is runtime identity, not package manager identity.

The demo does not run `bun install`, `bun add`, `bun pm`, `deno task`, `deno install`, `deno add`, `deno cache`, or `deno vendor`. It does not introduce Bun or Deno lockfile ownership, resolver behavior, materializer behavior, package-manager mutation, or npm/npx fallback behavior.

## Current limitations

- Workspace runtime does not yet implicitly inherit into RunTargets.
- Deno import maps, JSR, and `npm:` support are not implemented.
- Bun package-manager behavior is not implemented.
- Native xTest runtime switching is not implemented.
- JavaScript bridge runtime switching is not implemented.

## Demo commands

```sh
tspack doctor runtime
```

```sh
tspack check
```

```sh
tspack pack --verify
```

```sh
tspack why left-pad
```

```sh
tspack run bun-hello --once
```

```sh
tspack run deno-hello --once
```
