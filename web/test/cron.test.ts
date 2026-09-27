import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  canEditParams, daysOfWeekAsString, expression, fragStrToNum, fragmentToInt, hmCompare, invalidFields, isValid, listAsCronString,
  parseCron, parseThCron, replaceParam, setMode, thExpression, thExtTimespec, valuesToField, CRON_TH_WD, DEF_CRON,
} from '../src/cronlogic';

test('CronUtils helpers', () => {
  assert.deepEqual(fragmentToInt('1-3,5'), [1, 2, 3, 5]);
  assert.equal(fragStrToNum('0 0 22 * DEC FRI'), '0 0 22 * 12 5');
  assert.equal(fragStrToNum('@sunset+1h 1 * MON'), '@sunset+1h 1 * 1'); // "sun" in @sunset is kept
  assert.equal(daysOfWeekAsString('1-3'), 'MON,TUE,WED');
  assert.equal(daysOfWeekAsString('*'), 'SUN,MON,TUE,WED,THU,FRI,SAT');
  assert.equal(listAsCronString([1, 2, 3, 5, 8, 9]), '5,8,9,1-3');
  assert.equal(hmCompare('0 30 7 * * *', '0 0 8 * * *'), -1);
  assert.equal(valuesToField([], 0, 24), '*');
  assert.equal(valuesToField([3, 1, 2], 0, 24), '1-3');
});

test('patterns', () => {
  for (const ok of ['0 0 * * * *', '0 0 22 * * 5', '*/10 * * * * *', '0 30 7 1-15 1,6 1,2,3,4,5', '@sunset', '@sunrise-1h30m * * *', '@sunset+2h 1 * 0']) assert.equal(isValid(ok), true, ok);
  for (const bad of ['0 0 24 * * *', '0 60 * * * *', '* * * * *', '@sunset+25h', '0 0 * 0 * *']) assert.equal(isValid(bad), false, bad);
  assert.equal(CRON_TH_WD.test('* 30 7 * * 1,2,3'), true);
  assert.equal(CRON_TH_WD.test('0 30 7 * * 1'), false);
});

test('editor state round trip (setCron / generateExpression)', () => {
  const c = parseCron('0 30 7 * * 1,2,3,4,5');
  assert.equal(c.mode, 'time');
  assert.deepEqual(c.wdays, [true, true, true, true, true, false, false]);
  assert.equal(c.months.every(Boolean), true);
  assert.equal(expression(c), '0 30 7 * * 1,2,3,4,5');
  const all = parseCron('0 0 * * * *');
  assert.equal(expression(all), DEF_CRON);
  const sun = parseCron('@sunrise-1h30m * * 0,6');
  assert.equal(sun.mode, 'beforeRise');
  assert.equal(expression(sun), '@sunrise-1h30m * * 0,6');
  assert.equal(expression(parseCron('@sunset')), '@sunset+0h0m * * *');
  const raw = parseCron('0 0 12 * */2 *');
  assert.equal(raw.monthsRaw, '*/2');
  assert.equal(expression(raw), '0 0 12 * */2 *');
  const toSun = setMode(parseCron('*/5 */10 */2 * * *'), 'afterSet');
  assert.equal(expression(toSun), '@sunset+0h0m * * *');
  assert.deepEqual([...invalidFields({ ...c, hours: '25' })], ['hours']);
  const mar = { ...c, months: c.months.map((_, i) => i === 2 || i === 3 || i === 4 || i === 8) };
  assert.equal(expression(mar), '0 30 7 * 9,3-5 1,2,3,4,5');
});

test('Wall Display thermostat rules', () => {
  const r = parseThCron('* 15 6 * * MON,TUE');
  assert.deepEqual([r.h, r.m], [6, 15]);
  assert.equal(thExpression(r.h, r.m, r.wdays), '* 15 6 * * 1,2');
  assert.equal(thExtTimespec(r.h, r.m, r.wdays), '* 15 6 * * MON,TUE');
  assert.equal(thExpression(8, 0, Array(7).fill(false)), '* 0 8 * * 0,1,2,3,4,5,6');
});

test('parameter editors', () => {
  assert.equal(canEditParams('"id":0,"on":true'), true);
  assert.equal(canEditParams('"id":0'), false);
  assert.equal(replaceParam('"id":0,"on":true,"brightness":50', 'brightness', '80'), '"id":0,"on":true,"brightness":80');
  assert.equal(replaceParam('"id":0,"rgb":[255,0,0]', 'rgb', '[1,2,3]'), '"id":0,"rgb":[1,2,3]');
});
