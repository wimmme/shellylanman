// Portions derived from ShellyScanner (https://github.com/usnasoft/shellyscanner),
// Copyright (C) Antonio Flaccomio / usnasoft, licensed under GPL-3.0:
// view/lightsEditor (DialogEditLights, WhitePanel, CCTPanel, RGBPanel, RGBCCTPanel).
//
// The lights editor: one panel per light of the device, with switch,
// brightness/gain, colour, white and colour temperature.
import type { Device, Module } from '../api';
import { interacting, onOffButton, send, slider } from '../command';
import { cssRGB, isLight, kelvinPreset, kelvinToRgb, presetColor, PRESETS, rgbwToRgb } from '../commandlogic';
import { allDevices, onDevicesChanged } from '../devices';
import { h } from '../dom';
import { t } from '../i18n';
import { openModal } from '../modal';

const PRESET_NAMES = ['lights.red', 'lights.green', 'lights.yellow', 'lights.blue', 'lights.violet', 'lights.white'] as const;

function labelled(text: HTMLElement, control: HTMLElement): HTMLElement {
  return h('div', { class: 'light-field' }, text, control);
}

function preview(color: string): HTMLElement {
  const p = h('div', { class: 'light-preview', 'aria-hidden': 'true' });
  p.style.background = color;
  return p;
}

function head(d: Device, m: Module, text: HTMLElement): HTMLElement {
  return h('div', { class: 'light-head' }, text, onOffButton(!!m.on, m.inputOn, () => void send(d, { key: m.key!, action: 'toggle' }), m.label));
}

/** WhitePanel: switch and brightness. */
function whitePanel(d: Device, m: Module): HTMLElement {
  const label = h('span', {}, `${m.label} ${m.brightness ?? 0}%`);
  return h('div', { class: 'light-panel' }, head(d, m, label),
    slider(m.min ?? 0, m.max ?? 100, m.brightness ?? 0, 1, t('cmd.brightness'), (v) => { label.textContent = `${m.label} ${v}%`; },
      (v) => void send(d, { key: m.key!, action: 'brightness', value: v })));
}

/** CCTPanel: switch, brightness, colour temperature with 3000/4500/6000 K buttons and a preview. */
function cctPanel(d: Device, m: Module, withHead = true): HTMLElement {
  const label = h('span', {}, `${m.label} ${m.brightness ?? 0}%`);
  const k = m.tempK ?? m.tMin ?? 2700;
  const kLabel = h('span', {}, `${t('lights.temperature')}: ${k}K`);
  const pv = preview(cssRGB(kelvinToRgb(k)));
  const setK = (v: number): void => void send(d, { key: m.key!, action: 'temp', value: v });
  return h('div', { class: 'light-panel' },
    withHead ? head(d, m, label) : label,
    slider(m.min ?? 0, m.max ?? 100, m.brightness ?? 0, 1, t('cmd.brightness'), (v) => { label.textContent = `${m.label} ${v}%`; },
      (v) => void send(d, { key: m.key!, action: 'brightness', value: v })),
    labelled(kLabel, slider(m.tMin ?? 2700, m.tMax ?? 6500, k, 1, t('lights.temperature'),
      (v) => { kLabel.textContent = `${t('lights.temperature')}: ${v}K`; pv.style.background = cssRGB(kelvinToRgb(v)); }, setK)),
    h('div', { class: 'row' }, ...[3000, 4500, 6000].map((p) => h('button', { class: 'btn', onclick: () => setK(kelvinPreset(p, m)) }, `${p}K`))),
    pv);
}

/** RGBPanel: switch, gain, red/green/blue (and white for RGBW), preset colours and a preview. */
function rgbPanel(d: Device, m: Module, withHead = true): HTMLElement {
  const gain = m.gain ?? 0;
  const [r0, g0, b0] = m.rgb ?? [0, 0, 0];
  const rgbw = m.kind === 'rgbw';
  const label = h('span', {}, `${m.label} ${gain}%`);
  const cur = { r: r0 ?? 0, g: g0 ?? 0, b: b0 ?? 0, w: rgbw ? m.white ?? 0 : -1 };
  const pv = preview(cssRGB(rgbwToRgb(cur.r, cur.g, cur.b, cur.w)));
  const upd = (): void => { pv.style.background = cssRGB(rgbwToRgb(cur.r, cur.g, cur.b, cur.w)); };
  const channel = (name: 'lights.red' | 'lights.green' | 'lights.blue', k: 'r' | 'g' | 'b'): HTMLElement => {
    const l = h('span', {}, `${t(name)}: ${cur[k]}`);
    return labelled(l, slider(0, 255, cur[k], 1, t(name), (v) => { cur[k] = v; l.textContent = `${t(name)}: ${v}`; upd(); },
      () => void send(d, { key: m.key!, action: 'color', rgb: [cur.r, cur.g, cur.b] })));
  };
  const panel = h('div', { class: 'light-panel' },
    withHead ? head(d, m, label) : label,
    slider(0, 100, gain, 1, t('cmd.gain'), (v) => { label.textContent = `${m.label} ${v}%`; },
      (v) => void send(d, { key: m.key!, action: 'gain', value: v })),
    channel('lights.red', 'r'), channel('lights.green', 'g'), channel('lights.blue', 'b'));
  if (rgbw) {
    const wl = h('span', {}, `${t('lights.white')}: ${cur.w}`);
    panel.append(labelled(wl, slider(0, 255, cur.w, 1, t('lights.white'), (v) => { cur.w = v; wl.textContent = `${t('lights.white')}: ${v}`; upd(); },
      (v) => void send(d, { key: m.key!, action: 'white', value: v }))));
  }
  panel.append(h('div', { class: 'row presets' }, ...PRESETS.map((p, i) => {
    const b = h('button', { class: 'preset', 'aria-label': t(PRESET_NAMES[i]!), title: t(PRESET_NAMES[i]!),
      onclick: () => void send(d, { key: m.key!, action: 'color', ...presetColor(m.kind, p) }) });
    b.style.background = cssRGB(p);
    return b;
  })), pv);
  return panel;
}

/** RGBCCTPanel: white / colour mode and the matching panel. */
function rgbcctPanel(d: Device, m: Module): HTMLElement {
  const color = !!m.colorMode;
  const name = `mode-${m.key}`;
  const radio = (value: 0 | 1, text: string): HTMLElement => h('label', { class: 'row' },
    h('input', { type: 'radio', name, checked: color === (value === 1),
      onchange: () => void send(d, { key: m.key!, action: 'mode', value }) }), text);
  const sub = color ? rgbPanel(d, { ...m, kind: 'rgb' }, false) : cctPanel(d, m, false);
  return h('div', { class: 'light-panel' },
    h('div', { class: 'light-head' }, h('div', { class: 'row light-mode' }, radio(0, t('lights.white')), radio(1, t('lights.color'))),
      onOffButton(!!m.on, m.inputOn, () => void send(d, { key: m.key!, action: 'toggle' }), m.label)),
    sub);
}

function panelFor(d: Device, m: Module): HTMLElement | null {
  switch (m.kind) {
    case 'rgbcct': return rgbcctPanel(d, m);
    case 'rgb': case 'rgbw': return rgbPanel(d, m);
    case 'cct': return cctPanel(d, m);
    case 'light': return whitePanel(d, m);
  }
  return null;
}

/** Open the lights editor of a device (DialogEditLights). */
export function openLights(id: string): void {
  const find = (): Device | undefined => allDevices().find((x) => x.id === id);
  const d0 = find();
  if (!d0) return;
  const lights0 = (d0.modules ?? []).filter((m) => isLight(m.kind));
  const body = h('div', { class: 'lights' });
  const draw = (): void => {
    const d = find();
    if (!d) return;
    const lights = (d.modules ?? []).filter((m) => isLight(m.kind));
    const parts: HTMLElement[] = [];
    if (lights.length > 1) { // "All channels" off / on, one after the other
      const all = async (on: boolean): Promise<void> => {
        for (const m of lights) await send(d, { key: m.key!, action: on ? 'on' : 'off' });
      };
      parts.push(h('div', { class: 'light-all' }, h('span', {}, t('lights.all')),
        h('button', { class: 'btn', onclick: () => void all(false) }, t('cmd.off')),
        h('button', { class: 'btn', onclick: () => void all(true) }, t('cmd.on'))));
    }
    for (const m of lights) { const p = panelFor(d, m); if (p) parts.push(p); }
    body.replaceChildren(...parts);
  };
  draw();
  // Follow the device while the dialog is open, but not in the middle of a drag.
  const off = onDevicesChanged(() => { if (!interacting()) draw(); });
  openModal(lights0.length === 1 ? lights0[0]!.label || d0.name || d0.hostname : t('lights.title'), body, [{ label: t('common.close') }], off);
}
