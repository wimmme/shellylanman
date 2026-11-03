// Local firmware download via QR code (the one new feature, DECISIONS §4.4): the "Shelly index"
// and "Local update" columns of the Firmware page; the ⚡ of the second opens the access-point
// wizard (DECISIONS §29), which gives the phone the right file before it joins the device's own
// access point.
import type { IndexRow } from '../api';
import { h } from '../dom';
import { t } from '../i18n';
import { openFirmwareWizard } from './apwizard';

/** "-" as on the Devices and Checklist pages: nothing to show here. */
export const na = (title?: string): HTMLElement => h('span', { class: 'muted', title }, '-');

/** The index cell: the latest stable version in Shelly's index (and its source). */
export function indexCell(r: IndexRow | undefined, loading: boolean): Node {
  if (!r) return loading ? document.createTextNode('…') : na();
  if (r.error) return na(r.error);
  return h('span', { title: t(r.source === 'shelly-tools' ? 'lfw.srcArchive' : 'lfw.srcShelly') }, r.latest ?? '');
}

/**
 * The local-update cell: ⚡ through the device's access point, always there for a model in the
 * index. It follows the tick: the beta when beta is ticked, else the stable version, also when the
 * device already runs it or a newer one (a re-flash, DECISIONS P29-2/3). Greyed only when the
 * index has no beta.
 */
export function localCell(r: IndexRow | undefined, loading: boolean, stage: 'stable' | 'beta'): Node {
  if (!r) return loading ? document.createTextNode('…') : na();
  if (r.error) return na(r.error);
  const version = stage === 'beta' ? r.beta : r.latest;
  const newer = stage === 'beta' ? r.betaNewer : r.newer;
  const b = h('button', { class: 'btn small', disabled: !version, title: version ? t(newer ? 'lfw.toVersion' : 'lfw.reflashTip', { version }) : undefined,
    onclick: (e: Event) => { e.stopPropagation(); openFirmwareWizard({ id: r.id, stage }); } }, '⚡ ' + t('lfw.viaAP'));
  // Browsers show no tooltip on a disabled button: the reason sits on a wrapper (P12-3).
  return version ? b : h('span', { class: 'tip-wrap', title: t('lfw.noBeta') }, b);
}
