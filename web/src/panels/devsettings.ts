// Portions derived from ShellyScanner (https://github.com/usnasoft/shellyscanner),
// Copyright (C) Antonio Flaccomio / usnasoft, licensed under GPL-3.0:
// view/devsettings (DialogDeviceSettings, PanelWIFI, PanelResLogin,
// PanelMQTTG1/G2/Mix, PanelOthers) and view/DialogDeviceSelection.
//
// "Devices settings": one tab per section, applied to all selected devices,
// with a result line per device.
import { ApiError, configApi, type ConfigForm, type ResultLine } from '../api';
import { allDevices } from '../devices';
import { h } from '../dom';
import { t, type Key } from '../i18n';
import { confirmDialog, openModal } from '../modal';

export type SettingsTab = 'wifi1' | 'wifi2' | 'login' | 'mqtt' | 'others';
const TABS: { id: SettingsTab; label: Key }[] = [
  { id: 'wifi1', label: 'cfg.tab.wifi1' }, { id: 'wifi2', label: 'cfg.tab.wifi2' }, { id: 'login', label: 'cfg.tab.login' },
  { id: 'mqtt', label: 'cfg.tab.mqtt' }, { id: 'others', label: 'cfg.tab.others' },
];

const IPV4 = /^((0|1\d?\d?|2[0-4]?\d?|25[0-5]?|[3-9]\d?)\.){3}(0|1\d?\d?|2[0-4]?\d?|25[0-5]?|[3-9]\d?)$/;

/** A server validation error "invalid: key" in the user's language. */
export function errorText(e: unknown): string {
  if (e instanceof ApiError) {
    const m = /^invalid: (\w+)$/.exec(e.message);
    if (m) return t(('cfg.err.' + m[1]) as Key);
    if (e.status === 409) return t('cfg.allExcluded');
    return e.message;
  }
  return e instanceof Error ? e.message : String(e);
}

/** The per-device results of an apply (dlgSetMultiMsg*). */
export function showResults(title: string, lines: ResultLine[]): void {
  const list = h('ul', { class: 'results' }, ...lines.map((l) => h('li', {},
    h('span', {}, l.name + ' — '),
    h('span', { class: 'res ' + l.result }, t(('cfg.res.' + l.result) as Key)),
    l.message ? h('span', { class: 'muted' }, ` (${l.message})`) : null)));
  openModal(title, lines.length ? list : h('p', {}, t('cfg.res.none')), [{ label: t('common.close') }]);
}

// ---- small form helpers -------------------------------------------------------------

let seq = 0;
function input(type = 'text', value = ''): HTMLInputElement {
  const i = h('input', { type, id: `cf${++seq}`, autocomplete: type === 'password' ? 'new-password' : 'off' });
  i.value = value;
  return i;
}
function check(checked: boolean): HTMLInputElement {
  const c = h('input', { type: 'checkbox', id: `cf${++seq}` });
  c.checked = checked;
  return c;
}
function field(label: string, control: HTMLElement, ...extra: (Node | null)[]): HTMLElement {
  return h('div', { class: 'field cfg-field' }, h('label', { for: control.id }, label), h('div', { class: 'row' }, control, ...extra));
}
function checkRow(label: string, c: HTMLInputElement): HTMLElement {
  return h('label', { class: 'row cfg-check' }, c, label);
}
/** Yes / No / Keep radios; value null selects Keep, which is shown only when needed. */
function triState(label: string, value: boolean | null | undefined): { el: HTMLElement; get: () => boolean | null; set: (v: boolean | null) => void; enable: (on: boolean) => void } {
  const name = `tri${++seq}`;
  const yes = h('input', { type: 'radio', name }), no = h('input', { type: 'radio', name }), keep = h('input', { type: 'radio', name });
  const keepLabel = h('label', { class: 'row' }, keep, t('cfg.keep'));
  const set = (v: boolean | null): void => {
    yes.checked = v === true; no.checked = v === false; keep.checked = v === null;
    keepLabel.classList.toggle('hidden', v !== null && !keep.checked);
  };
  set(value ?? null);
  return {
    el: h('div', { class: 'field cfg-field' }, h('span', { class: 'cfg-label' }, label),
      h('div', { class: 'row' }, h('label', { class: 'row' }, yes, t('cfg.yes')), h('label', { class: 'row' }, no, t('cfg.no')), keepLabel)),
    get: () => (yes.checked ? true : no.checked ? false : null),
    set,
    enable: (on) => { [yes, no, keep].forEach((r) => { r.disabled = !on; }); },
  };
}
function showPwd(pwd: HTMLInputElement): HTMLElement {
  const c = check(false);
  c.addEventListener('change', () => { pwd.type = c.checked ? 'text' : 'password'; });
  return h('label', { class: 'row cfg-check' }, c, t('cfg.showPwd'));
}
function intOrNull(i: HTMLInputElement): number | undefined {
  return i.value.trim() === '' ? undefined : Number(i.value);
}

/** DialogDeviceSelection: pick one on-line device to copy settings from. */
function pickDevice(onPick: (id: string) => void): void {
  const list = allDevices().filter((d) => d.status === 'online' && ['1', '2', '3', '4'].includes(d.gen))
    .sort((a, b) => (a.name || a.hostname).localeCompare(b.name || b.hostname));
  const filter = input('search');
  filter.placeholder = t('filter.placeholder');
  const box = h('div', { class: 'pick-list' });
  let close = (): void => {};
  const draw = (): void => {
    const q = filter.value.toLowerCase();
    box.replaceChildren(...list.filter((d) => `${d.name} ${d.hostname} ${d.ip}`.toLowerCase().includes(q)).map((d) =>
      h('button', { class: 'pick', onclick: () => { close(); onPick(d.id); } }, `${d.name || d.hostname} — ${d.ip}`)));
  };
  filter.addEventListener('input', draw);
  draw();
  close = openModal(t('cfg.copyFrom'), h('div', {}, filter, box), [{ label: t('common.cancel') }]);
}

// ---- tabs -----------------------------------------------------------------------------

interface TabView {
  body: HTMLElement;
  /** Returns the request body, or throws a message to show. */
  collect(): Promise<Record<string, unknown> | null>;
}

function wifiTab(f: ConfigForm, ids: string[], section: 'wifi1' | 'wifi2'): TabView {
  const w = f.wifi!;
  const single = ids.length === 1;
  const enabled = check(w.enabled), ssid = input('text', w.ssid), pwd = input('password');
  const ip = input('text', w.ip), mask = input('text', w.netmask), gw = input('text', w.gateway), dns = input('text', w.dns);
  const name = `wm${++seq}`;
  const dhcp = h('input', { type: 'radio', name }), stat = h('input', { type: 'radio', name }), keep = h('input', { type: 'radio', name });
  const keepLabel = h('label', { class: 'row' }, keep, t('cfg.keep'));
  dhcp.checked = w.static === false; stat.checked = w.static === true; keep.checked = w.static === null;
  keepLabel.classList.toggle('hidden', w.static !== null);
  const sync = (): void => {
    const on = enabled.checked;
    [ssid, pwd, dhcp, keep].forEach((e) => { e.disabled = !on; });
    stat.disabled = !(on && (w.static === true || single));
    const st = stat.checked || keep.checked;
    ip.disabled = !(on && stat.checked && single);
    [mask, gw, dns].forEach((e) => { e.disabled = !(on && st); });
  };
  [enabled, dhcp, stat, keep].forEach((e) => e.addEventListener('change', sync));
  ip.addEventListener('blur', () => { // PanelWIFI: fill netmask and gateway from a new IP
    if (IPV4.test(ip.value) && !mask.value && !gw.value) { mask.value = '255.255.255.0'; gw.value = ip.value.slice(0, ip.value.lastIndexOf('.') + 1); }
  });
  sync();
  const copy = h('button', { class: 'btn', onclick: () => pickDevice(async (id) => {
    const g = (await configApi.form(section, [id])).wifi!;
    enabled.checked = g.enabled; ssid.value = g.ssid;
    if (g.static && single) stat.checked = true; else if (g.static && !keepLabel.classList.contains('hidden')) keep.checked = true; else dhcp.checked = true;
    gw.value = g.gateway; mask.value = g.netmask; dns.value = g.dns;
    sync();
  }) }, t('cfg.copyFrom'));
  const body = h('div', { class: 'cfg-tab' },
    checkRow(t('cfg.enabled'), enabled),
    field(t('cfg.ssid'), ssid), field(t('cfg.password'), pwd, showPwd(pwd)),
    h('div', { class: 'field cfg-field' }, h('span', { class: 'cfg-label' }, t('cfg.dhcp')),
      h('div', { class: 'row' }, h('label', { class: 'row' }, dhcp, t('cfg.enabled')), h('label', { class: 'row' }, stat, t('cfg.static')), keepLabel)),
    field(t('cfg.ip'), ip), field(t('cfg.netmask'), mask), field(t('cfg.gateway'), gw), field(t('cfg.dns'), dns),
    h('div', { class: 'row' }, copy));
  return {
    body,
    async collect() {
      if (!(await confirmDialog(t('cfg.tab.wifi'), t('cfg.wifiConfirm'), t('cfg.apply'), true))) return null;
      return { enabled: enabled.checked, ssid: ssid.value, password: pwd.value, mode: dhcp.checked ? 'dhcp' : stat.checked ? 'static' : 'keep',
        ip: ip.value, netmask: mask.value, gateway: gw.value, dns: dns.value, confirm: true };
    },
  };
}

function loginTab(f: ConfigForm): TabView {
  const l = f.login!;
  const enabled = check(l.enabled), user = input('text', l.user), pwd = input('password');
  const sync = (): void => { user.disabled = !(enabled.checked && f.variant === 'g1'); pwd.disabled = !enabled.checked; };
  enabled.addEventListener('change', sync);
  sync();
  return {
    body: h('div', { class: 'cfg-tab' }, checkRow(t('cfg.enabled'), enabled), field(t('cfg.user'), user), field(t('cfg.password'), pwd, showPwd(pwd))),
    async collect() { return { enabled: enabled.checked, user: user.value, password: pwd.value }; },
  };
}

function mqttTab(f: ConfigForm, ids: string[]): TabView {
  const q = f.mqtt!;
  const single = ids.length === 1;
  const enabled = check(q.enabled), server = input('text', q.server), user = input('text', q.user), pwd = input('password');
  const noPwd = check(q.noPassword), prefix = input('text', q.prefix), defPrefix = check(false);
  const g1 = f.variant === 'g1', g2 = f.variant === 'g2';
  const num = (v: number | undefined): HTMLInputElement => { const i = input('number', v === undefined ? '' : String(v)); i.min = '0'; return i; };
  const rMax = num(q.reconnectMax), rMin = num(q.reconnectMin), keepAlive = num(q.keepAlive), qos = num(q.qos), period = num(q.updatePeriod);
  qos.max = '2';
  const clean = triState(t('cfg.mqtt.cleanSession'), q.cleanSession), retain = triState(t('cfg.mqtt.retain'), q.retain);
  const control = triState(t('cfg.mqtt.control'), q.control), rpc = triState(t('cfg.mqtt.rpc'), q.rpc);
  const rpcNtf = triState(t('cfg.mqtt.rpcNtf'), q.rpcNtf), statusNtf = triState(t('cfg.mqtt.statusNtf'), q.statusNtf);
  const sync = (): void => {
    const on = enabled.checked;
    [server, noPwd, defPrefix, rMax, rMin, keepAlive, qos, period].forEach((e) => { e.disabled = !on; });
    [user, pwd].forEach((e) => { e.disabled = !(on && !noPwd.checked); });
    prefix.disabled = !(on && !defPrefix.checked && single);
    [clean, retain, control, rpc, rpcNtf, statusNtf].forEach((x) => x.enable(on));
  };
  [enabled, noPwd, defPrefix].forEach((e) => e.addEventListener('change', sync));
  sync();
  const copy = h('button', { class: 'btn', onclick: () => pickDevice(async (id) => {
    const g = (await configApi.form('mqtt', [id])).mqtt!;
    enabled.checked = g.enabled; server.value = g.server; user.value = g.user;
    if (g2) { control.set(g.control ?? null); rpc.set(g.rpc ?? null); rpcNtf.set(g.rpcNtf ?? null); statusNtf.set(g.statusNtf ?? null); }
    if (g1) { rMax.value = String(g.reconnectMax ?? ''); rMin.value = String(g.reconnectMin ?? ''); keepAlive.value = String(g.keepAlive ?? '');
      qos.value = String(g.qos ?? ''); period.value = String(g.updatePeriod ?? ''); clean.set(g.cleanSession ?? null); retain.set(g.retain ?? null); }
    sync();
  }) }, t('cfg.copyFrom'));
  const body = h('div', { class: 'cfg-tab' },
    checkRow(t('cfg.enabled'), enabled),
    g2 ? h('div', {}, control.el, rpc.el, rpcNtf.el, statusNtf.el) : null,
    field(t('cfg.server'), server), field(t('cfg.user'), user), field(t('cfg.password'), pwd, showPwd(pwd)),
    checkRow(t('cfg.noPwd'), noPwd),
    field(t('cfg.mqtt.prefix'), prefix, h('label', { class: 'row cfg-check' }, defPrefix, t('cfg.mqtt.defaultPrefix'))),
    g1 ? h('div', {}, field(t('cfg.mqtt.reconnectMax'), rMax), field(t('cfg.mqtt.reconnectMin'), rMin), clean.el,
      field(t('cfg.mqtt.keepAlive'), keepAlive), field(t('cfg.mqtt.qos'), qos), retain.el, field(t('cfg.mqtt.updatePeriod'), period)) : null,
    h('p', { class: 'muted' }, t('cfg.mqtt.reboot')),
    h('div', { class: 'row' }, copy));
  return {
    body,
    async collect() {
      const b: Record<string, unknown> = { enabled: enabled.checked, server: server.value, user: user.value, password: pwd.value,
        noPassword: noPwd.checked, defaultPrefix: defPrefix.checked, prefix: prefix.value };
      if (g1) Object.assign(b, { reconnectMax: intOrNull(rMax), reconnectMin: intOrNull(rMin), keepAlive: intOrNull(keepAlive), qos: intOrNull(qos),
        updatePeriod: intOrNull(period), cleanSession: clean.get() ?? undefined, retain: retain.get() ?? undefined });
      if (g2) Object.assign(b, { control: control.get() ?? undefined, rpc: rpc.get() ?? undefined, rpcNtf: rpcNtf.get() ?? undefined, statusNtf: statusNtf.get() ?? undefined });
      return b;
    },
  };
}

function othersTab(f: ConfigForm): TabView {
  const o = f.others!;
  const part = `op${++seq}`;
  const selNTP = h('input', { type: 'radio', name: part }), selCloud = h('input', { type: 'radio', name: part }), selReset = h('input', { type: 'radio', name: part });
  selNTP.checked = true;
  const ntp = input('text', o.ntp);
  const radios = (value: boolean | null): { el: HTMLElement; get: () => boolean | null; all: HTMLInputElement[] } => {
    const name = `or${++seq}`;
    const on = h('input', { type: 'radio', name }), off = h('input', { type: 'radio', name });
    on.checked = value === true; off.checked = value === false;
    return { el: h('div', { class: 'row' }, h('label', { class: 'row' }, on, t('cfg.enable')), h('label', { class: 'row' }, off, t('cfg.disable'))),
      get: () => (on.checked ? true : off.checked ? false : null), all: [on, off] };
  };
  const cloud = radios(o.cloud), reset = radios(o.reset);
  const sync = (): void => {
    ntp.disabled = !selNTP.checked;
    cloud.all.forEach((r) => { r.disabled = !selCloud.checked; });
    reset.all.forEach((r) => { r.disabled = !selReset.checked; });
  };
  [selNTP, selCloud, selReset].forEach((e) => e.addEventListener('change', sync));
  sync();
  return {
    body: h('div', { class: 'cfg-tab' },
      h('fieldset', { class: 'cfg-part' }, h('legend', {}, h('label', { class: 'row' }, selNTP, t('cfg.ntp'))), ntp),
      h('fieldset', { class: 'cfg-part' }, h('legend', {}, h('label', { class: 'row' }, selCloud, t('cfg.cloud'))), cloud.el),
      h('fieldset', { class: 'cfg-part' }, h('legend', {}, h('label', { class: 'row' }, selReset, t('cfg.reset'))), reset.el)),
    async collect() {
      if (selNTP.checked) return { part: 'ntp', ntp: ntp.value };
      const v = (selCloud.checked ? cloud : reset).get();
      return { part: selCloud.checked ? 'cloud' : 'reset', enable: v ?? undefined };
    },
  };
}

// ---- the dialog ------------------------------------------------------------------------

/** Open "Devices settings" for the given devices on one tab. onApplied runs after each apply. */
export function openDeviceSettings(ids: string[], tab: SettingsTab = 'wifi1', onApplied?: () => void): void {
  const devs = allDevices().filter((d) => ids.includes(d.id) && d.gen !== 'bth');
  if (devs.length === 0) { openModal(t('cfg.title'), h('p', {}, t('cfg.allExcluded')), [{ label: t('common.close') }]); return; }
  const title = devs.length === 1 ? t('cfg.title1', { device: devs[0]!.name || devs[0]!.hostname }) : t('cfg.titleMany', { n: devs.length });
  const devIds = devs.map((d) => d.id);
  const tabsBar = h('div', { class: 'tabs', role: 'tablist' });
  const note = h('div');
  const content = h('div', { class: 'cfg-body' });
  let current: SettingsTab = tab;
  let view: TabView | null = null;
  let token = 0;

  const load = async (): Promise<void> => {
    const my = ++token;
    view = null;
    tabsBar.replaceChildren(...TABS.map((x) => h('button', { class: 'tab' + (x.id === current ? ' active' : ''), role: 'tab', 'aria-selected': String(x.id === current),
      onclick: () => { if (x.id !== current) { current = x.id; void load(); } } }, t(x.label))));
    note.replaceChildren();
    content.replaceChildren(h('div', { class: 'spinner' }));
    try {
      const f = await configApi.form(current, devIds);
      if (my !== token) return;
      if (f.excluded.length) note.replaceChildren(h('div', { class: 'banner warn' }, t('cfg.excluded', { list: f.excluded.map((e) => e.name).join(', ') })));
      view = current === 'wifi1' || current === 'wifi2' ? wifiTab(f, devIds, current) : current === 'login' ? loginTab(f) : current === 'mqtt' ? mqttTab(f, devIds) : othersTab(f);
      content.replaceChildren(view.body);
    } catch (e) {
      if (my === token) content.replaceChildren(h('p', { class: 'banner warn' }, errorText(e)));
    }
  };
  const doApply = async (): Promise<boolean> => {
    if (!view) return false;
    let body: Record<string, unknown> | null;
    try { body = await view.collect(); } catch (e) { note.replaceChildren(h('div', { class: 'banner warn' }, errorText(e))); return false; }
    if (!body) return false;
    try {
      const res = await configApi.apply(current, devIds, body);
      showResults(t(TABS.find((x) => x.id === current)!.label), res.results);
      onApplied?.();
      void load();
      return true;
    } catch (e) {
      note.replaceChildren(h('div', { class: 'banner warn', role: 'alert' }, errorText(e)));
      return false;
    }
  };
  openModal(title, h('div', { class: 'cfg' }, tabsBar, note, content), [
    { label: t('cfg.apply'), kind: 'primary', onClick: async () => { await doApply(); return false; } },
    { label: t('cfg.applyClose'), onClick: () => doApply() },
    { label: t('common.close') },
  ], undefined, 'wide');
  void load();
}
