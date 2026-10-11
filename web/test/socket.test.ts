import { test } from 'node:test';
import assert from 'node:assert/strict';
import { newServer } from '../src/socket';

// After a reconnect: another version answers → the page asks to be loaded again.
test('newServer', () => {
  assert.equal(newServer('0.9.7', '0.9.8'), true);
  assert.equal(newServer('0.9.7', '0.9.7'), false);
  assert.equal(newServer('', '0.9.8'), false); // the page does not know its version yet
  assert.equal(newServer('0.9.7', undefined), false);
});
