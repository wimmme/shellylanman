// Portions derived from ShellyScanner (https://github.com/usnasoft/shellyscanner),
// Copyright (C) Antonio Flaccomio / usnasoft, licensed under GPL-3.0:
// view/devsettings/PanelFWUpdate and FWUpdateTable.
//
// "FW Update": current, new stable and new beta firmware per device, with a
// checkbox per version, the select buttons, a filter, "Check" and the update
// of the ticked devices. Used as the first tab of Devices settings and on the
// Firmware page.
import { ApiError, firmwareApi, localFwApi, type FirmwareRow, type IndexRow } from '../api';
import { allDevices } from '../devices';
import { h } from '../dom';
import { betaCell, count, initialChoice, matches, requests, selectAll, stableCell, toggle, type Cell, type Choice } from '../firmwarelogic';
import { t, type Key } from '../i18n';
import { confirmDialog } from '../modal';
import type { EventSocket } from '../socket';
import { showResults } from './devsettings';
import { indexCell } from './localfw';

const tr = (k: string, v?: Record<string, number>): string => t(k as Key, v);

// Row changes pushed by the server (progress, rebooting, checked again after the restart).
const rowListeners = new Set<(r: FirmwareRow) => void>();
export function wireFirmwareEvents(socket: EventSocket): void {
  socket.on('firmware.row', (ev) => { const r = ev.data as FirmwareRow; rowListeners.forEach((l) => l(r)); });
}

export interface FirmwarePanel {
  el: HTMLElement;
  /** Update the ticked devices after a confirmation; false when nothing was sent. */
  apply(): Promise<boolean>;
  dispose(): void;
}

/**
 * The panel for the given devices (all devices when ids is empty). withIndex
 * adds the "Shelly index" column with the local download (Firmware page only).
 */
export function firmwarePanel(ids: string[], withIndex = false): FirmwarePanel {
  let rows: FirmwareRow[] = [];
  let index = new Map<string, IndexRow>();
  let indexLoading = withIndex;
  const choice = new Map<string, Choice>();
  const checking = new Set<string>();
  let filter = '';

  const tbody = h('tbody');
  const counter = h('span', { class: 'muted' });
  const filterInput = h('input', { type: 'search', placeholder: t('filter.placeholder'), 'aria-label': t('filter.label'), size: 12 });
  const checkBtn = h('button', { class: 'btn', disabled: true }, t('fw.check'));
  const body = h('div', { class: 'fw-body' }, h('div', { class: 'spinner' }));

  const setRow = (r: FirmwareRow, preselect: boolean): void => {
    const i = rows.findIndex((x) => x.id === r.id);
    if (i < 0) return;
    rows[i] = r;
    if (preselect || r.updating) choice.set(r.id, initialChoice(r));
  };

  const cellNode = (r: FirmwareRow, cell: Cell, col: 'stable' | 'beta'): Node => {
    if (cell.type === 'empty') return h('input', { type: 'checkbox', disabled: true, 'aria-label': t(col === 'stable' ? 'fw.col.stable' : 'fw.col.beta') });
    if (cell.type === 'text') return document.createTextNode(cell.text);
    const box = h('input', { type: 'checkbox', checked: choice.get(r.id) === col });
    box.addEventListener('change', () => { choice.set(r.id, toggle(choice.get(r.id) ?? null, col, box.checked)); draw(); });
    const label = h('label', { class: 'row fw-check' }, box, cell.label);
    if (cell.tip) label.title = cell.tip;
    return label;
  };

  const draw = (): void => {
    const shown = rows.filter((r) => matches(r, filter));
    tbody.replaceChildren(...shown.map((r) => {
      const busy = checking.has(r.id) || !!r.updating;
      const status = r.rebooting ? 'offline' : busy ? 'reading' : r.status;
      const tr_ = h('tr', {},
        h('td', {}, h('span', { class: 'pill ' + status }, t(('status.' + status) as Key))),
        h('td', {}, r.name),
        h('td', { title: r.currentBuild ?? '' }, r.current ?? ''),
        h('td', {}, cellNode(r, stableCell(r, tr), 'stable')),
        h('td', {}, cellNode(r, betaCell(r), 'beta')),
        withIndex ? h('td', {}, indexCell(index.get(r.id), indexLoading)) : null);
      tr_.addEventListener('dblclick', () => { // browseAction
        const d = allDevices().find((x) => x.id === r.id);
        if (d && d.status !== 'ghost' && d.gen !== 'blu' && d.gen !== 'bth') window.open(`http://${d.port === 80 ? d.ip : `${d.ip}:${d.port}`}`, '_blank', 'noopener');
      });
      return tr_;
    }));
    const c = count(rows, choice, filter);
    counter.textContent = t('fw.count', { stable: c.stable, beta: c.beta });
  };

  const select = (what: 'stable' | 'beta' | 'none'): void => {
    for (const r of rows) if (matches(r, filter)) choice.set(r.id, selectAll(r, choice.get(r.id) ?? null, what));
    draw();
  };

  const load = async (): Promise<void> => {
    checkBtn.disabled = true;
    try {
      rows = await firmwareApi.rows(ids);
      rows.sort((a, b) => a.name.localeCompare(b.name));
      choice.clear();
      for (const r of rows) choice.set(r.id, initialChoice(r));
      body.replaceChildren(h('div', { class: 'table-wrap' }, h('table', { class: 'data fw' },
        h('thead', {}, h('tr', {}, ...(['col.status', 'col.device', 'fw.col.current', 'fw.col.stable', 'fw.col.beta', ...(withIndex ? ['lfw.col'] : [])] as Key[]).map((k) => h('th', { scope: 'col', title: k === 'lfw.col' ? t('lfw.colTip') : undefined }, t(k))))),
        tbody)));
      draw();
      if (withIndex) void loadIndex();
    } catch (e) {
      body.replaceChildren(h('p', { class: 'banner warn' }, e instanceof ApiError ? e.message : String(e)));
    } finally {
      checkBtn.disabled = false;
    }
  };

  // The server-side comparison with Shelly's index (cached there for hours).
  const loadIndex = async (): Promise<void> => {
    indexLoading = true;
    try {
      index = new Map((await localFwApi.index(ids)).map((r) => [r.id, r]));
    } catch { /* the column stays empty: the rest of the page works without it */ }
    indexLoading = false;
    draw();
  };

  // "Check": every row is read again, each as soon as it is ready.
  checkBtn.addEventListener('click', async () => {
    checkBtn.disabled = true;
    const list = rows.filter((r) => r.known && !r.updating);
    list.forEach((r) => checking.add(r.id));
    draw();
    await Promise.all(list.map(async (r) => {
      try {
        const [fresh] = await firmwareApi.rows([r.id]);
        if (fresh) setRow(fresh, true);
      } catch { /* keep the old row */ }
      checking.delete(r.id);
      draw();
    }));
    checkBtn.disabled = false;
  });
  filterInput.addEventListener('input', () => { filter = filterInput.value; draw(); });

  const onRow = (r: FirmwareRow): void => {
    if (!rows.some((x) => x.id === r.id)) return;
    // A progress event carries no versions: keep the ones we have.
    const old = rows.find((x) => x.id === r.id)!;
    setRow(r.valid || !r.known ? r : { ...old, updating: r.updating, progress: r.progress, rebooting: r.rebooting, status: r.status }, !r.updating);
    draw();
  };
  rowListeners.add(onRow);

  const el = h('div', { class: 'fw-panel' }, body,
    h('div', { class: 'toolbar' },
      h('button', { class: 'btn', onclick: () => select('none') }, t('fw.deselect')),
      h('button', { class: 'btn', onclick: () => select('stable') }, t('fw.selectStable')),
      h('button', { class: 'btn', onclick: () => select('beta') }, t('fw.selectBeta')),
      counter, h('div', { class: 'spacer' }), filterInput, checkBtn));
  void load();

  return {
    el,
    async apply() {
      const items = requests(rows, choice, filter);
      if (items.length === 0) return false;
      if (!(await confirmDialog(t('fw.title'), t('fw.confirm', { n: items.length }), t('fw.update')))) return false;
      const res = await firmwareApi.update(items);
      const problems = res.results.filter((l) => l.result !== 'ok');
      if (problems.length) showResults(t('fw.title'), problems);
      // fillTable(false): redrawn without ticks; updating rows then follow the server's events.
      for (const l of res.results) {
        const i = rows.findIndex((r) => r.id === l.id);
        if (i < 0) continue;
        choice.set(l.id, null);
        if (l.result === 'ok') rows[i] = { ...rows[i]!, updating: true, progress: -1 };
        if (l.result === 'queued' && !rows[i]!.known) rows[i] = { ...rows[i]!, queued: true };
      }
      draw();
      return true;
    },
    dispose() { rowListeners.delete(onRow); },
  };
}
