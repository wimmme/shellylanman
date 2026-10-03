// The device selection shared by Devices, Checklist and Firmware (DECISIONS
// P12-2): ticking a device on one page ticks it on the others. Kept per browser
// tab in sessionStorage, so a reload keeps it and a second tab has its own.

const KEY = 'sl_selection';

const load = (): string[] => {
  try {
    const v: unknown = JSON.parse(sessionStorage.getItem(KEY) ?? '[]');
    return Array.isArray(v) ? v.filter((x): x is string => typeof x === 'string') : [];
  } catch { return []; }
};

class SharedSelection extends Set<string> {
  private ready = false;
  private listeners = new Set<() => void>();

  constructor() {
    super();
    for (const id of load()) super.add(id);
    this.ready = true;
  }

  override add(id: string): this {
    if (!this.has(id)) { super.add(id); this.changed(); }
    return this;
  }

  override delete(id: string): boolean {
    const had = super.delete(id);
    if (had) this.changed();
    return had;
  }

  override clear(): void {
    if (this.size === 0) return;
    super.clear();
    this.changed();
  }

  /** Replace the whole selection at once (one save, one notification). */
  replace(ids: Iterable<string>): void {
    super.clear();
    for (const id of ids) super.add(id);
    this.changed();
  }

  /** Called after every change; returns the function that stops listening. */
  onChange(f: () => void): () => void {
    this.listeners.add(f);
    return () => { this.listeners.delete(f); };
  }

  private changed(): void {
    if (!this.ready) return;
    try { sessionStorage.setItem(KEY, JSON.stringify([...this])); } catch { /* private mode */ }
    this.listeners.forEach((f) => f());
  }
}

export const selected = new SharedSelection();

/** The ids in the page address (#/checklist?ids=a,b), if any. */
export function idsInHash(hash: string): string[] {
  const m = /[?&]ids=([^&]*)/.exec(hash);
  return m ? decodeURIComponent(m[1]!).split(',').filter(Boolean) : [];
}

/**
 * Which devices a list page shows when it opens: the selected devices that still
 * exist, or every device (an empty list). Taken once, so unticking a row does
 * not make it disappear under the mouse.
 */
export function openingScope(selection: Iterable<string>, known: (id: string) => boolean): string[] {
  return [...selection].filter(known);
}

/**
 * An address with ids (#/checklist?ids=a,b — a link, the Charts button) sets the
 * selection; the ids are then taken out of the address so a reload does not
 * set them again.
 */
export function takeHashIds(page: string): void {
  const ids = idsInHash(location.hash);
  if (!ids.length) return;
  selected.replace(ids);
  history.replaceState(null, '', '#/' + page);
}
