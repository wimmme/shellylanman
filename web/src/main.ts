// Application shell: sidebar, top bar, hash router, connection state, first run.
import { api, type FullSettings, type Status } from './api';
import { initAppearance } from './appearance';
import { h, icon, ICONS } from './dom';
import { isLang, setLang, storedLang, t } from './i18n';
import { aboutPage } from './pages/about';
import { errorState, loadingState, placeholder, type Page } from './pages/common';
import { checklistPage } from './pages/checklist';
import { deferredPage, sidebarBadge, wireDeferredEvents } from './pages/deferred';
import { devicesPage } from './pages/devices';
import { firmwarePage } from './pages/firmware';
import { wireFirmwareEvents } from './panels/firmware';
import { settingsPage } from './pages/settings';
import { EventSocket, type ConnState } from './socket';
import { loadDevices, setArchiveInUse, wireDeviceEvents } from './devices';

const pages: Page[] = [
  devicesPage,
  checklistPage,
  placeholder('charts', 'nav.charts', ICONS.charts, 'page.charts.text'),
  firmwarePage,
  deferredPage,
  settingsPage(() => renderShell()),
  aboutPage,
];

let status: Status = { firstRunDone: true, authEnabled: false, clients: 0 };
let conn: ConnState = 'connecting';
let version = '';
const app = document.getElementById('app')!;

function currentPage(): Page {
  const id = location.hash.replace(/^#\/?/, '').split('?')[0];
  return pages.find((p) => p.id === id) || pages[0]!;
}

function connLabel(): HTMLElement {
  const key = conn === 'ok' ? 'conn.ok' : conn === 'down' ? 'conn.down' : 'conn.connecting';
  return h('span', { class: 'conn ' + conn, id: 'conn', role: 'status' }, t(key));
}

let shown: Page | null = null;

function renderShell(): void {
  const page = currentPage();
  shown?.dispose?.();
  shown = page;
  const nav = h('ul', { class: 'nav' }, ...pages.map((p) =>
    h('li', {}, h('a', { href: '#/' + p.id, class: p === page ? 'active' : undefined, 'aria-current': p === page ? 'page' : undefined, title: t(p.title) },
      icon(p.icon), h('span', { class: 'label' }, t(p.title)), p.id === 'deferred' ? sidebarBadge() : null))));
  const main = h('main', { class: 'main', id: 'main' });
  const shell = h('div', { class: 'shell' },
    h('aside', { class: 'sidebar' },
      h('div', { class: 'brand' }, icon(ICONS.logo, 22), h('span', { class: 'label' }, t('app.name'))),
      h('nav', { 'aria-label': 'Main' }, nav),
      h('div', { class: 'nav-foot' }, version && (version === 'dev' ? 'dev' : `v${version}`))),
    h('header', { class: 'topbar' }, h('h1', {}, t(page.title)), h('div', { class: 'spacer' }), connLabel()),
    main);
  app.replaceChildren(shell);
  document.title = `${t(page.title)} · ${t('app.name')}`;

  if (!status.authEnabled && !sessionDismissed()) {
    const banner = h('div', { class: 'banner warn', role: 'note' }, t('banner.noAuth'),
      h('button', { class: 'btn close', onclick: () => { dismiss(); banner.remove(); } }, t('banner.dismiss')));
    main.append(banner);
  }
  const content = h('div', { style: 'display:contents' }, loadingState());
  main.append(content);
  Promise.resolve()
    .then(() => { const frag = h('div', { style: 'display:contents' }); return Promise.resolve(page.render(frag)).then(() => frag); })
    .then((frag) => content.replaceWith(frag))
    .catch((err) => content.replaceChildren(errorState(err)));
}

function sessionDismissed(): boolean {
  try { return sessionStorage.getItem('sl_noauth_dismissed') === '1'; } catch { return false; }
}
function dismiss(): void {
  try { sessionStorage.setItem('sl_noauth_dismissed', '1'); } catch { /* ignore */ }
}

function firstRun(defaultLang: string): void {
  const langSel = h('select', { id: 'frLang' }, h('option', { value: 'en' }, 'English'), h('option', { value: 'nl' }, 'Nederlands'));
  langSel.value = defaultLang;
  langSel.addEventListener('change', () => { if (isLang(langSel.value)) { setLang(langSel.value, false); back.remove(); firstRun(langSel.value); } });
  const start = h('button', { class: 'btn primary', onclick: async () => {
    start.setAttribute('disabled', '');
    await api.updateSettings({ firstRunDone: true, language: langSel.value });
    status.firstRunDone = true;
    back.remove();
    renderShell();
  } }, t('firstrun.start'));
  const back = h('div', { class: 'modal-back' },
    h('div', { class: 'modal', role: 'dialog', 'aria-modal': 'true', 'aria-labelledby': 'frTitle' },
      h('header', { id: 'frTitle' }, t('firstrun.title')),
      h('div', { class: 'body' },
        h('p', {}, t('firstrun.text')),
        h('div', { class: 'field' }, h('label', { for: 'frLang' }, t('firstrun.language')), langSel),
        h('p', { class: 'muted' }, t('firstrun.authNote'))),
      h('footer', {}, start)));
  document.body.append(back);
  start.focus();
}

async function boot(): Promise<void> {
  initAppearance();
  const [st, settings, about] = await Promise.all([api.status(), api.settings(), api.about()]);
  status = st;
  version = about.version;
  const stored = storedLang();
  setLang(stored ?? (isLang(settings.language) ? settings.language : 'en'), false);
  setArchiveInUse((settings as FullSettings).archive?.use !== false);

  const socket = new EventSocket((s) => {
    conn = s;
    document.getElementById('conn')?.replaceWith(connLabel());
  });
  socket.on('settings.changed', (ev) => {
    const s = ev.data as { language?: string; archive?: { use?: boolean } } | undefined;
    if (s?.archive) setArchiveInUse(s.archive.use !== false);
    if (!storedLang() && isLang(s?.language)) { setLang(s.language, false); renderShell(); }
  });
  wireDeviceEvents(socket);
  wireDeferredEvents(socket);
  wireFirmwareEvents(socket);
  socket.connect();
  void loadDevices().catch(() => { /* retried on the socket's hello */ });

  window.addEventListener('hashchange', renderShell);
  renderShell();
  if (!status.firstRunDone) firstRun(settings.language);
}

boot().catch((err) => {
  app.replaceChildren(errorState(err));
});
