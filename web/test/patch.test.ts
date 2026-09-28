import { test } from 'node:test';
import assert from 'node:assert/strict';
import { patch } from '../src/dom';

// Just enough of the DOM for patch(): elements, attributes, children, text.
class El {
  attributes: { name: string; value: string }[] = [];
  children: El[] = [];
  parent: El | null = null;
  constructor(public tagName: string, public text = '') {}
  get outerHTML(): string {
    const a = this.attributes.map((x) => ` ${x.name}="${x.value}"`).join('');
    return `<${this.tagName}${a}>${this.text}${this.children.map((c) => c.outerHTML).join('')}</${this.tagName}>`;
  }
  hasAttribute(n: string): boolean { return this.attributes.some((x) => x.name === n); }
  getAttribute(n: string): string | null { return this.attributes.find((x) => x.name === n)?.value ?? null; }
  setAttribute(n: string, v: string): void { this.removeAttribute(n); this.attributes.push({ name: n, value: v }); }
  removeAttribute(n: string): void { this.attributes = this.attributes.filter((x) => x.name !== n); }
  remove(): void { if (this.parent) { this.parent.children.splice(this.parent.children.indexOf(this), 1); this.parent = null; } }
  replaceWith(n: El): void { const p = this.parent!; n.remove(); p.children[p.children.indexOf(this)] = n; n.parent = p; this.parent = null; }
  insertBefore(n: El, ref: El | null): void {
    n.remove();
    const i = ref ? this.children.indexOf(ref) : this.children.length;
    this.children.splice(i, 0, n); n.parent = this;
  }
}

function el(tag: string, attrs: Record<string, string> = {}, ...kids: (El | string)[]): El {
  const e = new El(tag.toUpperCase());
  for (const [k, v] of Object.entries(attrs)) e.setAttribute(k, v);
  for (const k of kids) { if (typeof k === 'string') e.text += k; else e.insertBefore(k, null); }
  return e;
}

const table = (rows: [string, string, string][]): El =>
  el('table', { class: 'data' }, el('tbody', {}, ...rows.map(([id, cls, val]) =>
    el('tr', { 'data-id': id, class: cls }, el('td', {}, id), el('td', {}, val)))));

test('unchanged rows and cells are kept; changed cells are swapped', () => {
  const host = el('div');
  const old = table([['a', '', '1'], ['b', '', '2']]);
  host.insertBefore(old, null);
  const rowA = old.children[0]!.children[0]!, rowB = old.children[0]!.children[1]!;
  const cellB0 = rowB.children[0]!;
  const out = patch(old as unknown as Element, table([['a', '', '1'], ['b', 'selected', '3']]) as unknown as Element);
  assert.equal(out, old);
  const body = old.children[0]!;
  assert.equal(body.children[0], rowA, 'unchanged row kept');
  assert.equal(body.children[1], rowB, 'changed row patched in place');
  assert.equal(rowB.getAttribute('class'), 'selected');
  assert.equal(rowB.children[0], cellB0, 'unchanged cell kept');
  assert.equal(rowB.children[1]!.text, '3');
  assert.equal(rowB.children.length, 2);
});

test('rows are matched by data-id across reordering, additions and removals', () => {
  const host = el('div');
  const old = table([['a', '', '1'], ['b', '', '2'], ['c', '', '3']]);
  host.insertBefore(old, null);
  const [a, , c] = old.children[0]!.children;
  patch(old as unknown as Element, table([['c', '', '3'], ['d', '', '4'], ['a', '', '1']]) as unknown as Element);
  const body = old.children[0]!;
  assert.deepEqual(body.children.map((r) => r.getAttribute('data-id')), ['c', 'd', 'a']);
  assert.equal(body.children[0], c);
  assert.equal(body.children[2], a);
});
