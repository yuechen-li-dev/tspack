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

  export type VSCodeExtensions = {
    recommendations?: string[];
    unwantedRecommendations?: string[];
  };

  export type RuntimeProfile = 'nodejs' | 'bun' | 'deno';

  export type NpmSource = {
    kind: 'npm';
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

  export type DependencySource = NpmSource | GitSource | PathSource | WorkspaceSource;

  export type DependencyOptions = {
    key?: string;
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

  export type TargetRow = {
    name: string;
    export?: string;
    entry: string;
    runtime: string;
    types?: string;
    deps?: TargetDependencyRefLike[];
    peers?: TargetDependencyRefLike[];
    optional?: boolean;
    [key: string]: Primitive | Primitive[] | TargetDependencyRefLike[] | undefined;
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

  export type PackageRow = {
    name: string;
    root: string;
    manifest: string;
  };

  export type PackageAnnotationProps = {
    name?: string;
    dependencies?: {
      values: DependencyIntent[];
    };
    children?: ManifestNode;
  };

  export type PackageProps = {
    name: string;
    version: string;
    kind: 'library' | 'app' | 'service';
    license?: string;
    dependencies?: {
      values: DependencyIntent[];
    };
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

  export type SecurityProps = {
    acknowledgedCapabilities?: AcknowledgedCapability[];
    acknowledgedLifecycleCategories?: AcknowledgedLifecycleCategory[];
  };

  export type UpdatePolicyProps = {
    rows: UpdatePolicyRow[];
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
  export function git(ref: string, options?: Omit<GitSource, 'kind' | 'ref'>): GitSource;
  export function path(pathValue: string): PathSource;
  export function workspace(name: string, options?: Omit<WorkspaceSource, 'kind' | 'name'>): WorkspaceSource;

  export function dep(source: DependencySource, options?: DependencyOptions): DepIntent;
  export function peer(source: DependencySource, options?: PeerOptions): PeerIntent;
  export function tool(source: DependencySource, options?: DependencyOptions): ToolIntent;
  export function Env(name: string, options?: Omit<RunTargetEnvRow, 'name'>): RunTargetEnvRow;
  export function Service(name: string, options?: Omit<RunTargetServiceRequirementRow, 'name' | 'kind'>): RunTargetServiceRequirementRow;
  export function json<T extends JSONValue>(value: T): T;

  export const TsConfig: {
    manifestEditor(options?: ManifestEditorTsConfigOptions): TsConfigManifestEditor;
  };

  export const VSCode: {
    settings<T extends VSCodeSettings = VSCodeSettings>(value?: T): T;
    extensions<T extends VSCodeExtensions = VSCodeExtensions>(value?: T): T;
  };

  export const Workspace: ManifestComponent<WorkspaceProps>;
  export const Packages: ManifestComponent<PackagesProps>;
  export const Package: ManifestComponent<PackageProps>;
  export const PackageAnnotations: ManifestComponent<PackageAnnotationProps>;
  export const Policies: ManifestComponent<PoliciesProps>;
  export const Targets: ManifestComponent<TargetsProps>;
  export const RunTargets: ManifestComponent<RunTargetsProps>;
  export const Tools: ManifestComponent<ToolsProps>;
  export const Boundaries: ManifestComponent<BoundariesProps>;
  export const Publish: ManifestComponent<PublishProps>;
  export const Security: ManifestComponent<SecurityProps>;
  export const UpdatePolicy: ManifestComponent<UpdatePolicyProps>;
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
