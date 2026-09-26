import { test } from 'node:test';
import assert from 'node:assert/strict';
import type { Module } from '../src/api';
import {
  editIndex, eventLabel, kelvinPreset, kelvinToRgb, mixedPart, presetColor, rgbwToRgb, shownEvents, stepTarget, stepTRVG1, thermoText,
} from '../src/commandlogic';

const mod = (kind: string, extra: Partial<Module> = {}): Module => ({ kind, index: 0, label: '', ...extra });

test('event labels follow LabelsBundle, unknown events show x', () => {
  assert.equal(eventLabel('shortpush_url'), '1');
  assert.equal(eventLabel('input.button_longpush'), 'L');
  assert.equal(eventLabel('bthomedevice.rotate_left'), 'Left');
  assert.equal(eventLabel('something.else'), 'x');
});

test('disabled event buttons show only when there are at most five', () => {
  const ev = (enabled: boolean) => ({ event: 'input.button_push', enabled });
  assert.deepEqual(shownEvents(mod('input', { events: [ev(true), ev(false)] })), [0, 1]);
  const six = [ev(false), ev(true), ev(false), ev(false), ev(true), ev(false)];
  assert.deepEqual(shownEvents(mod('input', { events: six })), [1, 4]);
});

test('the lights-editor button goes on the last colour module, or white when crowded', () => {
  assert.equal(editIndex([mod('rgb'), mod('cct')]), 1);
  assert.equal(editIndex([mod('rgb'), mod('light'), mod('light')]), 2);
  assert.equal(editIndex([mod('light')]), -1); // a single dimmer: no editor button
  assert.equal(editIndex([mod('light'), mod('relay')]), -1);
  assert.equal(editIndex([mod('cct')]), 0);
});

test('mixed cells: white lights get a slider up to two modules, then one line each', () => {
  assert.equal(mixedPart(mod('light'), 2), 'white');
  assert.equal(mixedPart(mod('light'), 4), 'whiteSynthetic');
  assert.equal(mixedPart(mod('rgb'), 3), 'rgbSynthetic');
  assert.equal(mixedPart(mod('input', { enabled: false }), 4), 'none');
  assert.equal(mixedPart(mod('input'), 4), 'input');
});

test('thermostat steps follow getUnitDivision and stop at the limits', () => {
  assert.equal(stepTarget(mod('thermostat', { target: 21, div: 2, min: 5, max: 35 }), 1), 21.5);
  assert.equal(stepTarget(mod('thermostat', { target: 21, div: 10, min: 4, max: 30 }), -1), 20.9);
  assert.equal(stepTarget(mod('thermostat', { target: 35, div: 2, min: 5, max: 35 }), 1), null);
  assert.equal(stepTRVG1(mod('thermostat', { target: 30.8, min: 4, max: 31 }), 1), 31);
  assert.equal(stepTRVG1(mod('thermostat', { target: 20, min: 4, max: 31 }), -1), 19.5);
});

test('target text in °C and °F', () => {
  assert.equal(thermoText(21.5, 'C'), '21.5°C');
  assert.equal(thermoText(21, 'F'), '69.8°F');
});

test('ColorUtil colours', () => {
  assert.deepEqual(kelvinToRgb(6600), [255, 255, 255]);
  assert.deepEqual(kelvinToRgb(2700).slice(0, 1), [255]);
  assert.deepEqual(rgbwToRgb(10, 20, 30, -1), [10, 20, 30]);
  assert.deepEqual(rgbwToRgb(255, 0, 0, 255), [255, 170, 170]);
});

test('preset colours: RGBW white is the white channel', () => {
  assert.deepEqual(presetColor('rgbw', [255, 255, 255]), { rgb: [0, 0, 0], white: 255 });
  assert.deepEqual(presetColor('rgbw', [255, 0, 0]), { rgb: [255, 0, 0], white: 0 });
  assert.deepEqual(presetColor('rgb', [255, 255, 255]), { rgb: [255, 255, 255] });
  assert.equal(kelvinPreset(6000, mod('cct', { tMin: 3000, tMax: 5000 })), 5000);
});
