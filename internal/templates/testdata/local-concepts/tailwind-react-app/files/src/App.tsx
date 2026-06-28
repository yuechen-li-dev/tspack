export function App() {
  return (
    <main className="min-h-screen bg-slate-950 px-6 py-16 text-white">
      <section className="mx-auto max-w-3xl rounded-3xl border border-slate-800 bg-slate-900/80 p-8 shadow-2xl shadow-cyan-950/30">
        <p className="text-sm font-semibold uppercase tracking-[0.3em] text-cyan-300">
          Local Tailwind concept
        </p>
        <h1 className="mt-4 text-4xl font-bold tracking-tight sm:text-5xl">
          React, Vite, TypeScript, and Tailwind from an explicit concept stack.
        </h1>
        <p className="mt-6 text-lg leading-8 text-slate-300">
          This fixture keeps Tailwind as a local concept so templates can
          compose real frontend stacks without adding a dedicated built-in
          Tailwind template permutation.
        </p>
      </section>
    </main>
  );
}
