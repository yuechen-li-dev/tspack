export function createDefaultModel() {
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

export function renderRuntimeSwitchNotes(model = createDefaultModel()) {
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

function renderRuntimeRow(runtime) {
  return `    <article class="runtime-card" data-runtime="${escapeAttribute(runtime.name)}">
      <h2>${escapeHtml(runtime.name)}</h2>
      <p>RunTarget: <code>${escapeHtml(runtime.runTarget)}</code></p>
      <p>Port: ${runtime.port}</p>
      <p>Status: ${escapeHtml(runtime.state)}</p>
    </article>`;
}

function renderNote(note) {
  return `    <li data-note-id="${escapeAttribute(note.id)}">${escapeHtml(note.text)}</li>`;
}

function escapeHtml(value) {
  return value
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

function escapeAttribute(value) {
  return escapeHtml(value);
}
