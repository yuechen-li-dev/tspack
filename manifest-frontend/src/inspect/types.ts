export type InspectBrowserName = 'chromium' | 'firefox' | 'webkit' | 'cdp' | 'vscode';

export type Bounds = {
  x: number;
  y: number;
  width: number;
  height: number;
};

export type UIInspectNode = {
  id: string;
  tag: string;
  role?: string;
  name?: string;
  text?: string;
  bounds: Bounds;
  visible: boolean;
  focusable?: boolean;
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
  children: UIInspectNode[];
};

export type UIHitTest = {
  point: {
    x: number;
    y: number;
  };
  elements: UIInspectNode[];
};

export type UIInspectResult = {
  target: {
    url: string;
  };
  browser: {
    name: InspectBrowserName;
    backend?: 'playwright' | 'cdp' | 'vscode' | 'browser-path' | 'platform-webview';
  };
  viewport: {
    width: number;
    height: number;
    deviceScaleFactor?: number;
  };
  root: UIInspectNode | null;
  hitTests: UIHitTest[];
  diagnostics: {
    code: string;
    message: string;
  }[];
};

export type CDPTargetSummary = {
  index: number;
  id: string;
  type: string;
  title: string;
  url: string;
  webSocketDebuggerUrl?: string;
};

export type CDPTargetListResult = {
  command: 'inspect';
  mode: 'list-targets';
  cdp: string;
  endpoint: string;
  targets: CDPTargetSummary[];
  diagnostics: Array<{ code: string; message: string }>;
};
