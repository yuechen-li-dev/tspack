import type { InspectBounds, InspectNode, InspectResult } from './inspectTypes';

export type InspectTreeNode = {
  id: string;
  label: string;
  description: string;
  tooltip: string;
  node: InspectNode;
  children: InspectTreeNode[];
};

function quoteLabel(value: string): string {
  return `"${value}"`;
}

function classNameFromNode(node: InspectNode): string | undefined {
  const rawClass = (node as { className?: unknown }).className;
  if (typeof rawClass === 'string' && rawClass.trim()) {
    const firstClass = rawClass.trim().split(/\s+/)[0];
    return `.${firstClass}`;
  }
  return undefined;
}

export function buildInspectNodeLabel(node: InspectNode): string {
  const role = node.role?.trim();
  const name = node.name?.trim() || node.text?.trim();
  if (role && name) {
    return `${role} ${quoteLabel(name)}`;
  }
  if (role) {
    return role;
  }

  const tag = node.tag?.trim();
  const className = classNameFromNode(node);
  if (tag && className) {
    return `${tag} ${className}`;
  }
  if (tag) {
    return tag;
  }
  if (name) {
    return quoteLabel(name);
  }
  return 'node';
}

export function formatCompactBounds(bounds: InspectBounds | undefined): string {
  if (!bounds) {
    return '';
  }
  return `${bounds.x},${bounds.y},${bounds.width},${bounds.height}`;
}

export function buildInspectNodeDescription(node: InspectNode): string {
  const parts: string[] = [];
  const bounds = formatCompactBounds(node.bounds);
  if (bounds) {
    parts.push(bounds);
  }
  if (node.visible === false) {
    parts.push('hidden');
  } else if (node.visible === true) {
    parts.push('visible');
  }
  if (node.focusable === true) {
    parts.push('focusable');
  }
  return parts.join(' · ');
}

function appendTooltipLine(lines: string[], label: string, value: unknown): void {
  if (value === undefined || value === null || value === '') {
    return;
  }
  lines.push(`${label}: ${String(value)}`);
}

export function buildInspectNodeTooltip(node: InspectNode): string {
  const lines: string[] = [];
  appendTooltipLine(lines, 'tag', node.tag);
  appendTooltipLine(lines, 'role', node.role);
  appendTooltipLine(lines, 'name', node.name);
  appendTooltipLine(lines, 'text', node.text);
  appendTooltipLine(lines, 'bounds', formatCompactBounds(node.bounds));
  appendTooltipLine(lines, 'visible', node.visible);
  appendTooltipLine(lines, 'focusable', node.focusable);
  if (node.source) {
    appendTooltipLine(lines, 'source', node.source.raw);
    appendTooltipLine(lines, 'sourceFile', node.source.file);
    appendTooltipLine(lines, 'sourceLine', node.source.line);
    appendTooltipLine(lines, 'sourceColumn', node.source.column);
    appendTooltipLine(lines, 'component', node.source.component);
    appendTooltipLine(lines, 'symbol', node.source.symbol);
    appendTooltipLine(lines, 'sourceParseError', node.source.parseError);
  }
  if (node.style) {
    appendTooltipLine(lines, 'display', node.style.display);
    appendTooltipLine(lines, 'position', node.style.position);
    appendTooltipLine(lines, 'zIndex', node.style.zIndex);
    appendTooltipLine(lines, 'pointerEvents', node.style.pointerEvents);
    appendTooltipLine(lines, 'opacity', node.style.opacity);
    appendTooltipLine(lines, 'overflow', node.style.overflow);
    appendTooltipLine(lines, 'fontSize', node.style.fontSize);
    appendTooltipLine(lines, 'fontWeight', node.style.fontWeight);
  }
  return lines.join('\n');
}

export function getInspectNodeContextValue(node: InspectNode): string {
  if (node.source?.file) {
    return 'inspectNodeWithSource';
  }
  return 'inspectNode';
}

function buildTreeNode(node: InspectNode, path: string): InspectTreeNode {
  const children = Array.isArray(node.children) ? node.children : [];
  return {
    id: node.id || path,
    label: buildInspectNodeLabel(node),
    description: buildInspectNodeDescription(node),
    tooltip: buildInspectNodeTooltip(node),
    node,
    children: children.map((child, index) => {
      return buildTreeNode(child, `${path}.${index}`);
    }),
  };
}

export function buildInspectTree(result: InspectResult): InspectTreeNode[] {
  if (!result.root) {
    return [];
  }
  return [buildTreeNode(result.root, 'root')];
}

export function serializeInspectNode(node: InspectNode): string {
  return JSON.stringify(node, null, 2);
}
