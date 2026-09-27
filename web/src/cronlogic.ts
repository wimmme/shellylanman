// Portions derived from ShellyScanner (https://github.com/usnasoft/shellyscanner),
// Copyright (C) Antonio Flaccomio / usnasoft, licensed under GPL-3.0:
// view/scheduler/CronUtils, AbstractCronPanel (setCron, generateExpression,
// computeMonths, computeDaysOfWeek) and walldisplay/ThermJobPanel.
//
// Shelly timespecs: "sec min hour day month weekday", or
// "@sunrise|@sunset[+|-Hh[Mm]] day month weekday".

const WEEK_DAYS = ['SUN', 'MON', 'TUE', 'WED', 'THU', 'FRI', 'SAT'];
const MONTHS = ['JAN', 'FEB', 'MAR', 'APR', 'MAY', 'JUN', 'JUL', 'AUG', 'SEP', 'OCT', 'NOV', 'DEC'];

const R_0_59 = '([1-5]?\\d)';
const R_00_59 = '([0-5]?\\d)';
const R_0_23 = '(1\\d|2[0-3]|\\d)';
const R_1_31 = '([12]\\d|3[01]|[1-9])';
const R_1_9999 = '([1-9]\\d{0,3})';
const R_1_12 = '(1[0-2]|[1-9])';
const R_0_6 = '([0-6])';
const frag = (r: string): string => `(${r}-${r}|\\*/${R_1_9999}|${r}/${R_1_9999}|${r})`;
const list = (r: string): string => `\\*|${frag(r)}(,${frag(r)})*`;
const R_HOURS = list(R_0_23), R_MINUTES = list(R_0_59), R_DAYS = list(R_1_31), R_MONTHS = list(R_1_12), R_WDAYS = list(R_0_6);

const whole = (r: string): RegExp => new RegExp(`^(?:${r})$`);
export const HOUR_0_23 = whole(R_0_23);
export const MINUTE_0_59 = whole(R_0_59);
export const HOURS = whole(R_HOURS);
export const MINUTES = whole(R_MINUTES);
export const SECONDS = MINUTES;
export const DAYS = whole(R_DAYS);
export const CRON = whole(`(${R_MINUTES}) (${R_MINUTES}) (${R_HOURS}) (${R_DAYS}) (${R_MONTHS}) (${R_WDAYS})`);
export const SUNSET = whole(`@(sunset|sunrise)((\\+|-)(?<HOUR>${R_0_23})h((?<MINUTE>${R_00_59})m)?)?( (?<DAY>${R_DAYS}) (?<MONTH>${R_MONTHS}) (?<WDAY>${R_WDAYS}))?`);
/** Wall Display thermostat rules: "* m h * * d,d,…". */
export const CRON_TH_WD = whole(`\\* (${R_0_59}) (${R_0_23}) \\* \\* (([0-6])(,[0-6])*)`);

export const isValid = (exp: string): boolean => CRON.test(exp) || SUNSET.test(exp);

/** fragmentToInt: the numbers of "1-3,5" ("*\/x" and "x/y" are ignored). */
export function fragmentToInt(f: string): number[] {
  const out: number[] = [];
  for (const m of f.matchAll(/(\d+-\d+|\d+)/g)) {
    const [a, b] = m[1]!.split('-').map(Number);
    if (b === undefined) out.push(a!);
    else for (let v = a!; v <= b; v++) out.push(v);
  }
  return out;
}

/** fragStrToNum: JAN → 1, MON → 1 (not inside "@sunset"/"@sunrise"). */
export function fragStrToNum(s: string): string {
  MONTHS.forEach((m, i) => { s = s.replace(new RegExp(m, 'gi'), String(i + 1)); });
  WEEK_DAYS.forEach((d, i) => { s = s.replace(new RegExp(`([^@])${d}`, 'gi'), `$1${i}`); });
  return s;
}

/** daysOfWeekAsString: "1-3" → "MON,TUE,WED" (the only form the Wall Display thermostat understands). */
export function daysOfWeekAsString(s: string): string {
  return fragmentToInt(s === '*' ? '0-6' : s).map((n) => WEEK_DAYS[n]).join(',');
}

/** listAsCronString: Shelly wants single values before ranges ("5,1-3" instead of "1-3,5"). */
export function listAsCronString(values: number[]): string {
  const single: string[] = [], group: string[] = [];
  const l = [...values, Number.MAX_SAFE_INTEGER];
  let init = l[0]!, last = init;
  for (let i = 1; i < l.length; i++) {
    if (l[i]! > last + 1) {
      if (init === last) single.push(String(init));
      else if (init === last - 1) single.push(`${init},${last}`);
      else group.push(`${init}-${last}`);
      init = last = l[i]!;
    } else {
      last = l[i]!;
    }
  }
  return [...single, ...group].join(',');
}

/** hmCompare: timespecs by hour and minute. */
export function hmCompare(a: string, b: string): number {
  const k = (t: string): string => { const p = t.split(' '); return String(Number(p[2])).padStart(2, '0') + String(Number(p[1])).padStart(2, '0'); };
  return k(a) < k(b) ? -1 : k(a) > k(b) ? 1 : 0;
}

// ---- the editor's state (AbstractCronPanel) ----------------------------------------------

export type Mode = 'time' | 'beforeRise' | 'afterRise' | 'beforeSet' | 'afterSet';

export interface Cron {
  mode: Mode;
  seconds: string; minutes: string; hours: string; days: string;
  months: boolean[];   // JAN … DEC
  monthsRaw?: string;  // a "x/y" value the check boxes cannot show (they are then disabled)
  wdays: boolean[];    // MON … SUN, as the check boxes
  wdaysRaw?: string;
}

export const DEF_CRON = '0 0 * * * *';

export function monthsValue(c: Cron): string {
  if (c.monthsRaw) return c.monthsRaw;
  const seq = c.months.flatMap((on, i) => (on ? [i + 1] : []));
  return seq.length === 12 || seq.length === 0 ? '*' : listAsCronString(seq);
}

/** computeDaysOfWeek: Sunday first as 0; no ranges (the app does not understand them here). */
export function wdaysValue(c: Cron): string {
  if (c.wdaysRaw) return c.wdaysRaw;
  const seq = [...(c.wdays[6] ? [0] : []), ...c.wdays.slice(0, 6).flatMap((on, i) => (on ? [i + 1] : []))];
  return seq.length === 7 || seq.length === 0 ? '*' : seq.join(',');
}

/** setCron: parse a timespec (assumed valid). */
export function parseCron(line: string): Cron {
  line = fragStrToNum(line);
  let mode: Mode = 'time';
  let v: string[];
  const sm = SUNSET.exec(line);
  if (sm) {
    const g = sm.groups ?? {};
    v = ['0', g.MINUTE ?? '0', g.HOUR ?? '0', g.DAY ?? '*', g.MONTH ?? '*', g.WDAY ?? '*'];
    mode = line.startsWith('@sunrise-') ? 'beforeRise' : line.startsWith('@sunrise') ? 'afterRise' : line.startsWith('@sunset-') ? 'beforeSet' : 'afterSet';
  } else {
    v = line.split(' ');
  }
  const c: Cron = { mode, seconds: v[0] ?? '0', minutes: v[1] ?? '0', hours: v[2] ?? '*', days: v[3] ?? '*', months: Array(12).fill(false), wdays: Array(7).fill(false) };
  const months = v[4] ?? '*';
  if (months.includes('/')) c.monthsRaw = months;
  else for (const m of fragmentToInt(months === '*' ? '1-12' : months)) c.months[m - 1] = true;
  const wd = v[5] ?? '*';
  if (wd.includes('/')) c.wdaysRaw = wd;
  else for (const d of fragmentToInt(wd === '*' ? '0-6' : wd)) c.wdays[(d + 6) % 7] = true;
  return c;
}

/** Switching the mode: sunrise/sunset modes have no seconds and need plain hours/minutes. */
export function setMode(c: Cron, mode: Mode): Cron {
  const n = { ...c, mode };
  if (mode !== 'time') {
    n.seconds = '0';
    if (!HOUR_0_23.test(n.hours)) n.hours = '0';
    if (!MINUTE_0_59.test(n.minutes)) n.minutes = '0';
  }
  return n;
}

/** generateExpression. */
export function expression(c: Cron): string {
  const prefix = { beforeRise: '@sunrise-', afterRise: '@sunrise+', beforeSet: '@sunset-', afterSet: '@sunset+' } as const;
  const head = c.mode === 'time' ? `${c.seconds} ${c.minutes} ${c.hours}` : `${prefix[c.mode]}${c.hours}h${c.minutes}m`;
  return `${head} ${c.days} ${monthsValue(c)} ${wdaysValue(c)}`;
}

/** Which fields are invalid (shown in red). */
export function invalidFields(c: Cron): Set<'seconds' | 'minutes' | 'hours' | 'days'> {
  const bad = new Set<'seconds' | 'minutes' | 'hours' | 'days'>();
  if (c.mode === 'time') {
    if (!SECONDS.test(c.seconds)) bad.add('seconds');
    if (!MINUTES.test(c.minutes)) bad.add('minutes');
    if (!HOURS.test(c.hours)) bad.add('hours');
  } else {
    if (!HOUR_0_23.test(c.hours)) bad.add('hours');
    if (!MINUTE_0_59.test(c.minutes)) bad.add('minutes');
  }
  if (!DAYS.test(c.days)) bad.add('days');
  return bad;
}

/** CronValuesDialog result: all or none → "*". */
export function valuesToField(selected: number[], min: number, max: number): string {
  return selected.length === 0 || selected.length === max - min ? '*' : listAsCronString([...selected].sort((a, b) => a - b));
}

// ---- Wall Display thermostat rules (ThermJobPanel) ------------------------------------------

export const DEF_TH_CRON = '* 0 0 * * 0,1,2,3,4,5,6';

/** The weekday field of a thermostat rule: no days = every day. */
export function thWdays(wdays: boolean[]): string {
  const seq = [...(wdays[6] ? [0] : []), ...wdays.slice(0, 6).flatMap((on, i) => (on ? [i + 1] : []))];
  return seq.length === 0 ? '0,1,2,3,4,5,6' : seq.join(',');
}

export function thExpression(h: number, m: number, wdays: boolean[]): string {
  return `* ${m} ${h} * * ${thWdays(wdays)}`;
}

/** getExtTimespec: what the device is sent (day names). */
export function thExtTimespec(h: number, m: number, wdays: boolean[]): string {
  return `* ${m} ${h} * * ${daysOfWeekAsString(thWdays(wdays))}`;
}

export function parseThCron(line: string): { h: number; m: number; wdays: boolean[] } {
  const v = fragStrToNum(line).split(' ');
  const wdays = Array(7).fill(false) as boolean[];
  const wd = v[5] ?? '*';
  for (const d of fragmentToInt(wd === '*' ? '0-6' : wd)) wdays[(d + 6) % 7] = true;
  return { h: Number(v[2]) || 0, m: Number(v[1]) || 0, wdays };
}

// ---- parameter editors (ParamEditorDialog) ----------------------------------------------------

export const PARAM_PATTERNS = {
  on: /"on"\s*:\s*((true)|(false))/,
  brightness: /"brightness"\s*:\s*(\d+)/,
  white: /"white"\s*:\s*(\d+)/,
  ct: /"ct"\s*:\s*(\d+)/,
  pos: /"pos"\s*:\s*(\d+)/,
  slat_pos: /"slat_pos"\s*:\s*(\d+)/,
  rgb: /"rgb"\s*:\s*\[\s*(\d+)\s*,\s*(\d+)\s*,\s*(\d+)\s*\]/,
} as const;

/** The sliders of each numeric parameter (SliderPar): [min, max]. */
export const PARAM_RANGES: Record<'brightness' | 'white' | 'ct' | 'pos' | 'slat_pos', [number, number]> = {
  brightness: [0, 100], white: [0, 255], ct: [2700, 6500], pos: [0, 100], slat_pos: [0, 100],
};

/** canEdit: at least one known parameter. */
export function canEditParams(par: string): boolean {
  return Object.values(PARAM_PATTERNS).some((r) => r.test(par));
}

export function replaceParam(par: string, key: keyof typeof PARAM_PATTERNS, value: string): string {
  return par.replace(PARAM_PATTERNS[key], `"${key}":${value}`);
}
