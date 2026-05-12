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
