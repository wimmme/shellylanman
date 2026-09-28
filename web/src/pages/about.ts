import { api, type About, type Dep, type UpdateStatus } from '../api';
import { parseChangelog, type Release } from '../changelog';
import { h, ICONS } from '../dom';
import { formatDuration } from '../format';
import { LANGUAGES, t, type Key } from '../i18n';
import { openModal } from '../modal';
import { card, type Page } from './common';

const link = (href: string, text: string, cls?: string): HTMLAnchorElement =>
  h('a', { href, target: '_blank', rel: 'noopener noreferrer', class: cls }, text);

const HELP = ['devices', 'settings', 'checklist', 'firmware', 'backup', 'scheduler', 'scripts', 'charts'] as const;

async function textFile(path: string): Promise<string> {
  const r = await fetch(path);
  return r.ok ? r.text() : '';
}

function showText(title: string, path: string): void {
  const pre = h('pre', { class: 'about-text' }, t('state.loading'));
  openModal(title, pre, [{ label: t('common.close') }], undefined, 'wide');
  void textFile(path).then((s) => { pre.textContent = s; });
}

/** Version state from the (opt-in) release check. */
function versionState(u: UpdateStatus | null): HTMLElement {
  if (!u || u.mode === 'never' || !u.mode) return h('a', { href: '#/settings', class: 'about-state muted' }, t('about.checkOff'));
  if (u.newer && u.latest) return link(u.url || '#', t('about.newer', { version: u.latest }), 'about-state warn');
  return h('span', { class: 'about-state ok' }, t('about.upToDate'));
}

function hero(a: About, u: UpdateStatus | null): HTMLElement {
  const uptime = h('span', { class: 'muted' });
  const tick = (): void => { uptime.textContent = t('about.uptime', { time: formatDuration((Date.now() - a.started) / 1000) }); };
  tick();
  const timer = setInterval(() => { if (!uptime.isConnected) clearInterval(timer); else tick(); }, 30_000);
  return h('section', { class: 'card about-hero' },
    h('img', { src: '/logo.png', alt: 'ShellyLanMan', width: 120, height: 120, class: 'about-logo' }),
    h('div', { class: 'about-intro' },
      h('h1', {}, a.name),
      h('div', { class: 'about-tagline' }, t('about.tagline')),
      h('p', {}, t('about.what1')),
      h('p', {}, t('about.what2')),
      h('div', { class: 'about-meta' },
        h('span', { class: 'about-version mono' }, /^\d/.test(a.version) ? 'v' + a.version : a.version),
        versionState(u), uptime, link(a.source, a.source.replace('https://', '')))));
}

function system(a: About): HTMLElement {
  const langs = a.languages.map((id) => LANGUAGES.find((l) => l.id === id)?.label ?? id).join(', ');
  const row = (k: Key, ...v: (Node | string)[]): Node[] => [h('dt', {}, t(k)), h('dd', {}, ...v)];
  return card(t('about.system'), null,
    h('dl', { class: 'kv' },
      ...row('about.version', h('span', { class: 'mono' }, a.version + (a.commit ? ` (${a.commit})` : ''))),
      ...row('about.runtime', h('span', { class: 'mono' }, a.runtime.go)),
      ...row('about.platform', h('span', { class: 'mono' }, `${a.runtime.os}/${a.runtime.arch}` + (a.runtime.kernel ? ` · ${a.runtime.kernel}` : ''))),
      ...row('about.memory', t('about.memoryValue', { mb: a.runtime.memMB })),
      ...row('about.data', t('about.dataValue')),
      ...row('about.languages', langs),
      ...row('about.license', h('span', { class: 'mono' }, a.license), ' · ',
        h('button', { class: 'linkish', onclick: () => showText(t('about.licenseText'), '/api/v1/about/license') }, t('about.licenseText')), ' · ',
        h('button', { class: 'linkish', onclick: () => showText(t('about.notices'), '/api/v1/about/notices') }, t('about.notices'))),
    ));
}

function support(a: About): HTMLElement {
  const [coffee, paypal] = a.donate;
  return card(t('about.support'), null,
    h('p', {}, t('about.free')),
    h('div', { class: 'about-buttons' },
      link(a.issues, t('about.bug'), 'btn'),
      coffee && link(coffee.url, '☕ ' + t('about.coffee'), 'btn btn-coffee'),
      paypal && link(paypal.url, t('about.paypal'), 'btn btn-paypal')),
    h('p', { class: 'muted' }, t('about.notice')));
}

function basedOn(a: About): HTMLElement {
  const text: Key[] = ['about.basedOnSS', 'about.basedOnMD'];
  return card(t('about.basedOn'), null, h('div', { class: 'about-based' }, ...a.basedOn.map((c, i) =>
    h('div', { class: 'about-origin' },
      h('div', { class: 'about-origin-name' }, link(c.url, c.name), h('span', { class: 'muted' }, ` · ${c.author} · ${c.license}`)),
      h('p', {}, t(text[i] ?? 'about.basedOnMD', { author: c.author })),
      h('p', { class: 'about-coffee' }, '☕ ' + t('about.coffeeFor', { author: c.author }))))));
}

function releaseNotes(list: Release[]): HTMLElement {
  if (list.length === 0) return h('p', { class: 'muted' }, t('state.loading'));
  return h('div', { class: 'about-releases' }, ...list.map((r, i) => {
    const n = r.groups.reduce((s, g) => s + g.items.length, 0);
    return h('details', { class: 'about-release', open: i === 0 },
      h('summary', {}, h('span', { class: 'mono about-rel-v' }, r.version), i === 0 && h('span', { class: 'pill online' }, t('about.latest')),
        h('span', { class: 'muted' }, t('about.changes', { n })), h('span', { class: 'spacer' }), h('span', { class: 'muted mono' }, r.date)),
      r.summary && h('p', {}, r.summary),
      ...r.groups.map((g) => h('div', { class: 'about-group' }, h('div', { class: 'about-group-title' }, g.title),
        h('ul', {}, ...g.items.map((it) => h('li', {}, it))))));
  }));
}

function depTable(title: string, deps: Dep[]): HTMLElement {
  return h('div', {}, h('h3', { class: 'about-h3' }, title), h('div', { class: 'table-wrap' }, h('table', { class: 'data' },
    h('thead', {}, h('tr', {}, h('th', {}, t('about.depName')), h('th', {}, t('about.version')), h('th', {}, t('about.license')))),
    h('tbody', {}, ...deps.map((d) => h('tr', {}, h('td', { class: 'mono' }, d.name), h('td', { class: 'mono' }, d.version), h('td', {}, d.license)))))));
}

function tabsCard(a: About, releases: Release[], webDeps: Dep[]): HTMLElement {
  const body = h('div');
  const tabs = h('div', { class: 'tabs', role: 'tablist' });
  const count = (n: number): HTMLElement => h('span', { class: 'card-badge' }, String(n));
  const panes: [string, (Node | string)[], () => Node[]][] = [
    ['notes', [t('about.releaseNotes'), count(releases.length)], () => [releaseNotes(releases)]],
    ['deps', [t('about.deps'), count(a.deps.length + webDeps.length)], () => [depTable(t('about.depsGo'), a.deps), depTable(t('about.depsWeb'), webDeps)]],
    ['credits', [t('about.credits')], () => [h('dl', { class: 'kv' }, ...a.credits.flatMap((c) => [
      h('dt', {}, link(c.url, c.name)), h('dd', {}, `${c.what} — ${c.author} — ${c.license}`)]))]],
    ['help', [t('help.title')], () => HELP.map((k) =>
      h('details', { class: 'help-item' }, h('summary', {}, t(`help.${k}.title` as Key)), ...t(`help.${k}` as Key).split('\n').map((l) => h('p', {}, l))))],
  ];
  const show = (id: string): void => {
    tabs.querySelectorAll('button').forEach((b) => b.setAttribute('aria-selected', String(b.dataset.tab === id)));
    body.replaceChildren(...(panes.find((p) => p[0] === id)?.[2]() ?? []));
  };
  for (const [id, label] of panes) tabs.append(h('button', { role: 'tab', 'data-tab': id, onclick: () => show(id) }, ...label));
  show('notes');
  return h('section', { class: 'card' }, tabs, body);
}

export const aboutPage: Page = {
  id: 'about',
  title: 'nav.about',
  icon: ICONS.about,
  async render(main) {
    const [a, u, changelog, webDeps] = await Promise.all([
      api.about(),
      api.update().catch(() => null),
      textFile('/api/v1/about/changelog'),
      fetch('/deps.json').then((r) => (r.ok ? r.json() as Promise<Dep[]> : [])).catch(() => [] as Dep[]),
    ]);
    main.append(
      hero(a, u),
      h('div', { class: 'about-grid' }, system(a), support(a)),
      basedOn(a),
      tabsCard(a, parseChangelog(changelog), webDeps),
    );
  },
};
