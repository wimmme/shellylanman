// The device table (ShellyScanner: MainView + DevicesTable).
import { devicesApi, type Device, type DeviceStatus } from '../api';
import { commandCell, interacting, whenIdle } from '../command';
import { addressText, allDevices, compareAddress, onDevicesChanged, scanState } from '../devices';
import { h, icon, ICONS } from '../dom';
import { dateTime, formatTemp, formatUptime, meterSetText, metersText, moduleText, prefs, uptimeTooltip } from '../format';
import { t, type Key } from '../i18n';
import { confirmDialog, openModal } from '../modal';
import { openInfo } from '../panels/info';
import { openLogs } from '../panels/logs';
import { toast } from '../toast';
import { emptyState, type Page } from './common';

type ColKey = 'status' | 'type' | 'device' | 'name' | 'keyword' | 'mac' | 'ip' | 'ssid' | 'rssi' | 'cloud' | 'mqtt'
  | 'uptime' | 'temp' | 'measures' | 'logs' | 'source' | 'command';

interface Col {
  key: ColKey;
  label: Key;
  text: (d: Device) => string;              // filter, sort fallback, copy
  cell?: (d: Device) => Node | string;      // rendering (defaults to text)
  cmp?: (a: Device, b: Device) => number;   // sort
  live?: boolean;                           // empty for not logged / error / archived devices (DevicesTable.generateRow)
  tip?: (d: Device) => string;
}

const YES = '✓', NO = '✗';
const LOG_LABEL: Record<string, string> = { NONE: NO, UNDEFINED: '-', FILE: 'file', MQTT: 'mqtt', SOCKET: 'socket', UDP: 'udp' };
const isBLU = (d: Device): boolean => d.gen === 'blu' || d.gen === 'bth';
const hasLive = (d: Device): boolean => d.status !== 'login' && d.status !== 'error' && d.status !== 'ghost';
const num = (f: (d: Device) => number | undefined) => (a: Device, b: Device): number => (f(a) ?? -Infinity) - (f(b) ?? -Infinity);

const STATUS: Record<DeviceStatus, { cls: string; key: Key }> = {
  online: { cls: 'online', key: 'status.online' },
  offline: { cls: 'offline', key: 'status.offline' },
  login: { cls: 'login', key: 'status.login' },
  reading: { cls: 'reading', key: 'status.reading' },
  error: { cls: 'error', key: 'status.error' },
  ghost: { cls: 'ghost', key: 'status.ghost' },
};

function statusTip(d: Device): string {
  const tip = [t(STATUS[d.status]?.key ?? 'status.error')];
  if (d.rebootRequired) tip.push(t('status.rebootRequired'));
  if (d.status === 'online' && isBLU(d) && d.lastSeen > 0) tip.push(t('status.bluSeen', { when: dateTime(new Date(d.lastSeen), true) }));
  if ((d.status === 'offline' || d.status === 'ghost') && d.lastSeen > 0) tip.push(t('status.lastSeen', { when: dateTime(new Date(d.lastSeen), true) }));
  if (d.error) tip.push(d.error);
  if (d.paused) tip.push(t('logs.paused'));
  return tip.join(' — ');
}

function statusPill(d: Device): HTMLElement {
  const s = STATUS[d.status] ?? STATUS.error;
  let label = t(s.key);
  if (d.status === 'online' && d.rebootRequired) label += ' ↻';
  if (d.status === 'online' && isBLU(d)) label = 'BLU ' + label;
  return h('span', { class: 'pill ' + s.cls }, label);
}

function lines(items: string[]): Node {
  const box = h('div', { class: 'cell-lines' });
  for (const it of items) box.append(h('div', {}, it));
  return box;
}

const COLUMNS: Col[] = [
  { key: 'status', label: 'col.status', text: (d) => t(STATUS[d.status]?.key ?? 'status.error'), cell: statusPill, tip: statusTip,
    cmp: (a, b) => a.status.localeCompare(b.status) },
  { key: 'type', label: 'col.type', text: (d) => d.typeName },
  { key: 'device', label: 'col.device', text: (d) => d.hostname },
  { key: 'name', label: 'col.name', text: (d) => d.name },
  { key: 'keyword', label: 'col.keyword', text: (d) => d.keyword ?? '' },
  { key: 'mac', label: 'col.mac', text: (d) => d.mac },
  { key: 'ip', label: 'col.ip', text: addressText, cmp: compareAddress, tip: (d) => (d.parents?.length ? t('status.gateways', { list: d.parents.join(', ') }) : '') },
  { key: 'ssid', label: 'col.ssid', text: (d) => d.ssid ?? '' },
  { key: 'rssi', label: 'col.rssi', text: (d) => (isBLU(d) && !d.rssi ? '' : String(d.rssi)), cmp: num((d) => (hasLive(d) ? d.rssi : undefined)), live: true },
  { key: 'cloud', label: 'col.cloud', text: (d) => (isBLU(d) ? '' : `${d.cloudEnabled ? YES : NO} ${d.cloudConnected ? YES : NO}`), live: true },
  { key: 'mqtt', label: 'col.mqtt', text: (d) => (isBLU(d) ? '' : `${d.mqttEnabled ? YES : NO} ${d.mqttConnected ? YES : NO}`), live: true },
  { key: 'uptime', label: 'col.uptime', text: (d) => (d.uptime >= 0 ? formatUptime(d.uptime) : ''), cmp: num((d) => (hasLive(d) && d.uptime >= 0 ? d.uptime : undefined)),
    live: true, tip: (d) => (d.uptime >= 0 ? uptimeTooltip(d.uptime) : '') },
  { key: 'temp', label: 'col.temp', text: (d) => (d.internalTemp !== undefined ? formatTemp(d.internalTemp) : ''), cmp: num((d) => d.internalTemp), live: true },
  { key: 'measures', label: 'col.measures', text: (d) => metersText(d.meters), cell: (d) => lines((d.meters ?? []).map((m) => meterSetText(m))),
    cmp: num((d) => d.meters?.[0]?.values[0]?.value), live: true, tip: (d) => (d.meters ?? []).map((m) => meterSetText(m)).join('\n') },
  { key: 'logs', label: 'col.logs', text: (d) => (isBLU(d) ? '' : LOG_LABEL[d.logMode] ?? d.logMode), live: true },
  { key: 'source', label: 'col.source', text: (d) => (d.modules ?? []).map((m) => m.source).filter((s): s is string => !!s).join(' / '),
    cell: (d) => lines((d.modules ?? []).map((m) => m.source ?? '').filter(Boolean)), live: true },
  { key: 'command', label: 'col.command', text: (d) => (d.modules ?? []).map((m) => moduleText(m)).join(' + '),
    cell: (d) => commandCell(d) ?? '', live: true },
];

// ShellyScanner hides Keyword, MAC, SSID and Logs in the default view; the
// detailed view starts with every column.
const DEFAULT_HIDDEN: Record<'default' | 'detailed', ColKey[]> = { default: ['keyword', 'mac', 'ssid', 'logs'], detailed: [] };
const FILTER_COLS: { label: Key; cols: ColKey[] }[] = [
  { label: 'filter.all', cols: ['type', 'device', 'name', 'keyword', 'ip'] },
  { label: 'col.type', cols: ['type'] }, { label: 'col.device', cols: ['device'] },
  { label: 'col.name', cols: ['name'] }, { label: 'col.keyword', cols: ['keyword'] },
];

// ---- per-browser table state ---------------------------------------------------

const lsGet = (k: string): string | null => { try { return localStorage.getItem(k); } catch { return null; } };
const lsSet = (k: string, v: string): void => { try { localStorage.setItem(k, v); } catch { /* private mode */ } };

let view: 'default' | 'detailed' = lsGet('sl_view') === 'detailed' ? 'detailed' : 'default';
let sortKey: ColKey = 'ip';
let sortAsc = true;
let filterText = '';
let filterCol = prefs.defaultFilter();
const selected = new Set<string>();
let anchor: string | null = null;

function hiddenCols(): Set<ColKey> {
  const raw = lsGet(`sl_cols_${view}`);
  if (raw) { try { return new Set(JSON.parse(raw) as ColKey[]); } catch { /* fall back */ } }
  return new Set(DEFAULT_HIDDEN[view]);
}
function setHidden(s: Set<ColKey>): void { lsSet(`sl_cols_${view}`, JSON.stringify([...s])); }

function visibleRows(list: Device[]): Device[] {
  const q = filterText.trim().toLowerCase();
  const cols = FILTER_COLS[filterCol]?.cols ?? FILTER_COLS[0]!.cols;
  const rows = q ? list.filter((d) => cols.some((k) => COLUMNS.find((c) => c.key === k)!.text(d).toLowerCase().includes(q))) : [...list];
  const col = COLUMNS.find((c) => c.key === sortKey)!;
  const cmp = col.cmp ?? ((a: Device, b: Device) => col.text(a).localeCompare(col.text(b), undefined, { numeric: true }) || compareAddress(a, b));
  return rows.sort((a, b) => (sortAsc ? cmp(a, b) : cmp(b, a)));
}

function selectedDevices(): Device[] {
  return allDevices().filter((d) => selected.has(d.id));
}

// ---- actions ------------------------------------------------------------------

function loginDialog(d: Device): void {
  const gen1 = d.gen === '1';
  const user = h('input', { id: 'lgUser', autocomplete: 'username', value: 'admin' });
  const pass = h('input', { id: 'lgPass', type: 'password', autocomplete: 'current-password' });
  const all = h('input', { id: 'lgAll', type: 'checkbox', checked: true });
  const err = h('p', { class: 'muted', role: 'alert' });
  openModal(t('login.title'), h('div', {},
    h('p', {}, t('login.text', { device: d.hostname || addressText(d) })),
    gen1 && h('div', { class: 'field' }, h('label', { for: 'lgUser' }, t('login.user')), user),
    h('div', { class: 'field' }, h('label', { for: 'lgPass' }, t('login.password')), pass),
    h('label', { class: 'row' }, all, t('login.useForAll')),
    h('p', { class: 'muted' }, t('login.stored')), err), [
    { label: t('common.cancel') },
    { label: t('login.submit'), kind: 'primary', onClick: async () => {
      if (!pass.value) { err.textContent = t('login.needPassword'); return false; }
      const u = gen1 ? user.value : 'admin';
      if (all.checked) await devicesApi.setCredentials(u, pass.value);
      else await devicesApi.setDeviceCredentials(d.id, u, pass.value);
    } },
  ]);
}

/** Open the devices' own web UI (confirm for more than 8, like ShellyScanner). */
async function openWebUI(list: Device[]): Promise<void> {
  const targets = list.filter((d) => d.status !== 'ghost');
  if (targets.length > 8 && !(await confirmDialog(t('action.webUI'), t('action.webConfirm', { n: targets.length }), t('action.webUI'), false))) return;
  for (const d of targets) window.open(`http://${d.port === 80 ? d.ip : `${d.ip}:${d.port}`}`, '_blank', 'noopener');
}

/** Reboot (MainView.rebootAction): stored devices and BLU devices other than the TRV cannot. */
const rebootable = (d: Device): boolean => d.status !== 'ghost' && d.gen !== 'bth';

async function reboot(list: Device[]): Promise<void> {
  if (!(await confirmDialog(t('action.reboot'), t('action.rebootConfirm'), t('action.reboot')))) return;
  try {
    await devicesApi.reboot(list.map((d) => d.id));
  } catch (e) {
    toast(e instanceof Error ? e.message : String(e));
  }
}

async function removeGhosts(list: Device[]): Promise<void> {
  const ghosts = list.filter((d) => d.status === 'ghost');
  const msg = ghosts.some((d) => d.note) ? t('action.removeGhostNotes') : t('action.removeGhostConfirm');
  if (!(await confirmDialog(t('action.removeGhost'), msg, t('action.removeGhost')))) return;
  for (const d of ghosts) { await devicesApi.remove(d.id); selected.delete(d.id); }
}

function reload(list: Device[]): void {
  const login = list.find((d) => d.status === 'login');
  if (list.length === 1 && login) { loginDialog(login); return; }
  for (const d of list) void devicesApi.reload(d.id);
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

// Selection helpers (MainView: "Devices selection").
const SELECTORS: { label: Key; f: (d: Device) => boolean }[] = [
  { label: 'select.all', f: () => true },
  { label: 'select.online', f: (d) => d.status === 'online' },
  { label: 'select.reboot', f: (d) => d.status === 'online' && d.rebootRequired },
  { label: 'select.gen1', f: (d) => d.gen === '1' },
  { label: 'select.gen2', f: (d) => ['2', '3', '4'].includes(d.gen) },
  { label: 'select.wifi', f: (d) => ['1', '2', '3', '4'].includes(d.gen) },
  { label: 'select.blu', f: isBLU },
  { label: 'select.ghost', f: (d) => d.status === 'ghost' },
  { label: 'select.none', f: () => false },
];

function dropdown(label: string, items: { label: string; onClick: () => void; checked?: boolean }[]): HTMLElement {
  const menu = h('div', { class: 'menu hidden', role: 'menu' });
  for (const it of items) {
    menu.append(h('button', { role: it.checked === undefined ? 'menuitem' : 'menuitemcheckbox', 'aria-checked': it.checked === undefined ? undefined : String(it.checked),
      onclick: (e: Event) => { e.stopPropagation(); it.onClick(); if (it.checked === undefined) menu.classList.add('hidden'); } },
    it.checked === undefined ? '' : (it.checked ? '☑ ' : '☐ '), it.label));
  }
  const btn = h('button', { class: 'btn', 'aria-haspopup': 'menu', onclick: (e: Event) => {
    e.stopPropagation();
    const wasHidden = menu.classList.contains('hidden');
    document.querySelectorAll('.menu').forEach((m) => m.classList.add('hidden'));
    if (wasHidden) menu.classList.remove('hidden');
  } }, label, ' ▾');
  return h('div', { class: 'dropdown' }, btn, menu);
}

// ---- page ------------------------------------------------------------------------

export const devicesPage: Page = {
  id: 'devices',
  title: 'nav.devices',
  icon: ICONS.devices,
  render(main) {
    const host = h('div', { style: 'display:contents' });
    const filterInput = h('input', { type: 'search', placeholder: t('filter.placeholder'), 'aria-label': t('filter.label'), value: filterText, size: 16 });
    const filterSel = h('select', { 'aria-label': t('filter.column') }, ...FILTER_COLS.map((f, i) => h('option', { value: i }, t(f.label))));
    filterSel.value = String(filterCol);
    filterInput.addEventListener('input', () => { filterText = filterInput.value; draw(); });
    filterSel.addEventListener('change', () => { filterCol = Number(filterSel.value); draw(); });

    // Built once; draw() only refreshes the dynamic parts, so the filter field
    // keeps its focus while typing and devices keep updating.
    const bannerBox = h('div', { style: 'display:contents' });
    const summaryBox = h('div', { style: 'display:contents' });
    const actionsBox = h('div', { class: 'row' });
    const controlsBox = h('div', { class: 'row' });
    const badge = h('span', { class: 'card-badge' });
    const statusLine = h('div', { class: 'muted status-line', role: 'status' });
    const bodyBox = h('div');
    host.append(bannerBox, summaryBox, h('section', { class: 'card' },
      h('div', { class: 'card-head' }, h('h2', { class: 'card-title' }, t('devices.table.title')), badge, statusLine),
      h('div', { class: 'toolbar' }, actionsBox, h('div', { class: 'spacer' }), filterSel, filterInput, controlsBox),
      bodyBox));

    const draw = (): void => {
      if (interacting()) { whenIdle(draw); return; } // a slider is being dragged: redraw when it is released
      const all = allDevices();
      for (const id of [...selected]) if (!all.some((d) => d.id === id)) selected.delete(id);
      const rows = visibleRows(all);
      const hidden = hiddenCols();
      const cols = COLUMNS.filter((c) => !hidden.has(c.key));
      const sel = selectedDevices();
      const one = sel.length === 1 ? sel[0]! : null;
      const noGhost = sel.length > 0 && sel.every((d) => d.status !== 'ghost');
      const openMenu = document.querySelector('.menu:not(.hidden)') !== null;
      if (openMenu && host.isConnected) return; // do not close a menu the user is using

      const act = (label: Key, enabled: boolean, onClick: () => void, tip?: Key): HTMLElement =>
        h('button', { class: 'btn', disabled: !enabled, onclick: onClick, title: tip ? t(tip) : undefined }, t(label));
      actionsBox.replaceChildren(...[
        dropdown(t('select.menu'), SELECTORS.map((s) => ({ label: t(s.label), onClick: () => { selected.clear(); for (const d of all) if (s.f(d)) selected.add(d.id); redraw(); } }))),
        act('action.info', !!one, () => one && openInfo(one.id), 'action.infoTip'),
        act('action.logs', !!one && one.status !== 'ghost' && one.gen !== '-', () => one && openLogs(one.id)),
        act('action.webUI', noGhost && sel.some((d) => !isBLU(d)), () => void openWebUI(sel.filter((d) => !isBLU(d))), 'action.webUITip'),
        act(sel.length === 1 && one?.status === 'login' ? 'action.login' : 'action.reload', sel.length > 0, () => reload(sel)),
        act('action.reboot', sel.length > 0 && sel.every(rebootable), () => void reboot(sel), 'action.rebootTip'),
        sel.length > 0 && sel.every((d) => d.status === 'ghost') ? act('action.removeGhost', true, () => void removeGhosts(sel)) : null,
      ].filter((x): x is HTMLElement => x !== null));
      controlsBox.replaceChildren(
        dropdown(t('columns.menu'), COLUMNS.filter((c) => c.key !== 'status').map((c) => ({
          label: t(c.label), checked: !hidden.has(c.key),
          onClick: () => { const s = hiddenCols(); if (s.has(c.key)) s.delete(c.key); else s.add(c.key); setHidden(s); redraw(); },
        }))),
        h('button', { class: 'btn', 'aria-pressed': String(view === 'detailed'), title: t('action.viewTip'),
          onclick: () => { view = view === 'detailed' ? 'default' : 'detailed'; lsSet('sl_view', view); redraw(); } },
        t(view === 'detailed' ? 'action.viewDetailed' : 'action.viewDefault')),
        h('button', { class: 'btn', onclick: () => devicesApi.refresh(), title: t('action.refreshTip') }, icon(ICONS.refresh, 16), t('action.refresh')),
        h('button', { class: 'btn', onclick: () => devicesApi.rescan(), title: t('action.rescanTip') }, icon(ICONS.radar, 16), t('action.rescan')));

      badge.textContent = String(all.length);
      statusLine.textContent = filterText
        ? t('status.filtered', { shown: rows.length, total: all.length, selected: sel.length })
        : t('status.listed', { total: all.length, selected: sel.length });

      let body: Node;
      if (all.length === 0) {
        body = emptyState(t('devices.empty.title'), t('devices.empty.text'), ICONS.devices);
      } else {
        const allChecked = rows.length > 0 && rows.every((d) => selected.has(d.id));
        const head = h('tr', {},
          h('th', { class: 'sel' }, h('input', { type: 'checkbox', 'aria-label': t('select.all'), checked: allChecked,
            onchange: () => { if (allChecked) rows.forEach((d) => selected.delete(d.id)); else rows.forEach((d) => selected.add(d.id)); redraw(); } })),
          ...cols.map((c) => {
            const active = c.key === sortKey;
            const th = h('th', { scope: 'col', class: 'sortable' + (active ? (sortAsc ? ' sort-asc' : ' sort-desc') : ''), tabindex: 0,
              'aria-sort': active ? (sortAsc ? 'ascending' : 'descending') : 'none' }, c.key === 'status' ? '' : t(c.label));
            const toggle = (): void => { if (active) sortAsc = !sortAsc; else { sortKey = c.key; sortAsc = true; } redraw(); };
            th.addEventListener('click', toggle);
            th.addEventListener('keydown', (e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); toggle(); } });
            if (c.key === 'status') th.title = t('col.status');
            if (c.key === 'cloud' || c.key === 'mqtt') th.title = t(c.key === 'cloud' ? 'col.cloudTip' : 'col.mqttTip');
            return th;
          }));
        const tbody = h('tbody');
        for (const d of rows) {
          const tr = h('tr', { 'data-id': d.id, class: selected.has(d.id) ? 'selected' : '', 'aria-selected': String(selected.has(d.id)) },
            h('td', { class: 'sel' }, h('input', { type: 'checkbox', 'aria-label': d.hostname, checked: selected.has(d.id),
              onclick: (e: Event) => e.stopPropagation(),
              onchange: () => { if (selected.has(d.id)) selected.delete(d.id); else selected.add(d.id); anchor = d.id; redraw(); } })),
            ...cols.map((c) => {
              const empty = c.live && !hasLive(d);
              const td = h('td', { class: 'c-' + c.key }, empty ? '' : (c.cell ? c.cell(d) : c.text(d)));
              const tip = !empty && c.tip ? c.tip(d) : '';
              if (tip) td.title = tip;
              return td;
            }));
          tr.addEventListener('click', (e) => {
            if (window.getSelection()?.toString()) return; // the user is selecting text to copy (T14)
            if (e.shiftKey && anchor) { // range selection
              const ids = rows.map((r) => r.id);
              const [a, b] = [ids.indexOf(anchor), ids.indexOf(d.id)].sort((x, y) => x - y);
              for (const id of ids.slice(a!, b! + 1)) selected.add(id);
            } else if (e.ctrlKey || e.metaKey) {
              if (selected.has(d.id)) selected.delete(d.id); else selected.add(d.id);
              anchor = d.id;
            } else {
              selected.clear(); selected.add(d.id); anchor = d.id;
            }
            redraw();
          });
          tr.addEventListener('dblclick', () => {
            if (prefs.dblClick() === 'WEB' && d.status !== 'ghost' && !isBLU(d)) void openWebUI([d]);
            else openInfo(d.id);
          });
          tbody.append(tr);
        }
        body = h('div', { class: 'table-wrap' }, h('table', { class: 'data devices ' + view }, h('thead', {}, head), tbody));
      }
      const banner = scanBanner();
      bannerBox.replaceChildren(...(banner ? [banner] : []));
      summaryBox.replaceChildren(summary(all));
      const old = bodyBox.querySelector('.table-wrap');
      const [sx, sy] = old ? [old.scrollLeft, old.scrollTop] : [0, 0];
      bodyBox.replaceChildren(body);
      const wrap = bodyBox.querySelector('.table-wrap');
      if (wrap) { wrap.scrollLeft = sx; wrap.scrollTop = sy; } // keep the user's scroll position across updates
    };
    // User actions redraw even with a menu open (the menu belongs to the old toolbar).
    const redraw = (): void => { document.querySelectorAll('.menu').forEach((m) => m.classList.add('hidden')); draw(); };

    // Keyboard shortcuts (MainView): Ctrl+F filter, Ctrl+E clear filter, Ctrl+S next filter column.
    const onKey = (e: KeyboardEvent): void => {
      if (!(e.ctrlKey || e.metaKey)) return;
      const k = e.key.toLowerCase();
      if (k === 'f') { e.preventDefault(); filterInput.focus(); }
      if (k === 'e') { e.preventDefault(); filterText = ''; filterInput.value = ''; redraw(); filterInput.focus(); }
      if (k === 's') { e.preventDefault(); filterCol = (filterCol + 1) % FILTER_COLS.length; filterSel.value = String(filterCol); redraw(); filterInput.focus(); }
    };
    const closeMenus = (): void => { document.querySelectorAll('.menu').forEach((m) => m.classList.add('hidden')); };
    document.addEventListener('keydown', onKey);
    document.addEventListener('click', closeMenus);
    draw();
    const off = onDevicesChanged(draw);
    const timer = setInterval(draw, 10_000); // time-based banners and "since" uptime
    dispose = (): void => { off(); clearInterval(timer); document.removeEventListener('keydown', onKey); document.removeEventListener('click', closeMenus); };
    main.append(host);
  },
  dispose() { dispose(); },
};

let dispose = (): void => {};
