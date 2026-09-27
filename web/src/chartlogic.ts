// Portions derived from ShellyScanner (https://github.com/usnasoft/shellyscanner),
// Copyright (C) Antonio Flaccomio / usnasoft, licensed under GPL-3.0:
// view/chart/ChartType, MeasuresChart (typeComboContent, initDataSet,
// newValue, uniqueName), TimeChartsExporter and UtilMiscellaneous.getDescName.
//
// Chart data without DOM or Chart.js, so it can be tested.
import type { Device, MeterSet } from './api';

export type ChartType = 'INT_TEMP' | 'RSSI' | 'P' | 'P_SUM' | 'Q' | 'S' | 'V' | 'VL' | 'VX' | 'I' | 'T_ALL' | 'H' | 'HD' | 'LUX' | 'FREQ' | 'DIST' | 'EM';

/** ChartType order and the meter type each one reads. */
export const CHART_TYPES: { type: ChartType; meter?: string }[] = [
  { type: 'INT_TEMP' }, { type: 'RSSI' }, { type: 'P', meter: 'W' }, { type: 'P_SUM' }, { type: 'Q', meter: 'VAR' }, { type: 'S', meter: 'VA' },
  { type: 'V', meter: 'V' }, { type: 'VL', meter: 'VL' }, { type: 'VX', meter: 'VX' }, { type: 'I', meter: 'I' }, { type: 'T_ALL', meter: 'T' },
  { type: 'H', meter: 'H' }, { type: 'HD', meter: 'HD' }, { type: 'LUX', meter: 'L' }, { type: 'FREQ', meter: 'FREQ' }, { type: 'DIST', meter: 'DMM' }, { type: 'EM' },
];

const T_TYPES = ['T', 'T1', 'T2', 'T3', 'T4'];

/** EMHolder devices: Pro 3EM / 3EM-63 (EMData or EM1Data), Pro EM-50, EM G3/G4 (EM1Data). */
export function hasEM(d: Device): boolean {
  return /^(Pro3EM|S3EMG3|ProEM|EMG3|S4EM)/.test(d.typeId);
}

/** typeComboContent: the chart types that apply to at least one device; RSSI always. */
export function availableTypes(devs: Device[]): ChartType[] {
  const set = new Set<ChartType>(['RSSI']);
  for (const d of devs) {
    let powerFound = false;
    for (const ms of d.meters ?? []) {
      for (const v of ms.values) {
        if (v.type === 'W') {
          set.add('P');
          if (powerFound) set.add('P_SUM');
          powerFound = true;
        } else {
          const t = CHART_TYPES.find((c) => c.meter === v.type || (c.type === 'T_ALL' && T_TYPES.includes(v.type)));
          if (t) set.add(t.type);
        }
      }
    }
    if (hasEM(d)) set.add('EM');
    if (d.internalTemp !== undefined && d.internalTemp !== null) set.add('INT_TEMP');
  }
  return CHART_TYPES.map((c) => c.type).filter((t) => set.has(t));
}

/** getDescName(d): the name, else the host name. */
export function descName(d: Device): string {
  return d.name || d.hostname;
}

/** getDescName(d, channel): the relay name of that channel, else "-n". */
export function descNameCh(d: Device, ch: number): string {
  const m = d.modules?.[ch];
  if (m && (m.kind === 'relay' || m.kind === 'switch') && m.label) return d.name ? `${d.name}-${m.label}` : m.label;
  return ch === 0 ? descName(d) : `${descName(d)}-${ch + 1}`;
}

function descNameLabel(d: Device, label: string | undefined, ch: number): string {
  return label ? `${descName(d)}-${label}` : descNameCh(d, ch);
}

/** uniqueName: "x", "x(1)", "x(2)" … */
export function uniqueName(taken: Set<string>, name: string): string {
  let n = name;
  for (let i = 1; taken.has(n); i++) n = `${name}(${i})`;
  taken.add(n);
  return n;
}

export interface SeriesDef { deviceId: string; name: string; meter?: number; meterType?: string; line?: number }

/** initDataSet: the series of one chart type for the devices (a device without values still gets a legend entry). */
export function seriesFor(type: ChartType, devs: Device[]): SeriesDef[] {
  const taken = new Set<string>();
  const out: SeriesDef[] = [];
  for (const d of devs) {
    const own: SeriesDef[] = [];
    const meters = d.meters ?? [];
    if (type === 'T_ALL') {
      meters.forEach((ms, i) => {
        for (const tt of T_TYPES) {
          const v = ms.values.find((x) => x.type === tt);
          if (!v) continue;
          const suffix = v.name ? '-' + v.name : tt === 'T' ? '' : '-' + tt.slice(1);
          own.push({ deviceId: d.id, name: uniqueName(taken, descName(d) + suffix), meter: i, meterType: tt });
        }
      });
    } else if (type === 'EM') {
      // filled from the EM records; one legend entry until they arrive
    } else if (type === 'INT_TEMP' || type === 'RSSI' || type === 'P_SUM') {
      own.push({ deviceId: d.id, name: uniqueName(taken, descName(d)) });
    } else {
      const mt = CHART_TYPES.find((c) => c.type === type)!.meter!;
      meters.forEach((ms, i) => {
        if (ms.total) return;
        const v = ms.values.find((x) => x.type === mt);
        if (!v) return;
        const name = v.name ? `${descName(d)}-${v.name}` : descNameLabel(d, ms.label, i);
        own.push({ deviceId: d.id, name: uniqueName(taken, name), meter: i, meterType: mt });
      });
    }
    if (own.length === 0) own.push({ deviceId: d.id, name: uniqueName(taken, descName(d)) });
    out.push(...own);
  }
  return out;
}

export interface SampleLike { t: number; rssi: number; temp?: number; meters?: MeterSet[] }

const toF = (c: number): number => c * 1.8 + 32;

/** newValue: the value of one series in one reading (null: none). */
export function valueOf(type: ChartType, s: SeriesDef, sample: SampleLike, fahrenheit: boolean): number | null {
  if (type === 'INT_TEMP') return sample.temp === undefined || sample.temp === null ? null : fahrenheit ? toF(sample.temp) : sample.temp;
  if (type === 'RSSI') return sample.rssi;
  const meters = sample.meters ?? [];
  if (type === 'P_SUM') {
    let found = false, sum = 0;
    for (const ms of meters) {
      const w = ms.values.find((v) => v.type === 'W');
      if (!w) continue;
      found = true;
      if (ms.total) { sum = w.value; break; }
      sum += w.value;
    }
    return found ? sum : null;
  }
  if (s.meter === undefined || !s.meterType) return null;
  const v = meters[s.meter]?.values.find((x) => x.type === s.meterType);
  if (!v) return null;
  return type === 'T_ALL' && fahrenheit ? toF(v.value) : v.value;
}

/** EM series names: phases a/b/c for a triphase meter, else the meter's label or channel. */
export function emSeriesName(d: Device, meter: string, line: number, lines: number, index: number): string {
  if (lines > 1) return `${descName(d)}-${String.fromCharCode(97 + line)}`;
  const label = d.meters?.[index]?.label;
  return descNameLabel(d, label, index);
}

// ---- CSV (TimeChartsExporter) ----------------------------------------------------------------

export interface Series { name: string; points: [number, number][] }

const two = (v: number): string => v.toFixed(2);
const dt = (ms: number): string => {
  const d = new Date(ms);
  const p = (n: number): string => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`;
};
const inRange = (t: number, r: [number, number] | null): boolean => !r || (t >= r[0] && t <= r[1]);

/** exportAsHorizontalCSV: per series its name, then rows of unix ms, times and values. */
export function horizontalCSV(series: Series[], sep: string, range: [number, number] | null): string {
  const lines: string[] = [];
  for (const s of series) {
    const pts = s.points.filter(([t]) => inRange(t, range));
    lines.push(s.name, pts.map(([t]) => String(t)).join(sep), pts.map(([t]) => dt(t)).join(sep), pts.map(([, v]) => two(v)).join(sep));
  }
  return lines.join('\r\n') + '\r\n';
}

/** exportAsVerticalCSV: three columns (unix time, timestamp, value) per series, side by side. */
export function verticalCSV(series: Series[], meas: string, sep: string, range: [number, number] | null, labels: { unix: string; time: string }): string {
  const head: string[] = [], sub: string[] = [];
  series.forEach((s, i) => {
    if (i > 0) { head.push('', '', ''); sub.push(''); }
    head.push(s.name);
    sub.push(labels.unix, labels.time, meas);
  });
  const lines = [head.join(sep), sub.join(sep)];
  const max = Math.max(0, ...series.map((s) => s.points.length));
  for (let i = 0; i < max; i++) {
    const row: string[] = [];
    let valid = false;
    series.forEach((s, j) => {
      if (j > 0) row.push('');
      const p = s.points[i];
      if (p && inRange(p[0], range)) { row.push(String(p[0]), dt(p[0]), two(p[1])); valid = true; } else row.push('', '', '');
    });
    if (valid) lines.push(row.join(sep));
  }
  return lines.join('\r\n') + '\r\n';
}
