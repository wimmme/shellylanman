import type { Device } from '../api';
import { h, icon, ICONS } from '../dom';
import { dateTime } from '../format';
import { t, type Key } from '../i18n';

export interface Page {
  id: string;
  title: Key;
  icon: string;
  render(main: HTMLElement): void | Promise<void>;
  /** Called when the user leaves the page (stop live updates, timers). */
  dispose?(): void;
}

/**
 * The status pill of every table (DECISIONS P25-5): green on line, red off line, blue while
 * read or updated, grey archived. ↻: restart needed; orange ring and ⚡: newer firmware.
 */
export function statusPill(cls: string, label: string, marks: { reboot: boolean; update: boolean } = { reboot: false, update: false }): HTMLElement {
  return h('span', { class: 'pill ' + cls + (marks.update ? ' update' : '') }, label, marks.reboot ? ' ↻' : null,
    marks.update ? h('span', { class: 'pill-upd', 'aria-hidden': 'true' }, '⚡') : null);
}

/** The hover text of a device's status pill, the same on every page. */
export function statusTip(d: Device): string {
  const isBLU = d.gen === 'blu' || d.gen === 'bth';
  const tip = [t(('status.' + d.status) as Key)];
  if (d.rebootRequired) tip.push(t('status.rebootRequired'));
  if (d.updateAvailable) tip.push(d.updateVersion ? t('status.updateAvailable', { version: d.updateVersion }) : t('status.updateAvailableShort'));
  if (d.status === 'online' && isBLU && d.lastSeen > 0) tip.push(t('status.bluSeen', { when: dateTime(new Date(d.lastSeen), true) }));
  if (d.status === 'searching') tip.push(t('status.searchingTip'));
  if ((d.status === 'offline' || d.status === 'ghost' || d.status === 'searching') && d.lastSeen > 0) tip.push(t('status.lastSeen', { when: dateTime(new Date(d.lastSeen), true) }));
  if (d.error) tip.push(d.error);
  if (d.status === 'error') tip.push(t('status.errorTip'));
  if (d.paused) tip.push(t('logs.paused'));
  return tip.join(' — ');
}

/** The small turning circle after a device's name while it is read or updated (P25-5). */
export function busySpinner(): HTMLElement {
  return h('span', { class: 'spinner small', role: 'status', 'aria-label': t('state.loading') });
}

export function card(title: string, badge: string | null, ...body: (Node | string)[]): HTMLElement {
  return h('section', { class: 'card' },
    h('div', { class: 'card-head' }, h('h2', { class: 'card-title' }, title), badge !== null && h('span', { class: 'card-badge' }, badge)),
    ...body);
}

export function emptyState(title: string, text: string, iconPath: string = ICONS.empty): HTMLElement {
  return h('div', { class: 'state' }, icon(iconPath, 40), h('div', { class: 'title' }, title), h('div', {}, text));
}

export function loadingState(): HTMLElement {
  return h('div', { class: 'state' }, h('div', { class: 'spinner', role: 'status', 'aria-label': t('state.loading') }), t('state.loading'));
}

export function errorState(err: unknown): HTMLElement {
  return h('div', { class: 'state' }, h('div', { class: 'title' }, t('state.error', { msg: err instanceof Error ? err.message : String(err) })));
}

/**
 * The line above Checklist and Firmware that says which devices they show (the
 * selected ones or all) and switches between the two (DECISIONS P12-2).
 * Nothing when all devices are shown and none is selected.
 */
export function scopeBanner(showingSelected: boolean, count: number, toggle: () => void): HTMLElement | null {
  if (!showingSelected && count === 0) return null;
  return h('div', { class: 'banner scope', role: 'status', title: t('scope.tip') },
    h('span', {}, t(showingSelected ? 'scope.selected' : 'scope.all', { n: count })),
    h('button', { class: 'btn', onclick: toggle }, t(showingSelected ? 'scope.showAll' : 'scope.showSelected')));
}

/** A page whose features arrive in a later phase. */
export function placeholder(id: string, title: Key, iconPath: string, text: Key): Page {
  return {
    id, title, icon: iconPath,
    render(main) {
      main.append(card(t(title), null, emptyState(t('state.notYet'), t(text), iconPath)));
    },
  };
}
