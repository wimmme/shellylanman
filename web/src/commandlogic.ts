// Portions derived from ShellyScanner (https://github.com/usnasoft/shellyscanner),
// Copyright (C) Antonio Flaccomio / usnasoft, licensed under GPL-3.0:
// DevicesCommandCellEditor (edit button position, thermostat steps and labels),
// the event button labels of LabelsBundle.properties and view/util/ColorUtil.
//
// The pure parts of the Command cell and the lights editor, kept free of the
// DOM so they can be tested.
import type { Module } from './api';

/** Buttons of an input's events: shown disabled only when there are at most this many (MAX_ACTIONS_SHOWN). */
export const MAX_ACTIONS_SHOWN = 5;

const EVENT_LABELS: Record<string, string> = {
  shortpush_url: '1', double_shortpush_url: '2', triple_shortpush_url: '3', longpush_url: 'L',
  btn_on_url: 'on', btn_off_url: 'off', out_on_url: 'on', out_off_url: 'off',
  shortpush_longpush_url: 'SL', longpush_shortpush_url: 'LS',
  'input.toggle_on': 'on', 'input.toggle_off': 'off', 'input.button_push': '1', 'input.button_doublepush': '2',
  'input.button_triplepush': '3', 'input.button_longpush': 'L',
  'bthomesensor.single_push': '1', 'bthomesensor.double_push': '2', 'bthomesensor.triple_push': '3', 'bthomesensor.long_push': 'L',
  'bthomedevice.single_push': '1', 'bthomedevice.double_push': '2', 'bthomedevice.triple_push': '3', 'bthomedevice.long_push': 'L',
  'bthomedevice.rotate_left': 'Left', 'bthomedevice.rotate_right': 'Right',
};

/** Label of an event button ("x" for events the original has no label for). */
export function eventLabel(event: string): string {
  return EVENT_LABELS[event] ?? 'x';
}

/** Indexes of the event buttons to draw: enabled ones, or all when there are few. */
export function shownEvents(m: Module): number[] {
  const ev = m.events ?? [];
  return ev.map((_, i) => i).filter((i) => ev[i]!.enabled || ev.length <= MAX_ACTIONS_SHOWN);
}

const isWhite = (k: string): boolean => k === 'light' || k === 'cct' || k === 'rgbcct';
const isRGB = (k: string): boolean => k === 'rgb' || k === 'rgbw' || k === 'rgbcct';
export const isLight = (k: string): boolean => isWhite(k) || isRGB(k);

/**
 * The module that carries the lights-editor button in a mixed cell: the last
 * CCT or RGB module, or white light when there are more than two modules.
 */
export function editIndex(mods: Module[]): number {
  for (let i = mods.length - 1; i >= 0; i--) {
    const k = mods[i]!.kind;
    if (k === 'cct' || isRGB(k) || (isWhite(k) && mods.length > 2)) return i;
  }
  return -1;
}

/** Per-module drawing in a mixed cell (DevicesCommandCellEditor, "mixed" branch). */
export function mixedPart(m: Module, count: number): 'relay' | 'input' | 'roller' | 'white' | 'whiteSynthetic' | 'rgbSynthetic' | 'camera' | 'sensor' | 'none' {
  switch (m.kind) {
    case 'relay': return 'relay';
    case 'input': return m.enabled === false ? 'none' : 'input';
    case 'cover': return 'roller';
    case 'camera': return 'camera';
    case 'sensor': return 'sensor';
  }
  if (isWhite(m.kind)) return count <= 2 ? 'white' : 'whiteSynthetic';
  if (isRGB(m.kind)) return 'rgbSynthetic';
  return 'none';
}

/** Thermostat steps per °C (getUnitDivision, default 2). */
export const div = (m: Module): number => m.div || 2;

/** ▲ / ▼: one step, rounded to 0.1 °C; null at the range limit. */
export function stepTarget(m: Module, dir: 1 | -1): number | null {
  const t = m.target ?? 0;
  if (dir > 0 && m.max !== undefined && t >= m.max) return null;
  if (dir < 0 && m.min !== undefined && t <= m.min) return null;
  return Math.round(10 * t + (dir * 10) / div(m)) / 10;
}

/** Gen1 TRV: ±0.5 °C clamped to 4–31 (ThermostatG1.targetTempUp/Down). */
export function stepTRVG1(m: Module, dir: 1 | -1): number {
  const t = (m.target ?? 0) + dir * 0.5;
  return Math.min(m.max ?? 31, Math.max(m.min ?? 4, t));
}

/** Target temperature text: "21.5°C" or, in Fahrenheit, rounded to 0.1 °F. */
export function thermoText(c: number, unit: 'C' | 'F'): string {
  if (unit === 'F') return `${(Math.round(c * 18 + 320) / 10).toFixed(1)}°F`;
  return `${(Math.round(c * 10) / 10).toFixed(1)}°C`;
}

/** ColorUtil.kelvinToColor: an RGB approximation of a colour temperature. */
export function kelvinToRgb(kelvin: number): [number, number, number] {
  const k = Math.min(40000, Math.max(1000, kelvin)) / 100;
  const clamp = (v: number): number => Math.min(255, Math.max(0, v));
  const r = k <= 66 ? 255 : clamp(329.698727446 * Math.pow(k - 60, -0.1332047592));
  const g = k <= 66 ? clamp(99.4708025861 * Math.log(k) - 161.1195681661) : clamp(288.1221695283 * Math.pow(k - 60, -0.0755148492));
  const b = k >= 66 ? 255 : k <= 19 ? 0 : clamp(138.5177312231 * Math.log(k - 10) - 305.0447927307);
  return [Math.trunc(r), Math.trunc(g), Math.trunc(b)];
}

/** ColorUtil.rgbwColor: white adds 2× to each channel, scaled back into 0–255. */
export function rgbwToRgb(r: number, g: number, b: number, w: number): [number, number, number] {
  if (w < 0) return [r, g, b];
  let rr = r + w * 2, gg = g + w * 2, bb = b + w * 2;
  const max = Math.max(rr, gg, bb);
  if (max > 255) {
    rr = Math.trunc((rr * 255) / max); gg = Math.trunc((gg * 255) / max); bb = Math.trunc((bb * 255) / max);
  }
  return [rr, gg, bb];
}

export const cssRGB = ([r, g, b]: [number, number, number]): string => `rgb(${r}, ${g}, ${b})`;

/** Preset colour buttons of the RGB panel: red, green, yellow, blue, violet, white. */
export const PRESETS: [number, number, number][] = [[255, 0, 0], [0, 255, 0], [255, 255, 0], [0, 0, 255], [255, 0, 255], [255, 255, 255]];

/**
 * The colour command of a preset: RGBW lights get (r,g,b,white 0), and the
 * white preset is pure white channel (0,0,0,255); RGB lights get (r,g,b).
 */
export function presetColor(kind: string, p: [number, number, number]): { rgb: number[]; white?: number } {
  const white = p[0] === 255 && p[1] === 255 && p[2] === 255;
  if (kind === 'rgbw') return white ? { rgb: [0, 0, 0], white: 255 } : { rgb: [...p], white: 0 };
  return { rgb: [...p] };
}

/** CCT preset buttons (3000, 4500, 6000 K), clamped to the light's range. */
export function kelvinPreset(k: number, m: Module): number {
  return Math.min(m.tMax ?? 6500, Math.max(m.tMin ?? 2700, k));
}
