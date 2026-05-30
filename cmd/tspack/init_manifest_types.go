package main

const initManifestTypesDTS = "" +
	"/**\n" +
	" * Authoring-time types for `tspack/manifest`.\n" +
	" * Keep this aligned with the manifest frontend parser and Go IR validator.\n" +
	" * This file is type-surface support only; it is not runtime implementation.\n" +
	" */\n" +
	"\n" +
	"declare module 'tspack/manifest' {\n" +
	"  export type Primitive = string | number | boolean | null;\n" +
	"\n" +
	"  export type RuntimeProfile = 'nodejs' | 'bun' | 'deno';\n" +
	"\n" +
	"  export type NpmSource = {\n" +
	"    kind: 'npm';\n" +
	"    package: string;\n" +
	"    range: string;\n" +
	"  };\n" +
	"\n" +
	"  export type GitSource = {\n" +
	"    kind: 'git';\n" +
	"    ref: string;\n" +
	"    commit?: string;\n" +
	"    tag?: string;\n" +
	"    branch?: string;\n" +
	"  };\n" +
	"\n" +
	"  export type PathSource = {\n" +
	"    kind: 'path';\n" +
	"    path: string;\n" +
	"  };\n" +
	"\n" +
	"  export type WorkspaceSource = {\n" +
	"    kind: 'workspace';\n" +
	"    name: string;\n" +
	"    target?: string;\n" +
	"  };\n" +
	"\n" +
	"  export type DependencySource = NpmSource | GitSource | PathSource | WorkspaceSource;\n" +
	"\n" +
	"  export type DepIntent = {\n" +
	"    kind: 'dep';\n" +
	"    source: DependencySource;\n" +
	"  };\n" +
	"\n" +
	"  export type PeerIntent = {\n" +
	"    kind: 'peer';\n" +
	"    source: DependencySource;\n" +
	"    optional?: boolean;\n" +
	"  };\n" +
	"\n" +
	"  export type ToolIntent = {\n" +
	"    kind: 'tool';\n" +
	"    source: DependencySource;\n" +
	"  };\n" +
	"\n" +
	"  export type DependencyIntent = DepIntent | PeerIntent | ToolIntent;\n" +
	"\n" +
	"  export type TypePolicy = {\n" +
	"    mode?: string;\n" +
	"    strict?: boolean;\n" +
	"    [key: string]: Primitive | Primitive[] | Record<string, Primitive> | undefined;\n" +
	"  };\n" +
	"\n" +
	"  export type BoundaryRow = {\n" +
	"    from?: string;\n" +
	"    transitiveFrom?: string;\n" +
	"    to?: string;\n" +
	"    allow?: string[];\n" +
	"    deny?: string[];\n" +
	"    allowDeps?: string[];\n" +
	"    denyDeps?: string[];\n" +
	"    denyTypeDeps?: string[];\n" +
	"    allowOnly?: string[];\n" +
	"    [key: string]: Primitive | Primitive[] | undefined;\n" +
	"  };\n" +
	"\n" +
	"\n" +
	"  export type LifecycleScriptName =\n" +
	"    | 'preinstall'\n" +
	"    | 'install'\n" +
	"    | 'postinstall'\n" +
	"    | 'prepack'\n" +
	"    | 'prepare'\n" +
	"    | 'postpack'\n" +
	"    | 'prepublish'\n" +
	"    | 'prepublishOnly'\n" +
	"    | 'postpublish';\n" +
	"\n" +
	"  export type AcknowledgedCapability = {\n" +
	"    package: string;\n" +
	"    kind: 'lifecycleScript';\n" +
	"    script: LifecycleScriptName;\n" +
	"    command: string;\n" +
	"    reason: string;\n" +
	"    behaviorFixture?: string;\n" +
	"    behaviorReport?: string;\n" +
	"  };\n" +
	"\n" +
	"  export type BoundaryPolicy = {\n" +
	"    mode?: string;\n" +
	"    rows?: BoundaryRow[];\n" +
	"    [key: string]: Primitive | Primitive[] | BoundaryRow[] | undefined;\n" +
	"  };\n" +
	"\n" +
	"  export type DependencyRefLike = DependencyIntent;\n" +
	"\n" +
	"  export type TargetRow = {\n" +
	"    name: string;\n" +
	"    export?: string;\n" +
	"    entry: string;\n" +
	"    runtime: string;\n" +
	"    types?: string;\n" +
	"    deps?: DependencyRefLike[];\n" +
	"    peers?: DependencyRefLike[];\n" +
	"    optional?: boolean;\n" +
	"    [key: string]: Primitive | Primitive[] | DependencyRefLike[] | undefined;\n" +
	"  };\n" +
	"\n" +
	"  export type RunTargetReady =\n" +
	"    | {\n" +
	"        kind: 'http';\n" +
	"        path: string;\n" +
	"      }\n" +
	"    | {\n" +
	"        kind: 'tcp';\n" +
	"        host?: string;\n" +
	"        port: number;\n" +
	"      }\n" +
	"    | {\n" +
	"        kind: 'stdout-match';\n" +
	"        pattern: string;\n" +
	"        stream?: 'stdout' | 'stderr' | 'both';\n" +
	"      };\n" +
	"\n" +
	"  export type RunTargetRow = {\n" +
	"    name: string;\n" +
	"    runtime: 'system' | 'node' | 'bun' | 'deno';\n" +
	"    command: string[];\n" +
	"    url: string;\n" +
	"    cwd?: 'workspace' | 'package';\n" +
	"    ready?: RunTargetReady;\n" +
	"  };\n" +
	"\n" +
	"  export type PackageRow = {\n" +
	"    name: string;\n" +
	"    root: string;\n" +
	"    manifest: string;\n" +
	"  };\n" +
	"\n" +
	"  export type PackageProps = {\n" +
	"    name: string;\n" +
	"    version: string;\n" +
	"    kind: 'library' | 'app';\n" +
	"    license?: string;\n" +
	"    dependencies?: {\n" +
	"      values: DependencyIntent[];\n" +
	"    };\n" +
	"    children?: ManifestNode;\n" +
	"  };\n" +
	"\n" +
	"  export type WorkspaceProps = {\n" +
	"    name: string;\n" +
	"    runtime?: RuntimeProfile;\n" +
	"    children?: ManifestNode;\n" +
	"  };\n" +
	"\n" +
	"  export type PackagesProps = {\n" +
	"    rows: PackageRow[];\n" +
	"  };\n" +
	"\n" +
	"  export type PoliciesProps = {\n" +
	"    types?: TypePolicy;\n" +
	"    boundaries?: BoundaryPolicy;\n" +
	"  };\n" +
	"\n" +
	"  export type TargetsProps = {\n" +
	"    rows: TargetRow[];\n" +
	"  };\n" +
	"\n" +
	"  export type RunTargetsProps = {\n" +
	"    rows: RunTargetRow[];\n" +
	"  };\n" +
	"\n" +
	"  export type SecurityProps = {\n" +
	"    acknowledgedCapabilities: AcknowledgedCapability[];\n" +
	"  };\n" +
	"\n" +
	"  export type ToolsProps = {\n" +
	"    values: DependencyRefLike[];\n" +
	"  };\n" +
	"\n" +
	"  export type BoundariesProps = {\n" +
	"    rows: BoundaryRow[];\n" +
	"  };\n" +
	"\n" +
	"  export type PublishProps = {\n" +
	"    include: string[];\n" +
	"    exclude?: string[];\n" +
	"  };\n" +
	"\n" +
	"  export type ManifestNode =\n" +
	"    | JSX.Element\n" +
	"    | ManifestNode[]\n" +
	"    | string\n" +
	"    | number\n" +
	"    | boolean\n" +
	"    | null\n" +
	"    | undefined;\n" +
	"\n" +
	"  export type ManifestDocument = {\n" +
	"    readonly __tspackManifest: true;\n" +
	"  };\n" +
	"\n" +
	"  export type ManifestElement<P> = JSX.Element & {\n" +
	"    readonly __props?: P;\n" +
	"  };\n" +
	"\n" +
	"  export type ManifestComponent<P> = (props: P) => JSX.Element;\n" +
	"\n" +
	"  export function define(input: ManifestNode): ManifestDocument;\n" +
	"  export function defineWorkspace(input: ManifestNode): ManifestDocument;\n" +
	"  export function definePackage(input: ManifestNode): ManifestDocument;\n" +
	"\n" +
	"  export function defineDeps<T extends Record<string, DependencyIntent>>(deps: T): T;\n" +
	"\n" +
	"  export function npm(name: string, range: string): NpmSource;\n" +
	"  export function git(ref: string, options?: Omit<GitSource, 'kind' | 'ref'>): GitSource;\n" +
	"  export function path(pathValue: string): PathSource;\n" +
	"  export function workspace(name: string, options?: Omit<WorkspaceSource, 'kind' | 'name'>): WorkspaceSource;\n" +
	"\n" +
	"  export function dep(source: DependencySource): DepIntent;\n" +
	"  export function peer(source: DependencySource, options?: Omit<PeerIntent, 'kind' | 'source'>): PeerIntent;\n" +
	"  export function tool(source: DependencySource): ToolIntent;\n" +
	"\n" +
	"  export const Workspace: ManifestComponent<WorkspaceProps>;\n" +
	"  export const Packages: ManifestComponent<PackagesProps>;\n" +
	"  export const Package: ManifestComponent<PackageProps>;\n" +
	"  export const Policies: ManifestComponent<PoliciesProps>;\n" +
	"  export const Targets: ManifestComponent<TargetsProps>;\n" +
	"  export const RunTargets: ManifestComponent<RunTargetsProps>;\n" +
	"  export const Tools: ManifestComponent<ToolsProps>;\n" +
	"  export const Boundaries: ManifestComponent<BoundariesProps>;\n" +
	"  export const Publish: ManifestComponent<PublishProps>;\n" +
	"  export const Security: ManifestComponent<SecurityProps>;\n" +
	"\n" +
	"  export {};\n" +
	"}\n" +
	"\n" +
	"declare namespace JSX {\n" +
	"  interface Element {\n" +
	"    readonly __jsxElement?: true;\n" +
	"  }\n" +
	"\n" +
	"  interface ElementChildrenAttribute {\n" +
	"    children: {};\n" +
	"  }\n" +
	"\n" +
	"  interface IntrinsicElements {\n" +
	"    [elemName: string]: never;\n" +
	"  }\n" +
	"}\n" +
	""
