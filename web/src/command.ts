// Portions derived from ShellyScanner (https://github.com/usnasoft/shellyscanner),
// Copyright (C) Antonio Flaccomio / usnasoft, licensed under GPL-3.0:
// view/DevicesCommandCellEditor and DevicesCommandCellRenderer.
//
// The "Command" cell: the controls of a device's modules, drawn per layout
// (the array type the Java model returns from getModules()).
import { devicesApi, type Command, type Device, type Module } from './api';
import { editIndex, mixedPart, shownEvents, eventLabel, div, stepTarget, stepTRVG1, thermoText } from './commandlogic';
import { h } from './dom';
import { moduleText, prefs } from './format';
import { t } from './i18n';
import { confirmDialog } from './modal';
import { openLights } from './panels/lights';
import { toast } from './toast';

// ---- keep the table still while a slider is dragged --------------------------------

let active = 0;
let pending: (() => void) | null = null;

/** True while the user drags a slider in a Command cell or the lights editor. */
export function interacting(): boolean { return active > 0; }

/** Run fn now, or once the current interaction ends. */
export function whenIdle(fn: () => void): void {
  if (active > 0) pending = fn;
  else fn();
}

function flush(): void {
  if (active === 0 && pending) { const p = pending; pending = null; p(); }
}

/** Mark a slider: device updates wait until it is released. */
export function holdWhileDragging(el: HTMLElement): void {
  el.addEventListener('pointerdown', () => {
    active++;
    const up = (): void => {
      active = Math.max(0, active - 1);
      window.removeEventListener('pointerup', up);
      window.removeEventListener('pointercancel', up);
      setTimeout(flush, 50); // after the slider's change event
    };
    window.addEventListener('pointerup', up);
    window.addEventListener('pointercancel', up);
  });
}

// ---- sending ------------------------------------------------------------------------

export async function send(d: Device, cmd: Command): Promise<boolean> {
  try {
    await devicesApi.command(d.id, cmd);
    return true;
  } catch (e) {
    toast(t('cmd.error', { device: d.name || d.hostname, err: e instanceof Error ? e.message : String(e) }));
    return false;
  }
}

// ---- building blocks ----------------------------------------------------------------

/** The ON/OFF button; its text is highlighted while the associated input is on. */
export function onOffButton(on: boolean, inputOn: boolean | undefined, onClick: () => void, label?: string): HTMLButtonElement {
  return h('button', { class: 'cmd-onoff ' + (on ? 'on' : 'off') + (inputOn ? ' input-on' : ''), 'aria-pressed': String(on),
    'aria-label': label ? `${label}: ${t(on ? 'cmd.on' : 'cmd.off')}` : undefined, onclick: onClick }, t(on ? 'cmd.on' : 'cmd.off'));
}

/** A slider that reports every move (label) and sends on release (commit). */
export function slider(min: number, max: number, value: number, step: number, label: string,
  onMove: (v: number) => void, onCommit: (v: number) => void): HTMLInputElement {
  const s = h('input', { type: 'range', class: 'cmd-slider', min, max, step, value, 'aria-label': label });
  s.value = String(value);
  s.addEventListener('input', () => onMove(Number(s.value)));
  s.addEventListener('change', () => onCommit(Number(s.value)));
  holdWhileDragging(s);
  return s;
}

const iconBtn = (glyph: string, label: string, onClick: () => void, cls = ''): HTMLButtonElement =>
  h('button', { class: 'cmd-icon ' + cls, 'aria-label': label, title: label, onclick: onClick }, glyph);

function row(label: Node | string, ...controls: (Node | null)[]): HTMLElement {
  return h('div', { class: 'cmd-row' }, h('span', { class: 'cmd-label' }, label), h('span', { class: 'cmd-ctl' }, ...controls));
}

function editButton(d: Device): HTMLButtonElement {
  return iconBtn('✎', t('cmd.edit'), () => openLights(d.id), 'cmd-edit');
}

// ---- panels (one per module kind) ------------------------------------------------------

function relayPanel(d: Device, m: Module): HTMLElement {
  return row(m.label, onOffButton(!!m.on, m.inputOn, () => void send(d, { key: m.key!, action: 'toggle' }), m.label));
}

function rollerPanel(d: Device, m: Module): HTMLElement {
  const label = h('span', {}, m.calibrated ? `${m.label} ${m.position ?? 0}%` : m.label);
  const go = (action: string) => (): void => void send(d, { key: m.key!, action });
  const box = h('div', { class: 'cmd-block' }, row(label,
    iconBtn('▲', t('cmd.open'), go('open'), m.inputOn ? 'input-on' : ''),
    iconBtn('■', t('cmd.stop'), go('stop')),
    iconBtn('▼', t('cmd.close'), go('close'), m.inputOn1 ? 'input-on' : '')));
  if (m.calibrated) {
    box.append(slider(0, 100, m.position ?? 0, 1, t('cmd.position'),
      (v) => { label.textContent = `${m.label} ${v}%`; },
      (v) => void send(d, { key: m.key!, action: 'position', value: v })));
  }
  return box;
}

/** White light with slider (DevicesCommandCellEditor.getWhitePanel); RGBCCT uses gain in colour mode. */
function lightPanel(d: Device, m: Module, withEdit: boolean, action: 'brightness' | 'gain', value: number, min: number, max: number): HTMLElement {
  const label = h('span', {}, `${m.label} ${value}%`);
  return h('div', { class: 'cmd-block' },
    row(label, withEdit ? editButton(d) : null, onOffButton(!!m.on, m.inputOn, () => void send(d, { key: m.key!, action: 'toggle' }), m.label)),
    slider(min, max, value, 1, t(action === 'gain' ? 'cmd.gain' : 'cmd.brightness'),
      (v) => { label.textContent = `${m.label} ${v}%`; },
      (v) => void send(d, { key: m.key!, action, value: v })));
}

/** One line: label with value and ON/OFF (the "synthetic" panels of crowded cells). */
function syntheticPanel(d: Device, m: Module, value: number, withEdit: boolean): HTMLElement {
  return row(`${m.label} ${value}%`, withEdit ? editButton(d) : null,
    onOffButton(!!m.on, m.inputOn, () => void send(d, { key: m.key!, action: 'toggle' }), m.label));
}

function rgbwPanel(d: Device, m: Module): HTMLElement {
  const gain = m.gain ?? 0;
  const label = h('span', {}, `${m.label} ${gain}%`);
  return h('div', { class: 'cmd-block' },
    row(label, editButton(d), onOffButton(!!m.on, m.inputOn, () => void send(d, { key: m.key!, action: 'toggle' }), m.label)),
    slider(0, 100, gain, 1, t('cmd.gain'), (v) => { label.textContent = `${m.label} ${v}%`; },
      (v) => void send(d, { key: m.key!, action: 'gain', value: v })),
    slider(0, 255, m.white ?? 0, 1, t('lights.white'), () => {}, (v) => void send(d, { key: m.key!, action: 'white', value: v })));
}

function inputPanel(d: Device, m: Module): HTMLElement {
  const label = h('span', { class: m.events?.length ? '' : m.inputOn ? 'input-on' : '' }, m.label || '○');
  const buttons = shownEvents(m).map((i) => {
    const ev = m.events![i]!;
    return h('button', { class: 'cmd-event' + (m.inputOn ? ' input-on' : ''), disabled: !ev.enabled, title: ev.event,
      onclick: () => void send(d, { key: m.key!, action: 'event', event: i }) }, eventLabel(ev.event));
  });
  return row(label, ...buttons);
}

function cameraPanel(d: Device, m: Module): HTMLElement {
  const privacy = !!m.on;
  return row(t(m.motion ? 'cmd.motionYes' : 'cmd.motionNo'),
    iconBtn(privacy ? '🔒' : '🔓', t(privacy ? 'cmd.privacyOn' : 'cmd.privacyOff'),
      () => void send(d, { key: m.key!, action: 'privacy', value: privacy ? 0 : 1 })));
}

function breakerPanel(d: Device, m: Module): HTMLElement {
  const btn = onOffButton(!!m.on, false, async () => {
    if (!(await confirmDialog(t('cmd.toggle'), t('cmd.cbConfirm'), t('cmd.toggle'), false))) return;
    void send(d, { key: m.key!, action: 'toggle', confirm: true });
  }, m.label);
  if (m.locked) { btn.disabled = true; btn.title = t('cmd.cbLocked'); }
  return row(m.label, btn);
}

/** ThermostatInterface panel: target, ON/OFF (enable), ▲ ▼ and a slider in steps of 1/div °C. */
function thermostatPanel(d: Device, m: Module): HTMLElement {
  const unit = prefs.temp();
  const dv = div(m);
  const target = m.target ?? 0;
  const label = h('span', { class: m.enabled ? '' : 'muted' }, thermoText(target, unit));
  const up = stepTarget(m, 1), down = stepTarget(m, -1);
  const box = h('div', { class: 'cmd-block' }, row(label,
    onOffButton(!!m.enabled, m.enabled && m.running, () => void send(d, { key: m.key!, action: 'enable', value: m.enabled ? 0 : 1 }), t('cmd.thermostat')),
    iconBtn('▲', t('cmd.up'), () => { if (up !== null) void send(d, { key: m.key!, action: 'target', value: up }); }),
    iconBtn('▼', t('cmd.down'), () => { if (down !== null) void send(d, { key: m.key!, action: 'target', value: down }); })));
  box.append(slider(Math.round((m.min ?? 5) * dv), Math.round((m.max ?? 35) * dv), Math.round(target * dv), 1, t('cmd.target'),
    (v) => { label.textContent = thermoText(v / dv, unit); },
    (v) => void send(d, { key: m.key!, action: 'target', value: v / dv })));
  return box;
}

/** Gen1 TRV: current profile and target, ▲ ▼ ±0.5 °C, slider 4–31 °C (always °C, as the original). */
function trvG1Panel(d: Device, m: Module): HTMLElement {
  if (!m.enabled) return h('div', {}, t('cmd.trvPosition', { pos: (m.position ?? 0).toFixed(1) }));
  const target = m.target ?? 0;
  const label = h('span', { class: m.schedule ? '' : 'muted' }, `${m.label} ${target.toFixed(1)}°C`);
  const set = (v: number): void => void send(d, { key: m.key!, action: 'target', value: v });
  const box = h('div', { class: 'cmd-block' }, row(label,
    iconBtn('▲', t('cmd.up'), () => set(stepTRVG1(m, 1))),
    iconBtn('▼', t('cmd.down'), () => set(stepTRVG1(m, -1)))));
  box.append(slider((m.min ?? 4) * 2, (m.max ?? 31) * 2, Math.round(target * 2), 1, t('cmd.target'),
    (v) => { label.textContent = `${m.label} ${(v / 2).toFixed(1)}°C`; }, (v) => set(v / 2)));
  return box;
}

// ---- the cell -------------------------------------------------------------------------

/** The Command cell of a device, or null when it has nothing to show. */
export function commandCell(d: Device): HTMLElement | null {
  const mods = d.modules ?? [];
  if (mods.length === 0) return null;
  const parts: (HTMLElement | null)[] = [];
  const first = mods[0]!;
  switch (d.layout) {
    case 'relay': parts.push(...mods.map((m) => relayPanel(d, m))); break;
    case 'roller': parts.push(...mods.map((m) => rollerPanel(d, m))); break;
    case 'rgbcct': {
      const color = !!first.colorMode;
      parts.push(lightPanel(d, first, true, color ? 'gain' : 'brightness', (color ? first.gain : first.brightness) ?? 0, first.min ?? 0, first.max ?? 100));
      break;
    }
    case 'rgbw': parts.push(rgbwPanel(d, first)); break;
    case 'rgb': parts.push(lightPanel(d, first, true, 'gain', first.gain ?? 0, 0, 100)); break;
    case 'thermostat': parts.push(thermostatPanel(d, first)); break;
    case 'trvg1': parts.push(trvG1Panel(d, first)); break;
    case 'cb': parts.push(breakerPanel(d, first)); break;
    default: {
      const edit = editIndex(mods);
      mods.forEach((m, i) => {
        switch (mixedPart(m, mods.length)) {
          case 'relay': parts.push(relayPanel(d, m)); break;
          case 'input': parts.push(inputPanel(d, m)); break;
          case 'roller': parts.push(rollerPanel(d, m)); break;
          case 'white': parts.push(lightPanel(d, m, i === edit, 'brightness', m.brightness ?? 0, m.min ?? 0, m.max ?? 100)); break;
          case 'whiteSynthetic': parts.push(syntheticPanel(d, m, m.brightness ?? 0, i === edit)); break;
          case 'rgbSynthetic': parts.push(syntheticPanel(d, m, m.gain ?? 0, i === edit)); break;
          case 'camera': parts.push(cameraPanel(d, m)); break;
          case 'sensor': parts.push(h('div', {}, moduleText(m))); break;
        }
      });
    }
  }
  const cell = h('div', { class: 'cmd' }, ...parts.filter((p): p is HTMLElement => p !== null));
  // Clicking a control must not change the selection or open the info panel.
  cell.addEventListener('click', (e) => e.stopPropagation());
  cell.addEventListener('dblclick', (e) => e.stopPropagation());
  return cell.childElementCount ? cell : null;
}
