import { test } from 'node:test';
import assert from 'node:assert/strict';
import { formatDuration, formatMeter, formatUptime, meterSetText, metersText, moduleText } from '../src/format';

test('meter values follow METER_VAL_* formats', () => {
  assert.equal(formatMeter('W', 12.345, 'C'), '12.35W');
  assert.equal(formatMeter('V', 229.87, 'C'), '229.9V');
  assert.equal(formatMeter('I', 0.5, 'C'), '0.50A');
  assert.equal(formatMeter('PF', 0.97, 'C'), '0.97');
  assert.equal(formatMeter('FREQ', 49.98, 'C'), '50.0Hz');
  assert.equal(formatMeter('T', 21.44, 'C'), '21.4°C');
  assert.equal(formatMeter('T1', 20, 'F'), '68.0°F');
  assert.equal(formatMeter('H', 55.6, 'C'), '56%');
  assert.equal(formatMeter('EX', 1, 'C'), 'closed');
  assert.equal(formatMeter('LE', 2, 'C'), 'bright');
  assert.equal(formatMeter('BAT', 88, 'C'), '88%');
});

test('meter sets read like the ShellyScanner cell', () => {
  const s = { label: 'Oven', values: [{ type: 'W', value: 100 }, { type: 'V', value: 230 }] };
  assert.equal(meterSetText(s, 'C'), 'Oven P 100.00W V 230.0V');
  assert.equal(metersText([s, { values: [{ type: 'T', value: 20, name: 'Water' }] }], 'C'), 'Oven P 100.00W V 230.0V + T (Water) 20.0°C');
});

test('uptime formats', () => {
  assert.equal(formatUptime(90061, 'SEC'), '90061');
  assert.equal(formatUptime(90061, 'DAY'), '1 days, 1 hours, 1 min, 1 sec');
  const now = new Date(2026, 8, 26, 12, 0, 0).getTime();
  assert.equal(formatUptime(3600, 'FROM', now), '26/09/2026 11:00');
  assert.equal(formatUptime(-1, 'SEC'), '');
});

test('module text for the Command column', () => {
  assert.equal(moduleText({ kind: 'relay', index: 0, label: 'Pump', on: true }, 'C'), 'Pump: ON');
  assert.equal(moduleText({ kind: 'light', index: 0, label: 'Hall', on: true, brightness: 40 }, 'C'), 'Hall: ON 40%');
  assert.equal(moduleText({ kind: 'cover', index: 0, label: 'Blind', calibrated: false }, 'C'), 'Blind: n.c.');
  assert.equal(moduleText({ kind: 'input', index: 1, label: '', inputOn: true }, 'C'), 'In 1 ●');
});

test('durations show their two largest units', () => {
  assert.equal(formatDuration(59), '0 min');
  assert.equal(formatDuration(3 * 3600 + 5 * 60), '3 h 5 min');
  assert.equal(formatDuration(2 * 86400 + 14 * 3600 + 59), '2 d 14 h');
});
