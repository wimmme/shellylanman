// Portions derived from ShellyScanner (https://github.com/usnasoft/shellyscanner),
// Copyright (C) Antonio Flaccomio / usnasoft, licensed under GPL-3.0:
// view/chart/MeasuresChart and TimeChartsExporter.
//
// Charts of the devices chosen on the devices page (#/charts?ids=a,b). The
// server keeps the readings, so a chart starts with history; new readings
// arrive with the device updates. Range, graph type, series, markers, pause,
// zoom (drag, wheel while paused, Ctrl+R / Ctrl+P), CSV export, image copy.
import { ApiError, chartsApi, type Device } from '../api';
import {
  availableTypes, emSeriesName, horizontalCSV, seriesFor, valueOf, verticalCSV, type ChartType, type SampleLike, type Series, type SeriesDef,
} from '../chartlogic';
import { download } from '../csv';
import { allDevices, loadDevices, onDevicesChanged } from '../devices';
import { h, ICONS } from '../dom';
import { prefs } from '../format';
import { t, type Key } from '../i18n';
import { toast } from '../toast';
import { card, emptyState, type Page } from './common';

const RANGES = [0, 1, 5, 15, 30, 60]; // minutes; 0 = auto
const PALETTE = ['#38bdf8', '#f472b6', '#4ade80', '#f59f00', '#a78bfa', '#f87171', '#34d399', '#fbbf24', '#60a5fa', '#e879f9'];

function wantedIds(): string[] {
  const m = /[?&]ids=([^&]*)/.exec(location.hash);
  return m ? decodeURIComponent(m[1]!).split(',').filter(Boolean) : [];
}

const hms = (ms: number): string => new Date(ms).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });

let dispose = (): void => {};

export const chartsPage: Page = {
  id: 'charts',
  title: 'nav.charts',
  icon: ICONS.charts,
  async render(main) {
    const ids = wantedIds();
    if (!allDevices().length) await loadDevices().catch(() => {});
    const devs = (): Device[] => ids.map((id) => allDevices().find((d) => d.id === id)).filter((d): d is Device => !!d && d.status !== 'ghost');
    if (!ids.length || !devs().length) {
      main.append(card(t('nav.charts'), null, emptyState(t('chart.none.title'), t('chart.none.text'), ICONS.charts)));
      return;
    }
    const { Chart } = await import('../charts/chartjs');
    const fahrenheit = prefs.temp() === 'F';
    const types = availableTypes(devs());
    let type: ChartType = types.includes(prefs.chartDefault() as ChartType) ? (prefs.chartDefault() as ChartType) : types[0]!;
    let defs: SeriesDef[] = [];
    let data: [number, number][][] = [];
    let paused = false, markers = false, rangeMin = 0, showOnly = -1;
    const lastT = new Map<string, number>();
    const emState = new Map<string, { local: number; remote: number }>();

    const typeSel = h('select', { 'aria-label': t('chart.type') }, ...types.map((x) => h('option', { value: x }, t(('chart.t.' + x) as Key))));
    typeSel.value = type;
    const rangeSel = h('select', { 'aria-label': t('chart.range') }, ...RANGES.map((r, i) => h('option', { value: i }, t(('chart.range.' + r) as Key))));
    const seriesSel = h('select', { 'aria-label': t('chart.series') });
    const pauseBtn = h('button', { class: 'btn', 'aria-pressed': 'false', title: t('chart.pauseTip') }, '⏸');
    const markBtn = h('button', { class: 'btn', 'aria-pressed': 'false', title: t('chart.markersTip') }, '◆');
    const canvas = h('canvas', { 'aria-label': t('nav.charts') });

    const yLabel = (): string => (type === 'INT_TEMP' || type === 'T_ALL' ? t(fahrenheit ? 'chart.y.TF' : 'chart.y.T') : t(('chart.y.' + type) as Key));
    const chart = new Chart(canvas, {
      type: 'line',
      data: { datasets: [] },
      options: {
        animation: false, parsing: false, normalized: true, maintainAspectRatio: false,
        interaction: { mode: 'nearest', intersect: false },
        scales: {
          x: { type: 'linear', title: { display: true, text: t('chart.x') }, ticks: { callback: (v) => hms(Number(v)), maxRotation: 0 } },
          y: { type: 'linear', title: { display: true, text: '' }, ticks: { callback: (v) => Number(v).toFixed(2) } },
        },
        plugins: {
          legend: { position: 'bottom' },
          tooltip: { callbacks: { title: (items) => (items[0] ? new Date(items[0].parsed.x ?? 0).toLocaleTimeString([], { hour12: false, fractionalSecondDigits: 3 } as Intl.DateTimeFormatOptions) : ''), label: (c) => `${c.dataset.label}: ${Number(c.parsed.y).toFixed(2)}` } },
          zoom: {
            pan: { enabled: true, mode: 'x', onPanStart: () => { setPaused(true); return true; } },
            zoom: { drag: { enabled: true }, wheel: { enabled: false }, mode: 'x', onZoomComplete: () => setPaused(true) },
          },
        },
      },
    });

    const setPaused = (p: boolean): void => {
      paused = p;
      pauseBtn.setAttribute('aria-pressed', String(p));
      pauseBtn.textContent = p ? '▶' : '⏸';
      const z = chart.options.plugins!.zoom!;
      z.zoom!.wheel!.enabled = p; // the wheel zooms only while paused
      if (!p) { chart.resetZoom('none'); applyRange(); }
    };
    const applyRange = (): void => { // setRange
      const x = chart.options.scales!.x!;
      if (paused) return;
      const newest = Math.max(0, ...data.flatMap((d) => (d.length ? [d[d.length - 1]![0]] : [])));
      if (RANGES[rangeMin] && newest) { x.min = newest - RANGES[rangeMin]! * 60_000; x.max = newest; } else { delete x.min; delete x.max; }
    };
    const redraw = (): void => {
      chart.data.datasets = defs.map((s, i) => ({
        label: s.name, data: data[i]!.map(([x, y]) => ({ x, y })), borderColor: PALETTE[i % PALETTE.length], backgroundColor: PALETTE[i % PALETTE.length],
        borderWidth: 1.5, pointRadius: markers ? 2.5 : 0, hidden: showOnly >= 0 && showOnly !== i, tension: 0,
      }));
      chart.options.scales!.y!.title!.text = yLabel();
      applyRange();
      chart.update('none');
    };
    const push = (i: number, x: number, y: number): void => { // addOrUpdate
      const d = data[i]!;
      if (d.length && d[d.length - 1]![0] === x) d[d.length - 1]![1] = y;
      else if (!d.length || d[d.length - 1]![0] < x) d.push([x, y]);
      else { const j = d.findIndex(([t]) => t >= x); if (d[j]![0] === x) d[j]![1] = y; else d.splice(j, 0, [x, y]); }
    };
    const addSample = (s: SampleLike, deviceId: string): void => {
      defs.forEach((def, i) => {
        if (def.deviceId !== deviceId) return;
        const v = valueOf(type, def, s, fahrenheit);
        if (v !== null) push(i, s.t, v);
      });
    };
    const fillSeries = (): void => {
      seriesSel.replaceChildren(h('option', { value: -1 }, t('chart.showAll')), ...defs.map((s, i) => h('option', { value: i }, s.name)));
      showOnly = -1;
    };

    // EM: energy records read from the device (the last 30 minutes, then every minute)
    const loadEM = async (): Promise<void> => {
      const now = Math.floor(Date.now() / 1000);
      for (const d of devs()) {
        const st = emState.get(d.id) ?? { local: 0, remote: 0 };
        if (st.local && Date.now() < st.local + 60_000) continue;
        st.local = Date.now();
        emState.set(d.id, st);
        const start = st.remote === 0 ? now - 60 * 30 : st.remote + 60;
        try {
          const list = await chartsApi.emdata(d.id, start, start + 60 * 60);
          list.forEach((em, mi) => {
            for (let line = 0; line < em.lines; line++) {
              const name = emSeriesName(d, em.meter, line, em.lines, mi);
              let i = defs.findIndex((s) => s.deviceId === d.id && s.name === name);
              if (i < 0) {
                const placeholder = defs.findIndex((s) => s.deviceId === d.id && s.line === undefined && !data[defs.indexOf(s)]!.length);
                if (placeholder >= 0) { defs.splice(placeholder, 1); data.splice(placeholder, 1); }
                defs.push({ deviceId: d.id, name, line });
                data.push([]);
                i = defs.length - 1;
                fillSeries();
              }
              for (let k = line; k < em.data.length; k += em.lines) { const [ts, wh] = em.data[k]!; push(i, ts * 1000, wh); st.remote = Math.max(st.remote, ts); }
            }
          });
        } catch { /* EM-getData: ignored like the original */ }
      }
      redraw();
    };

    const init = async (): Promise<void> => { // initDataSet
      defs = seriesFor(type, devs());
      data = defs.map(() => []);
      emState.clear();
      fillSeries();
      if (type === 'EM') { await loadEM(); return; }
      try {
        const hist = await chartsApi.samples(devs().map((d) => d.id));
        for (const [id, list] of Object.entries(hist)) { for (const s of list) addSample(s, id); lastT.set(id, list.length ? list[list.length - 1]!.t : 0); }
      } catch (e) { toast(e instanceof ApiError ? e.message : String(e)); }
      redraw();
    };
    const onUpdate = (): void => { // update(UPDATE): readings of on-line devices
      if (type === 'EM') { void loadEM(); return; }
      let changed = false;
      for (const d of devs()) {
        if (d.status !== 'online' || !d.lastSeen || d.lastSeen <= (lastT.get(d.id) ?? 0)) continue;
        lastT.set(d.id, d.lastSeen);
        addSample({ t: d.lastSeen, rssi: d.rssi, temp: d.internalTemp, meters: d.meters }, d.id);
        changed = true;
      }
      if (changed && !paused) redraw();
    };

    typeSel.addEventListener('change', () => { type = typeSel.value as ChartType; setPaused(false); void init(); });
    rangeSel.addEventListener('change', () => { rangeMin = Number(rangeSel.value); setPaused(false); redraw(); });
    seriesSel.addEventListener('change', () => { showOnly = Number(seriesSel.value); redraw(); });
    pauseBtn.addEventListener('click', () => setPaused(!paused));
    markBtn.addEventListener('click', () => { markers = !markers; markBtn.setAttribute('aria-pressed', String(markers)); redraw(); });
    const series = (): Series[] => defs.map((s, i) => ({ name: s.name, points: data[i]! }));
    const exportCSV = (): void => {
      const x = chart.scales.x!;
      const range: [number, number] | null = paused ? [x.min, x.max] : null;
      const text = prefs.chartExport() === 'V'
        ? verticalCSV(series(), yLabel(), prefs.csvSeparator(), range, { unix: t('chart.csv.unix'), time: t('chart.csv.time') })
        : horizontalCSV(series(), prefs.csvSeparator(), range);
      download('shellylanman-chart.csv', text);
    };
    const copyImage = (): void => {
      canvas.toBlob(async (b) => {
        if (!b) return;
        try { await navigator.clipboard.write([new ClipboardItem({ 'image/png': b })]); toast(t('chart.copied'), 'info'); } catch {
          const url = URL.createObjectURL(b); const a = h('a', { href: url, download: 'shellylanman-chart.png' }); a.click(); setTimeout(() => URL.revokeObjectURL(url), 1000);
        }
      });
    };
    const clear = async (): Promise<void> => { await chartsApi.clear(devs().map((d) => d.id)).catch(() => {}); lastT.clear(); await init(); };
    const help = h('details', { class: 'chart-help' }, h('summary', {}, t('chart.help.title')), ...t('chart.help').split('\n').map((l) => h('p', {}, l)));

    const onKey = (e: KeyboardEvent): void => {
      if (!(e.ctrlKey || e.metaKey)) return;
      const k = e.key.toLowerCase();
      if (k === 'r') { e.preventDefault(); rangeSel.value = String((rangeMin + 1) % RANGES.length); rangeSel.dispatchEvent(new Event('change')); }
      if (k === 'p') { e.preventDefault(); setPaused(!paused); }
      if (k === 'c' && !window.getSelection()?.toString()) { e.preventDefault(); copyImage(); }
    };
    document.addEventListener('keydown', onKey);
    const off = onDevicesChanged(onUpdate);
    const emTimer = setInterval(() => { if (type === 'EM') void loadEM(); }, 60_000);
    dispose = (): void => { off(); clearInterval(emTimer); document.removeEventListener('keydown', onKey); chart.destroy(); };

    const title = devs().length === 1 ? t('chart.title1', { device: devs()[0]!.name || devs()[0]!.hostname }) : t('chart.titleMany', { n: devs().length });
    main.append(card(title, null,
      h('div', { class: 'toolbar chart-toolbar' },
        h('label', { class: 'row' }, t('chart.range'), rangeSel), h('label', { class: 'row' }, t('chart.type'), typeSel),
        h('label', { class: 'row' }, t('chart.series'), seriesSel), markBtn, pauseBtn,
        h('button', { class: 'btn', title: t('chart.csvTip'), onclick: exportCSV }, t('action.csv')),
        h('button', { class: 'btn', title: t('chart.copyTip'), onclick: copyImage }, t('common.copy')),
        h('div', { class: 'spacer' }),
        h('button', { class: 'btn', onclick: () => void clear() }, t('chart.clear'))),
      h('div', { class: 'chart-box' }, canvas), help));
    await init();
  },
  dispose() { dispose(); dispose = () => {}; },
};
