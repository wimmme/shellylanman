import { devicesApi, type Device, type DeviceStatus } from '../api';
import { addressText, allDevices, compareAddress, onDevicesChanged, scanState } from '../devices';
import { h, icon, ICONS } from '../dom';
import { t, type Key } from '../i18n';
import { confirmDialog, openModal } from '../modal';
import { card, emptyState, type Page } from './common';

// Phase 2 columns; the rest of ShellyScanner's table (SSID, RSSI, cloud, MQTT,
// uptime, temperature, measurements, source, command…) arrives in Phase 3.
type Col = { key: Key; value: (d: Device) => string; cmp?: (a: Device, b: Device) => number };
const COLUMNS: Col[] = [
  { key: 'col.type', value: (d) => d.typeName },
  { key: 'col.device', value: (d) => d.hostname },
  { key: 'col.name', value: (d) => d.name },
  { key: 'col.ip', value: addressText, cmp: compareAddress },
];

const STATUS: Record<DeviceStatus, { cls: string; key: Key }> = {
  online: { cls: 'online', key: 'status.online' },
  offline: { cls: 'offline', key: 'status.offline' },
  login: { cls: 'login', key: 'status.login' },
  reading: { cls: 'reading', key: 'status.reading' },
  error: { cls: 'error', key: 'status.error' },
  ghost: { cls: 'ghost', key: 'status.ghost' },
};

let sortCol = 3; // IP, ascending: ShellyScanner's default
let sortAsc = true;

function statusPill(d: Device): HTMLElement {
  const s = STATUS[d.status] ?? STATUS.error;
  let label = t(s.key);
  if (d.status === 'online' && d.rebootRequired) label += ' ↻';
  const tip = [t(s.key)];
  if (d.rebootRequired) tip.push(t('status.rebootRequired'));
  if ((d.status === 'offline' || d.status === 'ghost') && d.lastSeen > 0) tip.push(t('status.lastSeen', { when: new Date(d.lastSeen).toLocaleString() }));
  if (d.error) tip.push(d.error);
  if (d.parents?.length) tip.push(t('status.gateways', { list: d.parents.join(', ') }));
  return h('span', { class: 'pill ' + s.cls, title: tip.join(' — ') }, label);
}

function loginDialog(d: Device): void {
  const gen1 = d.gen === '1';
  const user = h('input', { id: 'lgUser', autocomplete: 'username', value: 'admin' });
  const pass = h('input', { id: 'lgPass', type: 'password', autocomplete: 'current-password' });
  const all = h('input', { id: 'lgAll', type: 'checkbox', checked: true });
  const err = h('p', { class: 'muted', role: 'alert' });
  const body = h('div', {},
    h('p', {}, t('login.text', { device: d.hostname || addressText(d) })),
    gen1 && h('div', { class: 'field' }, h('label', { for: 'lgUser' }, t('login.user')), user),
    h('div', { class: 'field' }, h('label', { for: 'lgPass' }, t('login.password')), pass),
    h('label', { class: 'row' }, all, t('login.useForAll')),
    h('p', { class: 'muted' }, t('login.stored')),
    err);
  openModal(t('login.title'), body, [
    { label: t('common.cancel') },
    {
      label: t('login.submit'), kind: 'primary', onClick: async () => {
        if (!pass.value) { err.textContent = t('login.needPassword'); return false; }
        const u = gen1 ? user.value : 'admin';
        if (all.checked) await devicesApi.setCredentials(u, pass.value);
        else await devicesApi.setDeviceCredentials(d.id, u, pass.value);
      },
    },
  ]);
}

function actions(d: Device): HTMLElement {
  const box = h('div', { class: 'row' });
  if (d.status === 'ghost') {
    box.append(
      h('button', { class: 'btn', onclick: () => devicesApi.reload(d.id) }, t('action.reload')),
      h('button', { class: 'btn', onclick: async () => {
        const msg = d.note ? t('action.removeGhostNotes') : t('action.removeGhostConfirm');
        if (await confirmDialog(t('action.removeGhost'), msg, t('action.removeGhost'))) await devicesApi.remove(d.id);
      } }, t('action.removeGhost')));
  } else if (d.status === 'login') {
    box.append(h('button', { class: 'btn primary', onclick: () => loginDialog(d) }, t('action.login')));
  } else {
    box.append(h('button', { class: 'btn', onclick: () => devicesApi.reload(d.id), title: t('action.reloadTip') }, t('action.reload')));
  }
  return box;
}

function scanBanner(): HTMLElement | null {
  const s = scanState();
  if (!s) return null;
  if (s.mdnsError) return h('div', { class: 'banner warn', role: 'status' }, t('scan.mdnsError', { err: s.mdnsError }));
  if ((s.mode === 'full' || s.mode === 'local') && s.mdnsInstances === 0 && Date.now() - s.startedAt > 30_000) {
    return h('div', { class: 'banner warn', role: 'status' }, t('scan.noMdns'));
  }
  if (s.scanning) return h('div', { class: 'banner', role: 'status' }, h('div', { class: 'spinner' }), t('scan.ipScanning'));
  if (s.mode === 'offline') return h('div', { class: 'banner', role: 'status' }, t('scan.offline'));
  return null;
}

function summary(list: Device[]): HTMLElement {
  const count = (f: (d: Device) => boolean): string => String(list.filter(f).length);
  const stats: [Key, string, string][] = [
    ['devices.summary.total', '', String(list.length)],
    ['devices.summary.online', 'ok', count((d) => d.status === 'online')],
    ['devices.summary.offline', 'err', count((d) => d.status === 'offline' || d.status === 'error')],
    ['devices.summary.login', 'warn', count((d) => d.status === 'login')],
    ['devices.summary.reboot', 'warn', count((d) => d.rebootRequired)],
    ['devices.summary.stored', '', count((d) => d.status === 'ghost')],
  ];
  return h('div', { class: 'summary' }, ...stats.map(([k, cls, v]) =>
    h('div', { class: 'stat ' + cls }, h('div', { class: 'k' }, t(k)), h('div', { class: 'v' }, v))));
}

function table(list: Device[], redraw: () => void): HTMLElement {
  const col = COLUMNS[sortCol]!;
  const cmp = col.cmp ?? ((a: Device, b: Device) => col.value(a).localeCompare(col.value(b)) || compareAddress(a, b));
  const rows = [...list].sort((a, b) => (sortAsc ? cmp(a, b) : cmp(b, a)));
  const head = h('tr', {}, h('th', { scope: 'col', title: t('col.status') }, ''),
    ...COLUMNS.map((c, i) => {
      const th = h('th', {
        scope: 'col', class: 'sortable' + (i === sortCol ? (sortAsc ? ' sort-asc' : ' sort-desc') : ''),
        'aria-sort': i === sortCol ? (sortAsc ? 'ascending' : 'descending') : 'none', tabindex: 0,
      }, t(c.key));
      const toggle = (): void => { if (sortCol === i) sortAsc = !sortAsc; else { sortCol = i; sortAsc = true; } redraw(); };
      th.addEventListener('click', toggle);
      th.addEventListener('keydown', (e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); toggle(); } });
      return th;
    }),
    h('th', { scope: 'col' }, ''));
  const body = h('tbody', {}, ...rows.map((d) => h('tr', { 'data-id': d.id },
    h('td', {}, statusPill(d)),
    ...COLUMNS.map((c) => h('td', {}, c.value(d))),
    h('td', {}, actions(d)))));
  return h('div', { class: 'table-wrap' }, h('table', { class: 'data' }, h('thead', {}, head), body));
}

export const devicesPage: Page = {
  id: 'devices',
  title: 'nav.devices',
  icon: ICONS.devices,
  render(main) {
    const host = h('div', { style: 'display:contents' });
    const draw = (): void => {
      const list = allDevices();
      const tools = h('div', { class: 'row', style: 'margin-left:auto' },
        h('button', { class: 'btn', onclick: () => devicesApi.refresh(), title: t('action.refreshTip') }, icon(ICONS.refresh, 16), t('action.refresh')),
        h('button', { class: 'btn', onclick: () => devicesApi.rescan(), title: t('action.rescanTip') }, icon(ICONS.radar, 16), t('action.rescan')));
      const content = list.length > 0 ? table(list, draw) : emptyState(t('devices.empty.title'), t('devices.empty.text'), ICONS.devices);
      const c = card(t('devices.table.title'), String(list.length), content);
      c.querySelector('.card-head')!.append(tools);
      host.replaceChildren(...[scanBanner(), summary(list), c].filter((x): x is HTMLElement => x !== null));
    };
    draw();
    const off = onDevicesChanged(draw);
    const timer = setInterval(draw, 10_000); // time-based banners (no mDNS answer after 30 s)
    dispose = (): void => { off(); clearInterval(timer); };
    main.append(host);
  },
  dispose() { dispose(); },
};

let dispose = (): void => {};
