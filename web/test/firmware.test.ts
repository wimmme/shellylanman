import { test } from 'node:test';
import assert from 'node:assert/strict';
import type { Device, FirmwareRow } from '../src/api';
import { betaCell, count, initialChoice, matches, placeholderRows, requests, selectAll, stableCell, toggle, type Choice } from '../src/firmwarelogic';

const row = (id: string, extra: Partial<FirmwareRow> = {}): FirmwareRow => ({ id, name: id, status: 'online', gen: '2', known: true, valid: true, progress: -1, ...extra });
const tr = (k: string, v?: Record<string, number>): string => (v ? `${k}:${v.n}` : k);

test('cells like createTableRow and FWCellRendered', () => {
  const r = row('a', { current: '1.7.5', stable: '1.8.0', stableBuild: 'b/1.8.0-g1', beta: '1.9.0-beta1', betaBuild: 'b/1.9.0-beta1-g2', preselect: true });
  assert.deepEqual(stableCell(r, tr), { type: 'check', label: '1.8.0', tip: 'b/1.8.0-g1' });
  assert.deepEqual(betaCell(r), { type: 'check', label: '1.9.0-beta1', tip: 'b/1.9.0-beta1-g2' });
  assert.equal(initialChoice(r), 'stable');
  assert.deepEqual(stableCell(row('x', { known: false }), tr), { type: 'check', label: 'fw.any', tip: '' });
  assert.deepEqual(stableCell(row('x', { known: false, queued: true }), tr), { type: 'text', text: 'fw.requested' });
  assert.deepEqual(stableCell(row('x', { updating: true }), tr), { type: 'text', text: 'fw.loading:0' });
  assert.deepEqual(stableCell(row('x', { updating: true, progress: 42 }), tr), { type: 'text', text: 'fw.loading:42' });
  assert.deepEqual(stableCell(row('x', { updating: true, gen: '1' }), tr), { type: 'text', text: 'fw.updating' });
  assert.deepEqual(stableCell(row('x', { updating: true, rebooting: true }), tr), { type: 'text', text: 'fw.rebooting' });
  assert.deepEqual(stableCell(row('x'), tr), { type: 'empty' });
  assert.deepEqual(betaCell(row('x', { updating: true, betaBuild: 'b' })), { type: 'empty' });
  assert.equal(initialChoice(row('x', { known: false })), null);
});

test('ticks, select buttons, count and requests', () => {
  assert.equal(toggle('stable', 'beta', true), 'beta');
  assert.equal(toggle('beta', 'beta', false), null);
  assert.equal(toggle('stable', 'beta', false), 'stable');
  const a = row('a', { stableBuild: 's', betaBuild: 'b' }), b = row('b', { stableBuild: 's' }), c = row('c', { known: false });
  assert.equal(selectAll(b, null, 'beta'), null);     // no beta: unchanged
  assert.equal(selectAll(c, null, 'stable'), 'stable'); // "any"
  assert.equal(selectAll(a, 'beta', 'none'), null);
  const ch = new Map<string, Choice>([['a', 'beta'], ['b', 'stable'], ['c', 'stable']]);
  assert.deepEqual(count([a, b, c], ch, ''), { stable: 2, beta: 1 });
  assert.deepEqual(count([a, b, c], ch, 'b'), { stable: 1, beta: 0 });
  assert.deepEqual(requests([a, b, c], ch, ''), [{ id: 'a', stage: 'beta' }, { id: 'b', stage: 'stable' }, { id: 'c', stage: 'any' }]);
  assert.equal(matches(row('x', { name: 'Kitchen', current: '1.7.5' }), '1.7'), true);
  assert.equal(matches(row('x', { name: 'Kitchen' }), 'hall'), false);
});

test('rows appear before the devices answer, without BTHome devices', () => {
  const d = (id: string, gen: string, name: string, hostname: string): Device => ({
    id, mac: id, gen, typeId: '', typeName: '', hostname, name, ip: '1.2.3.4', port: 80,
    status: 'online', managed: true, battery: false, lastSeen: 0, rebootRequired: false,
    rssi: 0, cloudEnabled: false, cloudConnected: false, mqttEnabled: false, mqttConnected: false, uptime: 0, logMode: 'NONE',
  });
  const devs = [d('b', '2', 'Pump', 'shellyplus1-b'), d('a', '1', '', 'shellyplug-s-a'), d('t', 'bth', 'Door', 'sbdw-t')];
  const all = placeholderRows(devs, []);
  assert.deepEqual(all.map((r) => r.name), ['Pump (shellyplus1-b)', 'shellyplug-s-a']);
  assert.ok(all.every((r) => r.known && !r.valid && initialChoice(r) === null));
  assert.equal(stableCell(all[0]!, (k) => k).type, 'empty'); // no "any" tick while loading
  assert.deepEqual(placeholderRows(devs, ['a', 'gone']).map((r) => r.id), ['a']);
});
