// "Create profile" from a device (DECISIONS §30): read the device, list what a profile could take over with a
// tick (the settings that differ from the factory's are ticked at first), then the profile editor opens with
// the ticked ones; there the user gives the name (required) and the passwords, which a device does not tell.
// The dialog opens at once and shows that the device is being read; a failure is shown in it.
import { profilesApi, type Device, type ProfileDraft } from '../api';
import { h } from '../dom';
import { t } from '../i18n';
import { openFrame } from '../modal';
import { toast } from '../toast';
import { errorState, loadingState } from '../pages/common';
import { openProfileEditor, stepText } from '../pages/settings-profiles';
import { draftForm, draftLines } from '../profilelogic';

export async function openProfileFromDevice(device: Device): Promise<void> {
  const name = device.name || device.hostname || device.id;
  let gone = false;
  const frame = openFrame(t('pfd.title', { name }), () => { gone = true; }, false);
  const next = h('button', { class: 'btn primary', disabled: true }, t('pfd.next'));
  frame.footer.append(h('button', { class: 'btn', onclick: () => frame.close() }, t('common.cancel')), next);
  frame.body.append(loadingState());

  let draft: ProfileDraft;
  try {
    draft = await profilesApi.fromDevice(device.id);
  } catch (e) {
    if (!gone) frame.body.replaceChildren(errorState(e));
    return;
  }
  if (gone) return;
  const lines = draftLines(draft);
  const ticked = new Set(lines.filter((l) => l.deviates).map((l) => l.step));

  const list = h('ul', { class: 'prof-pick' }, ...lines.map((l) => {
    const box = h('input', { type: 'checkbox', checked: ticked.has(l.step), id: 'pfd-' + l.step });
    box.addEventListener('change', () => { if (box.checked) ticked.add(l.step); else ticked.delete(l.step); });
    return h('li', {}, h('label', { for: box.id }, box, ' ', stepText(l.step, l.value)),
      h('span', { class: l.deviates ? 'prof-differs' : 'muted' }, ' · ', t(l.deviates ? 'pfd.differs' : 'pfd.same')));
  }));
  frame.body.replaceChildren(h('p', {}, t('pfd.intro')), lines.length ? list : h('p', { class: 'muted' }, t('pfd.none')));
  next.addEventListener('click', () => {
    frame.close();
    openProfileEditor(null, (n) => toast(t('pfd.saved', { name: n }), 'info'), draftForm(draft, ticked));
  });
  next.removeAttribute('disabled');
  next.focus();
}
