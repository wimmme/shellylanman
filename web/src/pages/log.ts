// The Log page: ShellyLanMan's own log, the last lines kept in memory by the
// server (DECISIONS §27). Not the devices' logs (Checklist → Logs).
import { logApi } from '../api';
import { h, ICONS } from '../dom';
import { t } from '../i18n';
import { clock, copyText, matches, merge, stamp, type LogEntry, type LogFilter, type LogLevel } from '../loglogic';
import type { EventSocket } from '../socket';
import { card, emptyState, type Page } from './common';

let socket: EventSocket | null = null;
let stop: (() => void) | null = null;

/** Called once at start-up: the page needs the socket for new lines. */
export function wireLogEvents(s: EventSocket): void { socket = s; }

export const logPage: Page = {
  id: 'log',
  title: 'nav.log',
  icon: ICONS.log,
  async render(main) {
    let list: LogEntry[] = [];
    const filter: LogFilter = { level: 'info', text: '', after: 0 };
    let paused = false;

    const pre = h('pre', { class: 'log applog', tabindex: 0, 'aria-live': 'off' });
    const count = h('span', { class: 'muted', role: 'status' });
    const empty = h('div', {}, emptyState(t('log.empty'), t('log.emptyText'), ICONS.log));

    const line = (e: LogEntry): HTMLElement => h('div', { class: 'logline lvl-' + e.level.toLowerCase() },
      h('span', { class: 'logtime', title: stamp(e.time) }, clock(e.time)), ' ',
      h('span', { class: 'loglevel' }, e.level), ' ', h('span', { class: 'logmsg' }, e.msg),
      e.attrs ? h('span', { class: 'logattrs' }, ' ' + e.attrs) : null);
    const draw = (): void => {
      const lines = list.filter((e) => matches(e, filter));
      pre.replaceChildren(...lines.map(line));
      count.textContent = t('log.count', { shown: lines.length, total: list.length });
      empty.hidden = lines.length > 0;
      pre.hidden = lines.length === 0;
      pre.scrollTop = pre.scrollHeight;
    };
    const add = (entries: LogEntry[]): void => {
      const known = list.length ? list[list.length - 1]!.seq : 0;
      list = merge(list, entries);
      if (paused || !list.length || list[list.length - 1]!.seq === known) return;
      if (list.length >= 1000) { draw(); return; } // the oldest lines fall off: draw again
      const atBottom = pre.scrollTop + pre.clientHeight >= pre.scrollHeight - 20;
      for (const e of list.filter((x) => x.seq > known && matches(x, filter))) pre.append(line(e));
      empty.hidden = pre.childElementCount > 0;
      pre.hidden = pre.childElementCount === 0;
      count.textContent = t('log.count', { shown: pre.childElementCount, total: list.length });
      if (atBottom) pre.scrollTop = pre.scrollHeight;
    };

    const level = h('select', { 'aria-label': t('log.level'), onchange: () => { filter.level = level.value as LogLevel; draw(); } },
      h('option', { value: 'info' }, t('log.level.info')),
      h('option', { value: 'warn' }, t('log.level.warn')),
      h('option', { value: 'error' }, t('log.level.error')));
    const search = h('input', { type: 'search', placeholder: t('log.search'), 'aria-label': t('log.search'),
      oninput: () => { filter.text = search.value; draw(); } });
    const pauseBtn = h('button', { class: 'btn', 'aria-pressed': 'false', title: t('log.pauseTip'), onclick: () => {
      paused = !paused;
      pauseBtn.setAttribute('aria-pressed', String(paused));
      pauseBtn.textContent = t(paused ? 'log.resume' : 'log.pause');
      if (!paused) draw();
    } }, t('log.pause'));
    const clearBtn = h('button', { class: 'btn', title: t('log.clearTip'), onclick: () => {
      filter.after = list.length ? list[list.length - 1]!.seq : filter.after;
      draw();
    } }, t('log.clear'));
    const copyBtn = h('button', { class: 'btn', onclick: () => void navigator.clipboard?.writeText(copyText(list, filter)) }, t('common.copy'));

    main.append(card(t('nav.log'), null,
      h('p', { class: 'muted' }, t('log.hint')),
      h('div', { class: 'row log-tools' }, h('label', { class: 'row' }, t('log.level'), level), search, pauseBtn, clearBtn, copyBtn, count),
      pre, empty));

    // New lines first (they are kept while the list loads), then the list; a line may come twice, merge drops it.
    const pending: LogEntry[] = [];
    let loaded = false;
    const offs = [
      socket?.on('log.entry', (ev) => {
        const e = ev.data as LogEntry;
        if (loaded) add([e]); else pending.push(e);
      }),
      socket?.on('hello', () => { // the socket was down: fetch what was missed
        if (!loaded) return;
        const last = list.length ? list[list.length - 1]!.seq : 0;
        void logApi.list(last).then((r) => add(r.entries)).catch(() => { /* the next line fills the gap */ });
      }),
    ];
    stop = () => { offs.forEach((off) => off?.()); };
    const first = await logApi.list(0);
    list = merge([], [...first.entries, ...pending]);
    loaded = true;
    draw();
  },
  dispose() { stop?.(); stop = null; },
};
