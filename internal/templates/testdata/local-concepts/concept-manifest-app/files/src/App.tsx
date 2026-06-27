import { designSystemProject } from "./design-system";
import { cx } from "./ui";

export function App() {
  const titleClassName = cx("hero-card", "design-system-card");

  return (
    <main className="app-shell">
      <section className={titleClassName}>
        <p className="eyebrow">Local concept-rendered manifest</p>
        <h1>{designSystemProject}</h1>
        <p>
          This fixture proves a local template can combine built-in concepts,
          local concept manifest contributions, and normal source projections.
        </p>
      </section>
    </main>
  );
}
