import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { PALETTES, insideHomeAssistant, startLook } from '../src/appearance';

test('every palette has its CSS, and a light variant when it says so', () => {
  const css = readFileSync('public/app.css', 'utf8');
  for (const p of PALETTES) {
    if (p.id === 'default') continue; // the base :root / [data-theme] rules
    assert.ok(css.includes(`html[data-palette="${p.id}"]{`), `${p.id}: dark rules`);
    assert.equal(css.includes(`html[data-palette="${p.id}"][data-theme="light"]{`), p.light, `${p.id}: light rules`);
  }
});

test('inside Home Assistant the Home Assistant palette is the default, following the browser', () => {
  const ing = '/api/hassio_ingress/abc123/';
  assert.deepEqual(startLook(ing, true, null, null), { theme: 'dark', palette: 'homeassistant', follow: true });
  assert.deepEqual(startLook(ing, false, null, null), { theme: 'light', palette: 'homeassistant', follow: true });
  // A choice made by the user wins, inside Home Assistant too.
  assert.deepEqual(startLook(ing, false, 'dark', 'nord'), { theme: 'dark', palette: 'nord', follow: false });
  assert.deepEqual(startLook(ing, true, 'light', null), { theme: 'light', palette: 'default', follow: false });
  // On the LAN port: the default dark look, as before.
  assert.deepEqual(startLook('/', false, null, null), { theme: 'dark', palette: 'default', follow: false });
  assert.equal(insideHomeAssistant('/api/hassio_ingress/x/'), true);
  assert.equal(insideHomeAssistant('/'), false);
});
