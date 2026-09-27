// Local firmware download via QR code (the one new feature, DECISIONS §4.4):
// the modal with the QR code, the link, and the device name, model and target
// version, so the phone gets the right file before it joins the device's own
// access point.
import { ApiError, localFwApi, type IndexRow } from '../api';
import { h } from '../dom';
import { dateTime } from '../format';
import { t, type Key } from '../i18n';
import { openModal } from '../modal';
import { toast } from '../toast';

/** The index cell: latest stable version (source) and the ⚡ button when it is newer. */
export function indexCell(r: IndexRow | undefined, loading: boolean): Node {
  if (!r) return document.createTextNode(loading ? '…' : '');
  if (r.error) {
    const s = h('span', { class: 'muted' }, '—');
    s.title = r.error;
    return s;
  }
  const label = h('span', { title: t(r.source === 'shelly-tools' ? 'lfw.srcArchive' : 'lfw.srcShelly') }, r.latest ?? '');
  if (!r.newer) return label;
  return h('span', { class: 'row lfw-cell' }, label,
    h('button', { class: 'btn small', title: t('lfw.buttonTip'), 'aria-label': t('lfw.buttonTip'), onclick: (e: Event) => { e.stopPropagation(); void openLocalDownload(r.id); } }, '⚡'));
}

export async function openLocalDownload(id: string): Promise<void> {
  let link;
  try {
    link = await localFwApi.link(id);
  } catch (e) {
    toast(e instanceof ApiError || e instanceof Error ? e.message : String(e));
    return;
  }
  const url = h('input', { readonly: true, value: link.url, 'aria-label': t('lfw.link') });
  const copy = h('button', { class: 'btn', onclick: async () => {
    try { await navigator.clipboard.writeText(link.url); toast(t('lfw.copied'), 'info'); } catch { url.select(); }
  } }, t('common.copy'));
  const badge = (k: Key, v: string): HTMLElement => h('div', { class: 'lfw-badge' }, h('span', { class: 'muted' }, t(k)), h('strong', {}, v));
  openModal(t('lfw.title'), h('div', { class: 'lfw' },
    link.warning ? h('div', { class: 'banner warn', role: 'alert' }, t(('lfw.warn.' + link.warning) as Key)) : null,
    h('div', { class: 'lfw-grid' },
      h('img', { class: 'lfw-qr', src: link.qr, alt: t('lfw.qrAlt'), width: 320, height: 320 }),
      h('div', { class: 'lfw-info' },
        badge('lfw.device', link.name), badge('lfw.model', link.model),
        badge('lfw.version', `${link.current} → ${link.version}`),
        badge('lfw.source', t(link.source === 'shelly-tools' ? 'lfw.srcArchive' : 'lfw.srcShelly')),
        badge('lfw.file', link.fileName),
        badge('lfw.expires', dateTime(new Date(link.expires), true)))),
    h('div', { class: 'row' }, url, copy),
    h('p', { class: 'muted' }, t('lfw.howto'))), [{ label: t('common.close') }], undefined, 'wide');
}
