import { AsyncLocalStorage } from "node:async_hooks";
import crypto from "node:crypto";
import fs from "node:fs";
import path from "node:path";
import type { AssertionFailure, TestSnapshotUpdate } from "./types.js";

type SnapshotKind = "text" | "json";

type SnapshotContext = {
  testFilePath: string;
  rootDir: string;
  updateSnapshots: boolean;
  updates: TestSnapshotUpdate[];
};

const snapshotStorage = new AsyncLocalStorage<SnapshotContext | undefined>();
let fallbackContext: SnapshotContext | undefined;

export function setSnapshotContext(context: SnapshotContext | undefined): void {
  fallbackContext = context;
  snapshotStorage.enterWith(context);
}

function currentSnapshotContext(): SnapshotContext | undefined {
  return snapshotStorage.getStore() ?? fallbackContext;
}

export function assertTextSnapshot(
  value: unknown,
  name: string,
  reason: string,
): void {
  if (typeof value !== "string") {
    throw snapshotFailure(
      "TSPACK_SNAPSHOT_TEXT_VALUE_INVALID",
      "snapshotText value must be a string",
      reason,
      { valueType: typeof value },
    );
  }

  assertSnapshot("text", name, normalizeTextSnapshot(value), reason);
}

export function assertJsonSnapshot(
  value: unknown,
  name: string,
  reason: string,
): void {
  assertSnapshot("json", name, `${stableJson(value, reason)}\n`, reason);
}

function assertSnapshot(
  kind: SnapshotKind,
  name: string,
  actual: string,
  reason: string,
): void {
  const context = currentSnapshotContext();
  if (!context) {
    throw snapshotFailure(
      "TSPACK_SNAPSHOT_WRITE_DISABLED",
      "snapshot assertions require native xTest file execution context",
      reason,
      { snapshotName: name },
    );
  }

  validateSnapshotName(name, reason);
  const snapshotPath = snapshotFilePath(context.testFilePath, name, kind);
  const displayPath = normalizePath(
    path.relative(context.rootDir, snapshotPath),
  );

  if (!fs.existsSync(snapshotPath)) {
    if (context.updateSnapshots) {
      writeSnapshot(snapshotPath, actual, reason, displayPath, context);
      return;
    }

    throw snapshotFailure(
      "TSPACK_SNAPSHOT_MISSING",
      `snapshot missing: ${displayPath}`,
      reason,
      {
        snapshot: displayPath,
        updateHint: "run tspack test --update-snapshots to write it",
      },
    );
  }

  const expected = normalizeStoredSnapshot(
    fs.readFileSync(snapshotPath, "utf8"),
  );
  if (expected === actual) {
    return;
  }

  if (context.updateSnapshots) {
    writeSnapshot(snapshotPath, actual, reason, displayPath, context);
    return;
  }

  throw snapshotFailure(
    "TSPACK_SNAPSHOT_MISMATCH",
    `snapshot mismatch: ${displayPath}`,
    reason,
    buildMismatchDetails(displayPath, expected, actual),
  );
}

function writeSnapshot(
  snapshotPath: string,
  actual: string,
  reason: string,
  displayPath: string,
  context: SnapshotContext,
): void {
  try {
    fs.mkdirSync(path.dirname(snapshotPath), { recursive: true });
    fs.writeFileSync(snapshotPath, actual, "utf8");
    context.updates.push({ path: displayPath, reason });
  } catch (cause) {
    throw snapshotFailure(
      "TSPACK_SNAPSHOT_WRITE_FAILED",
      `snapshot write failed: ${(cause as Error).message}`,
      reason,
      { snapshot: displayPath },
    );
  }
}

function validateSnapshotName(name: string, reason: string): void {
  const invalid =
    typeof name !== "string" ||
    name.length === 0 ||
    name.startsWith(".") ||
    name.includes("..") ||
    name.includes("/") ||
    name.includes("\\") ||
    path.isAbsolute(name) ||
    !/^[A-Za-z0-9_-][A-Za-z0-9_.-]*$/.test(name);

  if (invalid) {
    throw snapshotFailure(
      "TSPACK_SNAPSHOT_INVALID_NAME",
      `invalid snapshot name: ${String(name)}`,
      reason,
      { snapshotName: name },
    );
  }
}

function snapshotFilePath(
  testFilePath: string,
  name: string,
  kind: SnapshotKind,
): string {
  const testDir = path.dirname(testFilePath);
  const namespace = path.basename(testFilePath);
  const extension = kind === "text" ? "snap.txt" : "snap.json";
  return path.join(testDir, "__snapshots__", namespace, `${name}.${extension}`);
}

function normalizeTextSnapshot(value: string): string {
  const normalized = value.replace(/\r\n/g, "\n").replace(/\r/g, "\n");
  if (normalized.endsWith("\n")) {
    return normalized;
  }
  return `${normalized}\n`;
}

function normalizeStoredSnapshot(value: string): string {
  return normalizeTextSnapshot(value);
}

function stableJson(value: unknown, reason: string): string {
  return JSON.stringify(toStableJsonValue(value, new Set(), reason), null, 2);
}

function toStableJsonValue(
  value: unknown,
  seen: Set<object>,
  reason: string,
): unknown {
  if (value === null) {
    return null;
  }

  const valueType = typeof value;
  if (valueType === "string" || valueType === "boolean") {
    return value;
  }
  if (valueType === "number") {
    if (!Number.isFinite(value)) {
      throwJsonUnsupported("non-finite number", reason);
    }
    return value;
  }
  if (
    valueType === "undefined" ||
    valueType === "function" ||
    valueType === "symbol" ||
    valueType === "bigint"
  ) {
    throwJsonUnsupported(valueType, reason);
  }

  if (typeof value !== "object") {
    throwJsonUnsupported(valueType, reason);
  }
  if (seen.has(value)) {
    throwJsonUnsupported("circular reference", reason);
  }

  seen.add(value);
  try {
    if (Array.isArray(value)) {
      return value.map((entry) => toStableJsonValue(entry, seen, reason));
    }

    const prototype = Object.getPrototypeOf(value);
    if (prototype !== Object.prototype && prototype !== null) {
      throwJsonUnsupported("non-plain object", reason);
    }

    const result: Record<string, unknown> = {};
    for (const key of Object.keys(value).sort()) {
      result[key] = toStableJsonValue(
        (value as Record<string, unknown>)[key],
        seen,
        reason,
      );
    }
    return result;
  } finally {
    seen.delete(value);
  }
}

function throwJsonUnsupported(detail: string, reason: string): never {
  throw snapshotFailure(
    "TSPACK_SNAPSHOT_JSON_UNSUPPORTED",
    `snapshotJson value is unsupported: ${detail}`,
    reason,
    { unsupported: detail },
  );
}

function buildMismatchDetails(
  displayPath: string,
  expected: string,
  actual: string,
): Record<string, unknown> {
  const firstDifference = firstDifferingLine(expected, actual);
  return {
    snapshot: displayPath,
    expectedHash: sha256(expected),
    actualHash: sha256(actual),
    firstDifferenceLine: firstDifference.line,
    expectedLine: firstDifference.expectedLine,
    actualLine: firstDifference.actualLine,
  };
}

function firstDifferingLine(
  expected: string,
  actual: string,
): { line: number; expectedLine: string; actualLine: string } {
  const expectedLines = expected.split("\n");
  const actualLines = actual.split("\n");
  const max = Math.max(expectedLines.length, actualLines.length);
  for (let index = 0; index < max; index += 1) {
    const expectedLine = expectedLines[index] ?? "";
    const actualLine = actualLines[index] ?? "";
    if (expectedLine !== actualLine) {
      return { line: index + 1, expectedLine, actualLine };
    }
  }
  return { line: 1, expectedLine: "", actualLine: "" };
}

function sha256(value: string): string {
  return `sha256-${crypto.createHash("sha256").update(value).digest("hex")}`;
}

function snapshotFailure(
  code: string,
  message: string,
  reason: string | undefined,
  details: Record<string, unknown>,
): AssertionFailure {
  const error = new Error(message) as AssertionFailure;
  error.code = code;
  error.assertion = "snapshot";
  error.reason = reason;
  error.details = details;
  return error;
}

function normalizePath(value: string): string {
  return value.replace(/\\/g, "/");
}
