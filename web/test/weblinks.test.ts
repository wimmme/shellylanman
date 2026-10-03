import { test } from 'node:test';
import assert from 'node:assert/strict';
import { deviceURL, manyAtOnce, openMode } from '../src/weblinks';

test('device addresses and the default way to open them', () => {
  assert.equal(deviceURL('192.168.0.86', 80), 'http://192.168.0.86');
  assert.equal(deviceURL('192.168.0.86', 8081), 'http://192.168.0.86:8081');
  // No storage (private mode, tests): a new tab, several at once.
  assert.equal(openMode(), 'tab');
  assert.equal(manyAtOnce(), true);
});
