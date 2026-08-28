import {
  Audit,
  Build,
  CurrentHost,
  Finally,
  ForEach,
  MatchResult,
  On,
  Pack,
  Sequence,
  Test,
  Workflow,
} from "tspack/manifest";

const build = Build();

Workflow("M77", {
  triggers: [],
  flow: Sequence(
    build,
    MatchResult(build, {
      succeeded: result => Pack(result.artifacts),
      failed: () => Audit(),
      cancelled: () => Audit(),
      timedOut: () => Audit(),
    }),
    Finally(Test(), Audit()),
    ForEach("platform", [CurrentHost()] as const, platform => On(platform, Test())),
  ),
});
