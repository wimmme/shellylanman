// The Log page's logic: ShellyLanMan's own log (DECISIONS §27). Pure functions,
// so they are tested without a browser.

export interface LogEntry {
  seq: number;
  time: number; // Unix ms
  level: string; // DEBUG, INFO, WARN, ERROR
  msg: string;
  attrs?: string;
}

export type LogLevel = 'info' | 'warn' | 'error';

const RANK: Record<string, number> = { DEBUG: 0, INFO: 1, WARN: 2, ERROR: 3 };
const MIN: Record<LogLevel, number> = { info: 1, warn: 2, error: 3 };

/** As many lines as the server keeps. */
export const MAX_LINES = 1000;

/** Adds entries newer than the last one (the same line may come from the list and from the socket), keeps the last MAX_LINES. */
export function merge(list: LogEntry[], incoming: LogEntry[], max: number = MAX_LINES): LogEntry[] {
  let last = list.length ? list[list.length - 1]!.seq : 0;
  const out = list.slice();
  for (const e of incoming.slice().sort((a, b) => a.seq - b.seq)) {
    if (e.seq <= last) continue;
    out.push(e);
    last = e.seq;
  }
  return out.length > max ? out.slice(out.length - max) : out;
}

export interface LogFilter {
  level: LogLevel;
  text: string;
  /** Lines up to this sequence number are hidden (the Clear button). */
  after: number;
}

export function matches(e: LogEntry, f: LogFilter): boolean {
  if (e.seq <= f.after) return false;
  if ((RANK[e.level] ?? 1) < MIN[f.level]) return false;
  const q = f.text.trim().toLowerCase();
  return q === '' || lineText(e).toLowerCase().includes(q);
}

const two = (n: number): string => String(n).padStart(2, '0');

/** Local time, HH:MM:SS (the date is in the tooltip). */
export function clock(ms: number): string {
  const d = new Date(ms);
  return `${two(d.getHours())}:${two(d.getMinutes())}:${two(d.getSeconds())}`;
}

export function stamp(ms: number): string {
  const d = new Date(ms);
  return `${d.getFullYear()}-${two(d.getMonth() + 1)}-${two(d.getDate())} ${clock(ms)}`;
}

/** The searchable text of one line, as the text log writes it. */
export function lineText(e: LogEntry): string {
  return `${e.level} ${e.msg}${e.attrs ? ' ' + e.attrs : ''}`;
}

/** What Copy puts on the clipboard: the lines that are shown, with their full date. */
export function copyText(list: LogEntry[], f: LogFilter): string {
  return list.filter((e) => matches(e, f)).map((e) => `${stamp(e.time)} ${lineText(e)}`).join('\n');
}
