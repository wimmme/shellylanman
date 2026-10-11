import { test } from 'node:test';
import assert from 'node:assert/strict';
import { errorOf } from '../src/api';

// {"error": {"code", "message", "retryAfter"}} (DECISIONS P22-2).
test('errorOf reads the coded error', () => {
  assert.deepEqual(errorOf({ error: { code: 'device_offline', message: 'timeout' } }), { code: 'device_offline', message: 'timeout' });
  assert.equal(errorOf({ error: { code: 'too_many_tries', message: 'wait', retryAfter: 4 } }).retryAfter, 4);
});

test('errorOf gives nothing for other answers', () => {
  assert.deepEqual(errorOf({}), {});
  assert.deepEqual(errorOf(null), {});
  assert.deepEqual(errorOf({ error: 'text of an old server' }), {});
});
