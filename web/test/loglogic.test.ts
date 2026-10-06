import { test } from 'node:test';
import assert from 'node:assert/strict';
import { clock, copyText, lineText, matches, merge, stamp, type LogEntry, type LogFilter } from '../src/loglogic';

const e = (seq: number, level = 'INFO', msg = 'm', attrs?: string): LogEntry => ({ seq, time: new Date(2026, 9, 6, 14, 5, 9).getTime(), level, msg, attrs });
const all: LogFilter = { level: 'info', text: '', after: 0 };

test('merge: only newer lines, in order, no doubles', () => {
  const a = merge([], [e(1), e(2), e(3)]);
  assert.deepEqual(a.map((x) => x.seq), [1, 2, 3]);
  const b = merge(a, [e(2), e(3), e(4)]); // the list and the socket overlap
  assert.deepEqual(b.map((x) => x.seq), [1, 2, 3, 4]);
  const c = merge(b, [e(6), e(5)]); // out of order
  assert.deepEqual(c.map((x) => x.seq), [1, 2, 3, 4, 5, 6]);
});

test('merge: keeps the last lines', () => {
  const list = merge([], [1, 2, 3, 4, 5].map((n) => e(n)), 3);
  assert.deepEqual(list.map((x) => x.seq), [3, 4, 5]);
  assert.deepEqual(merge(list, [e(6)], 3).map((x) => x.seq), [4, 5, 6]);
});

test('level: info shows everything, warn hides info, error shows errors only', () => {
  const lines = [e(1, 'INFO'), e(2, 'WARN'), e(3, 'ERROR')];
  const seqs = (level: 'info' | 'warn' | 'error'): number[] => lines.filter((x) => matches(x, { ...all, level })).map((x) => x.seq);
  assert.deepEqual(seqs('info'), [1, 2, 3]);
  assert.deepEqual(seqs('warn'), [2, 3]);
  assert.deepEqual(seqs('error'), [3]);
});

test('search: message, level and attributes, any case', () => {
  const x = e(1, 'WARN', 'Home Assistant discovery', 'service=shellylanman err="connection refused"');
  assert.ok(matches(x, { ...all, text: 'discovery' }));
  assert.ok(matches(x, { ...all, text: 'REFUSED' }));
  assert.ok(matches(x, { ...all, text: 'warn' }));
  assert.ok(!matches(x, { ...all, text: 'grondwaterpomp' }));
  assert.ok(matches(x, { ...all, text: '  ' }));
});

test('clear hides what was there, shows what comes after', () => {
  assert.ok(!matches(e(5), { ...all, after: 5 }));
  assert.ok(matches(e(6), { ...all, after: 5 }));
});

test('time, line and copy text', () => {
  const x = e(1, 'INFO', 'starting', 'version=0.9.3');
  assert.equal(clock(x.time), '14:05:09');
  assert.equal(stamp(x.time), '2026-10-06 14:05:09');
  assert.equal(lineText(x), 'INFO starting version=0.9.3');
  assert.equal(lineText(e(2)), 'INFO m');
  assert.equal(copyText([x, e(2, 'WARN', 'careful')], { ...all, level: 'warn' }), '2026-10-06 14:05:09 WARN careful');
});
