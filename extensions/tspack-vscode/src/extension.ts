import * as vscode from 'vscode';
import {
  buildInspectTree,
  getInspectNodeContextValue,
  serializeInspectNode,
  type InspectTreeNode,
} from './inspectTree';
import { inspectCdpTarget, listCdpTargets, TspackCliError } from './tspackCli';
import type {
  CdpTargetSummary,
  InspectDiagnostic,
  InspectNode,
  InspectResult,
} from './inspectTypes';
import {
  buildRevealTarget,
  isSourceHintMalformed,
  resolveSourceHintPath,
} from './revealSource';

class InspectTreeItem extends vscode.TreeItem {
  readonly inspectNode: InspectNode;

  constructor(model: InspectTreeNode) {
    const collapsibleState = model.children.length > 0
      ? vscode.TreeItemCollapsibleState.Collapsed
      : vscode.TreeItemCollapsibleState.None;
    super(model.label, collapsibleState);
    this.id = model.id;
    this.description = model.description;
    this.tooltip = model.tooltip;
    this.contextValue = getInspectNodeContextValue(model.node);
    this.inspectNode = model.node;
  }
}

class InspectTreeProvider implements vscode.TreeDataProvider<InspectTreeNode> {
  private readonly changed = new vscode.EventEmitter<
    InspectTreeNode | undefined | null | void
  >();
  private roots: InspectTreeNode[] = [];

  readonly onDidChangeTreeData = this.changed.event;

  setResult(result: InspectResult): void {
    this.roots = buildInspectTree(result);
    this.changed.fire();
  }

  clear(): void {
    this.roots = [];
    this.changed.fire();
  }

  getTreeItem(element: InspectTreeNode): vscode.TreeItem {
    return new InspectTreeItem(element);
  }

  getChildren(element?: InspectTreeNode): Thenable<InspectTreeNode[]> {
    if (element) {
      return Promise.resolve(element.children);
    }
    return Promise.resolve(this.roots);
  }
}

function getConfig(): {
  endpoint: string;
  tspackPath: string;
  targetIndex: number;
} {
  const config = vscode.workspace.getConfiguration('tspack.inspect');
  return {
    endpoint: config.get<string>('cdpEndpoint', 'http://127.0.0.1:9229'),
    tspackPath: config.get<string>('tspackPath', 'tspack'),
    targetIndex: config.get<number>('targetIndex', 0),
  };
}

function writeDiagnostics(
  channel: vscode.OutputChannel,
  diagnostics: InspectDiagnostic[] | undefined,
): void {
  if (!diagnostics || diagnostics.length === 0) {
    return;
  }
  channel.appendLine('Diagnostics:');
  for (const diagnostic of diagnostics) {
    channel.appendLine(`- ${diagnostic.code}: ${diagnostic.message}`);
  }
  channel.appendLine('');
}

function writeRawFailure(
  channel: vscode.OutputChannel,
  error: TspackCliError,
): void {
  channel.appendLine(`Error: ${error.code}`);
  channel.appendLine(error.message);
  if (error.stderr.trim()) {
    channel.appendLine('stderr:');
    channel.appendLine(error.stderr);
  }
  if (error.stdout.trim()) {
    channel.appendLine('stdout:');
    channel.appendLine(error.stdout);
  }
  channel.show(true);
}

function userFacingErrorMessage(error: unknown, endpoint: string): string {
  if (error instanceof TspackCliError) {
    if (error.code === 'TSPACK_CLI_NOT_FOUND') {
      return 'TSPack CLI not found. Set tspack.inspect.tspackPath.';
    }
    if (error.message.includes('TSPACK_INSPECT_CDP_CONNECT_FAILED')) {
      return [
        `Unable to reach CDP endpoint ${endpoint}.`,
        'Start VS Code/Chromium with --remote-debugging-port=9229.',
      ].join(' ');
    }
    return error.message;
  }
  if (error instanceof Error) {
    return error.message;
  }
  return 'TSPack inspect failed.';
}

function targetQuickPickLabel(target: CdpTargetSummary): string {
  const title = target.title || '(untitled)';
  return `[${target.index}] ${target.type}: ${title}`;
}

async function inspectTarget(
  provider: InspectTreeProvider,
  channel: vscode.OutputChannel,
  targetIndex: number,
): Promise<void> {
  const config = getConfig();
  channel.appendLine(
    `Running: ${config.tspackPath} inspect --cdp ${config.endpoint} --target ${targetIndex} --json`,
  );
  try {
    const inspected = await inspectCdpTarget(
      config.tspackPath,
      config.endpoint,
      targetIndex,
    );
    channel.appendLine(`Inspected CDP target ${targetIndex}.`);
    writeDiagnostics(channel, inspected.result.diagnostics);
    provider.setResult(inspected.result);
    if (!inspected.result.root) {
      vscode.window.showInformationMessage(
        'TSPack inspect returned no root UI node. See the TSPack Inspect output channel for diagnostics.',
      );
    }
  } catch (error) {
    provider.clear();
    if (error instanceof TspackCliError) {
      writeRawFailure(channel, error);
    }
    vscode.window.showErrorMessage(
      userFacingErrorMessage(error, config.endpoint),
    );
  }
}

async function inspectTargetsCommand(
  provider: InspectTreeProvider,
  channel: vscode.OutputChannel,
): Promise<void> {
  const config = getConfig();
  channel.appendLine(
    `Running: ${config.tspackPath} inspect --cdp ${config.endpoint} --list-targets --json`,
  );
  try {
    const listed = await listCdpTargets(config.tspackPath, config.endpoint);
    writeDiagnostics(channel, listed.result.diagnostics);
    if (!listed.result.targets || listed.result.targets.length === 0) {
      vscode.window.showInformationMessage('No inspectable CDP targets found.');
      return;
    }

    const picked = await vscode.window.showQuickPick(
      listed.result.targets.map((target) => ({
        label: targetQuickPickLabel(target),
        description: target.url,
        detail: target.id,
        target,
      })),
      { placeHolder: 'Select a CDP target to inspect' },
    );
    if (!picked) {
      return;
    }

    await vscode.workspace.getConfiguration('tspack.inspect').update(
      'targetIndex',
      picked.target.index,
      vscode.ConfigurationTarget.Workspace,
    );
    await inspectTarget(provider, channel, picked.target.index);
  } catch (error) {
    if (error instanceof TspackCliError) {
      writeRawFailure(channel, error);
    }
    vscode.window.showErrorMessage(
      userFacingErrorMessage(error, config.endpoint),
    );
  }
}

async function chooseWorkspaceRoot(): Promise<vscode.WorkspaceFolder | undefined> {
  const workspaceFolders = vscode.workspace.workspaceFolders ?? [];
  if (workspaceFolders.length === 0) {
    vscode.window.showWarningMessage(
      'Open a workspace folder before revealing source hints.',
    );
    return undefined;
  }
  if (workspaceFolders.length === 1) {
    return workspaceFolders[0];
  }

  const picked = await vscode.window.showQuickPick(
    workspaceFolders.map((folder) => ({
      label: folder.name,
      description: folder.uri.fsPath,
      folder,
    })),
    { placeHolder: 'Select the workspace root for this source hint' },
  );
  return picked?.folder;
}

function clampDocumentPosition(
  document: vscode.TextDocument,
  target: { line: number; column: number },
): vscode.Position {
  const lastLine = Math.max(0, document.lineCount - 1);
  const line = Math.min(Math.max(0, target.line), lastLine);
  const lineText = document.lineAt(line).text;
  const column = Math.min(Math.max(0, target.column), lineText.length);
  return new vscode.Position(line, column);
}

async function revealSourceForNode(node: InspectNode | undefined): Promise<void> {
  if (!node) {
    vscode.window.showWarningMessage('No inspect node selected.');
    return;
  }

  const source = node.source;
  if (!source) {
    vscode.window.showWarningMessage(
      'Selected inspect node has no TSPack source hint.',
    );
    return;
  }
  if (isSourceHintMalformed(source)) {
    vscode.window.showWarningMessage(
      'Source hint is malformed and cannot be revealed.',
    );
    return;
  }
  if (!source.file) {
    vscode.window.showWarningMessage(
      'Selected inspect node has no TSPack source hint.',
    );
    return;
  }

  const workspaceRoot = await chooseWorkspaceRoot();
  if (!workspaceRoot) {
    return;
  }

  const resolution = await resolveSourceHintPath(
    workspaceRoot.uri.fsPath,
    source.file,
  );
  if (!resolution.ok) {
    if (resolution.reason === 'notFound') {
      vscode.window.showWarningMessage(
        `Source hint file was not found: ${resolution.displayPath ?? source.file}`,
      );
      return;
    }
    if (
      resolution.reason === 'unsafePath'
      || resolution.reason === 'outsideWorkspace'
    ) {
      vscode.window.showWarningMessage(
        'Refusing to open source hint outside workspace.',
      );
      return;
    }
    vscode.window.showWarningMessage(
      'Selected inspect node has no TSPack source hint.',
    );
    return;
  }

  const target = buildRevealTarget(resolution.realPath, source);
  const document = await vscode.workspace.openTextDocument(
    vscode.Uri.file(target.file),
  );
  const editor = await vscode.window.showTextDocument(document, {
    preview: false,
  });
  const position = clampDocumentPosition(document, target);
  editor.selection = new vscode.Selection(position, position);
  editor.revealRange(
    new vscode.Range(position, position),
    vscode.TextEditorRevealType.InCenterIfOutsideViewport,
  );
}

function showSelectedNode(channel: vscode.OutputChannel, node: InspectNode): void {
  channel.clear();
  channel.appendLine('Selected TSPack inspect node JSON:');
  channel.appendLine(serializeInspectNode(node));
  channel.show(true);
}

export function activate(context: vscode.ExtensionContext): void {
  const provider = new InspectTreeProvider();
  const channel = vscode.window.createOutputChannel('TSPack Inspect');
  let selectedNode: InspectNode | undefined;

  const treeView = vscode.window.createTreeView('tspackInspectTree', {
    treeDataProvider: provider,
  });
  context.subscriptions.push(treeView, channel);

  treeView.onDidChangeSelection(
    (event) => {
      const selected = event.selection[0];
      if (!selected) {
        return;
      }
      selectedNode = selected.node;
      showSelectedNode(channel, selected.node);
    },
    undefined,
    context.subscriptions,
  );

  context.subscriptions.push(vscode.commands.registerCommand(
    'tspack.inspect.targets',
    async () => {
      await inspectTargetsCommand(provider, channel);
    },
  ));

  context.subscriptions.push(vscode.commands.registerCommand(
    'tspack.inspect.refresh',
    async () => {
      const config = getConfig();
      await inspectTarget(provider, channel, config.targetIndex);
    },
  ));

  context.subscriptions.push(vscode.commands.registerCommand(
    'tspack.inspect.copyNodeJson',
    async (item?: InspectTreeItem | InspectTreeNode) => {
      const node = item instanceof InspectTreeItem
        ? item.inspectNode
        : item?.node ?? selectedNode;
      if (!node) {
        vscode.window.showWarningMessage('No TSPack inspect node selected.');
        return;
      }
      await vscode.env.clipboard.writeText(serializeInspectNode(node));
      vscode.window.showInformationMessage('Copied TSPack inspect node JSON.');
    },
  ));

  context.subscriptions.push(vscode.commands.registerCommand(
    'tspack.inspect.revealSource',
    async (item?: InspectTreeItem | InspectTreeNode) => {
      const node = item instanceof InspectTreeItem
        ? item.inspectNode
        : item?.node ?? selectedNode;
      await revealSourceForNode(node);
    },
  ));

  context.subscriptions.push(vscode.commands.registerCommand(
    'tspack.inspect.setCdpEndpoint',
    async () => {
      const config = getConfig();
      const nextEndpoint = await vscode.window.showInputBox({
        prompt: 'CDP endpoint for tspack inspect',
        value: config.endpoint,
        placeHolder: 'http://127.0.0.1:9229',
      });
      if (!nextEndpoint) {
        return;
      }
      await vscode.workspace.getConfiguration('tspack.inspect').update(
        'cdpEndpoint',
        nextEndpoint,
        vscode.ConfigurationTarget.Workspace,
      );
    },
  ));
}

export function deactivate(): void {
  // Nothing to dispose beyond context subscriptions.
}
