// Local firmware download via QR code (the one new feature, DECISIONS §4.4): the "Shelly index"
// column of the Firmware page; its ⚡ opens the access-point wizard (DECISIONS §29), which gives the
// phone the right file before it joins the device's own access point.
import type { IndexRow } from '../api';
import { h } from '../dom';
import { t } from '../i18n';
import { openFirmwareWizard } from './apwizard';

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
    h('button', { class: 'btn small', title: t('lfw.buttonTip'), 'aria-label': t('lfw.buttonTip'), onclick: (e: Event) => { e.stopPropagation(); openFirmwareWizard({ id: r.id }); } }, '⚡'));
}
