export async function runChecks(page, checks) {
  const results = [];
  for (const check of checks) {
    if (check.kind === "visible") await page.locator(check.selector).waitFor({ state: "visible" });
    else if (check.kind === "hidden") await page.locator(check.selector).waitFor({ state: "hidden" });
    else if (check.kind === "count") {
      const count = await page.locator(check.selector).count();
      if (count !== check.value) fail("TSPACK_SCENARIO_ASSERTION_FAILED", `${check.selector} count was ${count}, expected ${check.value}.`);
    } else if (check.kind === "text") {
      await page.locator(check.selector).filter({ hasText: check.value }).waitFor({ state: "visible" });
    } else if (check.kind === "no-horizontal-overflow") {
      const overflow = await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth);
      if (overflow) fail("TSPACK_SCENARIO_ASSERTION_FAILED", "Page has horizontal overflow.");
    } else if (check.kind === "class") {
      const classes = await page.locator(check.selector).getAttribute("class");
      if (!classes?.split(/\s+/).includes(check.value)) fail("TSPACK_SCENARIO_ASSERTION_FAILED", `${check.selector} did not have class ${check.value}.`);
    } else if (check.kind === "focused") {
      const focused = await page.locator(check.selector).evaluate(element => document.activeElement === element);
      if (!focused) fail("TSPACK_SCENARIO_ASSERTION_FAILED", `${check.selector} was not focused.`);
    } else if (check.kind === "reduced-motion") {
      const reduced = await page.evaluate(() => window.matchMedia("(prefers-reduced-motion: reduce)").matches);
      if (!reduced) fail("TSPACK_SCENARIO_ASSERTION_FAILED", "Reduced-motion media preference was not applied.");
    } else if (check.kind === "topmost-at-point") {
      const expectedCount = await page.locator(check.expected).count();
      if (expectedCount === 0) fail("TSPACK_SCENARIO_EXPECTED_ELEMENT_MISSING", `Topmost assertion expected selector ${check.expected} did not match an element.`);
      if (expectedCount !== 1) fail("TSPACK_SCENARIO_EXPECTED_ELEMENT_AMBIGUOUS", `Topmost assertion expected selector ${check.expected} matched ${expectedCount} elements; expected exactly one.`);
      const actual = await page.evaluate(({ x, y, expected }) => {
        if (x < 0 || y < 0 || x >= window.innerWidth || y >= window.innerHeight) {
          return { error: `point (${x}, ${y}) is outside viewport ${window.innerWidth}x${window.innerHeight}` };
        }
        const element = document.elementFromPoint(x, y);
        if (element === null) return { error: `document.elementFromPoint(${x}, ${y}) returned no element` };
        const expectedHost = element.closest(expected);
        const semanticHost = element.closest("[data-machina-box]");
        return {
          actual: {
            tagName: element.tagName.toLowerCase(),
            className: typeof element.className === "string" ? element.className : "",
            layout: semanticHost?.getAttribute("data-machina-layout") ?? null,
            box: semanticHost?.getAttribute("data-machina-box") ?? null,
          },
          matchesExpected: expectedHost !== null,
        };
      }, { x: check.x, y: check.y, expected: check.expected });
      if (actual.error) fail("TSPACK_SCENARIO_POINT_INVALID", `Topmost assertion ${check.expected}: ${actual.error}.`);
      if (!actual.matchesExpected) {
        fail("TSPACK_SCENARIO_TOPMOST_MISMATCH", `At (${check.x}, ${check.y}), expected ${check.expected}; actual ${formatTopmost(actual.actual)}.`);
      }
      results.push({ kind: check.kind, x: check.x, y: check.y, expected: check.expected, actual: actual.actual });
    } else fail("TSPACK_SCENARIO_ASSERTION_INVALID", `Unknown assertion kind ${JSON.stringify(check.kind)}.`);
  }
  return results;
}

export function validateDeclaration(value) {
  if (!value || typeof value !== "object" || typeof value.artifactDirectory !== "string" || !Array.isArray(value.scenarios)) {
    fail("TSPACK_SCENARIO_DECLARATION_INVALID", "Expected artifactDirectory and scenarios[].");
  }
  for (const item of value.scenarios) {
    if (!item.name || !item.viewport || !Number.isInteger(item.viewport.width) || !Number.isInteger(item.viewport.height)) {
      fail("TSPACK_SCENARIO_DECLARATION_INVALID", "Every scenario requires name and integer viewport width/height.");
    }
    for (const check of [...(item.assertions ?? []), ...(item.after ?? [])]) {
      if (check?.kind !== "topmost-at-point") continue;
      if (!Number.isInteger(check.x) || !Number.isInteger(check.y) || typeof check.expected !== "string" || check.expected.length === 0) {
        fail("TSPACK_SCENARIO_DECLARATION_INVALID", "A topmost-at-point assertion requires integer x/y and a non-empty expected selector.");
      }
    }
  }
}

export function fail(code, message) {
  throw new Error(`${code}: ${message}`);
}

function formatTopmost(actual) {
  const semantic = actual.layout && actual.box ? ` data-machina-layout=${actual.layout} data-machina-box=${actual.box}` : "";
  return `<${actual.tagName}${semantic}>`;
}
