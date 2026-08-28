import { Audit, Build, MatchResult, Test } from "tspack/manifest";

Build().passed;
Test().artifacts;
Audit().targets;

MatchResult(Build(), {
  succeeded: () => Audit(),
  failed: () => Audit(),
});
