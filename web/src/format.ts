// Formatting of device values, following ShellyScanner's LabelsBundle
// (METER_LBL_*, METER_VAL_*, col_uptime_*), and the per-browser display
// preferences of its General settings tab (uptime format, temperature unit,
// double-click action, default filter column).
import type { MeterSet, Module } from './api';

export type UptimeMode = 'SEC' | 'DAY' | 'FROM';
export type TempUnit = 'C' | 'F';
export type DblClick = 'DET' | 'WEB';

const KEYS = { uptime: 'sl_uptime_mode', temp: 'sl_temp_unit', dbl: 'sl_dclick', filter: 'sl_default_filter', csv: 'sl_csv_sep', chart: 'sl_chart_def', chartExp: 'sl_chart_export' } as const;

function lsGet(k: string): string | null {
  try { return localStorage.getItem(k); } catch { return null; }
}
function lsSet(k: string, v: string): void {
  try { localStorage.setItem(k, v); } catch { /* private mode */ }
}

export const prefs = {
  uptime: (): UptimeMode => (lsGet(KEYS.uptime) as UptimeMode) || 'SEC', // ShellyScanner default: seconds
  temp: (): TempUnit => (lsGet(KEYS.temp) === 'F' ? 'F' : 'C'),
  dblClick: (): DblClick => (lsGet(KEYS.dbl) === 'WEB' ? 'WEB' : 'DET'), // default: device info
  defaultFilter: (): number => Number(lsGet(KEYS.filter) || 0),
  setUptime: (v: UptimeMode) => lsSet(KEYS.uptime, v),
  setTemp: (v: TempUnit) => lsSet(KEYS.temp, v),
  setDblClick: (v: DblClick) => lsSet(KEYS.dbl, v),
  setDefaultFilter: (v: number) => lsSet(KEYS.filter, String(v)),
  csvSeparator: (): string => lsGet(KEYS.csv) || ',', // CSV_SEPARATOR, default ","
  setCsvSeparator: (v: string) => lsSet(KEYS.csv, v),
  chartDefault: (): string => lsGet(KEYS.chart) || 'INT_TEMP', // CHART_DEF
  setChartDefault: (v: string) => lsSet(KEYS.chart, v),
  chartExport: (): 'H' | 'V' => (lsGet(KEYS.chartExp) === 'V' ? 'V' : 'H'), // CHART_EXPORT
  setChartExport: (v: 'H' | 'V') => lsSet(KEYS.chartExp, v),
};

/** METER_LBL_*: the short label in front of a value. */
export const METER_LABEL: Record<string, string> = {
  W: 'P', VAR: 'Q', VA: 'S', PF: 'pf', V: 'V', VL: 'V', VX: 'xV', I: 'I', FREQ: 'f', BAT: 'bat', BATE: 'bat',
  T: 'T', T1: 'T', T2: 'T', T3: 'T', T4: 'T', H: 'H', HD: 'H', L: 'L', EX: 'S', PERC: 'An', NUM: 'N',
  DMM: 'dist', RAIN: 'rain', VIB: 'vib', ANG: 'ang', ANG1: 'ang', ANG2: 'ang', CHANNEL: 'ch', LIGHT: 'L', LE: 'L',
};

const TEMP_TYPES = new Set(['T', 'T1', 'T2', 'T3', 'T4']);
const ENUM_VALUES: Record<string, string[]> = {
  BATE: ['Normal', 'Low'], EX: ['open', 'closed'], VIB: ['no', 'yes'], LIGHT: ['dark', 'light'], LE: ['dark', 'twilight', 'bright'],
};

const fixed = (v: number, d: number): string => v.toFixed(d);

export function celsiusToF(c: number): number {
  return c * 1.8 + 32;
}

export function formatTemp(c: number, unit: TempUnit = prefs.temp()): string {
  return unit === 'F' ? `${fixed(celsiusToF(c), 1)}°F` : `${fixed(c, 1)}°C`;
}

/** METER_VAL_*: a value with its unit. */
export function formatMeter(type: string, v: number, unit: TempUnit = prefs.temp()): string {
  if (TEMP_TYPES.has(type)) return formatTemp(v, unit);
  const e = ENUM_VALUES[type];
  if (e) return e[Math.trunc(v)] ?? String(v);
  switch (type) {
    case 'W': return `${fixed(v, 2)}W`;
    case 'VAR': return `${fixed(v, 2)}VAR`;
    case 'VA': return `${fixed(v, 2)}VA`;
    case 'PF': return fixed(v, 2);
    case 'V': return `${fixed(v, 1)}V`;
    case 'VL': return `${fixed(v, 2)}V`;
    case 'VX': return fixed(v, 2);
    case 'I': return `${fixed(v, 2)}A`;
    case 'FREQ': return `${fixed(v, 1)}Hz`;
    case 'BAT': return `${fixed(v, 0)}%`;
    case 'H': return `${fixed(v, 0)}%`;
    case 'HD': return `${fixed(v, 1)}%`;
    case 'L': return `${fixed(v, 0)}lux`;
    case 'PERC': return fixed(v, 1);
    case 'NUM': case 'CHANNEL': return fixed(v, 0);
    case 'DMM': case 'RAIN': return `${fixed(v, 0)}mm`;
    case 'ANG': case 'ANG1': case 'ANG2': return `${fixed(v, 1)}°`;
  }
  return String(v);
}

/** One meter set as text: "Label P 12.00W V 230.0V" (DevicesTable.cellValueAsString). */
export function meterSetText(m: MeterSet, unit: TempUnit = prefs.temp()): string {
  const parts: string[] = [];
  if (m.label) parts.push(m.label);
  for (const v of m.values) {
    const name = v.name ? ` (${v.name})` : '';
    parts.push(`${METER_LABEL[v.type] ?? v.type}${name} ${formatMeter(v.type, v.value, unit)}`);
  }
  return parts.join(' ');
}

export function metersText(ms: MeterSet[] | undefined, unit: TempUnit = prefs.temp()): string {
  return (ms ?? []).map((m) => meterSetText(m, unit)).join(' + ');
}

/** Uptime in the chosen format (UptimeCellRenderer, col_uptime_as_*). */
export function formatUptime(s: number, mode: UptimeMode = prefs.uptime(), now = Date.now()): string {
  if (s < 0) return '';
  if (mode === 'DAY') {
    const d = Math.floor(s / 86400), h = Math.floor((s % 86400) / 3600), m = Math.floor((s % 3600) / 60), sec = s % 60;
    return `${d} days, ${h} hours, ${m} min, ${sec} sec`;
  }
  if (mode === 'FROM') return dateTime(new Date(now - s * 1000), false);
  return String(s);
}

/** Tooltip: "d days, h hours, m minutes, s seconds / Up since: …" (col_uptime_tooltip). */
export function uptimeTooltip(s: number, now = Date.now()): string {
  const d = Math.floor(s / 86400), h = Math.floor((s % 86400) / 3600), m = Math.floor((s % 3600) / 60), sec = s % 60;
  return `${d} days, ${h} hours, ${m} minutes, ${sec} seconds — up since ${dateTime(new Date(now - s * 1000), true)}`;
}

/** dd/mm/yyyy hh:mm[:ss], as ShellyScanner prints dates. */
export function dateTime(t: Date, seconds: boolean): string {
  const p = (n: number): string => String(n).padStart(2, '0');
  const time = `${p(t.getHours())}:${p(t.getMinutes())}` + (seconds ? `:${p(t.getSeconds())}` : '');
  return `${p(t.getDate())}/${p(t.getMonth() + 1)}/${t.getFullYear()} ${time}`;
}

/** Read-only text of a module (the Command column until controls arrive). */
export function moduleText(m: Module, unit: TempUnit = prefs.temp()): string {
  const label = m.label ? `${m.label}: ` : '';
  switch (m.kind) {
    case 'relay': case 'cb': case 'camera':
      return label + (m.on ? 'ON' : 'OFF');
    case 'light': case 'cct':
      return label + (m.on ? `ON ${m.brightness ?? 0}%` : 'OFF');
    case 'rgb': case 'rgbw': case 'rgbcct':
      return label + (m.on ? `ON ${m.brightness ?? 0}%` : 'OFF');
    case 'cover':
      return label + (m.calibrated ? `${m.position ?? 0}%` : 'n.c.') + (m.state && m.state !== 'stopped' ? ` (${m.state})` : '');
    case 'input':
      return (m.label || `In ${m.index}`) + (m.inputOn ? ' ●' : ' ○');
    case 'thermostat':
      return label + (m.target !== undefined ? formatTemp(m.target, unit) : '') + (m.position !== undefined ? ` · ${m.position}%` : '') + (m.on === false ? ' (off)' : '');
    case 'sensor':
      return `${m.label}: ${m.on ? 'yes' : 'no'}`;
  }
  return m.label;
}
