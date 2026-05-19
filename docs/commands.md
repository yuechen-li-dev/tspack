# Command inventory (M24)

| Command | Purpose | Mutates manifest/lock? | Notable non-goals | Details |
|---|---|---|---|---|
| `tspack check` | Validate manifest/frontend, graph, boundaries, type surfaces, and lock consistency when lock exists. | No / No | Does not resolve or install packages. | `docs/contract.md` |
| `tspack update` | Resolve sources and write deterministic `ts-lock.toml`. | No / **Yes (lock)** | Not a package installer/materializer command. | `docs/lockfile.md`, `docs/source-resolvers.md` |
| `tspack sync` | Materialize compatibility `node_modules` from lock/store. | No / No | Does not re-resolve versions. | `docs/materialization.md` |
| `tspack why` | Explain why dependency/target/lock package is present. | No / No | Not a resolver/editor command. | `docs/why.md` |
| `tspack pack` | Create deterministic package archives. | No / No | Not a build pipeline or publish command. | `docs/pack.md` |
| `tspack run [target]` | Start declared manifest `RunTargets` and wait for readiness. | No / No | Not `npm run`; no package.json script inference. | `docs/run.md` |
| `tspack test` | Run test backends (native xTest and/or Vitest). | No / No | Not a generic task runner. | `docs/test-command.md`, `docs/native-test-harness.md` |
| `tspack artifact` | Run standalone native suite artifacts. | No / No | Not package artifact packing. | `docs/artifacts.md` |
| `tspack bench` | Run native benchmark units (`*.benchmark.tsx`). | No / No | Not a general profiling framework. | `docs/benchmarks.md` |
| `tspack doom` | Run quarantined prophecy/doom units (`*.prophecy.tsx`). | No / No | Not a generic chaos platform. | `docs/doom.md` |
| `tspack inspect <url\|target>` | Structural UI inspection and run-target inspection. | No / No | Not screenshot diffing/visual testing; not auto-attach. | `docs/inspect.md` (**experimental**) |

## Stability

- Stable core package surface: `check`, `update`, `sync`, `why`, `pack`.
- Stable native harness surface: `test`, `artifact`, `bench`, `doom`.
- Experimental surface: `inspect`.
