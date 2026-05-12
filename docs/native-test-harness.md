# Native TSX Test Harness (M16)

M16 introduces an opt-in, TSPack-native unit test substrate.

## Scope

- Native test file naming: `*.tspack.test.tsx`
- TSX tags are used for static discovery and metadata
- Test callback bodies are ordinary TypeScript code
- Assertions require explicit human-readable reasons
- `assert.*` is the primary assertion API
- `expect(...).matcher(...).because(reason)` is available
- Fixture, command, filesystem, golden, archive, and diagnostics helpers are intentionally out of scope in M16
- Vitest remains separate and unchanged for existing test suites

## TSX discovery model

Supported tags:

- `<Suite name="...">`
- `<Fact name="...">{() => {}}</Fact>`
- `<Theory name="..."> ... </Theory>`
- `<Case ... />`

Discovery is static and deterministic:

- names must be string literals
- case values must be literal string/number/boolean/null
- spread syntax is rejected
- dynamic test generation is not supported

## Result IDs

Example IDs:

- `math/addition works`
- `math/string lengths[0]`
- `math/string lengths[1]`
