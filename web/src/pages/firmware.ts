// The Firmware page: the FW Update panel for all devices, or for the
// devices given in the address (#/firmware?ids=a,b).
import { h, ICONS } from '../dom';
import { t } from '../i18n';
import { firmwarePanel, type FirmwarePanel } from '../panels/firmware';
import { toast } from '../toast';
import { card, type Page } from './common';

function wantedIds(): string[] {
  const m = /[?&]ids=([^&]*)/.exec(location.hash);
  return m ? decodeURIComponent(m[1]!).split(',').filter(Boolean) : [];
}

let panel: FirmwarePanel | null = null;

export const firmwarePage: Page = {
  id: 'firmware',
  title: 'nav.firmware',
  icon: ICONS.firmware,
  render(main) {
    const ids = wantedIds();
    panel = firmwarePanel(ids, true);
    const p = panel;
    const update = h('button', { class: 'btn primary', onclick: async () => {
      update.setAttribute('disabled', '');
      try { await p.apply(); } catch (e) { toast(e instanceof Error ? e.message : String(e)); } finally { update.removeAttribute('disabled'); }
    } }, t('fw.update'));
    main.append(card(t('fw.title'), ids.length ? String(ids.length) : t('fw.all'),
      h('p', { class: 'muted' }, t('fw.intro')), p.el, h('div', { class: 'row' }, update)));
  },
  dispose() { panel?.dispose(); panel = null; },
};
