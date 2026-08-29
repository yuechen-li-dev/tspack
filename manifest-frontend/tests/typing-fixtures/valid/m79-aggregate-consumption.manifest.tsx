import {
  CollectAll,
  CurrentHost,
  ForEach,
  GreaterThan,
  MatchResult,
  On,
  ParallelForEach,
  Process,
  Test,
  When,
  Windows,
} from "tspack/manifest";

const runs = ForEach(
  "platform",
  [CurrentHost(), Windows()] as const,
  platform => On(platform, Test()),
  {
    mode: ParallelForEach({ concurrency: 2 }),
    failure: CollectAll(),
  },
);

const first = runs[0];
const count: number = runs.length;

MatchResult(first, {
  succeeded: result => When(
    GreaterThan(result.failed, 0),
    Process("record", { command: ["node", "--version"] }),
  ),
  failed: failure => Process(failure.kind, { command: ["node", "--version"] }),
  cancelled: cancellation => Process(cancellation.kind, { command: ["node", "--version"] }),
  timedOut: timeout => Process(timeout.kind, { command: ["node", "--version"] }),
});

ForEach("run", runs, run => MatchResult(run, {
  succeeded: result => When(
    GreaterThan(result.failed, 0),
    Process("record", { command: ["node", "--version"] }),
  ),
  failed: failure => Process(failure.kind, { command: ["node", "--version"] }),
  cancelled: cancellation => Process(cancellation.kind, { command: ["node", "--version"] }),
  timedOut: timeout => Process(timeout.kind, { command: ["node", "--version"] }),
}));

void count;
