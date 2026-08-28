export function Toast() {
  return (
    <section role="alert" aria-label="Save failed">
      <span>Unable to save</span>
      <button type="button" disabled>
        Dismiss
      </button>
    </section>
  );
}
