// Opening a device's own web page (DECISIONS P12-11): in a new tab, or in
// this tab for browsers that allow no new tabs (kiosk browsers on a wall
// tablet). Per browser, like the appearance settings.

const KEY = 'sl_open_pages';

export type OpenMode = 'tab' | 'same';

export function openMode(): OpenMode {
  try { return localStorage.getItem(KEY) === 'same' ? 'same' : 'tab'; } catch { return 'tab'; }
}

export function setOpenMode(m: OpenMode): void {
  try { if (m === 'tab') localStorage.removeItem(KEY); else localStorage.setItem(KEY, m); } catch { /* private mode */ }
}

/** True when several device pages can open at once (one tab each). */
export const manyAtOnce = (): boolean => openMode() === 'tab';

/** A device's web address, without the default port. */
export function deviceURL(ip: string, port: number): string {
  return `http://${port === 80 ? ip : `${ip}:${port}`}`;
}

/**
 * Open device pages. "This tab" opens the first only and replaces the whole
 * window — inside Home Assistant the Home Assistant window, since a device's
 * http:// page cannot load inside its https:// frame.
 */
export function openPages(urls: string[]): void {
  if (!urls.length) return;
  if (openMode() === 'same') {
    try { (window.top ?? window).location.href = urls[0]!; } catch { window.location.href = urls[0]!; }
    return;
  }
  for (const u of urls) window.open(u, '_blank', 'noopener');
}
