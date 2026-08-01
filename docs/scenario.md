# Browser scenarios

`tspack scenario <scenario.json> --run <RunTarget> [--root .]` owns the
declared target lifecycle: it starts the target, waits for its readiness
contract, launches Playwright, applies each scenario viewport, records browser
diagnostics, writes requested screenshots, closes the browser, and stops the
target even when an assertion fails.

Scenario JSON is declarative. It does not provide arbitrary JavaScript
execution. `artifactDirectory` is relative to the scenario file.

```json
{
  "artifactDirectory": "artifacts/browser-proof",
  "scenarios": [{
    "name": "overlap",
    "viewport": { "width": 700, "height": 500 },
    "assertions": [{
      "kind": "topmost-at-point",
      "x": 320,
      "y": 240,
      "expected": "[data-machina-box='modalBox']"
    }],
    "screenshot": "overlap.png"
  }]
}
```

`topmost-at-point` is a reusable browser assertion. It requires integral
viewport coordinates and a selector that matches exactly one expected element.
TSPack calls `document.elementFromPoint(x, y)` and succeeds when the actual
element is that expected element or one of its descendants. Its report records
the point, expected selector, actual tag/class, and the nearest
`data-machina-layout` / `data-machina-box` host when present. Missing,
ambiguous, out-of-bounds, and mismatched assertions use distinct diagnostics.

On any scenario failure TSPack writes a `*-failure.png` capture when possible,
includes console/page/request diagnostics in the failure, then performs the
same browser and host cleanup as a successful run.
