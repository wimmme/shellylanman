import { test } from 'node:test';
import assert from 'node:assert/strict';
import { CATALOGUES, LANGUAGES } from '../src/i18n';

test('every language has exactly the English keys', () => {
  const en = Object.keys(CATALOGUES.en).sort();
  for (const [lang, cat] of Object.entries(CATALOGUES)) {
    assert.deepEqual(Object.keys(cat).sort(), en, `catalogue ${lang} differs from en`);
  }
});

test('no empty translations', () => {
  for (const [lang, cat] of Object.entries(CATALOGUES)) {
    for (const [k, v] of Object.entries(cat)) assert.ok(v.trim() !== '', `${lang}.${k} is empty`);
  }
});

test('placeholders match between languages', () => {
  const ph = (s: string): string[] => (s.match(/\{\w+\}/g) || []).sort();
  for (const [lang, cat] of Object.entries(CATALOGUES)) {
    for (const [k, v] of Object.entries(CATALOGUES.en)) {
      assert.deepEqual(ph(cat[k]!), ph(v), `placeholders differ for ${lang}.${k}`);
    }
  }
});

test('every catalogue is offered in the language list', () => {
  assert.deepEqual(LANGUAGES.map((l) => l.id).sort(), Object.keys(CATALOGUES).sort());
});
