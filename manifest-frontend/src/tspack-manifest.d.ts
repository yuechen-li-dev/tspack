/**
 * Authoring-time types for `tspack/manifest`.
 * Keep this aligned with the manifest frontend parser and Go IR validator.
 * This file is type-surface support only; it is not runtime implementation.
 */

declare module 'tspack/manifest' {
  export type Primitive = string | number | boolean | null;

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

  export type DepIntent = {
    kind: 'dep';
    source: DependencySource;
  };

  export type PeerIntent = {
    kind: 'peer';
    source: DependencySource;
    optional?: boolean;
  };

  export type ToolIntent = {
    kind: 'tool';
    source: DependencySource;
  };

  export type DependencyIntent = DepIntent | PeerIntent | ToolIntent;

  export type TypePolicy = {
    mode?: string;
    strict?: boolean;
    [key: string]: Primitive | Primitive[] | Record<string, Primitive> | undefined;
  };

  export type BoundaryRow = {
    from?: string;
    to?: string;
    allow?: string[];
    deny?: string[];
    [key: string]: Primitive | Primitive[] | undefined;
  };

  export type BoundaryPolicy = {
    mode?: string;
    rows?: BoundaryRow[];
    [key: string]: Primitive | Primitive[] | BoundaryRow[] | undefined;
  };

  export type DependencyRefLike = DependencyIntent;

  export type TargetRow = {
    name: string;
    export?: string;
    entry: string;
    runtime: string;
    types?: string;
    deps?: DependencyRefLike[];
    peers?: DependencyRefLike[];
    optional?: boolean;
    [key: string]: Primitive | Primitive[] | DependencyRefLike[] | undefined;
  };

  export type RunTargetReady = {
    kind: 'http';
    path: string;
  };

  export type RunTargetRow = {
    name: string;
    runtime: 'system' | 'node';
    command: string[];
    url: string;
    ready?: RunTargetReady;
  };

  export type PackageRow = {
    name: string;
    root: string;
    manifest: string;
  };

  export type PackageProps = {
    name: string;
    version: string;
    kind: 'library' | 'app';
    license?: string;
    dependencies?: {
      values: DependencyIntent[];
    };
    children?: ManifestNode;
  };

  export type WorkspaceProps = {
    name: string;
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

  export function defineDeps<T extends Record<string, DependencyIntent>>(deps: T): T;

  export function npm(name: string, range: string): NpmSource;
  export function git(ref: string, options?: Omit<GitSource, 'kind' | 'ref'>): GitSource;
  export function path(pathValue: string): PathSource;
  export function workspace(name: string, options?: Omit<WorkspaceSource, 'kind' | 'name'>): WorkspaceSource;

  export function dep(source: DependencySource): DepIntent;
  export function peer(source: DependencySource, options?: Omit<PeerIntent, 'kind' | 'source'>): PeerIntent;
  export function tool(source: DependencySource): ToolIntent;

  export const Workspace: ManifestComponent<WorkspaceProps>;
  export const Packages: ManifestComponent<PackagesProps>;
  export const Package: ManifestComponent<PackageProps>;
  export const Policies: ManifestComponent<PoliciesProps>;
  export const Targets: ManifestComponent<TargetsProps>;
  export const RunTargets: ManifestComponent<RunTargetsProps>;
  export const Tools: ManifestComponent<ToolsProps>;
  export const Boundaries: ManifestComponent<BoundariesProps>;
  export const Publish: ManifestComponent<PublishProps>;

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
