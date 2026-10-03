// Portions derived from ShellyScanner (https://github.com/usnasoft/shellyscanner),
// Copyright (C) Antonio Flaccomio / usnasoft, licensed under GPL-3.0:
// view/checklist/CheckListTable (renderers) and CheckListView (enabling rules).
//
// The pure parts of the checklist page, testable without a DOM.
import type { Cell, ChecklistRow } from './api';

export type Col = 'eco' | 'led' | 'logs' | 'ble' | 'ap' | 'roaming' | 'wifi1' | 'wifi2' | 'extender' | 'scripts' | 'autoFW';

export const TRUE = '✓', FALSE = '✗', NA = '-';

export const isG1 = (r: ChecklistRow): boolean => r.gen === '1';
export const isG2 = (r: ChecklistRow): boolean => ['2', '3', '4'].includes(r.gen);
export const isBLU = (r: ChecklistRow): boolean => r.gen === 'blu' || r.gen === 'bth';

/** The cell text and colour class (CheckRenderer, BooleanRenderer, StringJudgedRenderer, BLERenderer). */
export function cellView(col: Col, v: Cell, good?: boolean): { text: string; cls: string } {
  if (v === null || v === undefined) return { text: NA, cls: '' };
  if (Array.isArray(v)) return { text: String(v.length), cls: v.length ? '' : 'chk-dim' };
  if (typeof v === 'boolean') {
    const text = v ? TRUE : FALSE;
    if (good === undefined) return { text, cls: '' };
    return { text, cls: v === good ? 'chk-ok' : 'chk-bad' };
  }
  if (col === 'extender') {
    if (String(v) === '0') return { text: '0', cls: 'chk-bad chk-dim' };
    if (v === FALSE) return { text: FALSE, cls: 'chk-ok' };
  }
  return { text: String(v), cls: '' };
}

// ---- enabling rules (CheckListView.selListener) -------------------------------------------

export function sameBoolean(rows: ChecklistRow[], col: Col, only?: (r: ChecklistRow) => boolean): boolean {
  if (!rows.length) return false;
  const v0 = rows[0]![col];
  return rows.every((r) => typeof r[col] === 'boolean' && r[col] === v0 && (!only || only(r)));
}

/** All numbers, or all "✗", or all neither "✗" nor "-" (sameStringValuesOrInt). */
export function sameStringOrInt(rows: ChecklistRow[], col: Col): boolean {
  if (!rows.length) return false;
  const v0 = rows[0]![col];
  if (!(typeof v0 === 'number' || (typeof v0 === 'string' && v0 !== NA))) return false;
  return rows.every((r) => {
    const v = r[col];
    return (typeof v === 'number' && typeof v0 === 'number') ||
      (typeof v0 === 'string' && typeof v === 'string' && ((v === FALSE && v0 === FALSE) || (v !== NA && v !== FALSE && v0 !== FALSE)));
  });
}

/** The common value when all rows are equal, not "-" and Gen2+ (sameObjectValues). */
export function sameObject(rows: ChecklistRow[], col: Col): Cell | undefined {
  if (!rows.length) return undefined;
  const v0 = rows[0]![col];
  if (v0 === null || v0 === NA) return undefined;
  for (const r of rows.slice(1)) if (JSON.stringify(r[col]) !== JSON.stringify(v0) || !isG2(r)) return undefined;
  return v0;
}

export const editable = (rows: ChecklistRow[], col: Col): boolean => rows.length > 0 && rows.every((r) => r[col] !== null && r[col] !== NA);

// ---- what a device has, for why.ts (DECISIONS P12-3, ShellyLanMan's own) ------------------

const known = (v: Cell): boolean => v !== null && v !== undefined && v !== NA;

/** Whether a device has the setting an action switches (the `has` of whyDisabled in why.ts). */
export const HAS: Record<string, (r: ChecklistRow) => boolean> = {
  eco: (r) => typeof r.eco === 'boolean',
  led: (r) => isG1(r) && typeof r.led === 'boolean',
  logs: (r) => (isG1(r) ? typeof r.logs === 'boolean' : isG2(r) && known(r.logs)),
  ble: (r) => (isG2(r) || isBLU(r)) && r.ble !== null,
  ap: (r) => isG2(r) && typeof r.ap === 'boolean',
  roaming: (r) => known(r.roaming),
  extender: (r) => known(r.extender),
  autoFW: (r) => known(r.autoFW),
};
