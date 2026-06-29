const features = [
  "package.json remains the compatibility substrate",
  "ordinary npm scripts continue to build the app",
  "TSPack can observe the project before a manifest exists",
];

export function App() {
  return (
    <main className="app-shell">
      <section className="hero-card">
        <p className="eyebrow">Existing Vite + React + TypeScript app</p>
        <h1>Incremental adoption starts without migration.</h1>
        <p className="lede">
          This dogfood project intentionally has no TSPack manifest. It should
          work with npm first, then provide a realistic surface for read-only
          TSPack adoption reports.
        </p>
        <ul>
          {features.map((feature) => (
            <li key={feature}>{feature}</li>
          ))}
        </ul>
      </section>
    </main>
  );
}
