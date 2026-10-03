import { h, icon, ICONS } from '../dom';
import { t, type Key } from '../i18n';

export interface Page {
  id: string;
  title: Key;
  icon: string;
  render(main: HTMLElement): void | Promise<void>;
  /** Called when the user leaves the page (stop live updates, timers). */
  dispose?(): void;
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
