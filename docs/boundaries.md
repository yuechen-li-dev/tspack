# Runtime boundaries (M4)

M4 validates runtime imports discovered from source graph walking against M3 target dependency permissions.

Rules implemented:
- runtime external import must be target-allowed
- undeclared imports fail
- tool dependencies imported by runtime targets fail
- peer scope violations fail
- optional peer leaks fail
- explicit `denyDeps` fail
- explicit `allowDeps` rows are conservative: if row matches and package not in `allowDeps`, produce allow violation when target would otherwise allow

Boundary `from` matching supports:
- exact path
- prefix form ending in `/**`

Node builtin policy in M4:
- builtins are classified by scanner
- dependency boundary checks ignore builtins for now (TODO: environment/runtime policy)

M4 non-goals:
- package fetching/resolution
- lockfile/store/node_modules flows
- build/test/dev/publish command implementation
