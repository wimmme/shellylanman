import { test } from 'node:test';
import assert from 'node:assert/strict';
import type { Device } from '../src/api';
import { availableTypes, horizontalCSV, seriesFor, uniqueName, valueOf, verticalCSV } from '../src/chartlogic';

const dev = (id: string, extra: Partial<Device>): Device => ({
  id, mac: id, gen: '2', typeId: 'Plus1', typeName: 'x', hostname: 'h-' + id, name: '', ip: '', port: 80, status: 'online', managed: true, battery: false,
  lastSeen: 0, rebootRequired: false, rssi: -60, cloudEnabled: false, cloudConnected: false, mqttEnabled: false, mqttConnected: false, uptime: 0, logMode: '', ...extra,
} as Device);

const em = dev('em', { typeId: 'Pro3EM', name: 'Laadpaal', meters: [
  { label: 'net', values: [{ type: 'W', value: 100 }, { type: 'V', value: 230 }] },
  { values: [{ type: 'W', value: 50 }, { type: 'V', value: 231 }] },
  { values: [{ type: 'W', value: 25 }, { type: 'V', value: 229 }] },
  { total: true, values: [{ type: 'W', value: 175 }] },
] });
const ht = dev('ht', { name: 'Hall', meters: [{ values: [{ type: 'T', value: 20 }, { type: 'H', value: 55 }] }, { values: [{ type: 'T1', value: 18, name: 'probe' }] }], internalTemp: 40 });

test('available types like typeComboContent', () => {
  assert.deepEqual(availableTypes([em]), ['RSSI', 'P', 'P_SUM', 'S', 'V', 'EM'].filter((x) => x !== 'S'));
  assert.deepEqual(availableTypes([ht]), ['INT_TEMP', 'RSSI', 'T_ALL', 'H']);
});

test('series names and values', () => {
  const p = seriesFor('P', [em]);
  assert.deepEqual(p.map((s) => s.name), ['Laadpaal-net', 'Laadpaal-2', 'Laadpaal-3']); // the total is left out
  const s0 = { t: 1, rssi: -60, meters: em.meters };
  assert.equal(valueOf('P', p[1]!, s0, false), 50);
  assert.equal(valueOf('P_SUM', seriesFor('P_SUM', [em])[0]!, s0, false), 175); // EMTotalMeters wins
  const tall = seriesFor('T_ALL', [ht]);
  assert.deepEqual(tall.map((s) => s.name), ['Hall', 'Hall-probe']);
  assert.equal(valueOf('T_ALL', tall[0]!, { t: 1, rssi: 0, meters: ht.meters }, true), 68);
  assert.equal(valueOf('INT_TEMP', seriesFor('INT_TEMP', [ht])[0]!, { t: 1, rssi: 0, temp: 40 }, false), 40);
  assert.deepEqual(seriesFor('LUX', [ht]).map((s) => s.name), ['Hall']); // legend entry only
  const taken = new Set<string>();
  assert.deepEqual([uniqueName(taken, 'a'), uniqueName(taken, 'a'), uniqueName(taken, 'a')], ['a', 'a(1)', 'a(2)']);
});

test('chart CSV exports', () => {
  const t0 = new Date(2026, 8, 27, 10, 0, 0).getTime();
  const series = [{ name: 'A', points: [[t0, 1.234], [t0 + 1000, 2]] as [number, number][] }, { name: 'B', points: [[t0, 5]] as [number, number][] }];
  assert.equal(horizontalCSV(series, ',', null), `A\r\n${t0},${t0 + 1000}\r\n2026-09-27 10:00:00,2026-09-27 10:00:01\r\n1.23,2.00\r\nB\r\n${t0}\r\n2026-09-27 10:00:00\r\n5.00\r\n`);
  const v = verticalCSV(series, 'W', ';', null, { unix: 'unix time', time: 'timestamp' }).split('\r\n');
  assert.equal(v[0], 'A;;;;B');
  assert.equal(v[1], 'unix time;timestamp;W;;unix time;timestamp;W');
  assert.equal(v[3], `${t0 + 1000};2026-09-27 10:00:01;2.00;;;;`);
  assert.equal(horizontalCSV(series, ',', [t0 + 500, t0 + 2000]).split('\r\n')[1], String(t0 + 1000));
});
