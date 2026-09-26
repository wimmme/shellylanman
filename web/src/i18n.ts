// UI strings in English and Dutch (DECISIONS.md §8, Q21). The language is a
// per-browser choice; the server setting is only the default for new browsers.
import en from './i18n/en.json';
import nl from './i18n/nl.json';

export type Lang = 'en' | 'nl';
export type Key = keyof typeof en;

export const CATALOGUES: Record<Lang, Record<string, string>> = { en, nl };
export const LANGUAGES: { id: Lang; label: string }[] = [
  { id: 'en', label: 'English' },
  { id: 'nl', label: 'Nederlands' },
];

const STORE_KEY = 'sl_lang';
let current: Lang = 'en';

export function isLang(v: unknown): v is Lang {
  return v === 'en' || v === 'nl';
}

/** The browser's stored choice, if any. */
export function storedLang(): Lang | null {
  try {
    const v = localStorage.getItem(STORE_KEY);
    return isLang(v) ? v : null;
  } catch {
    return null;
  }
}

export function setLang(lang: Lang, remember: boolean): void {
  current = lang;
  document.documentElement.lang = lang;
  if (remember) {
    try { localStorage.setItem(STORE_KEY, lang); } catch { /* private mode */ }
  }
}

export function lang(): Lang {
  return current;
}

/** Translate key, substituting {name} placeholders. Falls back to English, then the key. */
export function t(key: Key, vars: Record<string, string | number> = {}): string {
  const s = CATALOGUES[current][key] ?? CATALOGUES.en[key] ?? key;
  return s.replace(/\{(\w+)\}/g, (m, name: string) => (name in vars ? String(vars[name]) : m));
}
