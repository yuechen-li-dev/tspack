export default (
  <Suite name="live UI inspection">
    <Fact name="disabled nested dismiss button retains source provenance">
      {async () => {
        const page = await inspect.runTarget({
          browser: "chromium",
          selector: "main",
          viewport: "800x600",
        });
        const alert = inspect.findByRole(page.root, "alert", "Save failed");
        const dismiss = inspect.findByRole(alert, "button", "Dismiss");

        assert.inspect.visible(alert, "error toast should be visible");
        assert.inspect.role(alert, "alert", "toast should expose alert semantics");
        assert.inspect.focusable(
          dismiss,
          false,
          "disabled dismiss button should not be keyboard focusable",
        );
        assert.inspect.source(
          dismiss,
          {
            file: "src/Toast.tsx",
            component: "Toast",
            symbol: "Toast.DismissButton",
          },
          "dismiss button should retain workspace-relative source provenance",
        );
      }}
    </Fact>
  </Suite>
);
