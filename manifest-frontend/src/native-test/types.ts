export type Literal = string | number | boolean | null;

export type Diagnostic = {
  code: string;
  message: string;
  file: string;
  line?: number;
  column?: number;
  severity?: 'error' | 'warning' | 'info';
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

export type DiscoveredStandaloneArtifact = {
  id: string;
  filePath: string;
  suiteName: string;
  name: string;
  path: string;
  format?: string;
};

export type DiscoveredFact = {
  kind: 'fact';
  name: string;
  id: string;
  artifacts: DiscoveredArtifact[];
};

export type DiscoveredTheory = {
  kind: 'theory';
  name: string;
  cases: DiscoveredCase[];
  artifacts: DiscoveredArtifact[];
};

export type DiscoveryResult = {
  suiteName?: string;
  tests: string[];
  facts: DiscoveredFact[];
  theories: DiscoveredTheory[];
  standaloneArtifacts: DiscoveredStandaloneArtifact[];
  diagnostics: Diagnostic[];
};

export type DiscoveredTest = {
  id: string;
  name: string;
  kind: 'fact' | 'theory';
  filePath: string;
};

export type DiscoveredFile = {
  filePath: string;
  suiteName: string;
  tests: DiscoveredTest[];
  standaloneArtifacts: DiscoveredStandaloneArtifact[];
  diagnostics: Diagnostic[];
};

export type DiscoverOptions = {
  rootDir: string;
  patterns?: string[];
  ignore?: string[];
};

export type DiscoverFilesResult = {
  files: DiscoveredFile[];
  diagnostics: Diagnostic[];
};

export type ListedTest = {
  id: string;
  filePath: string;
  suiteName: string;
  name: string;
  kind: 'fact' | 'theory-case';
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
};

export type RunArtifactsOptions = {
  rootDir: string;
  files?: string[];
  filter?: string;
  artifactRoot?: string;
  listOnly?: boolean;
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

export type TestResult = {
  id: string;
  name: string;
  status: 'passed' | 'failed' | 'skipped';
  durationMs?: number;
  skipReason?: string;
  error?: AssertionFailure | Error;
  artifacts?: TestArtifact[];
};

export type StandaloneArtifactResult = {
  id: string;
  name: string;
  status: 'passed' | 'failed' | 'skipped';
  artifact?: TestArtifact;
  failure?: FailureInfo;
  skipReason?: string;
  durationMs?: number;
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
  durationMs?: number;
};

export type ReportedTest = {
  id: string;
  name: string;
  filePath?: string;
  status: 'passed' | 'failed' | 'skipped';
  durationMs?: number;
  failure?: FailureInfo;
  skipReason?: string;
  artifacts?: TestArtifact[];
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

export type AssertionFailure = Error & {
  code: string;
  assertion: string;
  reason: string;
  expected?: unknown;
  actual?: unknown;
};
