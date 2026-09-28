// Reads CHANGELOG.md (Keep a Changelog) into releases for the About page.

export interface Release { version: string; date: string; summary: string; groups: { title: string; items: string[] }[] }

/** Markdown decoration dropped: the page shows plain text. */
export function plain(s: string): string {
  return s.replace(/\*\*(.+?)\*\*/g, '$1').replace(/`([^`]+)`/g, '$1').replace(/\[([^\]]+)\]\([^)]+\)/g, '$1').trim();
}

export function parseChangelog(md: string): Release[] {
  const out: Release[] = [];
  let rel: Release | null = null;
  let group: Release['groups'][number] | null = null;
  let item = -1;
  for (const raw of md.split(/\r?\n/)) {
    const line = raw.trimEnd();
    const v = /^## \[([^\]]+)\](?:\s*-\s*(\S+))?/.exec(line);
    if (v) {
      rel = { version: v[1]!, date: v[2] ?? '', summary: '', groups: [] };
      out.push(rel);
      group = null; item = -1;
      continue;
    }
    if (!rel) continue;
    const g = /^### (.+)/.exec(line);
    if (g) { group = { title: plain(g[1]!), items: [] }; rel.groups.push(group); item = -1; continue; }
    const li = /^[-*] (.+)/.exec(line);
    if (li && group) { group.items.push(plain(li[1]!)); item = group.items.length - 1; continue; }
    if (line.trim() === '') { item = -1; continue; }
    if (group && item >= 0 && /^\s+/.test(raw)) { group.items[item] = plain(group.items[item] + ' ' + line.trim()); continue; }
    if (!group) rel.summary = plain((rel.summary + ' ' + line.trim()));
  }
  return out.filter((r) => r.groups.length > 0 || r.summary !== '');
}
