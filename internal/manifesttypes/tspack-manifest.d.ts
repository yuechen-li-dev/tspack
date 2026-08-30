/**
 * Authoring-time types for `tspack/manifest`.
 * Keep this aligned with the manifest frontend parser and Go IR validator.
 * This file is type-surface support only; it is not runtime implementation.
 */

declare module 'tspack/manifest' {
  export type Primitive = string | number | boolean | null;
  export type JSONValue = Primitive | JSONValue[] | { [key: string]: JSONValue };
  export type JSONObject = { [key: string]: JSONValue };

  export type TsConfigManifestEditor = {
    compilerOptions: JSONObject;
    include: string[];
    exclude: string[];
  };

  export type ManifestEditorTsConfigOptions = {
    include?: string[];
    exclude?: string[];
  };

  export type VSCodeSettings = JSONObject;
  export type VSCodeSettingsInput = JSONObject & {
    typescriptTsdk?: string;
  };

  export type VSCodeExtensions = {
    recommendations?: string[];
    unwantedRecommendations?: string[];
  };

  export type RuntimeProfile = 'nodejs' | 'bun' | 'deno';
  export type Compiler = 'tsc' | 'tscl' | 'scriptc' | 'perry' | 'rollup';

  export type NpmSource = {
    kind: 'npm';
    package: string;
    range: string;
  };

  export type JsrSource = {
    kind: 'jsr';
    package: string;
    range: string;
  };

  export type GitSource = {
    kind: 'git';
    ref: string;
    commit?: string;
    tag?: string;
    branch?: string;
  };

  export type PathSource = {
    kind: 'path';
    path: string;
  };

  export type WorkspaceSource = {
    kind: 'workspace';
    name: string;
    target?: string;
  };

  export type DependencySource = NpmSource | JsrSource | GitSource | PathSource | WorkspaceSource;

  export type DependencyDeclarationMetadata = {
    id?: string;
    origin?: {
      kind:
        | 'project-manifest'
        | 'package-manifest'
        | 'concept'
        | 'template'
        | 'compatibility'
        | 'generated'
        | 'explicit-user-operation';
      name?: string;
      sourcePath?: string;
      ref?: string;
    };
    layer?: 'base' | 'concept' | 'template' | 'project' | 'package' | 'compatibility' | 'explicit';
    layerOrder?: number;
    authority?: 'owned' | 'observed' | 'generated';
    editability?: 'editable' | 'derived' | 'observed' | 'generated' | 'concept-owned';
  };

  export type DependencyOptions = {
    key?: string;
    patch?: {
      path: string;
      version: string;
    };
    declaration?: DependencyDeclarationMetadata;
  };

  export type DepIntent = {
    kind: 'dep';
    source: DependencySource;
    key?: string;
  };

  export type PeerOptions = DependencyOptions & {
    optional?: boolean;
  };

  export type PeerIntent = {
    kind: 'peer';
    source: DependencySource;
    key?: string;
    optional?: boolean;
  };

  export type ToolIntent = {
    kind: 'tool';
    source: DependencySource;
    key?: string;
  };

  export type DependencyIntent = DepIntent | PeerIntent | ToolIntent;

  export type TypePolicy = {
    mode?: string;
    strict?: boolean;
    [key: string]: Primitive | Primitive[] | Record<string, Primitive> | undefined;
  };

  export type BoundaryRow = {
    from?: string;
    transitiveFrom?: string;
    to?: string;
    allow?: string[];
    deny?: string[];
    allowDeps?: string[];
    denyDeps?: string[];
    denyTypeDeps?: string[];
    allowOnly?: string[];
    [key: string]: Primitive | Primitive[] | undefined;
  };


  export type LifecycleScriptName =
    | 'preinstall'
    | 'install'
    | 'postinstall'
    | 'prepack'
    | 'prepare'
    | 'postpack'
    | 'prepublish'
    | 'prepublishOnly'
    | 'publish'
    | 'postpublish';

  export type LifecycleCategory = 'consumer-install' | 'maintainer-publish' | 'other';

  export type AcknowledgedLifecycleCategory = {
    category: LifecycleCategory;
    scripts?: LifecycleScriptName[];
    reason: string;
  };

  export type AcknowledgedCapability = {
    package: string;
    kind: 'lifecycleScript';
    script: LifecycleScriptName;
    command: string;
    reason: string;
    behaviorFixture?: string;
    behaviorReport?: string;
  };

  export type BoundaryPolicy = {
    mode?: string;
    rows?: BoundaryRow[];
    [key: string]: Primitive | Primitive[] | BoundaryRow[] | undefined;
  };

  export type UpdatePolicyKind = 'tool' | 'dep' | 'peer' | 'any';
  export type UpdatePolicyStrategy = 'manual' | 'pinned' | 'rolling';
  export type UpdatePolicyLevel = 'patch' | 'minor' | 'major' | 'latest';

  export type UpdatePolicyRow = {
    name: string;
    kind: UpdatePolicyKind;
    strategy: UpdatePolicyStrategy;
    level?: UpdatePolicyLevel;
    reason?: string;
    includePrerelease?: boolean;
    packages?: string[];
  };

  export type DependencyRefLike = DependencyIntent;

  export type TargetDependencyRefLike = string | DependencyIntent;

  export type CopelandNpmExportContract = {
    name: string;
    parameters: string[];
    result: string;
    remoteError?: string;
    promise?: boolean;
  };

  export type CopelandNpmContract = {
    package: string;
    exports: CopelandNpmExportContract[];
    components?: CopelandNpmComponentContract[];
  };

  export type CopelandNpmComponentProperty = {
    name: string;
    type: string;
    required?: boolean;
  };

  export type CopelandNpmComponentMember = {
    name: string;
    properties?: CopelandNpmComponentProperty[];
  };

  export type CopelandNpmComponentContract = {
    name: string;
    properties?: CopelandNpmComponentProperty[];
    members?: CopelandNpmComponentMember[];
  };

  export type TargetRow = {
    name: string;
	language?: string;
	compiler?: Compiler;
	compilerPath?: string;
	compilerConfig?: string;
	inputs?: string[];
	dependsOn?: string[];
	artifacts?: TargetArtifactRow[];
	artifact?: 'javaScript' | 'managedExecutable' | 'nativeExecutable' | 'wasmModule';
    export?: string;
    entry: string;
    runtime: string;
    types?: string;
    javascriptRuntime?: 'node' | 'browser';
	targetFramework?: string;
	runtimeIdentifier?: string;
    tsXmlProfile?: 'react-m0';
    npmContracts?: CopelandNpmContract[];
    deps?: TargetDependencyRefLike[];
    peers?: TargetDependencyRefLike[];
    optional?: boolean;
    [key: string]: Primitive | Primitive[] | TargetDependencyRefLike[] | CopelandNpmContract[] | TargetArtifactRow[] | undefined;
  };

  export type TargetArtifactRow = {
    name: string;
    kind: 'javaScript' | 'typeDeclarations' | 'sourceMap' | 'metadata';
    path: string;
    role?: 'runtimeEntry' | 'runtimeChunk' | 'typeDeclaration' | 'declarationChunk' | 'sourceMap' | 'metadata';
  };

  export type RunTargetReady =
    | {
        kind: 'http';
        path: string;
      }
    | {
        kind: 'tcp';
        host?: string;
        port: number;
      }
    | {
        kind: 'stdout-match';
        pattern: string;
        stream?: 'stdout' | 'stderr' | 'both';
      };

  export type RunTargetEnvRow = {
    name: string;
    required?: boolean;
    default?: string;
    secret?: boolean;
    description?: string;
  };

  export type RunTargetServiceRequirementBase = {
    kind?: 'service';
    name: string;
    expectStatus?: number;
    timeoutMs?: number;
    optional?: boolean;
    description?: string;
  };

  export type RunTargetServiceRequirementRow =
    | (RunTargetServiceRequirementBase & { tcp: string; http?: never })
    | (RunTargetServiceRequirementBase & { http: string; tcp?: never });

  export type RunTargetRow = {
    name: string;
    runtime?: 'system' | 'node' | 'bun' | 'deno';
    command: string[];
    url?: string;
    cwd?: 'workspace' | 'package';
    ready?: RunTargetReady;
    env?: RunTargetEnvRow[];
    requires?: RunTargetServiceRequirementRow[];
  };

  export type TestTargetRow = {
    name: string;
    harness: 'vitest' | 'xtest';
    config?: string;
    sources: string[];
    project?: string;
    requirements?: TargetDependencyRefLike[];
    fixtures?: LocalFixtureIntent[];
    dependsOn?: string[];
    builtFixtures?: BuiltArtifactFixtureIntent[];
  };

  export type LocalFixtureOptions = {
    name: string;
    binding?: string;
    mode?: 'package' | 'source';
  };

  export type LocalFixtureIntent = LocalFixtureOptions & {
    dependency: TargetDependencyRefLike;
  };

  export type BuiltArtifactFixtureBaseOptions = {
    name: string;
    binding: string;
  };

  export type BuiltArtifactFixtureOptions = BuiltArtifactFixtureBaseOptions & (
    | { artifact: string; artifacts?: never }
    | { artifact?: never; artifacts: string[] }
  );

  export type BuiltArtifactFixtureIntent = BuiltArtifactFixtureOptions & {
    target: string;
  };

  export type DevProxyRoute = {
    path: string;
    target?: string;
    webSocket?: boolean;
    secure?: boolean;
  };

  export type DevBackend = {
    kind: 'process' | 'aspnet';
    command?: string[];
    url: string;
    cwd?: 'workspace' | 'package';
    ready?: RunTargetReady;
    env?: RunTargetEnvRow[];
    ownsProcess?: boolean;
    proxyRoutes: DevProxyRoute[];
  };

  export type PackageRow = {
    name: string;
    root: string;
    manifest: string;
  };

  export type PackageAnnotationProps = {
    name?: string;
    dependencyDeclaration?: DependencyDeclarationMetadata;
    dependencies?: {
      values: DependencyIntent[];
    };
    children?: ManifestNode;
  };

  export type PackageProps = {
    name: string;
    publicationName?: string;
    version: string;
    kind: 'library' | 'app' | 'service';
    compiler?: Compiler;
    compilerPath?: string;
    license?: string;
    dependencies?: {
      values: DependencyIntent[];
    };
    dependencyDeclaration?: DependencyDeclarationMetadata;
    devBackend?: DevBackend;
    children?: ManifestNode;
  };

  export type WorkspaceProps = {
    name: string;
    runtime?: RuntimeProfile;
    children?: ManifestNode;
  };

  export type PackagesProps = {
    rows: PackageRow[];
  };

  export type PoliciesProps = {
    types?: TypePolicy;
    boundaries?: BoundaryPolicy;
  };

  export type TargetsProps = {
    rows: TargetRow[];
  };

  export type RunTargetsProps = {
    rows: RunTargetRow[];
  };

  export type TestTargetsProps = {
    rows: TestTargetRow[];
  };

  export type WorkflowPlatform = 'linux' | 'windows' | 'macos' | 'currentHost';
  export type WorkflowTriggerFilter = { branches?: string[]; paths?: string[] };
  export type WorkflowTrigger = { kind: 'manual' | 'push' | 'pullRequest'; branches?: string[]; paths?: string[] };
  export type WorkflowValue =
    | { kind: 'plain'; value: string }
    | { kind: 'secret'; name: string };
  export type WorkflowEnvRow = { name: string; value: WorkflowValue };
  export type WorkflowStepOptions = {
    name?: string;
    packages?: string[];
    env?: WorkflowEnvRow[];
    timeoutSeconds?: number;
  };
  export type WorkflowBuildStepOptions = WorkflowStepOptions & { targets?: string[] };
  export type WorkflowTestStepOptions = WorkflowStepOptions & {
    target?: string;
    filter?: string;
  };
  export type WorkflowAuditStepOptions = WorkflowStepOptions & {
    auditLevel?: 'any' | 'low' | 'moderate' | 'high' | 'critical';
    requireCoverage?: boolean;
  };
  export type WorkflowProcessStepOptions = WorkflowStepOptions & {
    command: string[];
    cwd?: 'workspace' | `package:${string}`;
    capabilities?: Array<'network' | 'workspaceRead' | 'workspaceWrite' | 'environment' | 'secrets' | 'process'>;
  };
  export type WorkflowShellStepOptions = WorkflowStepOptions & {
    script: string;
    shell?: 'sh' | 'powershell';
    cwd?: 'workspace' | `package:${string}`;
    capabilities?: Array<'network' | 'workspaceRead' | 'workspaceWrite' | 'environment' | 'secrets' | 'process'>;
  };
  export type WorkflowStep =
    | (WorkflowStepOptions & { operation: 'sync' | 'check' | 'pack' })
    | (WorkflowBuildStepOptions & { operation: 'build' })
    | (WorkflowTestStepOptions & { operation: 'test' })
    | (WorkflowAuditStepOptions & { operation: 'audit' })
    | (WorkflowProcessStepOptions & { operation: 'process' })
    | (WorkflowShellStepOptions & { operation: 'shellScript' });
  export type WorkflowValueCategory = 'control' | 'smallSerialized' | 'artifactReference' | 'regionLocal' | 'placement';
  export type WorkflowValueRef<T, Category extends WorkflowValueCategory = WorkflowValueCategory> = {
    readonly __workflowValueType?: T;
    readonly __workflowValueCategory?: Category;
  };
  export type WorkflowDiagnostic = {
    code: string;
    severity: 'info' | 'warning' | 'error';
    message: string;
  };
  export type WorkflowBuildArtifact = { package: string; target: string; kind: string; path: string };
  export type WorkflowBuildTarget = {
    package: string;
    target: string;
    compiler: string;
    succeeded: boolean;
    artifacts: WorkflowBuildArtifact[];
    diagnostics: WorkflowDiagnostic[];
  };
  export type WorkflowBuildResultRef = {
    artifacts: WorkflowValueRef<WorkflowBuildArtifact[], 'artifactReference'>;
    targets: WorkflowValueRef<WorkflowBuildTarget[], 'smallSerialized'>;
    diagnostics: WorkflowValueRef<WorkflowDiagnostic[], 'smallSerialized'>;
  };
  export type WorkflowTestEvidence = { id: string; name?: string; status: string; durationMs?: number; failure?: object };
  export type WorkflowTestResultRef = {
    passed: WorkflowValueRef<number, 'control'>;
    failed: WorkflowValueRef<number, 'control'>;
    skipped: WorkflowValueRef<number, 'control'>;
    durationMs: WorkflowValueRef<number, 'control'>;
    tests: WorkflowValueRef<WorkflowTestEvidence[], 'smallSerialized'>;
    diagnostics: WorkflowValueRef<WorkflowDiagnostic[], 'smallSerialized'>;
  };
  export type WorkflowAuditReference = { type: string; url: string };
  export type WorkflowAuditFinding = {
    id: string;
    aliases?: string[];
    summary: string;
    severity: 'unknown' | 'low' | 'moderate' | 'high' | 'critical';
    package: string;
    version: string;
    fixedVersions?: string[];
    references?: WorkflowAuditReference[];
    paths?: string[][];
  };
  export type WorkflowAuditCoverage = {
    source: string;
    packages: number;
    status: 'checked' | 'not-checked' | 'unsupported-ecosystem' | 'coverage-unknown';
    reason?: string;
  };
  export type WorkflowAuditReport = {
    packages: number;
    lockedPackages: number;
    coverageComplete: boolean;
    findings: WorkflowAuditFinding[];
    coverage?: WorkflowAuditCoverage[];
  };
  export type WorkflowAuditResultRef = {
    source: WorkflowValueRef<string, 'control'>;
    auditLevel: WorkflowValueRef<string, 'control'>;
    failing: WorkflowValueRef<number, 'control'>;
    report: WorkflowValueRef<WorkflowAuditReport, 'smallSerialized'>;
    diagnostics: WorkflowValueRef<WorkflowDiagnostic[], 'smallSerialized'>;
  };
  export type WorkflowBuildEffect = (Omit<WorkflowBuildStepOptions, 'targets'> & { operation: 'build' }) & WorkflowBuildResultRef;
  export type WorkflowTestEffect = (WorkflowTestStepOptions & { operation: 'test' }) & WorkflowTestResultRef;
  export type WorkflowAuditEffect = (Omit<WorkflowAuditStepOptions, 'auditLevel'> & { operation: 'audit' }) & WorkflowAuditResultRef;
  export type WorkflowTransferEffect = { operation: 'transfer' } & WorkflowBuildResultRef;
  export type WorkflowTypedEffect = WorkflowBuildEffect | WorkflowTestEffect | WorkflowAuditEffect | WorkflowTransferEffect;
  export type WorkflowIterationOutcome<T extends WorkflowTypedEffect> = T & {
    readonly __workflowIterationOutcomeType?: T;
  };
  export type WorkflowAggregate<T, Complete extends boolean = true> = WorkflowFlowNode & {
    readonly length: number;
    readonly [index: number]: T;
    readonly __workflowAggregateElementType?: T;
    readonly __workflowAggregateComplete?: Complete;
  };
  export type WorkflowProduces<T extends WorkflowTypedEffect> = {
    readonly __workflowProducedType?: T;
  };
  export type WorkflowProduced<T> =
    T extends WorkflowTypedEffect ? T :
    T extends WorkflowProduces<infer Result> ? Result :
    never;
  export type WorkflowFailureRef = {
    kind: 'failed' | 'cancelled' | 'timedOut';
    error: WorkflowValueRef<string, 'smallSerialized'>;
    diagnostics: WorkflowValueRef<WorkflowDiagnostic[], 'smallSerialized'>;
  };
  export type WorkflowNodeInput = WorkflowStep | WorkflowTypedEffect | WorkflowFlowNode;
  export type WorkflowPredicate = { readonly __workflowPredicate: true };
  export type WorkflowForEachMode = { readonly kind: 'parallel'; readonly concurrency: number };
  export type WorkflowForEachFailure = { readonly kind: 'failFast' | 'collectAll' };
  export type WorkflowForEachOptions = {
    mode?: WorkflowForEachMode;
    failure?: WorkflowForEachFailure;
  };
  export type WorkflowMatchArms<T extends WorkflowTypedEffect> = {
    succeeded: (result: T) => WorkflowNodeInput;
    failed: (failure: WorkflowFailureRef & { kind: 'failed' }) => WorkflowNodeInput;
    cancelled: (cancellation: WorkflowFailureRef & { kind: 'cancelled' }) => WorkflowNodeInput;
    timedOut: (timeout: WorkflowFailureRef & { kind: 'timedOut' }) => WorkflowNodeInput;
  };
  export type WorkflowMatrixValue = string | number | boolean | WorkflowPlatform;
  export type WorkflowJob = {
    identity: string;
    needs?: string[];
    runsOn?: WorkflowPlatform;
    matrix?: Record<string, WorkflowMatrixValue[]>;
    env?: WorkflowEnvRow[];
    steps: WorkflowStep[];
  };
  export type WorkflowFlowNode =
    | { kind: 'effect'; effect: WorkflowStep }
    | { kind: 'sequence'; children: WorkflowFlowNode[] }
    | { kind: 'parallel'; children: WorkflowBranch[] }
    | WorkflowBranch
    | { kind: 'region'; runsOn: WorkflowPlatform; env?: WorkflowEnvRow[]; children: WorkflowFlowNode[] };
  export type WorkflowBranch = {
    kind: 'branch';
    identity: string;
    children: WorkflowFlowNode[];
  };
  export type WorkflowDeclaration = {
    identity: string;
    triggers: WorkflowTrigger[];
  } & ({ flow: WorkflowFlowNode; jobs?: never } | { jobs: WorkflowJob[]; flow?: never });
  export type WorkflowOptions = {
    triggers: WorkflowTrigger[];
  } & ({ flow: WorkflowFlowNode; jobs?: never } | { jobs: WorkflowJob[]; flow?: never });
  export type WorkflowsProps = { rows: WorkflowDeclaration[] };

  export type SkyrimAssetPack = {
    name: string;
    source: string;
  };

  export type SkyrimExpectedRecord = {
    editorId: string;
    localFormId: string;
  };

  export type SkyrimCommand = {
    command: string[];
    cwd?: "workspace" | "package";
  };

  export type SkyrimTargetProps = {
    name: string;
    host: string;
    runtimeVersion: string;
    bridge: "MarionetteSSE.esp";
    nativeConfigure: SkyrimCommand;
    nativeBuild: SkyrimCommand;
    nativeTests: SkyrimCommand;
    nativeDll: string;
    assetCompilerProject: string;
    assetTestsProject: string;
    assetPacks: SkyrimAssetPack[];
    assetOutput: "build/assets/MarionetteSSE.esp";
    runtimeConfig: string;
    runtimeOverrideTarget?: string;
    runtimeOverrideFields?: SkyrimRuntimeOverride[];
    iniOverrideFields?: SkyrimINIOverride[];
    dllDestination: "SKSE/Plugins/MarionetteSSE.dll";
    configDestination: "SKSE/Plugins/MarionetteSSE.toml";
    expectedRecords: SkyrimExpectedRecord[];
    stalePlugins?: string[];
    runtimeEvidencePattern: string;
    readyMarker: string;
  };

  export type SkyrimRuntimeOverride = {
    path: string;
    type: "boolean" | "string" | "integer";
    secret?: boolean;
  };

  export type SkyrimINIOverride = {
    section: "General";
    key: "bAlwaysActive";
    type: "boolean";
  };

  export type SecurityProps = {
    acknowledgedCapabilities?: AcknowledgedCapability[];
    acknowledgedLifecycleCategories?: AcknowledgedLifecycleCategory[];
  };

  export type UpdatePolicyProps = {
    rows: UpdatePolicyRow[];
  };

  export type RegistryEndpoint = {
    url: string;
    tokenEnv?: string;
    fallbackOnNotFound?: boolean;
    allowedArtifactHosts?: string[];
  };

  export type RegistryPolicyProps = {
    allowedSources?: Array<'npm' | 'jsr'>;
    offline?: boolean;
    requireIntegrity?: boolean;
    requireAuditCoverage?: boolean;
    children?: ManifestNode;
  };

  export type RegistrySourceProps = {
    kind: 'npm' | 'jsr';
    endpoints: RegistryEndpoint[];
  };

  export type CompatFilesProps = {
    children?: ManifestNode;
  };

  export type JsonFileProps = {
    path: string;
    value: JSONValue;
  };

  export type ToolsProps = {
    values: DependencyRefLike[];
  };

  export type BoundariesProps = {
    rows: BoundaryRow[];
  };

  export type PublishProps = {
    include: string[];
    exclude?: string[];
  };

  export type ManifestNode =
    | JSX.Element
    | ManifestNode[]
    | string
    | number
    | boolean
    | null
    | undefined;

  export type ManifestDocument = {
    readonly __tspackManifest: true;
  };

  export type ManifestElement<P> = JSX.Element & {
    readonly __props?: P;
  };

  export type ManifestComponent<P> = (props: P) => JSX.Element;

  export function define(input: ManifestNode): ManifestDocument;
  export function defineWorkspace(input: ManifestNode): ManifestDocument;
  export function definePackage(input: ManifestNode): ManifestDocument;
  export function annotatePackage(input: ManifestNode): ManifestDocument;

  export function defineDeps<T extends Record<string, DependencyIntent>>(deps: T): T;

  export function npm(name: string, range: string): NpmSource;
  export function jsr(name: string, range: string): JsrSource;
  export function git(ref: string, options?: Omit<GitSource, 'kind' | 'ref'>): GitSource;
  export function path(pathValue: string): PathSource;
  export function workspace(name: string, options?: Omit<WorkspaceSource, 'kind' | 'name'>): WorkspaceSource;

  export function dep(source: DependencySource, options?: DependencyOptions): DepIntent;
  export function peer(source: DependencySource, options?: PeerOptions): PeerIntent;
  export function tool(source: DependencySource, options?: DependencyOptions): ToolIntent;
  export function localFixture(dependency: TargetDependencyRefLike, options: LocalFixtureOptions): LocalFixtureIntent;
  export function builtArtifactFixture(target: string, options: BuiltArtifactFixtureOptions): BuiltArtifactFixtureIntent;
  export function Env(name: string, options?: Omit<RunTargetEnvRow, 'name'>): RunTargetEnvRow;
  export function Service(name: string, options?: Omit<RunTargetServiceRequirementRow, 'name' | 'kind'>): RunTargetServiceRequirementRow;
  export function Workflow(identity: string, options: WorkflowOptions): WorkflowDeclaration;
  export function Job(identity: string, options: Omit<WorkflowJob, 'identity'>): WorkflowJob;
  export function Sequence(...nodes: WorkflowNodeInput[]): WorkflowFlowNode;
  export function Parallel(...branches: WorkflowBranch[]): WorkflowFlowNode;
  export function Branch(identity: string, ...nodes: WorkflowNodeInput[]): WorkflowBranch;
  export function On<T extends WorkflowTypedEffect>(platform: WorkflowPlatform, node: T): WorkflowFlowNode & WorkflowProduces<T>;
  export function On<T extends WorkflowTypedEffect>(platform: WorkflowPlatform, options: { env?: WorkflowEnvRow[] }, node: T): WorkflowFlowNode & WorkflowProduces<T>;
  export function On(platform: WorkflowPlatform, ...nodes: WorkflowNodeInput[]): WorkflowFlowNode;
  export function On(platform: WorkflowPlatform, options: { env?: WorkflowEnvRow[] }, ...nodes: WorkflowNodeInput[]): WorkflowFlowNode;
  export function MatchResult<T extends WorkflowTypedEffect>(source: T, arms: WorkflowMatchArms<T>): WorkflowFlowNode;
  export function Finally(body: WorkflowNodeInput, cleanup: WorkflowNodeInput): WorkflowFlowNode;
  export function ForEach<T extends string | number | boolean, R extends WorkflowNodeInput, O extends WorkflowForEachOptions | undefined = undefined>(identity: string, source: readonly T[], body: (value: T) => R, options?: O): WorkflowFlowNode & (O extends { failure: { kind: 'collectAll' } } ? WorkflowAggregate<WorkflowIterationOutcome<WorkflowProduced<R>>, true> : WorkflowAggregate<WorkflowProduced<R>, false>);
  export function ForEach<T extends WorkflowTypedEffect, R extends WorkflowNodeInput, O extends WorkflowForEachOptions | undefined = undefined>(identity: string, source: WorkflowAggregate<T, true>, body: (value: T) => R, options?: O): WorkflowFlowNode & (O extends { failure: { kind: 'collectAll' } } ? WorkflowAggregate<WorkflowIterationOutcome<WorkflowProduced<R>>, true> : WorkflowAggregate<WorkflowProduced<R>, false>);
  export function ParallelForEach(options: { concurrency: number }): WorkflowForEachMode;
  export function CollectAll(): WorkflowForEachFailure & { readonly kind: 'collectAll' };
  export function FailFast(): WorkflowForEachFailure & { readonly kind: 'failFast' };
  export function GreaterThan(value: WorkflowValueRef<number, 'control'>, threshold: number): WorkflowPredicate;
  export function LessThan(value: WorkflowValueRef<number, 'control'>, threshold: number): WorkflowPredicate;
  export function NotEmpty<T>(value: WorkflowValueRef<readonly T[], 'artifactReference' | 'smallSerialized'>): WorkflowPredicate;
  export function IsEmpty<T>(value: WorkflowValueRef<readonly T[], 'artifactReference' | 'smallSerialized'>): WorkflowPredicate;
  export function And(first: WorkflowPredicate, second: WorkflowPredicate, ...rest: WorkflowPredicate[]): WorkflowPredicate;
  export function Or(first: WorkflowPredicate, second: WorkflowPredicate, ...rest: WorkflowPredicate[]): WorkflowPredicate;
  export function Not(predicate: WorkflowPredicate): WorkflowPredicate;
  export function When(predicate: WorkflowPredicate, whenTrue: WorkflowNodeInput, whenFalse?: WorkflowNodeInput): WorkflowFlowNode;
  export function Manual(options?: WorkflowTriggerFilter): WorkflowTrigger;
  export function Push(options?: WorkflowTriggerFilter): WorkflowTrigger;
  export function PullRequest(options?: WorkflowTriggerFilter): WorkflowTrigger;
  export function Linux(): WorkflowPlatform;
  export function Windows(): WorkflowPlatform;
  export function MacOS(): WorkflowPlatform;
  export function CurrentHost(): WorkflowPlatform;
  export function Sync(options?: WorkflowStepOptions): WorkflowStep;
  export function Check(options?: WorkflowStepOptions): WorkflowStep;
  export function Build(options?: WorkflowBuildStepOptions): WorkflowBuildEffect;
  export function Test(options?: WorkflowTestStepOptions): WorkflowTestEffect;
  export function Pack(options?: WorkflowStepOptions): WorkflowStep;
  export function Pack(artifacts: WorkflowValueRef<WorkflowBuildArtifact[], 'artifactReference'>): WorkflowStep;
  export function Transfer(artifacts: WorkflowValueRef<WorkflowBuildArtifact[], 'artifactReference'>, target: WorkflowPlatform): WorkflowTransferEffect;
  export function Audit(options?: WorkflowAuditStepOptions): WorkflowAuditEffect;
  export function Process(name: string, options: WorkflowProcessStepOptions): WorkflowStep;
  export function ShellScript(name: string, options: WorkflowShellStepOptions): WorkflowStep;
  export function Plain(value: string): WorkflowValue;
  export function Secret(name: string): WorkflowValue;
  export function WorkflowEnv(name: string, value: WorkflowValue): WorkflowEnvRow;
  export function json<T extends JSONValue>(value: T): T;

  export const TsConfig: {
    manifestEditor(options?: ManifestEditorTsConfigOptions): TsConfigManifestEditor;
  };

  export const VSCode: {
    settings<T extends VSCodeSettingsInput = VSCodeSettingsInput>(value?: T): VSCodeSettings;
    extensions<T extends VSCodeExtensions = VSCodeExtensions>(value?: T): T;
  };

  export const Workspace: ManifestComponent<WorkspaceProps>;
  export const Packages: ManifestComponent<PackagesProps>;
  export const Package: ManifestComponent<PackageProps>;
  export const PackageAnnotations: ManifestComponent<PackageAnnotationProps>;
  export const Policies: ManifestComponent<PoliciesProps>;
  export const Targets: ManifestComponent<TargetsProps>;
  export const RunTargets: ManifestComponent<RunTargetsProps>;
  export const TestTargets: ManifestComponent<TestTargetsProps>;
  export const Workflows: ManifestComponent<WorkflowsProps>;
  export const SkyrimTarget: ManifestComponent<SkyrimTargetProps>;
  export const Tools: ManifestComponent<ToolsProps>;
  export const Boundaries: ManifestComponent<BoundariesProps>;
  export const Publish: ManifestComponent<PublishProps>;
  export const Security: ManifestComponent<SecurityProps>;
  export const UpdatePolicy: ManifestComponent<UpdatePolicyProps>;
  export const RegistryPolicy: ManifestComponent<RegistryPolicyProps>;
  export const RegistrySource: ManifestComponent<RegistrySourceProps>;
  export const CompatFiles: ManifestComponent<CompatFilesProps>;
  export const JsonFile: ManifestComponent<JsonFileProps>;

  export {};
}

declare namespace JSX {
  interface Element {
    readonly __jsxElement?: true;
  }

  interface ElementChildrenAttribute {
    children: {};
  }

  interface IntrinsicElements {
    [elemName: string]: never;
  }
}
