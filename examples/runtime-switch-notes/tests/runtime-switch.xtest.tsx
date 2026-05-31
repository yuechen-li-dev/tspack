import {
  createDefaultModel,
  renderRuntimeSwitchNotes,
  type RuntimeSwitchNotesModel,
} from "../src/ui";

export default (
  <Suite name="runtime switch notes">
    <Fact name="renders required controls">
      {() => {
        const html = renderRuntimeSwitchNotes(createDefaultModel());

        assert.true(
          html.includes("<h1>Runtime Switch Notes</h1>"),
          "page heading should render",
        );
        assert.true(
          html.includes('aria-label="Runtime status"'),
          "runtime status section should be labelled",
        );
        assert.true(
          html.includes(">New note</button>"),
          "new note button should render",
        );
        assert.true(
          html.includes("data-tspack-component=\"RuntimeSwitchNotes\""),
          "source hint component should render",
        );
      }}
    </Fact>

    <Fact name="default model has typed runtimes and notes">
      {() => {
        const model = createDefaultModel();

        assert.equal(model.runtimes.length, 3, "three runtime rows should render");
        assert.equal(model.notes.length, 2, "two starter notes should render");
        assert.type<RuntimeSwitchNotesModel>(
          model,
          "default model should satisfy the exported model type",
        );
      }}
    </Fact>

    <Fact name="stable minimal render snapshot">
      {() => {
        expect
          .snapshotText(renderRuntimeSwitchNotes(createDefaultModel()), "default-render")
          .because("default HTML should remain stable for inspect smoke tests");
      }}
    </Fact>
  </Suite>
);
