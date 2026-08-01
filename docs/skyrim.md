# Skyrim targets

Skyrim support is an explicit, static manifest extension. Ordinary TypeScript
manifests have no `skyrim` IR member, and ordinary `check`, `update`, `sync`,
`run`, `test`, and `pack` paths do not load a Skyrim profile, probe Windows, or
perform additional network access.

The Marionette architecture keeps authored responsibilities separate:

```text
manifest.tsx                 lifecycle and package topology
typed Skyrim TOML            authored records
Dominatus.Assets.Toml        binding and source diagnostics
Marionette asset compiler    validation and backend-neutral IR
Mutagen                      ESP emission and reopen verification
C++ plus runtime TOML        SKSE behavior and configuration
TSPack                       planning, transactional deployment, activation, launch
```

All selected asset packs link into exactly one bridge,
`build/assets/MarionetteSSE.esp`. It is generated output, not authored source.
Stable symbolic IDs and local FormIDs live in the allocation manifest; adding a
pack means adding its TOML source and one `assetPacks` row to `manifest.tsx`, not
adding another runtime ESP.

## Portable intent and machine profile

`<SkyrimTarget>` declares commands as argv arrays, project-relative inputs,
owned destinations, expected bridge records, the named host, SKSE launch intent,
and runtime evidence. The parser never executes manifest TypeScript.

Machine paths belong in ignored `.tspack/skyrim-hosts.toml`:

```toml
[hosts.skyrim-dev]
gameRoot = "C:\\Games\\Skyrim Special Edition"
dataDirectory = "C:\\Games\\Skyrim Special Edition\\Data"
skseLauncher = "C:\\Games\\Skyrim Special Edition\\skse64_loader.exe"
pluginState = "C:\\path\\to\\the-active-profile\\plugins.txt"
runtimeLogDirectory = "C:\\work\\MarionetteSSE\\dev"
runtimeVersion = "1.6.1170.0"

[hosts.skyrim-dev.tools]
cmake = "C:\\path\\to\\cmake.exe"
dotnet = "dotnet"
```

Copy the repository's example and resolve the paths for the actual active host.
TSPack validates the declared runtime and SKSE loader; it never silently falls
back to the normal Skyrim launcher.

## Commands

```powershell
tspack check
tspack doctor skyrim
tspack run skyrim --dry-run --json
tspack run skyrim --json
```

The final command configures and builds native code, runs native and asset tests,
links and verifies the bridge, compares it with the deployed bridge, writes a
deterministic plan, atomically materializes owned files, updates only the managed
plugin-state entry, launches the declared SKSE executable, and scans the newest
runtime log for the ready marker.

Plans and reports are under `build/skyrim`; the linked ESP and its reports are
under `build/assets`. Rollback copies are outside `Game/Data` under
`build/skyrim/rollback`. A failed transaction restores owned files and plugin
state. Repeating unchanged materialization is idempotent.

Humans and LLMs do not edit `plugins.txt`, `loadorder.txt`, or equivalent state.
TSPack preserves unrelated entries and ordering, enables `MarionetteSSE.esp`
once, and rejects or removes only explicitly declared stale Marionette bridges.
