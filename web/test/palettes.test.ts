import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { BASE_PALETTE, PALETTES, START_PALETTE, migratePalette, startLook } from '../src/appearance';

test('every palette has its CSS, and a light variant when it says so', () => {
  const css = readFileSync('public/app.css', 'utf8');
  for (const p of PALETTES) {
    if (p.id === BASE_PALETTE) continue; // the base :root / [data-theme] rules
    assert.ok(css.includes(`html[data-palette="${p.id}"]{`), `${p.id}: dark rules`);
    assert.equal(css.includes(`html[data-palette="${p.id}"][data-theme="light"]{`), p.light, `${p.id}: light rules`);
  }
  assert.ok(PALETTES.some((p) => p.id === START_PALETTE));
  assert.ok(PALETTES.some((p) => p.id === BASE_PALETTE));
});

test('the Home Assistant palette is the start look everywhere, following the system (P12-8)', () => {
  assert.deepEqual(startLook(true, null, null), { theme: 'dark', palette: 'homeassistant', follow: true });
  assert.deepEqual(startLook(false, null, null), { theme: 'light', palette: 'homeassistant', follow: true });
  // A choice made by the user wins.
  assert.deepEqual(startLook(false, 'dark', 'nord'), { theme: 'dark', palette: 'nord', follow: false });
  assert.deepEqual(startLook(true, 'light', null), { theme: 'light', palette: 'midnight', follow: false });
});

test('the palette stored as "default" before 0.6.0 is Midnight', () => {
  assert.equal(migratePalette('default'), 'midnight');
  assert.equal(migratePalette('nord'), 'nord');
  assert.equal(migratePalette(null), null);
  assert.deepEqual(startLook(true, 'dark', 'default'), { theme: 'dark', palette: 'midnight', follow: false });
});
