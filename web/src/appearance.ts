// Palette, theme, contrast, brightness, font and font size — per browser, in
// localStorage; the server never sees any of it.
//
// Adapted from MikroDash (https://github.com/SecOps-7/MikroDash,
// web/src/appearance.ts and web/src/gen/appearance-tables.ts), MIT licence,
// Copyright (c) 2026 MikroDash. The factor tables, the brightness formula and
// the "neutral level removes the override" rule are MikroDash's. Different
// here: base colours are read from the stylesheet instead of a generated table.

export type RGBA = [number, number, number, number];

/** The neutral notch of every slider: at this level no override is set. */
export const APPEAR_DEFAULT = 8;
export const SLIDER_MAX = 15;

export const KEYS = {
  theme: 'sl_theme',
  palette: 'sl_palette',
  contrast: 'sl_contrast',
  textBright: 'sl_text_bright',
  bgBright: 'sl_bg_bright',
  font: 'sl_font',
  fontSize: 'sl_font_size',
} as const;

/** Palettes in app.css. `light` marks those with a light variant. */
export const PALETTES: { id: string; label: string; light: boolean; swatch: [string, string] }[] = [
  { id: 'default', label: 'Default', light: true, swatch: ['#07090f', '#38bdf8'] },
  { id: 'nord', label: 'Nord', light: true, swatch: ['#1e2430', '#88c0d0'] },
  { id: 'catppuccin', label: 'Catppuccin', light: true, swatch: ['#11111b', '#89b4fa'] },
  { id: 'dracula', label: 'Dracula', light: false, swatch: ['#1c1e26', '#8be9fd'] },
  { id: 'tokyo', label: 'Tokyo Night', light: false, swatch: ['#13141e', '#7aa2f7'] },
  { id: 'gruvbox', label: 'Gruvbox', light: true, swatch: ['#1d2021', '#83a598'] },
  { id: 'rosepine', label: 'Rosé Pine', light: true, swatch: ['#14121e', '#c4a7e7'] },
  { id: 'rosepine-moon', label: 'Rosé Pine Moon', light: false, swatch: ['#1d1b30', '#c4a7e7'] },
  { id: 'onedark', label: 'One Dark', light: true, swatch: ['#21252b', '#61afef'] },
  { id: 'solarized', label: 'Solarized', light: true, swatch: ['#002b36', '#268bd2'] },
  { id: 'everforest', label: 'Everforest', light: false, swatch: ['#1e2528', '#a7c080'] },
  { id: 'kanagawa', label: 'Kanagawa', light: false, swatch: ['#16161d', '#7e9cd8'] },
  { id: 'monokai', label: 'Monokai', light: false, swatch: ['#1d1e19', '#66d9e8'] },
  { id: 'monokai-pro', label: 'Monokai Pro', light: false, swatch: ['#1e1c20', '#78dce8'] },
  { id: 'material', label: 'Material', light: true, swatch: ['#1b2528', '#80cbc4'] },
  { id: 'palenight', label: 'Palenight', light: false, swatch: ['#202336', '#82aaff'] },
  { id: 'github', label: 'GitHub', light: true, swatch: ['#010409', '#58a6ff'] },
  { id: 'homeassistant', label: 'Home Assistant', light: true, swatch: ['#111111', '#009ac7'] },
];

/** Fonts bundled under /fonts (OFL), plus the system font. */
export const FONTS: { id: string; label: string; family: string }[] = [
  { id: 'oxanium', label: 'Oxanium', family: "'Oxanium',sans-serif" },
  { id: 'inter', label: 'Inter', family: "'Inter',sans-serif" },
  { id: 'ibm-plex-sans', label: 'IBM Plex Sans', family: "'IBM Plex Sans',sans-serif" },
  { id: 'nunito', label: 'Nunito', family: "'Nunito',sans-serif" },
  { id: 'roboto', label: 'Roboto', family: "'Roboto',sans-serif" },
  { id: 'jetbrains-mono', label: 'JetBrains Mono', family: "'JetBrains Mono',monospace" },
  { id: 'system', label: 'System', family: 'system-ui,-apple-system,"Segoe UI",sans-serif' },
];
export const DEFAULT_FONT = 'oxanium';

/** `px: null` removes the property (browser default), which is not the same as 16px. */
export const FONT_SIZES: { id: 'xs' | 'sm' | 'normal' | 'md' | 'lg' | 'xl'; px: number | null }[] = [
  { id: 'xs', px: 12 }, { id: 'sm', px: 14 }, { id: 'normal', px: null },
  { id: 'md', px: 18 }, { id: 'lg', px: 20 }, { id: 'xl', px: 22 },
];

/** Indexed by level − 1 (from MikroDash). */
export const CONTRAST_FACTORS = [0.15, 0.25, 0.35, 0.5, 0.65, 0.8, 0.92, 1, 1.2, 1.5, 2, 2.75, 3.5, 4.5, 6];
export const BRIGHT_FACTORS = [0.2, 0.3, 0.42, 0.55, 0.65, 0.78, 0.9, 1, 1.05, 1.1, 1.17, 1.25, 1.33, 1.42, 1.5];

function lsGet(k: string): string | null {
  try { return localStorage.getItem(k); } catch { return null; }
}
function lsSet(k: string, v: string): void {
  try { localStorage.setItem(k, v); } catch { /* private mode */ }
}
function lsDel(k: string): void {
  try { localStorage.removeItem(k); } catch { /* private mode */ }
}
const root = (): HTMLElement => document.documentElement;

export function factor(table: number[], level: number): number {
  return table[Math.max(0, Math.min(table.length - 1, level - 1))]!;
}

/** Scale towards white (f > 1, interpolating so 255 stays 255) or towards black (f ≤ 1). Alpha untouched. */
export function scaleBright(c: RGBA, f: number): RGBA {
  const ch = (v: number): number => {
    const r = f > 1 ? v + (255 - v) * Math.min(1, f - 1) : v * f;
    return Math.min(255, Math.round(r));
  };
  return [ch(c[0]), ch(c[1]), ch(c[2]), c[3]];
}

/** Parse #rgb, #rrggbb, rgb() or rgba() as produced by the stylesheet. */
export function parseColor(s: string): RGBA | null {
  const v = s.trim();
  let m = /^#([0-9a-f]{3}|[0-9a-f]{6})$/i.exec(v);
  if (m) {
    let hex = m[1]!;
    if (hex.length === 3) hex = hex.split('').map((c) => c + c).join('');
    return [parseInt(hex.slice(0, 2), 16), parseInt(hex.slice(2, 4), 16), parseInt(hex.slice(4, 6), 16), 1];
  }
  m = /^rgba?\(\s*([\d.]+)\s*,\s*([\d.]+)\s*,\s*([\d.]+)\s*(?:,\s*([\d.]+)\s*)?\)$/i.exec(v);
  if (m) return [Number(m[1]), Number(m[2]), Number(m[3]), m[4] === undefined ? 1 : Number(m[4])];
  return null;
}

const rgba = (c: RGBA): string => `rgba(${c[0]},${c[1]},${c[2]},${+c[3].toFixed(3)})`;

/** The stylesheet's own value for a custom property, ignoring our inline override. */
function baseColor(prop: string): RGBA | null {
  const saved = root().style.getPropertyValue(prop);
  root().style.removeProperty(prop);
  const v = getComputedStyle(root()).getPropertyValue(prop);
  if (saved) root().style.setProperty(prop, saved);
  return parseColor(v);
}

function level(attr: string): number {
  return Number.parseInt(root().getAttribute(attr) || String(APPEAR_DEFAULT), 10) || APPEAR_DEFAULT;
}

/** Text: brightness moves the channels, contrast moves the alpha. */
export function reapplyTextVars(): void {
  const cl = level('data-contrast');
  const bl = level('data-text-bright');
  for (const p of ['--text-main', '--text-muted']) root().style.removeProperty(p);
  if (cl === APPEAR_DEFAULT && bl === APPEAR_DEFAULT) return;
  const cf = factor(CONTRAST_FACTORS, cl);
  const bf = factor(BRIGHT_FACTORS, bl);
  for (const p of ['--text-main', '--text-muted']) {
    const c = baseColor(p);
    if (!c) continue;
    const s = scaleBright(c, bf);
    root().style.setProperty(p, rgba([s[0], s[1], s[2], Math.min(1, s[3] * cf)]));
  }
}

/** Background: one slider; alpha stays as the palette set it. */
export function reapplyBgVars(): void {
  const bl = level('data-bg-bright');
  for (const p of ['--bg-deep', '--bg-card']) root().style.removeProperty(p);
  if (bl === APPEAR_DEFAULT) return;
  const bf = factor(BRIGHT_FACTORS, bl);
  for (const p of ['--bg-deep', '--bg-card']) {
    const c = baseColor(p);
    if (c) root().style.setProperty(p, rgba(scaleBright(c, bf)));
  }
}

function reapply(): void {
  reapplyTextVars();
  reapplyBgVars();
}

export function currentTheme(): 'dark' | 'light' {
  return root().getAttribute('data-theme') === 'light' ? 'light' : 'dark';
}
export function currentPalette(): string {
  return root().getAttribute('data-palette') || 'default';
}

/** The default palette removes the attribute: the base rules are the default. */
export function applyPalette(palette: string, theme: 'dark' | 'light'): void {
  if (!palette || palette === 'default') root().removeAttribute('data-palette');
  else root().setAttribute('data-palette', palette);
  root().setAttribute('data-theme', theme);
  lsSet(KEYS.palette, palette || 'default');
  lsSet(KEYS.theme, theme);
  reapply();
}

export function applyTheme(theme: 'dark' | 'light'): void {
  const p = PALETTES.find((x) => x.id === currentPalette());
  // A palette without a light variant falls back to the default one in light mode.
  applyPalette(theme === 'light' && p && !p.light ? 'default' : currentPalette(), theme);
}

export function applyLevel(attr: 'data-contrast' | 'data-text-bright' | 'data-bg-bright', value: number): void {
  const key = attr === 'data-contrast' ? KEYS.contrast : attr === 'data-text-bright' ? KEYS.textBright : KEYS.bgBright;
  root().setAttribute(attr, String(value));
  lsSet(key, String(value));
  if (attr === 'data-bg-bright') reapplyBgVars(); else reapplyTextVars();
}

export function applyFont(id: string): void {
  const f = FONTS.find((x) => x.id === id) || FONTS.find((x) => x.id === DEFAULT_FONT)!;
  root().style.setProperty('--font-ui', f.family);
  lsSet(KEYS.font, f.id);
}

export function applyFontSize(id: string): void {
  const s = FONT_SIZES.find((x) => x.id === id) || FONT_SIZES[2]!;
  if (s.px === null) root().style.removeProperty('font-size');
  else root().style.fontSize = s.px + 'px';
  lsSet(KEYS.fontSize, s.id);
}

export function storedFont(): string {
  return lsGet(KEYS.font) || DEFAULT_FONT;
}
export function storedFontSize(): string {
  return lsGet(KEYS.fontSize) || 'normal';
}

/** Run before first paint (preflight) and again at startup. */
export function initAppearance(): void {
  const theme = lsGet(KEYS.theme) === 'light' ? 'light' : 'dark';
  const palette = lsGet(KEYS.palette) || 'default';
  if (palette !== 'default') root().setAttribute('data-palette', palette);
  root().setAttribute('data-theme', theme);
  for (const [attr, key] of [['data-contrast', KEYS.contrast], ['data-text-bright', KEYS.textBright], ['data-bg-bright', KEYS.bgBright]] as const) {
    const v = Number.parseInt(lsGet(key) || '', 10);
    root().setAttribute(attr, String(v >= 1 && v <= SLIDER_MAX ? v : APPEAR_DEFAULT));
  }
  applyFont(storedFont());
  applyFontSize(storedFontSize());
  reapply();
}

export function resetAppearance(): void {
  for (const k of Object.values(KEYS)) lsDel(k);
  root().removeAttribute('data-palette');
  root().style.removeProperty('--font-ui');
  root().style.removeProperty('font-size');
  initAppearance();
}
