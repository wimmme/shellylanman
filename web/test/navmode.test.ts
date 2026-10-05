import { test } from 'node:test';
import assert from 'node:assert/strict';
import { navMode, setNavMode, toggled } from '../src/navmode';

test('sidebar mode: full by default, remembered, toggles', () => {
  const store = new Map<string, string>();
  (globalThis as { localStorage?: unknown }).localStorage = {
    getItem: (k: string) => store.get(k) ?? null,
    setItem: (k: string, v: string) => { store.set(k, v); },
  };
  assert.equal(navMode(), 'full');
  setNavMode(toggled(navMode()));
  assert.equal(navMode(), 'mini');
  setNavMode(toggled(navMode()));
  assert.equal(navMode(), 'full');
  store.set('sl_nav', 'garbage');
  assert.equal(navMode(), 'full');
});

test('sidebar mode: no storage (private mode) means full', () => {
  (globalThis as { localStorage?: unknown }).localStorage = {
    getItem: () => { throw new Error('denied'); },
    setItem: () => { throw new Error('denied'); },
  };
  assert.equal(navMode(), 'full');
  assert.doesNotThrow(() => setNavMode('mini'));
});
