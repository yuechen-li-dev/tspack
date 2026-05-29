import type { CDPTargetListResult, CDPTargetSummary } from "./types.js";

type RawTarget = {
  id?: string;
  targetId?: string;
  type?: string;
  title?: string;
  url?: string;
  webSocketDebuggerUrl?: string;
};

type CdpVersion = {
  webSocketDebuggerUrl?: string;
};

type CdpTargetInfo = {
  targetId?: string;
  type?: string;
  title?: string;
  url?: string;
};

type CdpTargetResponse = {
  targetInfos?: CdpTargetInfo[];
};

export function normalizeCdpEndpoint(endpoint: string | undefined): string {
  if (!endpoint) {
    throw new Error("TSPACK_INSPECT_CDP_ENDPOINT_REQUIRED");
  }
  let parsed: URL;
  try {
    parsed = new URL(endpoint);
  } catch {
    throw new Error("TSPACK_INSPECT_CDP_ENDPOINT_INVALID");
  }
  if (parsed.protocol !== "http:" && parsed.protocol !== "https:") {
    throw new Error("TSPACK_INSPECT_CDP_ENDPOINT_INVALID");
  }
  return parsed.origin;
}

async function fetchJson<T>(endpoint: string, suffix: string): Promise<T> {
  const url = `${endpoint}${suffix}`;
  let response: Response;
  try {
    response = await fetch(url);
  } catch {
    throw new Error("TSPACK_INSPECT_CDP_CONNECT_FAILED");
  }
  if (!response.ok) {
    throw new Error("TSPACK_INSPECT_CDP_CONNECT_FAILED");
  }
  return response.json() as Promise<T>;
}

async function tryFetchJson<T>(
  endpoint: string,
  suffix: string,
): Promise<T | undefined> {
  try {
    return await fetchJson<T>(endpoint, suffix);
  } catch {
    return undefined;
  }
}

function isInspectableTarget(target: RawTarget | CdpTargetInfo): boolean {
  return target.type === "page" || target.type === "webview";
}

function summarizeRawTargets(rawTargets: RawTarget[]): CDPTargetSummary[] {
  const targets: CDPTargetSummary[] = [];
  for (const target of rawTargets) {
    if (!isInspectableTarget(target)) {
      continue;
    }
    targets.push({
      index: targets.length,
      id: target.id ?? target.targetId ?? "",
      type: target.type ?? "unknown",
      title: target.title ?? "",
      url: target.url ?? "",
      webSocketDebuggerUrl: target.webSocketDebuggerUrl,
    });
  }
  return targets;
}

function summarizeTargetInfos(
  targetInfos: CdpTargetInfo[],
  browserWebSocketUrl: string,
): CDPTargetSummary[] {
  const targets: CDPTargetSummary[] = [];
  for (const target of targetInfos) {
    if (!isInspectableTarget(target)) {
      continue;
    }
    targets.push({
      index: targets.length,
      id: target.targetId ?? "",
      type: target.type ?? "unknown",
      title: target.title ?? "",
      url: target.url ?? "",
      webSocketDebuggerUrl: browserWebSocketUrl,
    });
  }
  return targets;
}

function waitForWebSocketOpen(socket: WebSocket): Promise<void> {
  return new Promise<void>((resolve, reject) => {
    const timeout = setTimeout(() => {
      reject(new Error("TSPACK_INSPECT_CDP_CONNECT_FAILED"));
    }, 5000);

    socket.addEventListener(
      "open",
      () => {
        clearTimeout(timeout);
        resolve();
      },
      { once: true },
    );

    socket.addEventListener(
      "error",
      () => {
        clearTimeout(timeout);
        reject(new Error("TSPACK_INSPECT_CDP_CONNECT_FAILED"));
      },
      { once: true },
    );
  });
}

async function sendCdpCommandOverWebSocket<T>(
  webSocketUrl: string,
  method: string,
): Promise<T> {
  let socket: WebSocket;
  try {
    socket = new WebSocket(webSocketUrl);
  } catch {
    throw new Error("TSPACK_INSPECT_CDP_CONNECT_FAILED");
  }

  try {
    await waitForWebSocketOpen(socket);

    return await new Promise<T>((resolve, reject) => {
      const timeout = setTimeout(() => {
        reject(new Error("TSPACK_INSPECT_CDP_CONNECT_FAILED"));
      }, 5000);

      socket.addEventListener("message", (event) => {
        const data = typeof event.data === "string" ? event.data : "";
        const message = JSON.parse(data) as {
          id?: number;
          result?: T;
          error?: unknown;
        };
        if (message.id !== 1) {
          return;
        }

        clearTimeout(timeout);
        if (message.error) {
          reject(new Error("TSPACK_INSPECT_CDP_CONNECT_FAILED"));
          return;
        }
        resolve(message.result as T);
      });

      socket.addEventListener(
        "error",
        () => {
          clearTimeout(timeout);
          reject(new Error("TSPACK_INSPECT_CDP_CONNECT_FAILED"));
        },
        { once: true },
      );

      socket.send(JSON.stringify({ id: 1, method }));
    });
  } finally {
    socket.close();
  }
}

async function listTargetsViaBrowserWebSocket(endpoint: string): Promise<{
  targets: CDPTargetSummary[];
  diagnostic?: { code: string; message: string };
}> {
  const version = await fetchJson<CdpVersion>(endpoint, "/json/version");
  if (!version.webSocketDebuggerUrl) {
    throw new Error("TSPACK_INSPECT_CDP_CONNECT_FAILED");
  }

  const result = await sendCdpCommandOverWebSocket<CdpTargetResponse>(
    version.webSocketDebuggerUrl,
    "Target.getTargets",
  );
  const targetInfos = Array.isArray(result.targetInfos)
    ? result.targetInfos
    : [];
  return {
    targets: summarizeTargetInfos(targetInfos, version.webSocketDebuggerUrl),
    diagnostic: {
      code: "TSPACK_INSPECT_CDP_TARGETS_FROM_WEBSOCKET",
      message: "CDP targets were discovered through Target.getTargets.",
    },
  };
}

export async function listCdpTargets(
  endpoint: string,
): Promise<CDPTargetListResult> {
  const rawTargets = await tryFetchJson<RawTarget[]>(endpoint, "/json/list");
  if (rawTargets) {
    const targets = summarizeRawTargets(rawTargets);
    if (targets.length > 0) {
      return {
        command: "inspect",
        mode: "list-targets",
        cdp: endpoint,
        endpoint,
        targets,
        diagnostics: [],
      };
    }
  }

  const fallback = await listTargetsViaBrowserWebSocket(endpoint);
  const diagnostics = fallback.diagnostic ? [fallback.diagnostic] : [];
  return {
    command: "inspect",
    mode: "list-targets",
    cdp: endpoint,
    endpoint,
    targets: fallback.targets,
    diagnostics,
  };
}
