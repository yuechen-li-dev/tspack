export type Literal = string | number | boolean | null;

export type Diagnostic = {
  code: string;
  message: string;
  file: string;
  line?: number;
  column?: number;
  severity?: "error" | "warning" | "info";
};

export type DiscoveredCase = {
  index: number;
  data: Record<string, Literal>;
  id: string;
};

export type DiscoveredArtifact = {
  name: string;
  path: string;
  format?: string;
  required: boolean;
};

export type DiscoveredProjectFixture = {
  from?: string;
  name?: string;
  keepOnFailure: boolean;
};

export type DiscoveredStandaloneArtifact = {
  id: string;
  filePath: string;
  suiteName: string;
  name: string;
  path: string;
  format?: string;
  project?: DiscoveredProjectFixture;
  cycleTimeSeconds?: number;
};

export type DiscoveredFact = {
  kind: "fact";
  name: string;
  id: string;
  artifacts: DiscoveredArtifact[];
  project?: DiscoveredProjectFixture;
};

export type DiscoveredTheory = {
  kind: "theory";
  name: string;
  cases: DiscoveredCase[];
  artifacts: DiscoveredArtifact[];
  project?: DiscoveredProjectFixture;
  cycleTimeSeconds?: number;
};

export type DiscoveredInvariant = {
  kind: "valid" | "invalid";
  name: string;
  id: string;
  project?: DiscoveredProjectFixture;
};

export type DiscoveredProphecy = {
  id: string;
  filePath: string;
  suiteName: string;
  name: string;
  foretell: { reason: string };
  cycleTimeSeconds: number;
};

export type DiscoveryResult = {
  suiteName?: string;
  tests: string[];
  facts: DiscoveredFact[];
  theories: DiscoveredTheory[];
  invariants: DiscoveredInvariant[];
  standaloneArtifacts: DiscoveredStandaloneArtifact[];
  benchmarks: DiscoveredBenchmark[];
  prophecies: DiscoveredProphecy[];
  diagnostics: Diagnostic[];
};

export type DiscoveredTest = {
  id: string;
  name: string;
  kind: "fact" | "theory" | "valid" | "invalid";
  filePath: string;
};

export type DiscoveredFile = {
  filePath: string;
  suiteName: string;
  tests: DiscoveredTest[];
  standaloneArtifacts: DiscoveredStandaloneArtifact[];
  benchmarks: DiscoveredBenchmark[];
  prophecies: DiscoveredProphecy[];
  diagnostics: Diagnostic[];
};

export type DoomEnvelope = {
  prophecyId: string;
  suiteName: string;
  name: string;
  foretell: { reason: string };
  phase: "before-doom";
};
export type DoomResult = {
  id: string;
  name: string;
  status: "passed" | "failed" | "skipped";
  exitCode?: number | null;
  signal?: string | null;
  stdout?: string;
  stderr?: string;
  envelopePath?: string;
  envelope?: DoomEnvelope;
  failure?: FailureInfo;
};
export type DoomRunResult = {
  prophecies: DoomResult[];
  diagnostics: Diagnostic[];
};
export type NativeDoomRunReport = {
  summary: NativeTestSummary;
  prophecies: DoomResult[];
  diagnostics: Diagnostic[];
};

export type DiscoverOptions = {
  rootDir: string;
  patterns?: string[];
  ignore?: string[];
};

export type DiscoverFilesResult = {
  rootDir: string;
  files: DiscoveredFile[];
  diagnostics: Diagnostic[];
};

export type ListedTest = {
  id: string;
  filePath: string;
  suiteName: string;
  name: string;
  kind: "fact" | "theory-case" | "valid" | "invalid";
  theoryName?: string;
  caseIndex?: number;
  caseData?: Record<string, unknown>;
  artifacts: DiscoveredArtifact[];
};

export type ListedStandaloneArtifact = DiscoveredStandaloneArtifact;

export type RunFilesOptions = {
  rootDir: string;
  files?: string[];
  filter?: string;
  listOnly?: boolean;
  artifactRoot?: string;
  defaultTimeoutSeconds?: number;
  updateSnapshots?: boolean;
  batch?: boolean;
};

export type RunArtifactsOptions = {
  rootDir: string;
  files?: string[];
  filter?: string;
  artifactRoot?: string;
  listOnly?: boolean;
  defaultTimeoutSeconds?: number;
};

export type FailureInfo = {
  code?: string;
  message: string;
  reason?: string;
  assertion?: string;
  actual?: unknown;
  expected?: unknown;
  details?: Record<string, unknown>;
};

export type TestSnapshotUpdate = {
  path: string;
  reason: string;
};

export type TestArtifact = {
  name: string;
  declaredPath: string;
  outputPath: string;
  format?: string;
  required: boolean;
  written: boolean;
  size?: number;
  hash?: string;
  reason?: string;
};

export type ProjectResultInfo = {
  sourcePath?: string;
  name?: string;
  kept: boolean;
  rootPath?: string;
};

export type TestResult = {
  id: string;
  name: string;
  status: "passed" | "failed" | "skipped";
  durationMs?: number;
  skipReason?: string;
  error?: AssertionFailure | Error;
  artifacts?: TestArtifact[];
  project?: ProjectResultInfo;
  snapshots?: TestSnapshotUpdate[];
};

export type StandaloneArtifactResult = {
  id: string;
  name: string;
  status: "passed" | "failed" | "skipped";
  artifact?: TestArtifact;
  failure?: FailureInfo;
  skipReason?: string;
  durationMs?: number;
  project?: ProjectResultInfo;
};

export type RunFilesResult = {
  results: TestResult[];
  diagnostics: Diagnostic[];
};

export type ArtifactRunResult = {
  artifacts: StandaloneArtifactResult[];
  diagnostics: Diagnostic[];
};

export type NativeTestSummary = {
  total: number;
  passed: number;
  failed: number;
  skipped: number;
  diagnostics: number;
  snapshotsUpdated?: number;
  durationMs?: number;
  project?: ProjectResultInfo;
};

export type ReportedTest = {
  id: string;
  name: string;
  filePath?: string;
  status: "passed" | "failed" | "skipped";
  durationMs?: number;
  failure?: FailureInfo;
  skipReason?: string;
  artifacts?: TestArtifact[];
  project?: ProjectResultInfo;
  snapshots?: TestSnapshotUpdate[];
};

export type NativeTestRunReport = {
  summary: NativeTestSummary;
  tests: ReportedTest[];
  diagnostics: Diagnostic[];
};

export type NativeArtifactRunReport = {
  summary: NativeTestSummary;
  artifacts: StandaloneArtifactResult[];
  diagnostics: Diagnostic[];
};

export type DiscoveredBenchmark = {
  id: string;
  filePath: string;
  suiteName: string;
  name: string;
  iterations: number;
  warmup: number;
  cycleTimeSeconds: number;
  project?: DiscoveredProjectFixture;
};

export type RunBenchmarksOptions = {
  rootDir: string;
  files?: string[];
  filter?: string;
  listOnly?: boolean;
  defaultCycleTimeSeconds?: number;
};

export type BenchmarkResult = {
  id: string;
  name: string;
  status: "passed" | "failed" | "skipped";
  iterations: number;
  warmup: number;
  totalMs?: number;
  meanMs?: number;
  minMs?: number;
  maxMs?: number;
  medianMs?: number;
  p95Ms?: number;
  opsPerSecond?: number;
  failure?: FailureInfo;
  skipReason?: string;
};

export type BenchmarkRunResult = {
  benchmarks: BenchmarkResult[];
  diagnostics: Diagnostic[];
};

export type NativeBenchmarkRunReport = {
  summary: NativeTestSummary;
  benchmarks: BenchmarkResult[];
  diagnostics: Diagnostic[];
};

export type AssertionFailure = Error & {
  code: string;
  assertion: string;
  reason?: string;
  expected?: unknown;
  actual?: unknown;
  details?: Record<string, unknown>;
};
