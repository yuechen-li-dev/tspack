export const INSPECT_ANALYZER_SCRIPT = String.raw`
({ selector, points }) => {
  const styleTags = new Set(['script', 'style', 'template']);
  const interactiveTags = new Set(['button', 'a', 'input', 'select', 'textarea', 'summary', 'details']);
  let nextId = 1;

  const compact = (value) => value.trim().replace(/\s+/g, ' ').slice(0, 160);

  const inferRole = (el) => {
    const explicit = el.getAttribute('role');
    if (explicit) {
      return explicit;
    }

    const tag = el.tagName.toLowerCase();
    if (tag === 'a' && el.href) return 'link';
    if (tag === 'button') return 'button';
    if (/^h[1-6]$/.test(tag)) return 'heading';
    if (tag === 'main' || tag === 'header' || tag === 'footer' || tag === 'nav' || tag === 'aside' || tag === 'form') {
      return tag;
    }

    return undefined;
  };

  const inferName = (el) => {
    const aria = el.getAttribute('aria-label');
    if (aria) {
      return compact(aria);
    }

    const txt = compact(el.textContent ?? '');
    return txt || undefined;
  };

  const isPositiveIntegerText = (value) => /^[1-9][0-9]*$/.test(value);

  const parseSourceLocation = (raw) => {
    const source = { raw };
    const value = raw.trim();

    if (!value) {
      source.parseError = 'empty source hint';
      return source;
    }

    if (!value.includes(':')) {
      source.file = value;
      return source;
    }

    const parts = value.split(':');
    const lastPart = parts[parts.length - 1];
    const previousPart = parts.length > 1 ? parts[parts.length - 2] : undefined;

    if (isPositiveIntegerText(lastPart) && previousPart !== undefined && isPositiveIntegerText(previousPart)) {
      const file = parts.slice(0, -2).join(':');
      if (!file) {
        source.parseError = 'missing source file';
        return source;
      }

      source.file = file;
      source.line = Number(previousPart);
      source.column = Number(lastPart);
      return source;
    }

    if (isPositiveIntegerText(lastPart)) {
      const file = parts.slice(0, -1).join(':');
      if (!file) {
        source.parseError = 'missing source file';
        return source;
      }

      source.file = file;
      source.line = Number(lastPart);
      return source;
    }

    source.parseError = 'invalid source line or column';
    return source;
  };

  const sourceHintFor = (el) => {
    const raw = el.getAttribute('data-tspack-source');
    const component = el.getAttribute('data-tspack-component');
    const symbol = el.getAttribute('data-tspack-symbol');

    if (raw === null && !component && !symbol) {
      return undefined;
    }

    const source = raw === null ? {} : parseSourceLocation(raw);
    if (component) {
      source.component = component;
    }
    if (symbol) {
      source.symbol = symbol;
    }

    return source;
  };

  const nodeFor = (el) => {
    const rect = el.getBoundingClientRect();
    const cs = window.getComputedStyle(el);
    const tag = el.tagName.toLowerCase();
    const visible = rect.width > 0 && rect.height > 0 && cs.display !== 'none' && cs.visibility !== 'hidden';

    const node = {
      id: 'node-' + nextId++,
      tag,
      role: inferRole(el),
      name: inferName(el),
      text: compact(el.textContent ?? '') || undefined,
      bounds: { x: rect.x, y: rect.y, width: rect.width, height: rect.height },
      visible,
      focusable: el.tabIndex >= 0,
      source: sourceHintFor(el),
      style: {
        display: cs.display,
        position: cs.position,
        zIndex: cs.zIndex,
        pointerEvents: cs.pointerEvents,
        opacity: cs.opacity,
        overflow: cs.overflow,
        fontSize: cs.fontSize,
        fontWeight: cs.fontWeight
      },
      children: []
    };

    for (const child of Array.from(el.children)) {
      const childTag = child.tagName.toLowerCase();
      if (styleTags.has(childTag)) {
        continue;
      }

      const childNode = nodeFor(child);
      if (childNode.visible || childNode.children.length > 0 || interactiveTags.has(childNode.tag)) {
        node.children.push(childNode);
      }
    }

    return node;
  };

  const root = selector ? document.querySelector(selector) : (document.body ?? document.documentElement);
  if (!root) {
    return { root: null, hitTests: [] };
  }

  const hitTests = points.map((point) => ({
    point,
    elements: document.elementsFromPoint(point.x, point.y).map((el) => nodeFor(el))
  }));

  return { root: nodeFor(root), hitTests };
}
`;
