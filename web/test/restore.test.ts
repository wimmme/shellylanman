import { test } from 'node:test';
import assert from 'node:assert/strict';
import type { RestoreItem } from '../src/api';
import { itemMessage, MULTI_ANSWERS, planOf, problemText, Q_ENABLE, Q_OVERRIDE, Q_SKIP, scriptAnswers, shortReason } from '../src/restorelogic';

const it = (key: string, type: RestoreItem['type'], value?: string, args?: string[]): RestoreItem => ({ key, type, value, args });

test('the check result is split like restoreDevice walks it', () => {
  const p = planOf([
    it('PRE_QUESTION_RESTORE_HOST', 'pre', 'other'), it('WARN_RESTORE_BTHOME', 'warn'),
    it('RESTORE_MQTT', 'ask', 'u'), it('RESTORE_LOGIN', 'ask', 'admin'), it(Q_OVERRIDE, 'ask', 'a, b'),
  ]);
  assert.equal(p.pre.length, 1);
  assert.equal(p.error, null);
  assert.deepEqual(p.passwords.map((x) => x.key), ['RESTORE_LOGIN', 'RESTORE_MQTT']); // the order of the original
  assert.equal(p.override?.value, 'a, b');
  assert.equal(p.enable, null);
});

test('script answers follow the original', () => {
  const both = planOf([it(Q_OVERRIDE, 'ask', 'a'), it(Q_ENABLE, 'ask', 'a')]);
  assert.deepEqual(scriptAnswers(both, 'overwrite'), { answers: { [Q_OVERRIDE]: 'true' }, enableAsked: true });
  assert.deepEqual(scriptAnswers(both, 'rename'), { answers: {}, enableAsked: false });
  assert.deepEqual(scriptAnswers(both, 'skip'), { answers: { [Q_SKIP]: 'true' }, enableAsked: false });
  const enableOnly = planOf([it(Q_ENABLE, 'ask', 'a')]);
  assert.deepEqual(scriptAnswers(enableOnly, null), { answers: {}, enableAsked: true });
  assert.deepEqual(MULTI_ANSWERS, { [Q_OVERRIDE]: 'true', [Q_ENABLE]: 'true' });
});

test('messages and problems', () => {
  const tr = (k: string): string => 'T:' + k;
  const m = itemMessage(it('ERR_RESTORE_PROFILE', 'error', '', ['rgbw', 'light']), 'PlusRGBWPM', tr);
  assert.equal(m.key, 'restore.msg.ERR_RESTORE_PROFILE');
  assert.deepEqual([m.vars.current, m.vars.backup], ['T:restore.profile.rgbw', 'T:restore.profile.lightx4']);
  assert.equal(itemMessage(it('ERR_RESTORE_PROFILE', 'error', '', ['x', 'y']), 'Plus1', tr).vars.current, 'x');
  assert.equal(problemText('ERR_UNKNOWN', tr), 'T:restore.err.ERR_UNKNOWN');
  assert.equal(problemText('Action "x" - error: y', tr), 'Action "x" - error: y');
  assert.equal(shortReason('short'), 'short');
  assert.equal(shortReason('x'.repeat(50)), undefined);
});
