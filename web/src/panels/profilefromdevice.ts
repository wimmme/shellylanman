// "Create profile" from a device (DECISIONS §30): read the device, list what a profile could take over with a
// tick (the settings that differ from the factory's are ticked at first), then the profile editor opens with
// the ticked ones; there the user gives the name (required) and the passwords, which a device does not tell.
import { profilesApi, type Device, type ProfileDraft } from '../api';
import { h } from '../dom';
import { t } from '../i18n';
import { openModal } from '../modal';
import { toast } from '../toast';
import { openProfileEditor, stepText } from '../pages/settings-profiles';
import { draftForm, draftLines } from '../profilelogic';

export async function openProfileFromDevice(device: Device): Promise<void> {
  let draft: ProfileDraft;
  try { draft = await profilesApi.fromDevice(device.id); } catch (e) { toast(e instanceof Error ? e.message : String(e)); return; }
  const lines = draftLines(draft);
  const ticked = new Set(lines.filter((l) => l.deviates).map((l) => l.step));
  const name = device.name || device.hostname || device.id;

  const list = h('ul', { class: 'prof-pick' }, ...lines.map((l) => {
    const box = h('input', { type: 'checkbox', checked: ticked.has(l.step), id: 'pfd-' + l.step });
    box.addEventListener('change', () => { if (box.checked) ticked.add(l.step); else ticked.delete(l.step); });
    return h('li', {}, h('label', { for: box.id }, box, ' ', stepText(l.step, l.value)),
      h('span', { class: l.deviates ? 'prof-differs' : 'muted' }, ' · ', t(l.deviates ? 'pfd.differs' : 'pfd.same')));
  }));
  const body = h('div', {}, h('p', {}, t('pfd.intro')), lines.length ? list : h('p', { class: 'muted' }, t('pfd.none')));
  openModal(t('pfd.title', { name }), body, [
    { label: t('common.cancel') },
    { label: t('pfd.next'), kind: 'primary', onClick: () => {
      const form = draftForm(draft, ticked);
      openProfileEditor(null, (n) => toast(t('pfd.saved', { name: n }), 'info'), form);
    } },
  ]);
}
