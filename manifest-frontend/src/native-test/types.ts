export type Literal = string | number | boolean | null;

export type Diagnostic = {
  code: string;
  message: string;
  file: string;
  line?: number;
  column?: number;
};

export type DiscoveredCase = {
  index: number;
  data: Record<string, Literal>;
  id: string;
};

export type DiscoveredFact = {
  kind: 'fact';
  name: string;
  id: string;
};

export type DiscoveredTheory = {
  kind: 'theory';
  name: string;
  cases: DiscoveredCase[];
};

export type DiscoveryResult = {
  suiteName?: string;
  tests: string[];
  facts: DiscoveredFact[];
  theories: DiscoveredTheory[];
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

export type RunFilesOptions = {
  rootDir: string;
  files?: string[];
  filter?: string;
  listOnly?: boolean;
};

export type RunFilesResult = {
  results: TestResult[];
  diagnostics: Diagnostic[];
};

export type AssertionFailure = Error & {
  code: string;
  assertion: string;
  reason: string;
  expected?: unknown;
  actual?: unknown;
};

export type TestResult = {
  id: string;
  name: string;
  status: 'passed' | 'failed';
  durationMs?: number;
  error?: AssertionFailure | Error;
};
