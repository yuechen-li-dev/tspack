# M68d test runtime profile

Measured on Windows amd64, 16 logical processors, Go cache on the local system
drive, Node 26.2.0. Every test measurement used count=1.

## Baseline

The initial full JSON run took 316.138 seconds. Historical M68c comparable runs
were 271.984 and 276.817 seconds. The CLI package was stable across three
dedicated runs: 71.033, 69.789, and 69.728 seconds wall time (median 69.789).
Project measured 3.132, 2.823, and 2.841 seconds (median 2.841); check measured
0.847, 0.828, and 0.819 (median 0.828); resolver measured 2.931, 2.837, and
2.812 (median 2.837); manifest measured 0.730, 0.722, and 0.718 (median 0.722).

## Slowest baseline packages

| Rank | Package | Test elapsed |
| ---: | --- | ---: |
| 1 | internal/cli | 72.65 s |
| 2 | internal/materialize | 4.90 s |
| 3 | internal/project | 3.23 s |
| 4 | internal/resolver | 2.93 s |
| 5 | internal/testcmd | 2.03 s |
| 6 | internal/installscript | 1.25 s |
| 7 | internal/templates | 1.12 s |
| 8 | internal/projectir | 1.02 s |
| 9 | internal/integrations/skyrim | 0.88 s |
| 10 | internal/check | 0.83 s |
| 11 | internal/store | 0.74 s |
| 12 | internal/pack | 0.74 s |
| 13 | internal/manifest | 0.66 s |
| 14 | internal/lockfile | 0.62 s |
| 15 | internal/audit | 0.61 s |
| 16 | internal/typesurface | 0.60 s |
| 17 | internal/why | 0.59 s |
| 18 | internal/integrations/browser | 0.58 s |
| 19 | internal/graph | 0.58 s |
| 20 | internal/compat | 0.57 s |

## Slowest baseline tests and classification

| Test | Elapsed | Dominant cost |
| --- | ---: | --- |
| RuntimeSwitchDoctorRuntimeReportsSelectedProfile | 4.24 s | repeated manifest Node startup |
| CLIRunRuntimeSwitchExplicitTargetsAcrossProfiles | 4.05 s | repeated manifest and child processes |
| CompatHelpersFixtureCommands | 2.51 s | frontend build and TSPack processes |
| TemplateManifestEditorTypecheck | 2.21 s | repeated Go init and tsc |
| RepoRootManifestNarrowsEditorTSConfig | 2.16 s | duplicate frontend build |
| CLICheckFormatBackendAndConfigBehavior | 1.73 s | child tool and TSPack processes |
| CLIRunStdoutMatchStreamSelectionAndEarlyExit | 1.52 s | intentional timeout/process |
| CLIInspectRunTimeoutAndExitedEarly | 1.51 s | intentional timeout/process |
| CLICheckFormatJSONMissingBackendIsStructured | 1.50 s | TSPack process |
| DoctorFormatReportsDefaultBiomeConfigSource | 1.44 s | manifest Node startup |
| DoctorRuntimeReportsSelected variants | 1.40-1.44 s | manifest Node startup |
| CLIRunBun/DenoRuntimeMissing | 1.42-1.43 s | process/PATH boundary |
| CLIRunTCPReadyTimeout | 1.28 s | intentional timeout |
| CLIRunTimeoutAndInvalidTimeout | 1.25 s | intentional timeout |
| Init manifest typecheck/list-files tests | 1.05-1.10 s | real tsc |

The remaining top thirty were primarily RunTarget child-process contracts,
manifest frontend cold starts, and real TypeScript checks. No network cost was
present in the default CLI package.

## Child and build profile

The old path performed one Node frontend process per manifest load. A
representative three-profile doctor matrix therefore started Node three times;
the optimized path starts one worker and sends three requests. Structural
source counts found 120 generic TSPack exec call sites before this pass and 76
after conversion of semantic suites. Direct CLI test tsc call sites are three;
the template matrix changed from three invocations to one.

A no-test, vet-disabled full package run took 196.767 seconds. This proves that
roughly 197 seconds of the local full-suite wall time is Go test-binary
compile/link and endpoint scanning, not test execution. Package test elapsed
after optimization is led by CLI at about 45 seconds. Changing production
package ownership solely to reduce forty Windows test-binary links would
violate the repository architecture and is not an M68d stabilization change.

## Final profile

Three complete final runs passed in 230.312, 234.650, and 282.756 seconds
(median 234.650). Dedicated CLI runs passed in 44.776, 40.873, and 40.967
seconds (median 40.967), down 41 percent from baseline. Dedicated project runs
passed in 3.479, 2.779, and 2.797 seconds (median 2.797).

| Rank | Package | Test elapsed |
| ---: | --- | ---: |
| 1 | internal/cli | 41.30 s |
| 2 | internal/materialize | 4.44 s |
| 3 | internal/resolver | 2.82 s |
| 4 | internal/project | 2.72 s |
| 5 | internal/testcmd | 1.94 s |
| 6 | internal/installscript | 1.10 s |
| 7 | internal/templates | 0.96 s |
| 8 | internal/projectir | 0.92 s |
| 9 | internal/integrations/skyrim | 0.82 s |
| 10 | internal/check | 0.78 s |

The remaining slow individual tests are justified boundaries: materialization
retry (1.95 s), one cold compatibility/frontend build (1.92 s), process exit
and timeout contracts (1.24-1.53 s), the reduced runtime-target matrix (1.29
s), and real TypeScript checks (0.54-1.06 s).
