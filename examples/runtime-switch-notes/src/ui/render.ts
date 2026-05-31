export type RuntimeStatus = {
  name: "Node.js" | "Bun" | "Deno";
  port: number;
  runTarget: string;
  state: "ready" | "optional";
};

export type Note = {
  id: string;
  text: string;
};

export type RuntimeSwitchNotesModel = {
  runtimes: RuntimeStatus[];
  notes: Note[];
};

export function createDefaultModel(): RuntimeSwitchNotesModel {
  return {
    runtimes: [
      {
        name: "Node.js",
        port: 4171,
        runTarget: "node-server",
        state: "ready",
      },
      {
        name: "Bun",
        port: 4172,
        runTarget: "bun-server",
        state: "optional",
      },
      {
        name: "Deno",
        port: 4173,
        runTarget: "deno-server",
        state: "optional",
      },
    ],
    notes: [
      {
        id: "note-1",
        text: "Switch the workspace runtime profile with one manifest line.",
      },
      {
        id: "note-2",
        text: "RunTargets stay explicit about node, bun, and deno.",
      },
    ],
  };
}

export function renderRuntimeSwitchNotes(
  model: RuntimeSwitchNotesModel = createDefaultModel(),
): string {
  const runtimeRows = model.runtimes.map(renderRuntimeRow).join("\n");
  const noteRows = model.notes.map(renderNote).join("\n");

  return `
<main
  data-tspack-source="src/app/index.ts:12:3"
  data-tspack-component="RuntimeSwitchNotes"
  data-tspack-symbol="RuntimeSwitchNotes.App"
>
  <h1>Runtime Switch Notes</h1>
  <section aria-label="Runtime status">
${runtimeRows}
  </section>
  <button type="button" data-tspack-component="NewNoteButton">New note</button>
  <ul aria-label="Sample notes">
${noteRows}
  </ul>
</main>`.trim();
}

function renderRuntimeRow(runtime: RuntimeStatus): string {
  return `    <article class="runtime-card" data-runtime="${escapeAttribute(runtime.name)}">
      <h2>${escapeHtml(runtime.name)}</h2>
      <p>RunTarget: <code>${escapeHtml(runtime.runTarget)}</code></p>
      <p>Port: ${runtime.port}</p>
      <p>Status: ${escapeHtml(runtime.state)}</p>
    </article>`;
}

function renderNote(note: Note): string {
  return `    <li data-note-id="${escapeAttribute(note.id)}">${escapeHtml(note.text)}</li>`;
}

function escapeHtml(value: string): string {
  return value
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

function escapeAttribute(value: string): string {
  return escapeHtml(value);
}
