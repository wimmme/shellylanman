import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { PALETTES } from '../src/appearance';

test('every palette has its CSS, and a light variant when it says so', () => {
  const css = readFileSync('public/app.css', 'utf8');
  for (const p of PALETTES) {
    if (p.id === 'default') continue; // the base :root / [data-theme] rules
    assert.ok(css.includes(`html[data-palette="${p.id}"]{`), `${p.id}: dark rules`);
    assert.equal(css.includes(`html[data-palette="${p.id}"][data-theme="light"]{`), p.light, `${p.id}: light rules`);
  }
});
