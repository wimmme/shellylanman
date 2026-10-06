// Tiny DOM helpers. Text always goes through textContent, never innerHTML, so
// device-supplied strings (names, hostnames) can never inject markup.

type Attrs = Record<string, string | number | boolean | EventListener | undefined>;
type Child = Node | string | null | undefined | false;

/** Create an element: h('button', {class: 'btn', onclick: fn}, 'Save'). */
export function h<K extends keyof HTMLElementTagNameMap>(tag: K, attrs: Attrs = {}, ...children: Child[]): HTMLElementTagNameMap[K] {
  const e = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs)) {
    if (v === undefined || v === false) continue;
    if (k.startsWith('on') && typeof v === 'function') {
      e.addEventListener(k.slice(2), v as EventListener);
    } else if (k === 'style') {
      // Through the CSSOM: the Content-Security-Policy forbids style attributes.
      e.style.cssText = String(v);
    } else if (v === true) {
      e.setAttribute(k, '');
    } else {
      e.setAttribute(k, String(v));
    }
  }
  append(e, ...children);
  return e;
}

export function append(parent: Node, ...children: Child[]): void {
  for (const c of children) {
    if (c === null || c === undefined || c === false) continue;
    parent.appendChild(typeof c === 'string' ? document.createTextNode(c) : c);
  }
}

/**
 * Bring `old` in line with `next` while keeping every node whose markup did not
 * change. A redraw then leaves the row under the mouse (hover) and a checkbox
 * being clicked in place. Table containers and rows are updated attribute by
 * attribute and their children matched by data-id (rows) or position (cells);
 * any other element is swapped whole when its markup differs. Kept nodes keep
 * their old listeners, so those must not depend on data that is not in the
 * markup. Returns the node now in the document.
 */
export function patch(old: Element, next: Element): Element {
  if (old.tagName !== next.tagName) { old.replaceWith(next); return next; }
  if (old.outerHTML === next.outerHTML) return old;
  if (!/^(TABLE|THEAD|TBODY|TR)$/.test(old.tagName)) { old.replaceWith(next); return next; }
  for (const a of [...old.attributes]) if (!next.hasAttribute(a.name)) old.removeAttribute(a.name);
  for (const a of [...next.attributes]) if (old.getAttribute(a.name) !== a.value) old.setAttribute(a.name, a.value);
  const key = (e: Element, i: number): string => e.getAttribute('data-id') ?? `#${i}`;
  const byKey = new Map<string, Element>();
  [...old.children].forEach((c, i) => byKey.set(key(c, i), c));
  const wanted = [...next.children].map((c, i) => {
    const o = byKey.get(key(c, i));
    if (!o) return c;
    byKey.delete(key(c, i));
    return patch(o, c);
  });
  for (const o of byKey.values()) o.remove();
  wanted.forEach((c, i) => { if (old.children[i] !== c) old.insertBefore(c, old.children[i] ?? null); });
  return old;
}

/** Replace box's children unless they would come out the same (keeps a button being clicked or hovered). */
export function keepOrReplace(box: Element, kids: Element[]): void {
  const same = box.children.length === kids.length && kids.every((k, i) => box.children[i]!.outerHTML === k.outerHTML);
  if (!same) box.replaceChildren(...kids);
}

/** An inline SVG icon from a 24×24 stroke path. */
export function icon(path: string, size = 18): SVGSVGElement {
  const ns = 'http://www.w3.org/2000/svg';
  const svg = document.createElementNS(ns, 'svg');
  svg.setAttribute('viewBox', '0 0 24 24');
  svg.setAttribute('width', String(size));
  svg.setAttribute('height', String(size));
  svg.setAttribute('fill', 'none');
  svg.setAttribute('stroke', 'currentColor');
  svg.setAttribute('stroke-width', '1.8');
  svg.setAttribute('stroke-linecap', 'round');
  svg.setAttribute('stroke-linejoin', 'round');
  svg.setAttribute('aria-hidden', 'true');
  const p = document.createElementNS(ns, 'path');
  p.setAttribute('d', path);
  svg.appendChild(p);
  return svg;
}

export const ICONS = {
  logo: 'M13 2 4 14h7l-1 8 9-12h-7z',
  menu: 'M4 6h16M4 12h16M4 18h16',
  menuOpen: 'M4 6h11M4 12h8M4 18h11M20 8l-4 4 4 4', // Home Assistant's "menu open": hamburger with an arrow
  logout: 'M15 4h3a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2h-3M10 16l-4-4 4-4M6 12h10',
  devices: 'M4 4h7v7H4zM13 4h7v7h-7zM4 13h7v7H4zM13 13h7v7h-7z',
  checklist: 'M9 11l2 2 4-4M5 4h14v16H5z',
  charts: 'M4 20V10M10 20V4M16 20v-7M22 20H2',
  firmware: 'M12 3v12m0 0-4-4m4 4 4-4M4 21h16',
  deferred: 'M12 7v5l3 3M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18z',
  log: 'M8 6h12M8 12h12M8 18h12M4 6h.01M4 12h.01M4 18h.01',
  settings: 'M12 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6zM19.4 15a1.7 1.7 0 0 0 .3 1.8l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-2.9 1.2V21a2 2 0 1 1-4 0v-.1a1.7 1.7 0 0 0-2.9-1.2l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1A1.7 1.7 0 0 0 3 15H3a2 2 0 1 1 0-4h.1A1.7 1.7 0 0 0 4.6 9l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1A1.7 1.7 0 0 0 10 4.9V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 2.9 1.2l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1A1.7 1.7 0 0 0 21 11h.1a2 2 0 1 1 0 4H21a1.7 1.7 0 0 0-1.6 0z',
  about: 'M12 16v-4M12 8h.01M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18z',
  sun: 'M12 3v1m0 16v1m9-9h-1M4 12H3m15.4-6.4-.7.7M6.3 17.7l-.7.7M17.7 17.7l-.7-.7M6.3 6.3l-.7-.7M12 8a4 4 0 1 0 0 8 4 4 0 0 0 0-8z',
  moon: 'M21 12.8A9 9 0 1 1 11.2 3 7 7 0 0 0 21 12.8z',
  empty: 'M3 7h18M3 7l2 13h14l2-13M8 7V4h8v3',
  refresh: 'M21 12a9 9 0 1 1-2.6-6.4M21 4v5h-5',
  radar: 'M12 12l6-6M12 3a9 9 0 1 0 9 9M12 7a5 5 0 1 0 5 5M12 11a1 1 0 1 0 1 1',
} as const;
