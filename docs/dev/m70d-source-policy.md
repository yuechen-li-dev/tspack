# M70d registry source policy

## Status

M70d adds a deterministic policy layer between Requirement Tape and registry
backends. It separates semantic package identity (`npm:react`) from the
concrete endpoint that supplied metadata and bytes. npm and JSR remain the only
built-in registry sources; an npm-compatible company registry is an endpoint,
not a new source kind.

## Before-state audit

| Assumption | Previous owner | Classification | M70d disposition |
| --- | --- | --- | --- |
| `npm` implies `https://registry.npmjs.org` | resolver HTTP client/project backend construction | registry endpoint | default retained; explicit ordered endpoints may replace it |
| `jsr` implies `https://npm.jsr.io` | JSR adapter/project backend construction | registry endpoint | default retained; an explicit compatibility endpoint may replace it |
| source-qualified resolver keys | resolver/Requirement Tape | semantic source | unchanged |
| one HTTP client per source | registry adapter | transport | endpoint clients are scoped entries in a deterministic backend chain |
| no registry authentication | HTTP transport | credentials | optional environment-token references are endpoint-scoped |
| tarball URL trusted from metadata | registry adapter | artifact endpoint | optional artifact-host allowlists preflight tarball requests |
| lock records source/hash/integrity only | lockfile | provenance | explicit policy adds endpoint and artifact-host evidence |
| metadata memo key is source/name | resolver | cache | safe because each run binds a source to one policy chain |
| no `.npmrc` loading | project/resolver | compatibility input | unchanged; no silent global npm configuration |

HTTP is owned by `internal/resolver.HTTPRegistryClient`. It uses the shared
bounded transport, attributes requests by source/kind/host, performs one
bounded `Retry-After` retry, and relies on Go's safe redirect handling. TSPack
also refuses to attach an endpoint credential to a different artifact host.

## Manifest surface

```tsx
<RegistryPolicy
  allowedSources={["npm", "jsr"]}
  requireIntegrity={true}
  requireAuditCoverage={false}
>
  <RegistrySource
    kind="npm"
    endpoints={[
      {
        url: "https://npm.company.example",
        tokenEnv: "COMPANY_REGISTRY_TOKEN",
        allowedArtifactHosts: ["npm.company.example", "artifacts.company.example"],
      },
      { url: "https://registry.npmjs.org" },
    ]}
  />
</RegistryPolicy>
```

`allowedSources` is an allowlist. `[]` denies every registry source. Omitting
`RegistryPolicy` retains public npm and official JSR behavior, performs no new
requests, and records no new lock fields, so existing locks do not churn.

Each endpoint supports `url`, optional `tokenEnv`, optional
`fallbackOnNotFound`, and optional `allowedArtifactHosts`. Raw secrets and URL
credentials are rejected from project truth. Diagnostic URLs remove userinfo
and redact token/auth/secret/key query parameters.

## Resolution and fallback

```text
Requirement Tape
  -> controlling semantic source/name/range
  -> allow/offline/trust preflight
  -> ordered endpoint chain
  -> source adapter
  -> exact version and artifact
  -> integrity/content verification
  -> store, lock, materialization
```

Endpoint policy never changes requirement precedence and never substitutes JSR
for npm. Metadata endpoints are tried sequentially. Connection failures,
timeouts, EOF, and 5xx may fall through. A 404 falls through only when the
failed endpoint declares `fallbackOnNotFound`. 401, 403, malformed metadata,
and other configuration/trust failures fail closed. Mirrors are not raced.

Artifact availability failure may advance the same ordered chain. The fallback
endpoint must advertise the exact selected version and the same integrity
before its artifact is accepted. Artifact integrity or content mismatch never
falls through. `requireIntegrity` rejects versions without advertised
integrity. If an existing locked package ID resolves to different bytes, update
reports `TSPACK_REGISTRY_TRUST_FAILED`.

## Provenance, lock, cache, and store

Under explicit policy, registry packages record optional `metadata_endpoint`,
`registry_endpoint` (the endpoint that supplied bytes), and `artifact_host`
lock fields. IDs remain `source:name@version`. Repeated
resolution under the same availability and policy is byte deterministic. A
different endpoint observation can change provenance fields but does not change
semantic identity, version, integrity, or content hash.

The content-addressed store deduplicates identical bytes across endpoints and
merges endpoint observations in deterministic order. Metadata memoization is
per source/name inside resolver state already bound to the declared endpoint
chain, so public and private metadata are never shared across policy instances.

## Offline and air-gap guarantees

`offline={true}` makes update/add fail immediately because they require
metadata. `sync` succeeds without network when the lock and every referenced
store artifact are present. A missing artifact produces
`TSPACK_REGISTRY_OFFLINE_MISS` without an HTTP attempt. A strict single internal
endpoint plus artifact-host allowlist prevents public fallback.

This is a precise store-reuse guarantee, not a claim that an uncached project
can bootstrap without metadata or artifacts.

## Trust, audit, diagnostics, and inspection

`requireAuditCoverage` makes `check` reject JSR locks because OSV currently has
no JSR ecosystem mapping. The default continues to allow partial coverage and
reports it honestly. `check` also rejects denied locked sources and endpoint
evidence outside the current chain; older locks with missing endpoint evidence
receive a refresh warning.

`why` human and JSON output include registry endpoint and artifact host when
recorded. Performance attribution remains `npm.metadata`, `npm.tarball`,
`jsr.metadata`, and `jsr.tarball`, with actual request host.

Primary diagnostics are `TSPACK_SOURCE_POLICY_DENIED`,
`TSPACK_REGISTRY_ENDPOINT_DENIED`, `TSPACK_REGISTRY_FALLBACK_EXHAUSTED`,
`TSPACK_REGISTRY_TRUST_FAILED`, `TSPACK_REGISTRY_OFFLINE_MISS`, and
`TSPACK_REGISTRY_AUTH_FAILED`.

## Deliberate boundaries

- No cross-source substitution (`npm:foo -> jsr:foo`).
- No dynamic backends, provider-specific auth, secret storage, `.npmrc`, or
  hidden machine-global endpoint selection.
- JSR overrides must serve JSR's npm compatibility contract; package and
  artifact validation enforce the compatibility boundary.
- There is no adaptive rate-control subsystem. Retry is deliberately bounded.
