import assert from "node:assert/strict";
import test from "node:test";
import { runChecks, validateDeclaration } from "./browser-scenario-assertions.mjs";

function createPage({ expectedCount = 1, evaluation }) {
  return {
    locator() {
      return {
        count: async () => expectedCount,
      };
    },
    evaluate: async () => evaluation,
  };
}

test("topmost-at-point records the actual semantic host", async () => {
  const page = createPage({
    evaluation: {
      actual: { tagName: "div", className: "proof-box modal", layout: "LayerProof", box: "modalBox" },
      matchesExpected: true,
    },
  });

  const results = await runChecks(page, [{ kind: "topmost-at-point", x: 320, y: 240, expected: "[data-machina-box='modalBox']" }]);

  assert.deepEqual(results, [{
    kind: "topmost-at-point",
    x: 320,
    y: 240,
    expected: "[data-machina-box='modalBox']",
    actual: { tagName: "div", className: "proof-box modal", layout: "LayerProof", box: "modalBox" },
  }]);
});

test("topmost-at-point reports missing, out-of-bounds, and mismatched assertions", async () => {
  await assert.rejects(
    () => runChecks(createPage({ expectedCount: 0 }), [{ kind: "topmost-at-point", x: 1, y: 1, expected: "#missing" }]),
    /TSPACK_SCENARIO_EXPECTED_ELEMENT_MISSING/,
  );
  await assert.rejects(
    () => runChecks(createPage({ evaluation: { error: "point (900, 1) is outside viewport 700x700" } }), [{ kind: "topmost-at-point", x: 900, y: 1, expected: "#expected" }]),
    /TSPACK_SCENARIO_POINT_INVALID/,
  );
  await assert.rejects(
    () => runChecks(createPage({
      evaluation: {
        actual: { tagName: "div", className: "proof-box overlay", layout: "LayerProof", box: "overlay" },
        matchesExpected: false,
      },
    }), [{ kind: "topmost-at-point", x: 1, y: 1, expected: "#expected" }]),
    /TSPACK_SCENARIO_TOPMOST_MISMATCH.*data-machina-box=overlay/,
  );
});

test("topmost-at-point declaration rejects non-integral coordinates", () => {
  assert.throws(
    () => validateDeclaration({
      artifactDirectory: "artifacts",
      scenarios: [{
        name: "invalid-coordinate",
        viewport: { width: 100, height: 100 },
        assertions: [{ kind: "topmost-at-point", x: 1.5, y: 1, expected: "#expected" }],
      }],
    }),
    /TSPACK_SCENARIO_DECLARATION_INVALID/,
  );
});
