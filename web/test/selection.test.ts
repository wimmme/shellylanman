import { test } from 'node:test';
import assert from 'node:assert/strict';
import { idsInHash, openingScope, selected } from '../src/selection';

test('ids in the address', () => {
  assert.deepEqual(idsInHash('#/checklist?ids=a%2Cb'), ['a', 'b']);
  assert.deepEqual(idsInHash('#/checklist?x=1&ids=c'), ['c']);
  assert.deepEqual(idsInHash('#/checklist'), []);
});

test('the opening scope is the selection that still exists, else all (empty)', () => {
  const known = (id: string): boolean => id !== 'gone';
  assert.deepEqual(openingScope(['a', 'gone', 'b'], known), ['a', 'b']);
  assert.deepEqual(openingScope(['gone'], known), []);
  assert.deepEqual(openingScope([], known), []);
});

test('the shared selection notifies once per change and works without storage', () => {
  let calls = 0;
  const off = selected.onChange(() => calls++);
  selected.replace(['a', 'b']);
  selected.add('b'); // already there: no change
  selected.add('c');
  selected.delete('x'); // not there: no change
  selected.delete('a');
  assert.deepEqual([...selected].sort(), ['b', 'c']);
  assert.equal(calls, 3);
  selected.clear();
  selected.clear(); // empty: no change
  assert.equal(calls, 4);
  off();
  selected.add('d');
  assert.equal(calls, 4);
  selected.clear();
});
