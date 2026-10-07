// The Firmware page: the FW Update panel for the selected devices, or for all
// devices when none is selected (DECISIONS P12-2).
import { allDevices, loadDevices } from '../devices';
import { h, ICONS } from '../dom';
import { t } from '../i18n';
import { openFirmwareWizard } from '../panels/apwizard';
import { firmwarePanel, type FirmwarePanel } from '../panels/firmware';
import { openingScope, selected, takeHashIds } from '../selection';
import { toast } from '../toast';
import { card, scopeBanner, type Page } from './common';

let panel: FirmwarePanel | null = null;

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
    const c = card(t('fw.title'), null, scopeBox, h('p', { class: 'muted' }, t('fw.intro')), panelBox, h('div', { class: 'row' }, update,
      h('button', { class: 'btn', title: t('apw.openTip'), onclick: () => openFirmwareWizard() }, t('apw.open'))));
    c.querySelector('.card-head')!.append(badge);
    main.append(c);
    show();
  },
  dispose() { panel?.dispose(); panel = null; },
};
