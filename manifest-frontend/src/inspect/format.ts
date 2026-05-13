import type { UIInspectNode, UIInspectResult } from './types.js';

export function formatInspectJson(result: UIInspectResult): string {
  return `${JSON.stringify(result, null, 2)}\n`;
}

function lineForNode(node: UIInspectNode): string {
  const parts = [node.tag];
  if (node.role) parts.push(node.role);
  if (node.name) parts.push(`"${node.name}"`);
  if (node.text && node.text !== node.name) parts.push(`text="${node.text}"`);
  return parts.join(' ');
}

function appendNode(lines: string[], node: UIInspectNode, depth: number): void {
  const pad = '  '.repeat(depth);
  lines.push(`${pad}${lineForNode(node)}`);
  lines.push(`${pad}  bounds: x=${node.bounds.x} y=${node.bounds.y} w=${node.bounds.width} h=${node.bounds.height}`);
  for (const child of node.children) appendNode(lines, child, depth + 1);
}

export function formatInspectText(result: UIInspectResult): string {
  const lines: string[] = [];
  lines.push(`UI Inspect: ${result.target.url}`);
  lines.push(`Browser: ${result.browser.name}`);
  lines.push(`Viewport: ${result.viewport.width} x ${result.viewport.height}`);
  lines.push('');
  if (result.root) appendNode(lines, result.root, 0);
  lines.push('');
  lines.push('Hit tests:');
  for (const hit of result.hitTests) {
    lines.push(`  point ${hit.point.x},${hit.point.y}`);
    for (const e of hit.elements) {
      lines.push(`    ${lineForNode(e)} x=${e.bounds.x} y=${e.bounds.y} w=${e.bounds.width} h=${e.bounds.height}`);
    }
  }
  return `${lines.join('\n')}\n`;
}
