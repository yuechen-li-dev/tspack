# TSPack Inspect VS Code Extension POC

This is the M39b proof-of-concept VS Code extension for viewing `tspack inspect` CDP output inside VS Code.

## Local development

```sh
npm install
npm run compile
npm test
```

## Runtime workflow

1. Start VS Code, Chromium, or another Chromium/Electron host with a remote debugging port, for example:

   ```sh
   code --remote-debugging-port=9229
   ```

2. Configure these VS Code settings when defaults are not correct:
   - `tspack.inspect.cdpEndpoint` (default `http://127.0.0.1:9229`)
   - `tspack.inspect.tspackPath` (default `tspack`)
   - `tspack.inspect.targetIndex` (default `0`)

3. Run **TSPack: Inspect CDP Targets** to list targets and inspect one.
4. Use **TSPack: Refresh Inspect Tree** to re-run inspect for the configured target.
5. Select a tree node to print JSON details in the **TSPack Inspect** output channel.
6. Use **TSPack: Copy Selected Inspect Node JSON** to copy the current node payload.

The extension intentionally shells out to `tspack inspect` and does not reimplement CDP inspection.
