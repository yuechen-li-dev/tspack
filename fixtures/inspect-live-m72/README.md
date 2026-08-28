# M72 live inspect fixture

This bounded fixture proves the real RunTarget readiness, native xTest browser
inspection, nested role navigation, focusability assertion, and source-hint
pipeline without test-owned server processes or sleeps.

From the repository root after building the manifest frontend bridge:

```powershell
tspack test --root fixtures/inspect-live-m72 --xtest --run dev
tspack inspect --root fixtures/inspect-live-m72 --run dev --selector '[role="alert"]' --bundle
```

Use `--env PORT=<free-port>` with either command when port 5198 is occupied.
