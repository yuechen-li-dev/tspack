import {
  Audit,
  Build,
  CollectAll,
  CurrentHost,
  ForEach,
  GreaterThan,
  NotEmpty,
  On,
  Pack,
  ParallelForEach,
  Sequence,
  Test,
  Transfer,
  When,
  Windows,
  Workflow,
} from "tspack/manifest";

const build = Build();
const portable = Transfer(build.artifacts, Windows());
const audit = Audit();

Workflow("M78", {
  triggers: [],
  flow: Sequence(
    ForEach(
      "platform",
      [CurrentHost(), Windows()] as const,
      platform => On(platform, Test()),
      {
        mode: ParallelForEach({ concurrency: 2 }),
        failure: CollectAll(),
      },
    ),
    build,
    portable,
    On(Windows(), Pack(portable.artifacts)),
    When(NotEmpty(build.artifacts), Pack(build.artifacts)),
    audit,
    When(GreaterThan(audit.failing, 0), Audit()),
  ),
});
