// Application shell: sidebar, top bar, hash router, connection state, first run.
import { api, authApi, onLoginRequired, type FullSettings, type Status, type UpdateStatus } from './api';
import { initAppearance } from './appearance';
import { h, icon, ICONS } from './dom';
import { LANGUAGES, isLang, setLang, storedLang, t } from './i18n';
import { navMode, setNavMode, toggled } from './navmode';
import { aboutPage } from './pages/about';
import { showLogin } from './pages/login';
import { errorState, loadingState, type Page } from './pages/common';
import { chartsPage } from './pages/charts';
import { checklistPage } from './pages/checklist';
import { deferredPage, sidebarBadge, wireDeferredEvents } from './pages/deferred';
import { devicesPage } from './pages/devices';
import { firmwarePage } from './pages/firmware';
import { wireFirmwareEvents } from './panels/firmware';
import { wireBLUEvents } from './panels/bluidentify';
import { settingsPage } from './pages/settings';
import { EventSocket, type ConnState } from './socket';
import { loadDevices, setArchiveInUse, wireDeviceEvents } from './devices';

const pages: Page[] = [
  devicesPage,
  checklistPage,
  chartsPage,
  firmwarePage,
  deferredPage,
  settingsPage(() => renderShell()),
  aboutPage,
];

let updateStatus: UpdateStatus | null = null;
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
  // Below 900px the sidebar is a drawer behind the ☰ button (P12-9). A page change
  // rebuilds the shell, which closes it.
  const setOpen = (open: boolean): void => {
    shell.classList.toggle('nav-open', open);
    menuBtn.setAttribute('aria-expanded', String(open));
  };
  const menuBtn = h('button', { class: 'menu-btn', 'aria-label': t('nav.menu'), title: t('nav.menu'), 'aria-controls': 'sidebar', 'aria-expanded': 'false',
    onclick: () => setOpen(!shell.classList.contains('nav-open')) }, icon(ICONS.menu, 22));
  // Wide screens: full or minimal sidebar, as in Home Assistant (remembered per browser).
  const mini = navMode() === 'mini';
  const sideBtn = h('button', { class: 'side-btn', 'aria-controls': 'sidebar', 'aria-expanded': String(!mini),
    title: t(mini ? 'nav.expand' : 'nav.collapse'), 'aria-label': t(mini ? 'nav.expand' : 'nav.collapse'),
    onclick: () => { setNavMode(toggled(navMode())); renderShell(); } }, icon(mini ? ICONS.menu : ICONS.menuOpen, 22));
  const logout = status.authEnabled && !status.ingress
    ? h('button', { class: 'btn logout', title: t('login.logoutTip'), 'aria-label': t('login.logout'), onclick: async () => { await authApi.logout().catch(() => {}); location.reload(); } },
      icon(ICONS.logout, 16), h('span', { class: 'label' }, t('login.logout')))
    : null;
  const shell = h('div', { class: mini ? 'shell nav-mini' : 'shell' },
    h('div', { class: 'nav-back', onclick: () => setOpen(false) }),
    h('aside', { class: 'sidebar', id: 'sidebar', onkeydown: (e: Event) => { if ((e as KeyboardEvent).key === 'Escape') { setOpen(false); menuBtn.focus(); } } },
      h('div', { class: 'brand' }, sideBtn, h('span', { class: 'label' }, t('app.name')), h('img', { src: 'logo.png', alt: '', width: 28, height: 28 })),
      h('nav', { 'aria-label': 'Main', onclick: () => setOpen(false) }, nav),
      h('div', { class: 'nav-foot' }, h('span', { class: 'label' }, version && (version === 'dev' ? 'dev' : `v${version}`)), logout)),
    h('header', { class: 'topbar' }, menuBtn, h('h1', {}, t(page.title)), h('div', { class: 'spacer' }), connLabel()),
    main);
  app.replaceChildren(shell);
  document.title = `${t(page.title)} · ${t('app.name')}`;

  if (!status.authEnabled && !status.ingress && !noAuthDismissed()) { // behind Home Assistant's login under ingress
    const banner = h('div', { class: 'banner warn', role: 'note' }, t('banner.noAuth'),
      h('button', { class: 'btn close', onclick: () => { dismiss(); banner.remove(); } }, t('banner.dismiss')));
    main.append(banner);
  }
  if (updateStatus?.newer && updateStatus.latest) { // ApplicationUpdateCHK: a newer release, with "skip this version"
    const latest = updateStatus.latest;
    const banner = h('div', { class: 'banner', role: 'note' }, t('update.available', { version: latest }),
      updateStatus.url ? h('a', { href: updateStatus.url, target: '_blank', rel: 'noopener' }, t('update.notes')) : null,
      h('button', { class: 'btn close', onclick: async () => { await api.updateSettings({ skipVersion: latest }).catch(() => {}); banner.remove(); } }, t('update.skip')));
    main.append(banner);
  }
  const content = h('div', { style: 'display:contents' }, loadingState());
  main.append(content);
  Promise.resolve()
    .then(() => { const frag = h('div', { style: 'display:contents' }); return Promise.resolve(page.render(frag)).then(() => frag); })
    .then((frag) => content.replaceWith(frag))
    .catch((err) => content.replaceChildren(errorState(err)));
}

// Dismissed once per browser: the warning is also in the first-run dialog and the server log.
function noAuthDismissed(): boolean {
  try { return localStorage.getItem('sl_noauth_dismissed') === '1'; } catch { return false; }
}
function dismiss(): void {
  try { localStorage.setItem('sl_noauth_dismissed', '1'); } catch { /* private mode */ }
}

function firstRun(defaultLang: string): void {
  const langSel = h('select', { id: 'frLang' }, ...LANGUAGES.map((l) => h('option', { value: l.id }, l.label)));
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
  status = await api.status();
  if (status.loggedIn === false) { // the optional UI password (DECISIONS §24)
    const nav = navigator.language.slice(0, 2);
    setLang(storedLang() ?? (isLang(nav) ? nav : 'en'), false);
    showLogin(app);
    return;
  }
  onLoginRequired(() => location.reload()); // the session ended: back to the login
  const [settings, about] = await Promise.all([api.settings(), api.about()]);
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
  wireBLUEvents(socket);
  socket.on('update.status', (ev) => { updateStatus = ev.data as UpdateStatus; renderShell(); });
  void api.update().then((u) => { updateStatus = u; if (u.newer) renderShell(); }).catch(() => {});
  socket.connect();
  void loadDevices().catch(() => { /* retried on the socket's hello */ });

  window.addEventListener('hashchange', renderShell);
  renderShell();
  if (!status.firstRunDone) firstRun(settings.language);
}

boot().catch((err) => {
  app.replaceChildren(errorState(err));
});
