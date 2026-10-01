// Portions derived from ShellyScanner (https://github.com/usnasoft/shellyscanner),
// Copyright (C) Antonio Flaccomio / usnasoft, licensed under GPL-3.0:
// view/checklist (CheckListView, CheckListTable, DialogWiFiDevicesInfo,
// DialogBluDevicesInfo).
//
// The configuration checklist: per device eco mode, LED, logs, Bluetooth,
// access point, roaming, Wi-Fi, range extender, scripts and automatic
// firmware update, with actions to change them.
import { configApi, devicesApi, type BLEItem, type ChecklistRow, type Device } from '../api';
import { cellView, editable, FALSE, isBLU, isG1, isG2, sameBoolean, sameObject, sameStringOrInt, type Col } from '../checklistlogic';
import { addressText, allDevices, loadDevices, onDevicesChanged } from '../devices';
import { h, ICONS } from '../dom';
import { t, type Key } from '../i18n';
import { confirmDialog, openModal } from '../modal';
import { openDeviceSettings } from '../panels/devsettings';
import { toast } from '../toast';
import { card, emptyState, type Page } from './common';

const COLS: { key: Col; label: Key; tip: Key; good?: boolean }[] = [
  { key: 'eco', label: 'chk.col.eco', tip: 'chk.tip.eco', good: true },
  { key: 'led', label: 'chk.col.led', tip: 'chk.tip.led', good: true },
  { key: 'logs', label: 'chk.col.logs', tip: 'chk.tip.logs', good: false },
  { key: 'ble', label: 'chk.col.ble', tip: 'chk.tip.ble' },
  { key: 'ap', label: 'chk.col.ap', tip: 'chk.tip.ap' },
  { key: 'roaming', label: 'chk.col.roaming', tip: 'chk.tip.roaming', good: false },
  { key: 'wifi1', label: 'chk.col.wifi1', tip: 'chk.tip.wifi1', good: true },
  { key: 'wifi2', label: 'chk.col.wifi2', tip: 'chk.tip.wifi2', good: true },
  { key: 'extender', label: 'chk.col.extender', tip: 'chk.tip.extender' },
  { key: 'scripts', label: 'chk.col.scripts', tip: 'chk.tip.scripts' },
  { key: 'autoFW', label: 'chk.col.autoFW', tip: 'chk.tip.autoFW' },
];

// ---- dialogs ------------------------------------------------------------------------------

function bleDialog(row: ChecklistRow, dev: Device | undefined): void {
  const filter = h('input', { type: 'search', placeholder: t('filter.placeholder') });
  const table = (head: Key[], rows: string[][]): HTMLElement => h('table', { class: 'data' },
    h('thead', {}, h('tr', {}, ...head.map((k) => h('th', {}, t(k))))),
    h('tbody', {}, ...rows.map((r) => h('tr', {}, ...r.map((c) => h('td', {}, c))))));
  const items = Array.isArray(row.ble) ? (row.ble as BLEItem[]) : [];
  let content: () => HTMLElement;
  if (isBLU(row)) { // DialogBluDevicesInfo: BTHome hosts (parent + others) and gateways
    const hosts = [dev?.parent, ...(dev?.parents ?? [])].filter((x): x is string => !!x).map((p) => {
      const g = allDevices().find((d) => d.id === p || d.hostname === p);
      return [g ? g.name || g.hostname : p, g ? addressText(g) : ''];
    });
    const now = Date.now() / 1000;
    content = () => {
      const q = filter.value.toLowerCase();
      const f = (r: string[]): boolean => r.join(' ').toLowerCase().includes(q);
      return h('div', {},
        h('h3', {}, t('chk.ble.hosts')), table(['col.device', 'col.ip'], hosts.filter(f)),
        h('h3', {}, t('chk.ble.gateways')), table(['col.device', 'col.ip', 'chk.ble.lastSeen'],
          items.map((g) => [g.name ?? '', g.address ?? '', String(Math.max(0, Math.round(now - (g.lastSeen ?? 0))))]).filter(f)));
    };
  } else { // DialogWiFiDevicesInfo: the BLU devices this gateway relays
    content = () => {
      const q = filter.value.toLowerCase();
      return table(['col.device', 'col.mac'], items.map((b) => [b.name ?? '', b.mac ?? '']).sort((a, b) => a[1]!.localeCompare(b[1]!))
        .filter((r) => r.join(' ').toLowerCase().includes(q)));
    };
  }
  const box = h('div', {}, content());
  filter.addEventListener('input', () => box.replaceChildren(content()));
  openModal(t(isBLU(row) ? 'chk.ble.bluTitle' : 'chk.ble.wifiTitle'), h('div', {}, filter, box), [{ label: t('common.close') }]);
}

// ---- page -----------------------------------------------------------------------------------

function wantedIds(): string[] {
  const m = /[?&]ids=([^&]*)/.exec(location.hash);
  return m ? decodeURIComponent(m[1]!).split(',').filter(Boolean) : [];
}

export const checklistPage: Page = {
  id: 'checklist',
  title: 'nav.checklist',
  icon: ICONS.checklist,
  async render(main) {
    const ids = wantedIds();
    let rows: ChecklistRow[] = [];
    const selected = new Set<string>();
    let anchor: string | null = null;
    let filter = '';
    const status = h('div', { class: 'muted status-line' });
    const toolbar = h('div', { class: 'row' });
    const hint = h('p', { class: 'muted chk-hint' }, t('chk.actionsHint'));
    const body = h('div');
    const filterInput = h('input', { type: 'search', placeholder: t('filter.placeholder'), 'aria-label': t('filter.label') });
    filterInput.addEventListener('input', () => { filter = filterInput.value.toLowerCase(); draw(); });

    const sel = (): ChecklistRow[] => rows.filter((r) => selected.has(r.id));
    const devOf = (id: string): Device | undefined => allDevices().find((d) => d.id === id);

    const run = async (action: string, value: boolean, mode?: string, only?: ChecklistRow[]): Promise<void> => {
      const targets = only ?? sel();
      if (!targets.length) return;
      try {
        const res = await configApi.checklistAction(targets.map((r) => r.id), action, value, mode);
        for (const nr of res.rows) { const i = rows.findIndex((r) => r.id === nr.id); if (i >= 0) rows[i] = nr; }
        if (res.errors.length) openModal(t('chk.errors'), h('ul', {}, ...res.errors.map((e) => h('li', {}, `${e.name} - ${e.message ?? ''}`))), [{ label: t('common.close') }]);
      } catch (e) { toast(e instanceof Error ? e.message : String(e)); }
      draw();
    };
    // Rows appear at once with name and status and are filled in as each
    // device answers (CheckListView.fill: one task per device).
    let gen = 0;
    const reload = async (): Promise<void> => {
      const my = ++gen;
      if (!allDevices().length) await loadDevices().catch(() => {}); // page opened directly: the list is still loading
      const devs = (ids.length ? ids.map((id) => devOf(id)).filter((d): d is Device => !!d) : allDevices())
        .sort((a, b) => a.ip.localeCompare(b.ip, undefined, { numeric: true }));
      rows = devs.map((d) => ({ id: d.id, host: d.name && d.name !== d.hostname ? d.name + ' (' + d.hostname + ')' : d.hostname, address: addressText(d),
        status: d.status, gen: d.gen, eco: null, led: null, logs: null, ble: null, ap: null, roaming: null, wifi1: null, wifi2: null,
        extender: null, scripts: null, autoFW: null, loading: true }));
      draw();
      const queue = rows.map((r) => r.id);
      const worker = async (): Promise<void> => {
        for (let id = queue.shift(); id !== undefined && my === gen; id = queue.shift()) {
          try {
            const [nr] = await configApi.checklist([id]);
            const i = rows.findIndex((r) => r.id === id);
            if (nr && i >= 0 && my === gen) rows[i] = nr;
          } catch { /* the row stays empty, like a device that did not answer */ }
          const i = rows.findIndex((r) => r.id === id);
          if (i >= 0) delete (rows[i] as { loading?: boolean }).loading;
          if (my === gen) draw();
        }
      };
      await Promise.all(Array.from({ length: 6 }, worker));
    };
    const webUI = async (list: ChecklistRow[]): Promise<void> => {
      if (list.length > 8 && !(await confirmDialog(t('action.webUI'), t('action.webConfirm', { n: list.length }), t('action.webUI'), false))) return;
      for (const r of list) window.open(`http://${r.address.replace(/:80$/, '')}`, '_blank', 'noopener');
    };
    const reboot = async (list: ChecklistRow[]): Promise<void> => {
      if (!(await confirmDialog(t('action.reboot'), t('action.rebootConfirm'), t('action.reboot')))) return;
      try { await devicesApi.reboot(list.map((r) => r.id)); } catch (e) { toast(e instanceof Error ? e.message : String(e)); }
    };

    /** The actions, with their enabled state for the selection. */
    const actions = (): { key: string; label: Key; tip?: Key; enabled: boolean; run?: () => void; menu?: { label: Key; run: () => void }[] }[] => {
      const s = sel();
      const g1Logs = sameBoolean(s, 'logs', isG1);
      const g2Logs = !g1Logs ? sameObject(s, 'logs') : undefined;
      const logsVal = s[0]?.logs;
      return [
        { key: 'eco', label: 'chk.act.eco', tip: 'chk.act.ecoTip', enabled: sameBoolean(s, 'eco'), run: () => void run('eco', !s[0]!.eco) },
        { key: 'led', label: 'chk.act.led', tip: 'chk.act.ledTip', enabled: sameBoolean(s, 'led', isG1), run: () => void run('led', !s[0]!.led) },
        g2Logs !== undefined
          ? { key: 'logs', label: 'chk.act.logs', tip: 'chk.act.logsTip', enabled: true, menu: [
            { label: 'chk.act.socket', run: () => void run('logs', !String(logsVal).includes('socket'), 'socket') },
            { label: 'chk.act.mqtt', run: () => void run('logs', !String(logsVal).includes('mqtt'), 'mqtt') }] }
          : { key: 'logs', label: 'chk.act.logs', tip: 'chk.act.logsTip', enabled: g1Logs, run: () => void run('logs', !s[0]!.logs) },
        { key: 'ble', label: 'chk.act.ble', tip: 'chk.act.bleTip', enabled: s.length === 1 && (isG2(s[0]!) || isBLU(s[0]!)) && s[0]!.ble !== null,
          run: () => bleDialog(s[0]!, devOf(s[0]!.id)) },
        { key: 'ap', label: 'chk.act.ap', tip: 'chk.act.apTip', enabled: sameBoolean(s, 'ap', isG2), run: () => void run('ap', s[0]!.ap !== true) },
        { key: 'roaming', label: 'chk.act.roaming', tip: 'chk.act.roamingTip', enabled: sameStringOrInt(s, 'roaming'), run: () => void run('roaming', s[0]!.roaming === FALSE) },
        { key: 'extender', label: 'chk.act.extender', tip: 'chk.act.extenderTip', enabled: sameStringOrInt(s, 'extender'), run: () => void run('extender', s[0]!.extender === FALSE) },
        { key: 'autoFW', label: 'chk.act.autoFW', tip: 'chk.act.autoFWTip', enabled: editable(s, 'autoFW'), menu: [
          { label: 'chk.act.stable', run: () => void run('autofw', true, 'stable') },
          { label: 'chk.act.beta', run: () => void run('autofw', true, 'beta') },
          { label: 'chk.act.none', run: () => void run('autofw', false, 'none') }] },
        { key: 'web', label: 'action.webUI', enabled: s.length > 0, run: () => void webUI(s) },
        { key: 'reboot', label: 'action.reboot', enabled: s.length > 0 && s.every((r) => r.gen !== 'bth' && r.status !== 'ghost'), run: () => void reboot(s) },
      ];
    };

    const menu = (items: { label: string; run: () => void }[], x: number, y: number): void => {
      document.querySelectorAll('.ctx-menu').forEach((m) => m.remove());
      const m = h('div', { class: 'menu ctx-menu', role: 'menu' }, ...items.map((it) =>
        h('button', { role: 'menuitem', onclick: () => { m.remove(); it.run(); } }, it.label)));
      m.style.left = `${x}px`;
      m.style.top = `${y}px`;
      document.body.append(m);
      const away = (): void => { m.remove(); document.removeEventListener('click', away); };
      setTimeout(() => document.addEventListener('click', away), 0);
    };

    const draw = (): void => {
      const acts = actions();
      const button = (a: (typeof acts)[number]): HTMLElement => {
        const b = h('button', { class: 'btn', disabled: !a.enabled }, t(a.label), a.menu ? ' ▾' : '');
        b.addEventListener('click', (e) => {
          if (a.menu) { const r = b.getBoundingClientRect(); menu(a.menu.map((m) => ({ label: t(m.label), run: m.run })), r.left, r.bottom); e.stopPropagation(); } else a.run?.();
        });
        // The tooltip sits on a wrapper: browsers show none on a disabled button.
        return a.tip ? h('span', { class: 'tip-wrap', title: t(a.tip) }, b) : b;
      };
      const settings = acts.filter((a) => a.key !== 'web' && a.key !== 'reboot');
      toolbar.replaceChildren(
        h('button', { class: 'btn', onclick: () => void reload(), title: t('action.refreshTip') }, t('action.refresh')),
        h('span', { class: 'toolbar-sep', 'aria-hidden': 'true' }),
        h('span', { class: 'toolbar-label muted' }, t('chk.actionsLabel')),
        ...settings.map(button),
        h('span', { class: 'toolbar-sep', 'aria-hidden': 'true' }),
        ...acts.filter((a) => a.key === 'web' || a.key === 'reboot').map(button));
      hint.hidden = selected.size > 0;
      const shown = rows.filter((r) => !filter || `${r.host} ${r.address}`.toLowerCase().includes(filter));
      status.textContent = t('status.listed', { total: rows.length, selected: selected.size });
      if (!rows.length) { body.replaceChildren(emptyState(t('devices.empty.title'), t('devices.empty.text'), ICONS.checklist)); return; }
      const tbody = h('tbody');
      for (const r of shown) {
        const tr = h('tr', { class: selected.has(r.id) ? 'selected' : '' },
          h('td', {}, h('span', { class: 'pill ' + r.status }, t(('status.' + r.status) as Key))),
          h('td', {}, r.host, (r as { loading?: boolean }).loading ? h('span', { class: 'spinner small', 'aria-label': t('state.loading') }) : null),
          h('td', { class: 'mono' }, r.address.replace(/:80$/, '')),
          ...COLS.map((c) => { const v = cellView(c.key, r[c.key], c.good); return h('td', { class: 'chk ' + v.cls }, v.text); }));
        tr.addEventListener('click', (e) => {
          if (e.shiftKey && anchor) {
            const list = shown.map((x) => x.id);
            const [a, b] = [list.indexOf(anchor), list.indexOf(r.id)].sort((x, y) => x - y);
            for (const id of list.slice(a!, b! + 1)) selected.add(id);
          } else if (e.ctrlKey || e.metaKey) {
            if (selected.has(r.id)) selected.delete(r.id); else selected.add(r.id);
            anchor = r.id;
          } else { selected.clear(); selected.add(r.id); anchor = r.id; }
          draw();
        });
        tr.addEventListener('dblclick', () => void webUI([r]));
        tr.addEventListener('contextmenu', (e) => { // popup: the column's action, then Web UI and Reboot
          e.preventDefault();
          if (!selected.has(r.id)) { selected.clear(); selected.add(r.id); draw(); }
          const td = (e.target as HTMLElement).closest('td');
          const idx = td ? [...td.parentElement!.children].indexOf(td) - 3 : -1;
          const col = COLS[idx]?.key;
          const all = actions();
          const items: { label: string; run: () => void }[] = [];
          const a = all.find((x) => x.key === col);
          if (col === 'wifi1' || col === 'wifi2') {
            items.push({ label: `${t('chk.edit')} (${t(COLS[idx]!.label)})`, run: () => openDeviceSettings(sel().map((x) => x.id), col, () => void reload()) });
          } else if (a && a.enabled) {
            if (a.menu) items.push(...a.menu.map((m) => ({ label: t(m.label), run: m.run })));
            else if (a.run) items.push({ label: t(a.label), run: a.run });
          }
          for (const k of ['web', 'reboot']) { const x = all.find((y) => y.key === k)!; if (x.enabled) items.push({ label: t(x.label), run: x.run! }); }
          menu(items, e.clientX, e.clientY);
        });
        tbody.append(tr);
      }
      body.replaceChildren(h('div', { class: 'table-wrap' }, h('table', { class: 'data checklist' },
        h('thead', {}, h('tr', {}, h('th', {}, ''), h('th', {}, t('col.device')), h('th', {}, t('col.ip')),
          ...COLS.map((c) => h('th', { title: t(c.tip) }, t(c.label))))), tbody)));
    };

    // A device that comes back on line is read again (CheckListView.update).
    const off = onDevicesChanged(() => {
      let refetch: string[] = [];
      for (const r of rows) {
        const d = devOf(r.id);
        if (d && d.status !== r.status) {
          if (d.status === 'online' && r.status !== 'online') refetch.push(r.id);
          r.status = d.status;
        }
      }
      if (refetch.length) void configApi.checklist(refetch).then((nr) => { for (const x of nr) { const i = rows.findIndex((r) => r.id === x.id); if (i >= 0) rows[i] = x; } draw(); });
      else draw();
      refetch = [];
    });
    disposeFn = off;
    main.append(card(t('chk.title'), null, h('div', { class: 'toolbar' }, toolbar, h('div', { class: 'spacer' }), filterInput), hint, status, body));
    void reload();
  },
  dispose() { disposeFn(); document.querySelectorAll('.ctx-menu').forEach((m) => m.remove()); },
};

let disposeFn = (): void => {};
