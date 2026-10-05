// The sidebar on wide screens: full (icons and labels) or minimal (icons only),
// remembered per browser like Home Assistant's sidebar. Below 900px the sidebar
// is a drawer behind ☰ either way (P12-9).

export type NavMode = 'full' | 'mini';

const KEY = 'sl_nav';

export function navMode(): NavMode {
  try { return localStorage.getItem(KEY) === 'mini' ? 'mini' : 'full'; } catch { return 'full'; }
}

export function setNavMode(mode: NavMode): void {
  try { localStorage.setItem(KEY, mode); } catch { /* private mode: for this page only */ }
}

export function toggled(mode: NavMode): NavMode {
  return mode === 'mini' ? 'full' : 'mini';
}
