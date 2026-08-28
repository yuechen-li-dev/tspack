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
  export type Compiler = 'tsc' | 'tscl' | 'scriptc' | 'perry';

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
    [key: string]: Primitive | Primitive[] | TargetDependencyRefLike[] | CopelandNpmContract[] | undefined;
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
  export type WorkflowTestStepOptions = WorkflowStepOptions & { filter?: string };
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
  export type WorkflowMatrixValue = string | number | boolean | WorkflowPlatform;
  export type WorkflowJob = {
    identity: string;
    needs?: string[];
    runsOn?: WorkflowPlatform;
    matrix?: Record<string, WorkflowMatrixValue[]>;
    env?: WorkflowEnvRow[];
    steps: WorkflowStep[];
  };
  export type WorkflowDeclaration = {
    identity: string;
    triggers: WorkflowTrigger[];
    jobs: WorkflowJob[];
  };
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
  export function Env(name: string, options?: Omit<RunTargetEnvRow, 'name'>): RunTargetEnvRow;
  export function Service(name: string, options?: Omit<RunTargetServiceRequirementRow, 'name' | 'kind'>): RunTargetServiceRequirementRow;
  export function Workflow(identity: string, options: Omit<WorkflowDeclaration, 'identity'>): WorkflowDeclaration;
  export function Job(identity: string, options: Omit<WorkflowJob, 'identity'>): WorkflowJob;
  export function Manual(options?: WorkflowTriggerFilter): WorkflowTrigger;
  export function Push(options?: WorkflowTriggerFilter): WorkflowTrigger;
  export function PullRequest(options?: WorkflowTriggerFilter): WorkflowTrigger;
  export function Linux(): WorkflowPlatform;
  export function Windows(): WorkflowPlatform;
  export function MacOS(): WorkflowPlatform;
  export function CurrentHost(): WorkflowPlatform;
  export function Sync(options?: WorkflowStepOptions): WorkflowStep;
  export function Check(options?: WorkflowStepOptions): WorkflowStep;
  export function Build(options?: WorkflowBuildStepOptions): WorkflowStep;
  export function Test(options?: WorkflowTestStepOptions): WorkflowStep;
  export function Pack(options?: WorkflowStepOptions): WorkflowStep;
  export function Audit(options?: WorkflowAuditStepOptions): WorkflowStep;
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
