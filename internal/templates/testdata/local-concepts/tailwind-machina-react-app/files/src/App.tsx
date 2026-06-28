import { MachinaLayoutExample } from "./machina-layout";

export function App() {
  return (
    <main className="min-h-screen bg-slate-950 px-6 py-10 text-slate-100">
      <section className="mx-auto flex max-w-5xl flex-col gap-8">
        <header className="rounded-3xl border border-cyan-400/20 bg-slate-900/80 p-8 shadow-2xl shadow-cyan-950/30">
          <p className="text-sm font-semibold uppercase tracking-[0.3em] text-cyan-300">
            Composed local concepts
          </p>
          <h1 className="mt-4 text-4xl font-bold tracking-tight sm:text-5xl">
            Tailwind CSS and MachinaLayout share one explicit concept stack.
          </h1>
          <p className="mt-6 max-w-3xl text-lg leading-8 text-slate-300">
            The base React app owns the shell and Vite config, Tailwind owns the
            stylesheet contribution, and MachinaLayout owns its layout module.
          </p>
        </header>

        <div className="rounded-3xl border border-slate-800 bg-white p-3 text-slate-950 shadow-2xl shadow-slate-950/30">
          <MachinaLayoutExample />
        </div>
      </section>
    </main>
  );
}
