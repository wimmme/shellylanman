import { test } from 'node:test';
import assert from 'node:assert/strict';
import type { ChecklistRow } from '../src/api';
import { cellView, editable, HAS, isG1, sameBoolean, sameObject, sameStringOrInt } from '../src/checklistlogic';
import { whyDisabled } from '../src/why';

const row = (id: string, gen: string, extra: Partial<ChecklistRow> = {}): ChecklistRow => ({
  id, host: id, address: '1.2.3.4', status: 'online', gen,
  eco: null, led: null, logs: null, ble: null, ap: null, roaming: null, wifi1: null, wifi2: null, extender: null, scripts: null, autoFW: null, ...extra,
});

test('cells render like the Java renderers', () => {
  assert.deepEqual(cellView('eco', true, true), { text: '✓', cls: 'chk-ok' });
  assert.deepEqual(cellView('logs', true, false), { text: '✓', cls: 'chk-bad' });
  assert.deepEqual(cellView('ap', false), { text: '✗', cls: '' });
  assert.deepEqual(cellView('wifi1', '✓', true), { text: '✓', cls: '' }); // strings are not coloured
  assert.deepEqual(cellView('ble', [], undefined), { text: '0', cls: 'chk-dim' });
  assert.deepEqual(cellView('extender', 0), { text: '0', cls: 'chk-bad chk-dim' });
  assert.deepEqual(cellView('extender', '✗'), { text: '✗', cls: 'chk-ok' });
  assert.deepEqual(cellView('scripts', null), { text: '-', cls: '' });
});

test('actions are enabled for uniform selections only', () => {
  const a = row('a', '1', { eco: false, led: true, logs: false, roaming: '✗', autoFW: '-' });
  const b = row('b', '1', { eco: false, led: true, logs: true, roaming: '-85', autoFW: 'stable' });
  const c = row('c', '2', { eco: false, logs: 'socket', roaming: '-70', autoFW: 'stable' });
  assert.equal(sameBoolean([a, b], 'eco'), true);
  assert.equal(sameBoolean([a, b], 'logs', isG1), false);
  assert.equal(sameBoolean([a, c], 'led', isG1), false);
  assert.equal(sameStringOrInt([b, c], 'roaming'), true);  // both enabled with a threshold
  assert.equal(sameStringOrInt([a, b], 'roaming'), false); // disabled and enabled
  assert.equal(sameObject([c], 'logs'), 'socket');
  assert.equal(sameObject([c, row('d', '2', { logs: 'mqtt' })], 'logs'), undefined);
  assert.equal(editable([b, c], 'autoFW'), true);
  assert.equal(editable([a, c], 'autoFW'), false);
});

test('why a checklist action is disabled', () => {
  let i = 0;
  const r = (gen: string, led: boolean | null): ChecklistRow => row(String(i++), gen, { led });
  assert.equal(whyDisabled([], true, HAS.led!), null);
  assert.deepEqual(whyDisabled([], false, HAS.led!), { key: 'why.none' });
  assert.deepEqual(whyDisabled([r('1', true), r('1', true)], false, HAS.ble!, true), { key: 'why.one' });
  assert.deepEqual(whyDisabled([r('1', true), r('2', null), r('3', null)], false, HAS.led!), { key: 'why.missing', n: 2 });
  assert.deepEqual(whyDisabled([r('1', true), r('1', false)], false, HAS.led!), { key: 'why.differ' });
});
