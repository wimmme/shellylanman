import { test } from 'node:test';
import assert from 'node:assert/strict';
import { editorDark, fromStored, IDE_DEFAULTS } from '../src/ideprefs';

test('the editor follows the app unless dark or light is chosen', () => {
  const p = (theme: 'auto' | 'dark' | 'light') => ({ ...IDE_DEFAULTS, theme });
  assert.equal(editorDark(p('auto'), 'dark'), true);
  assert.equal(editorDark(p('auto'), 'light'), false);
  assert.equal(editorDark(p('dark'), 'light'), true);
  assert.equal(editorDark(p('light'), 'dark'), false);
});

test('stored editor settings: defaults, and the setting before 0.6.3', () => {
  assert.equal(fromStored(null).theme, 'auto');
  assert.equal(fromStored('{"dark":true,"tabSize":2}').theme, 'dark');   // a dark choice stays dark
  assert.equal(fromStored('{"dark":true,"tabSize":2}').tabSize, 2);
  assert.equal(fromStored('{"dark":false}').theme, 'auto');              // the old default follows the app
  assert.equal(fromStored('{"theme":"light","dark":true}').theme, 'light');
  assert.equal(fromStored('{"theme":"pink"}').theme, 'auto');
  assert.equal(fromStored('not json').theme, 'auto');
  assert.ok(!('dark' in fromStored('{"dark":true}')));
});
