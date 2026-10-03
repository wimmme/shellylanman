// Why a button is disabled, for its tooltip (DECISIONS P12-3, P12-4).
import { t } from './i18n';

export type Why = { key: 'why.none' } | { key: 'why.one' } | { key: 'why.missing'; n: number } | { key: 'why.differ' };

/**
 * The reason an action is not available for the ticked items, or null when it
 * is: nothing ticked, more than one where one is needed, items the action does
 * not apply to (`has`), else values that differ.
 */
export function whyDisabled<T>(items: T[], enabled: boolean, has: (x: T) => boolean = () => true, oneOnly = false): Why | null {
  if (enabled) return null;
  if (!items.length) return { key: 'why.none' };
  if (oneOnly && items.length !== 1) return { key: 'why.one' };
  const n = items.filter((x) => !has(x)).length;
  if (n) return { key: 'why.missing', n };
  return { key: 'why.differ' };
}

/** A button's tooltip: what it does, then why it is disabled. */
export function tooltip(what: string, why: Why | null): string {
  const reason = why ? t(why.key, why.key === 'why.missing' ? { n: why.n } : {}) : '';
  return [what, reason].filter(Boolean).join('\n');
}
