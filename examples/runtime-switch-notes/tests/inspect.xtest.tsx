type InspectNode = {
  id: string;
  tag: string;
  role?: string;
  name?: string;
  text?: string;
  bounds: { x: number; y: number; width: number; height: number };
  visible: boolean;
  source?: {
    raw?: string;
    file?: string;
    line?: number;
    column?: number;
    component?: string;
    symbol?: string;
  };
  children: InspectNode[];
};

function sampleInspectTree(): InspectNode {
  return {
    id: "node-1",
    tag: "main",
    role: "main",
    name: "Runtime Switch Notes",
    text: "Runtime Switch Notes New note",
    bounds: { x: 0, y: 0, width: 720, height: 520 },
    visible: true,
    source: {
      raw: "src/app/index.ts:12:3",
      file: "src/app/index.ts",
      line: 12,
      column: 3,
      component: "RuntimeSwitchNotes",
      symbol: "RuntimeSwitchNotes.App",
    },
    children: [
      {
        id: "node-2",
        tag: "section",
        role: "region",
        name: "Runtime status",
        text: "Node.js Bun Deno",
        bounds: { x: 16, y: 80, width: 640, height: 260 },
        visible: true,
        children: [],
      },
      {
        id: "node-3",
        tag: "button",
        role: "button",
        name: "New note",
        text: "New note",
        bounds: { x: 16, y: 360, width: 96, height: 36 },
        visible: true,
        source: {
          file: "src/app/index.ts",
          component: "NewNoteButton",
          symbol: "RuntimeSwitchNotes.App",
        },
        children: [],
      },
    ],
  };
}

export default (
  <Suite name="runtime switch inspect">
    <Fact name="source hinted tree is inspectable">
      {() => {
        const root = sampleInspectTree();
        const button = inspect.findByRole(root, "button", "New note");

        assert.inspect.visible(root, "main should be visible");
        assert.inspect.role(root, "main", "main role should be preserved");
        assert.inspect.exists(button, "new note button should be discoverable");
        assert.inspect.boundsWithin(
          button,
          { minWidth: 80, minHeight: 32, maxX: 200, maxY: 420 },
          "button should have stable smoke-test bounds",
        );
        assert.inspect.source(
          root,
          {
            file: "src/app/index.ts",
            component: "RuntimeSwitchNotes",
            symbol: "RuntimeSwitchNotes.App",
          },
          "root should retain source hints",
        );
        expect
          .snapshotJson(
            {
              buttonName: button?.name,
              rootComponent: root.source?.component,
              roles: inspect.flatten(root).map((node) => node.role || node.tag),
            },
            "inspect-subtree",
          )
          .because("selected inspect tree data should remain stable");
      }}
    </Fact>
  </Suite>
);
