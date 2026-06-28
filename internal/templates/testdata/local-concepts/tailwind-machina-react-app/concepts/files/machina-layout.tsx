import type { LayoutRow, Rect } from "machinalayout";
import { resolveLayoutRows } from "machinalayout";
import { MachinaReactView } from "machinalayout/react";

const rootRect: Rect = { x: 0, y: 0, width: 720, height: 420 };

const rows: LayoutRow[] = [
  {
    id: "root",
    frame: { kind: "root" },
  },
  {
    id: "header",
    parent: "root",
    order: 0,
    frame: { kind: "anchor", left: 0, right: 0, top: 0, height: 72 },
    view: "header",
  },
  {
    id: "sidebar",
    parent: "root",
    order: 1,
    frame: { kind: "anchor", left: 0, top: 72, bottom: 0, width: 196 },
    view: "sidebar",
  },
  {
    id: "content",
    parent: "root",
    order: 2,
    frame: { kind: "anchor", left: 196, right: 0, top: 72, bottom: 0 },
    view: "content",
  },
];

const layout = resolveLayoutRows(rows, rootRect);

function HeaderView() {
  return <div className="machina-slot machina-header">MachinaLayout</div>;
}

function SidebarView() {
  return (
    <aside className="machina-slot machina-sidebar">
      Local concept fixture
    </aside>
  );
}

function ContentView() {
  return (
    <section className="machina-slot machina-content">
      <h1>React app arranged by MachinaLayout records.</h1>
      <p>
        TSPack composes this app from built-in React, Vite, and TypeScript
        concepts plus a local MachinaLayout concept that contributes the real
        npm dependency and integration files.
      </p>
    </section>
  );
}

const views = {
  header: HeaderView,
  sidebar: SidebarView,
  content: ContentView,
};

export function MachinaLayoutExample() {
  return (
    <div className="machina-stage">
      <MachinaReactView layout={layout} views={views} />
    </div>
  );
}
