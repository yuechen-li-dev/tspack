import fs from "node:fs";
import path from "node:path";
import { describe, expect, it } from "vitest";
import {
  materializedXTestGlobals,
  nativeTypecheckGlobals,
} from "../../src/native-test/xtest-globals.js";

function normalize(value: string): string {
  return value.replace(/\r\n/g, "\n").trimEnd();
}

describe("xtest globals declarations", () => {
  it("keeps the internal materialized globals aligned with the canonical declaration file", () => {
    const canonicalPath = path.resolve(process.cwd(), "src", "tspack-xtest.d.ts");
    const canonical = fs.readFileSync(canonicalPath, "utf8");

    expect(normalize(materializedXTestGlobals)).toBe(normalize(canonical));
  });

  it("keeps JSX intrinsic elements internal-only", () => {
    expect(materializedXTestGlobals).not.toContain("interface IntrinsicElements");
    expect(nativeTypecheckGlobals).toContain("interface IntrinsicElements");
  });
});
