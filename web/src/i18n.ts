// UI strings (DECISIONS.md §8, Q21: English and Dutch; more languages added on request). The language is a
// per-browser choice; the server setting is only the default for new browsers.
import bg from './i18n/bg.json';
import de from './i18n/de.json';
import en from './i18n/en.json';
import es from './i18n/es.json';
import fr from './i18n/fr.json';
import it from './i18n/it.json';
import nl from './i18n/nl.json';
import zh from './i18n/zh.json';

export type Lang = 'en' | 'nl' | 'de' | 'fr' | 'es' | 'it' | 'bg' | 'zh';
export type Key = keyof typeof en;

export const CATALOGUES: Record<Lang, Record<string, string>> = { en, nl, de, fr, es, it, bg, zh };
// Each language under its own name, as a language picker shows them.
export const LANGUAGES: { id: Lang; label: string }[] = [
  { id: 'en', label: 'English' },
  { id: 'nl', label: 'Nederlands' },
  { id: 'de', label: 'Deutsch' },
  { id: 'fr', label: 'Français' },
  { id: 'es', label: 'Español' },
  { id: 'it', label: 'Italiano' },
  { id: 'bg', label: 'Български' },
  { id: 'zh', label: '中文' },
];

const STORE_KEY = 'sl_lang';
let current: Lang = 'en';

export function isLang(v: unknown): v is Lang {
  return typeof v === 'string' && v in CATALOGUES;
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
