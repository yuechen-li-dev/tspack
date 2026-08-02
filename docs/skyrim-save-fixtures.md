# Skyrim disposable save fixtures

TSPack provisions local Skyrim save fixtures without accepting arbitrary source
or destination paths. The selected Skyrim host owns the save directory and the
ignored `.tspack/skyrim-hosts.toml` profile records only local provenance.

## Discover candidates

```powershell
tspack skyrim saves list --manual-only --require-sidecar --json --root <MarionetteSSE-root>
```

Discovery is read-only. It lists opaque, hash-derived candidate IDs, redacted
display names, SHA-256 values, file size/timestamps, autosave/quicksave/manual
classification, and whether the paired `.skse` sidecar exists. It never emits
the absolute save path or the original filename in normal output.

`saveDirectory` in the selected ignored host profile is authoritative when
present. Otherwise TSPack will use a single unambiguous Windows Documents or
OneDrive Documents Skyrim Special Edition save directory. It fails rather than
guessing when zero or multiple candidates are found; this is the required
escape hatch for Mod Organizer, Vortex, or any other redirected profile.

Safe list filters are `--manual-only`, `--exclude-autosaves`,
`--exclude-quicksaves`, `--require-sidecar`, `--modified-after <RFC3339>`, and
`--modified-before <RFC3339>`.

## Create and inspect

```powershell
tspack skyrim fixture create ed-m2b2d --from <candidate-id> --dry-run --json --root <MarionetteSSE-root>
tspack skyrim fixture create ed-m2b2d --from <candidate-id> --json --root <MarionetteSSE-root>
tspack skyrim fixture inspect ed-m2b2d --json --root <MarionetteSSE-root>
```

Creation requires an explicit, freshly discovered candidate ID. TSPack hashes
the source again immediately before staging, writes deterministic local names
`MarionetteFixture-<symbolic-id>.ess` and `.skse`, verifies the staged hashes,
then atomically replaces the fixture pair. A conflicting changed fixture needs
`--replace` after its dry-run plan; an unchanged fixture is idempotent. The
source `.ess` and sidecar are snapshotted before and after copying and must be
byte-identical with unchanged size and timestamp.

When the source has a sidecar, both files are mandatory and a partial copy rolls
back. When it has none, discovery and the fixture report say so explicitly.
Fixture inspection checks profile ownership, `disposable=true`, `readOnly=true`,
current hashes, sidecar expectation, and stored source provenance.

The ignored host profile receives the `testSaves.ed-m2b2d` mapping only after
the fixture verifies. The mapping is the authority for the secret Marionette
runtime filename; it is not a presenter protocol field and no save path can
cross the wire.

## Session bootstrap

After a fixture is provisioned, use the run-scoped overlay:

```powershell
tspack run skyrim --session-bootstrap --dry-run --json --root <MarionetteSSE-root>
```

It enables presenter transport and session bootstrap for that run only, forces
semantic actuation and host-request evaluation off, and derives the save
filename/read-only fields from the declared `ed-m2b2d` fixture. The committed
runtime TOML remains disabled and is restored when the launched session exits.
For a real launch it also writes the ignored local Aurelian controller config
under `build/msse-presenter-m1/aurelian-transport.json` from the selected host
profile's already-local profile/token values; it never prints the token.
