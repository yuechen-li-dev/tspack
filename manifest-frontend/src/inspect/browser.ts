import type { InspectOptions } from './index.js';
import type { UIInspectResult } from './types.js';

export async function runInspect(options: InspectOptions): Promise<UIInspectResult> {
  if (!/^https?:\/\//i.test(options.url ?? '')) {
    throw new Error('TSPACK_INSPECT_INVALID_TARGET');
  }
  if (options.browser !== 'chromium') {
    throw new Error('TSPACK_INSPECT_BROWSER_UNSUPPORTED');
  }

  let playwright: typeof import('playwright');
  try {
    playwright = await import('playwright');
  } catch {
    throw new Error('TSPACK_INSPECT_BROWSER_LAUNCH_FAILED');
  }

  const browser = await playwright.chromium.launch();
  try {
    const page = await browser.newPage({ viewport: { width: options.viewport.width, height: options.viewport.height } });
    await page.goto(options.url as string, { waitUntil: 'load' });

    const payload = await page.evaluate(({ selector, points }) => {
      const styleTags = new Set(['script', 'style', 'template']);
      const interactiveTags = new Set(['button', 'a', 'input', 'select', 'textarea', 'summary', 'details']);
      let nextId = 1;
      const compact = (value: string) => value.trim().replace(/\s+/g, ' ').slice(0, 160);
      const inferRole = (el: Element): string | undefined => {
        const explicit = el.getAttribute('role');
        if (explicit) return explicit;
        const tag = el.tagName.toLowerCase();
        if (tag === 'a' && (el as HTMLAnchorElement).href) return 'link';
        if (tag === 'button') return 'button';
        if (/^h[1-6]$/.test(tag)) return 'heading';
        if (tag === 'main' || tag === 'header' || tag === 'footer' || tag === 'nav' || tag === 'aside' || tag === 'form') return tag;
      };
      const inferName = (el: Element): string | undefined => {
        const aria = el.getAttribute('aria-label');
        if (aria) return compact(aria);
        const txt = compact(el.textContent ?? '');
        return txt || undefined;
      };
      const nodeFor = (el: Element): any => {
        const rect = el.getBoundingClientRect();
        const cs = window.getComputedStyle(el);
        const tag = el.tagName.toLowerCase();
        const visible = rect.width > 0 && rect.height > 0 && cs.display !== 'none' && cs.visibility !== 'hidden';
        const node: any = {
          id: `node-${nextId++}`,
          tag,
          role: inferRole(el),
          name: inferName(el),
          text: compact(el.textContent ?? '') || undefined,
          bounds: { x: rect.x, y: rect.y, width: rect.width, height: rect.height },
          visible,
          focusable: (el as HTMLElement).tabIndex >= 0,
          style: { display: cs.display, position: cs.position, zIndex: cs.zIndex, pointerEvents: cs.pointerEvents, opacity: cs.opacity, overflow: cs.overflow, fontSize: cs.fontSize, fontWeight: cs.fontWeight },
          children: []
        };
        for (const child of Array.from(el.children)) {
          const childTag = child.tagName.toLowerCase();
          if (styleTags.has(childTag)) continue;
          const c = nodeFor(child);
          if (c.visible || c.children.length > 0 || interactiveTags.has(c.tag)) node.children.push(c);
        }
        return node;
      };
      const root = selector ? document.querySelector(selector) : (document.body ?? document.documentElement);
      if (!root) return { root: null, hitTests: [] };
      const hits = points.map((p) => ({ point: p, elements: document.elementsFromPoint(p.x, p.y).map((el) => nodeFor(el)) }));
      return { root: nodeFor(root), hitTests: hits };
    }, { selector: options.selector, points: options.points });

    if (options.selector && !payload.root) throw new Error('TSPACK_INSPECT_SELECTOR_NOT_FOUND');

    return {
      target: { url: options.url as string },
      browser: { name: 'chromium' },
      viewport: { width: options.viewport.width, height: options.viewport.height },
      root: payload.root,
      hitTests: payload.hitTests,
      diagnostics: []
    };
  } catch (error: unknown) {
    if (error instanceof Error && error.message.startsWith('TSPACK_INSPECT_')) {
      throw error;
    }
    throw new Error('TSPACK_INSPECT_PAGE_LOAD_FAILED');
  } finally {
    await browser.close();
  }
}
