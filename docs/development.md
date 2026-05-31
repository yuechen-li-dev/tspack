# Development notes

## Manifest frontend bridge build layout

Run the canonical bridge build before repository-root Go CLI dogfood or release smoke commands:

```sh
cd manifest-frontend && npm run build
```

That build emits the current bridge layout:

- `manifest-frontend/dist/cli.js` for manifest parsing and migration checks.
- `manifest-frontend/dist/native-test-cli.js` for native xTest, doom, benchmark, and artifact commands.
- `manifest-frontend/dist/inspect-cli.js` for `tspack inspect`.

Go bridge discovery prefers the current `dist/<bridge>.js` files and accepts legacy `dist/src/<bridge>.js` files for older dev flows. Do not rely on a failing full `tsc -p tsconfig.json` compile to create bridge artifacts.
