import { test } from 'node:test';
import assert from 'node:assert/strict';
import { APPEAR_DEFAULT, BRIGHT_FACTORS, CONTRAST_FACTORS, SLIDER_MAX, factor, parseColor, scaleBright } from '../src/appearance';
import { backoff } from '../src/socket';

test('neutral level is factor 1', () => {
  assert.equal(factor(CONTRAST_FACTORS, APPEAR_DEFAULT), 1);
  assert.equal(factor(BRIGHT_FACTORS, APPEAR_DEFAULT), 1);
});

test('factor tables cover every slider position and clamp', () => {
  assert.equal(CONTRAST_FACTORS.length, SLIDER_MAX);
  assert.equal(BRIGHT_FACTORS.length, SLIDER_MAX);
  assert.equal(factor(BRIGHT_FACTORS, 0), BRIGHT_FACTORS[0]);
  assert.equal(factor(BRIGHT_FACTORS, 99), BRIGHT_FACTORS[SLIDER_MAX - 1]);
});

test('scaleBright: brighten interpolates to white, darken multiplies, alpha kept', () => {
  assert.deepEqual(scaleBright([255, 0, 100, 0.5], 1.5), [255, 128, 178, 0.5]);
  assert.deepEqual(scaleBright([200, 100, 50, 1], 0.5), [100, 50, 25, 1]);
  assert.deepEqual(scaleBright([10, 20, 30, 0.9], 1), [10, 20, 30, 0.9]);
});

test('parseColor handles stylesheet forms', () => {
  assert.deepEqual(parseColor('#07090f'), [7, 9, 15, 1]);
  assert.deepEqual(parseColor('#fff'), [255, 255, 255, 1]);
  assert.deepEqual(parseColor(' rgba(13,18,30,.85) '), [13, 18, 30, 0.85]);
  assert.deepEqual(parseColor('rgb(1, 2, 3)'), [1, 2, 3, 1]);
  assert.equal(parseColor('var(--x)'), null);
});

test('websocket backoff grows and caps at 30 s', () => {
  assert.equal(backoff(0), 1000);
  assert.equal(backoff(3), 8000);
  assert.equal(backoff(50), 30000);
});
