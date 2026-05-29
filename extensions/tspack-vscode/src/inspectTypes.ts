export type InspectBounds = {
  x: number;
  y: number;
  width: number;
  height: number;
};

export type InspectSourceHint = {
  raw?: string;
  file?: string;
  line?: number;
  column?: number;
  component?: string;
  symbol?: string;
  parseError?: string;
};

export type InspectNode = {
  id?: string;
  tag?: string;
  role?: string;
  name?: string;
  text?: string;
  bounds?: InspectBounds;
  visible?: boolean;
  focusable?: boolean;
  source?: InspectSourceHint;
  style?: {
    display?: string;
    position?: string;
    zIndex?: string;
    pointerEvents?: string;
    opacity?: string;
    overflow?: string;
    fontSize?: string;
    fontWeight?: string;
  };
  children?: InspectNode[];
};

export type InspectDiagnostic = {
  code: string;
  message: string;
};

export type InspectResult = {
  target?: {
    url?: string;
  };
  browser?: {
    name?: string;
    backend?: string;
  };
  viewport?: {
    width?: number;
    height?: number;
    deviceScaleFactor?: number;
  };
  root?: InspectNode | null;
  diagnostics?: InspectDiagnostic[];
  hitTests?: unknown[];
};

export type CdpTargetSummary = {
  index: number;
  id: string;
  type: string;
  title: string;
  url: string;
  webSocketDebuggerUrl?: string;
};

export type CdpTargetListResult = {
  command?: string;
  mode?: string;
  cdp?: string;
  endpoint?: string;
  targets: CdpTargetSummary[];
  diagnostics?: InspectDiagnostic[];
};
