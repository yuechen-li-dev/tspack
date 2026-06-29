# Incremental existing React dogfood project

This example intentionally represents a normal package.json-native Vite,
React, and TypeScript project before TSPack adoption.

It is not generated from a TSPack template and it should keep working with
ordinary npm commands:

```sh
npm install
npm run typecheck
npm run build
```

M62a uses this project as the friction surface for read-only incremental
adoption reporting. The checked-in state deliberately has no `manifest.tsx` and
no `ts-lock.toml`.
