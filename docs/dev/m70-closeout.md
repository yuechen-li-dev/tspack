# M70 registry plurality closeout

M70 is complete as a package-source foundation:

- **M70a:** registry backend abstraction, npm/JSR adapters, direct JSR
  compatibility resolution, mixed graphs, and shared store/materialization.
- **M70b:** explicit `add --source jsr`, source-aware authoring, npm default,
  and no registry auto-search.
- **M70c:** semantic versus compatibility identity, collision detection,
  multi-provenance storage, audit coverage states, and source-qualified tools.
- **M70x:** deterministic Requirement Tape, source-qualified peers,
  aliases/references, explicit precedence, and no SAT/backtracking.
- **M70d:** source allowlists, ordered endpoint mirrors/fallback,
  endpoint-scoped credentials, trust checks, provenance, and offline reuse.

```text
manifest intent
  -> Authoring IR
  -> dependency tape
  -> Requirement Tape
  -> controlling semantic requirement
  -> source policy
  -> endpoint selection
  -> registry backend
  -> exact package version/artifact
  -> content-addressed store
  -> lock and materialization
```

Endpoint availability cannot change Requirement Tape precedence or semantic
source identity. Explicit policy is inspectable, fallback is sequential, and
integrity/trust failure is not treated as availability.

Future compiler/toolchain plurality, workflow/CI IR, `deploy.tsx`, Tools
projection, and virtual-source dry-run may consume this foundation. They are not
part of M70 and are not implemented here.
