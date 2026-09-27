// Portions derived from ShellyScanner (https://github.com/usnasoft/shellyscanner),
// Copyright (C) Antonio Flaccomio / usnasoft, licensed under GPL-3.0:
// view/devsettings/PanelFWUpdate (createTableRow, select buttons, countSelection)
// and FWUpdateTable (cells, editingStopped, row filter).
//
// The FW Update table without DOM, so it can be tested.
import type { FirmwareRow } from './api';

export type Choice = 'stable' | 'beta' | null;

/** A version cell: a checkbox with the short version, a text, or nothing. */
export type Cell = { type: 'check'; label: string; tip: string } | { type: 'text'; text: string } | { type: 'empty' };

/** createTableRow / FWCellRendered for the "new stable" column. */
export function stableCell(r: FirmwareRow, tr: (k: string, v?: Record<string, number>) => string): Cell {
  if (!r.known) return r.queued ? { type: 'text', text: tr('fw.requested') } : { type: 'check', label: tr('fw.any'), tip: '' };
  if (r.updating) {
    if (r.rebooting) return { type: 'text', text: tr('fw.rebooting') };
    if (r.gen === '1') return { type: 'text', text: tr('fw.updating') };
    return { type: 'text', text: tr('fw.loading', { n: Math.max(0, r.progress) }) };
  }
  return r.stableBuild ? { type: 'check', label: r.stable || r.stableBuild, tip: r.stableBuild } : { type: 'empty' };
}

export function betaCell(r: FirmwareRow): Cell {
  if (!r.known || r.updating || !r.betaBuild) return { type: 'empty' };
  return { type: 'check', label: r.beta || r.betaBuild, tip: r.betaBuild };
}

/** The ticks when a row is (re)drawn: stable when the server preselected it. */
export function initialChoice(r: FirmwareRow): Choice {
  return r.known && !r.updating && r.preselect ? 'stable' : null;
}

/** Ticking one column unticks the other (FWUpdateTable.editingStopped). */
export function toggle(current: Choice, col: 'stable' | 'beta', on: boolean): Choice {
  if (on) return col;
  return current === col ? null : current;
}

/** "Select stable" / "Select beta" / "Deselect all" on one row. */
export function selectAll(r: FirmwareRow, current: Choice, what: 'stable' | 'beta' | 'none'): Choice {
  if (what === 'none') return null;
  const cell = what === 'stable' ? stableCell(r, (k) => k) : betaCell(r);
  return cell.type === 'check' ? what : current;
}

/** RowFilter over device, current, stable and beta (case-insensitive substring). */
export function matches(r: FirmwareRow, q: string): boolean {
  if (!q) return true;
  const s = q.toLowerCase();
  return [r.name, r.current, r.stable, r.beta].some((x) => (x ?? '').toLowerCase().includes(s));
}

/** countSelection over the rows shown. */
export function count(rows: FirmwareRow[], choice: Map<string, Choice>, q: string): { stable: number; beta: number } {
  let stable = 0, beta = 0;
  for (const r of rows) {
    if (!matches(r, q)) continue;
    const c = choice.get(r.id);
    if (c === 'stable') stable++;
    if (c === 'beta') beta++;
  }
  return { stable, beta };
}

/** The update requests of the rows shown: "any" for devices without firmware information. */
export function requests(rows: FirmwareRow[], choice: Map<string, Choice>, q: string): { id: string; stage: 'stable' | 'beta' | 'any' }[] {
  return rows.filter((r) => matches(r, q) && choice.get(r.id)).map((r) => ({ id: r.id, stage: r.known ? choice.get(r.id)! : 'any' }));
}
