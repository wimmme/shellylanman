// The Firmware page: the FW Update panel for the selected devices, or for all
// devices when none is selected (DECISIONS P12-2).
import { devicesApi } from '../api';
import { allDevices, loadDevices } from '../devices';
import { h, ICONS } from '../dom';
import { t } from '../i18n';
import { openFirmwareWizard } from '../panels/apwizard';
import { firmwarePanel, type FirmwarePanel } from '../panels/firmware';
import { openingScope, selected, takeHashIds } from '../selection';
import { toast } from '../toast';
import { card, scopeBanner, type Page } from './common';

let panel: FirmwarePanel | null = null;

// Shelly's own release notes, per API generation (DECISIONS §40).
const NOTES = { gen2: 'https://shelly-api-docs.shelly.cloud/gen2/changelog/', gen1: 'https://shelly-api-docs.shelly.cloud/gen1/#changelog' };

function releaseNotes(): HTMLElement {
  const link = (href: string, text: string): HTMLAnchorElement =>
    h('a', { href, target: '_blank', rel: 'noopener noreferrer', title: t('fw.notesTip') }, text);
  return h('p', { class: 'muted fw-notes' }, t('fw.notes'), ' ',
    link(NOTES.gen2, t('fw.notesGen2')), ' · ', link(NOTES.gen1, t('fw.notesGen1')));
}

export const firmwarePage: Page = {
  id: 'firmware',
  title: 'nav.firmware',
  icon: ICONS.firmware,
  async render(main) {
    takeHashIds('firmware');
    if (!allDevices().length) await loadDevices().catch(() => {}); // page opened directly: the list is still loading
    const known = (id: string): boolean => allDevices().some((d) => d.id === id);
    let ids = openingScope(selected, known);
    const scopeBox = h('div', { style: 'display:contents' });
    const panelBox = h('div', { style: 'display:contents' });
    const badge = h('span', { class: 'card-badge' });
    const update = h('button', { class: 'btn primary', onclick: async () => {
      if (!panel) return;
      update.setAttribute('disabled', '');
      try { await panel.apply(); } catch (e) { toast(e instanceof Error ? e.message : String(e)); } finally { update.removeAttribute('disabled'); }
    } }, t('fw.update'));
    const show = (): void => {
      panel?.dispose();
      panel = firmwarePanel(ids, true);
      panelBox.replaceChildren(panel.el);
      badge.textContent = ids.length ? String(ids.length) : t('fw.all');
      const banner = scopeBanner(ids.length > 0, ids.length || openingScope(selected, known).length, () => {
        ids = ids.length ? [] : openingScope(selected, known);
        show();
      });
      scopeBox.replaceChildren(...(banner ? [banner] : []));
    };
    const intro = h('p', { class: 'muted' }, t('fw.intro'));
    devicesApi.settings().then((s) => { if (s.firmware?.mode === 'local') intro.textContent = t('fw.introLocal'); }).catch(() => {});
    const c = card(t('fw.title'), null, scopeBox, intro, releaseNotes(), panelBox, h('div', { class: 'row' }, update,
      h('button', { class: 'btn', title: t('apw.openTip'), onclick: () => openFirmwareWizard() }, t('apw.open'))));
    c.querySelector('.card-head')!.append(badge);
    main.append(c);
    show();
    // #/firmware?apw=<id>: the code in the wizard on a PC, scanned with a phone, opens it here.
    const apw = /[?&]apw=([^&]*)/.exec(location.hash);
    if (apw) {
      const stage = /[?&]stage=beta/.test(location.hash) ? 'beta' : 'stable';
      history.replaceState(null, '', '#/firmware');
      const id = decodeURIComponent(apw[1]!);
      openFirmwareWizard(known(id) ? { id, stage } : {});
    }
  },
  dispose() { panel?.dispose(); panel = null; },
};
