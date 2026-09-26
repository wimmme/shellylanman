// Portions derived from ShellyScanner (https://github.com/usnasoft/shellyscanner),
// Copyright (C) Antonio Flaccomio / usnasoft, licensed under GPL-3.0:
// view/DialogDeferrables and the deferred-tasks button of MainView.
//
// Deferred tasks: actions queued for devices that were off line, run when
// they come back (kept in /data/deferred.json).
import { configApi, type DeferredTask } from '../api';
import { h, ICONS } from '../dom';
import { dateTime } from '../format';
import { t, type Key } from '../i18n';
import type { EventSocket } from '../socket';
import { toast } from '../toast';
import { card, emptyState, type Page } from './common';

let tasks: DeferredTask[] = [];
const listeners = new Set<() => void>();
const notify = (): void => listeners.forEach((l) => l());

export function waitingCount(): number {
  return tasks.filter((x) => x.status === 'WAITING').length;
}

export function onDeferredChanged(l: () => void): () => void {
  listeners.add(l);
  return () => listeners.delete(l);
}

async function load(): Promise<void> {
  tasks = await configApi.deferred();
  notify();
}

export function wireDeferredEvents(socket: EventSocket): void {
  socket.on('hello', () => { void load().catch(() => {}); });
  socket.on('deferred.changed', (ev) => {
    const before = new Map(tasks.map((x) => [x.id, x.status]));
    tasks = (ev.data as DeferredTask[]) || [];
    for (const x of tasks) { // MainView: a failed task is signalled
      if (x.status === 'FAIL' && before.get(x.id) !== 'FAIL' && before.has(x.id)) toast(t('deferred.failed', { device: x.deviceName, action: t(('deferred.desc.' + x.description) as Key) }));
    }
    notify();
  });
}

function statusText(x: DeferredTask): string {
  let s = t(('deferred.status.' + x.status) as Key);
  if (x.message) s += ' - ' + x.message.replace(/\n/g, '; ');
  return s;
}

export const deferredPage: Page = {
  id: 'deferred',
  title: 'nav.deferred',
  icon: ICONS.deferred,
  async render(main) {
    const badge = h('span');
    const body = h('div');
    const draw = (): void => {
      badge.textContent = String(waitingCount());
      if (tasks.length === 0) { body.replaceChildren(emptyState(t('deferred.empty.title'), t('deferred.empty.text'), ICONS.deferred)); return; }
      body.replaceChildren(h('div', { class: 'table-wrap' }, h('table', { class: 'data' },
        h('thead', {}, h('tr', {}, ...(['deferred.col.time', 'deferred.col.device', 'deferred.col.action', 'deferred.col.status'] as Key[]).map((k) => h('th', { scope: 'col' }, t(k))), h('th'))),
        h('tbody', {}, ...[...tasks].reverse().map((x) => h('tr', {},
          h('td', {}, dateTime(new Date(x.time), true)),
          h('td', {}, x.deviceName),
          h('td', {}, t(('deferred.desc.' + x.description) as Key)),
          h('td', { class: 'def-' + x.status.toLowerCase() }, statusText(x)),
          h('td', {}, x.status === 'WAITING' ? h('button', { class: 'btn', onclick: () => void configApi.cancelDeferred(x.id).catch((e) => toast(String(e))) }, t('deferred.cancel')) : '')))))));
    };
    await load().catch(() => {});
    draw();
    const off = onDeferredChanged(draw);
    disposeFn = off;
    main.append(card(t('nav.deferred'), null, h('p', { class: 'muted' }, t('deferred.intro'), ' ', badge), body));
  },
  dispose() { disposeFn(); },
};

let disposeFn = (): void => {};

let badgeEl: HTMLElement | null = null;

/** The pending count on the sidebar entry (one element, reused by every render of the shell). */
export function sidebarBadge(): HTMLElement {
  if (!badgeEl) {
    const b = h('span', { class: 'nav-badge hidden' });
    const upd = (): void => { const n = waitingCount(); b.textContent = String(n); b.classList.toggle('hidden', n === 0); };
    upd();
    onDeferredChanged(upd);
    badgeEl = b;
  }
  return badgeEl;
}
