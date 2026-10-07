// Settings → Profiles: what a new Shelly gets once it has joined the network (DECISIONS P20-11).
// Used by the wizard "Set up a new Shelly". Anything left on "Leave as is" is not touched on the device.
import { ApiError, profilesApi, type ProfileView } from '../api';
import { h } from '../dom';
import { t, type Key } from '../i18n';
import { confirmDialog, openModal } from '../modal';
import { toast } from '../toast';
import { describe, emptyForm, formOf, inputOf, type ProfileForm, type Tri } from '../profilelogic';

const k = (key: string): Key => key as Key;

/** One step of a profile, as a line: label and value. */
export function stepText(step: string, value: string): string {
  const v = value === 'on' || value === 'off' ? t(k('prof.' + value)) : value === 'stable' || value === 'beta' || value === 'none' ? t(k('prof.fw.' + value)) : value;
  return `${t(k('prof.step.' + step))}: ${v}`;
}

function message(err: unknown): string {
  if (err instanceof ApiError && err.message.startsWith('invalid: ')) {
    const code = err.message.slice('invalid: '.length);
    const key = 'prof.err.' + code;
    return t(k(key)) === key ? err.message : t(k(key));
  }
  return err instanceof Error ? err.message : String(err);
}

function triSelect(value: Tri, on: (v: Tri) => void): HTMLSelectElement {
  const s = h('select', {}, h('option', { value: '' }, t('prof.leave')), h('option', { value: 'on' }, t('prof.on')), h('option', { value: 'off' }, t('prof.off')));
  s.value = value;
  s.addEventListener('change', () => on(s.value as Tri));
  return s;
}

function row(label: string, control: HTMLElement, hint?: string): HTMLElement {
  const id = 'pf' + Math.random().toString(36).slice(2, 8);
  control.id = id;
  return h('div', { class: 'field' }, h('label', { for: id }, label), control, hint ? h('p', { class: 'muted' }, hint) : null);
}

function editor(existing: ProfileView | null, saved: () => void): void {
  const f: ProfileForm = existing ? formOf(existing) : emptyForm();
  const text = (key: keyof ProfileForm, opts: Record<string, string | number> = {}): HTMLInputElement => {
    const i = h('input', { type: 'text', value: String(f[key]), autocomplete: 'off', spellcheck: false, ...opts });
    i.addEventListener('input', () => { (f as unknown as Record<string, string>)[key] = i.value; });
    return i;
  };
  const secret = (key: 'loginPassword' | 'mqttPassword', isSet: boolean): HTMLInputElement => {
    const i = h('input', { type: 'password', autocomplete: 'new-password', placeholder: isSet ? '••••••••' : '' });
    i.addEventListener('input', () => { f[key] = i.value; });
    return i;
  };
  const tri = (key: 'login' | 'mqtt' | 'cloud' | 'eco' | 'ledOff' | 'ap' | 'roaming', onChange?: () => void): HTMLSelectElement =>
    triSelect(f[key], (v) => { f[key] = v; onChange?.(); });
  const fw = h('select', {}, h('option', { value: '' }, t('prof.leave')), ...['stable', 'beta', 'none'].map((v) => h('option', { value: v }, t(k('prof.fw.' + v)))));
  fw.value = f.autoFW;
  fw.addEventListener('change', () => { f.autoFW = fw.value as ProfileForm['autoFW']; });

  const loginBox = h('div', { class: 'prof-sub' });
  const mqttBox = h('div', { class: 'prof-sub' });
  const drawSub = (): void => {
    loginBox.replaceChildren(...(f.login === 'on' ? [
      row(t('prof.loginUser'), text('user', { size: 14 })),
      row(t('prof.password'), secret('loginPassword', !!existing?.loginPasswordSet), existing?.loginPasswordSet ? t('prof.passwordSet') : undefined)] : []));
    mqttBox.replaceChildren(...(f.mqtt === 'on' ? [
      row(t('prof.mqttServer'), text('mqttServer', { size: 30, placeholder: 'broker:1883' })),
      row(t('prof.mqttUser'), text('mqttUser', { size: 20 })),
      row(t('prof.password'), secret('mqttPassword', !!existing?.mqttPasswordSet), existing?.mqttPasswordSet ? t('prof.passwordSet') : undefined)] : []));
  };
  drawSub();
  const err = h('p', { class: 'sec-error', role: 'alert' });

  const body = h('div', { class: 'prof-form' },
    row(t('prof.name'), text('name', { size: 30 })),
    row(t('prof.namePattern'), text('namePattern', { size: 30, placeholder: '{model} {mac4}' }), t('prof.namePatternHint')),
    row(t('prof.wifi'), text('wifiSSID', { size: 30 }), t('prof.wifiHint')),
    row(t('prof.login'), tri('login', drawSub)), loginBox,
    row(t('prof.mqtt'), tri('mqtt', drawSub)), mqttBox,
    row(t('prof.ntp'), text('ntp', { size: 24, placeholder: 'pool.ntp.org' })),
    row(t('prof.cloud'), tri('cloud')),
    row(t('prof.eco'), tri('eco')),
    row(t('prof.ledOff'), tri('ledOff')),
    row(t('prof.ap'), tri('ap')),
    row(t('prof.roaming'), tri('roaming')),
    row(t('prof.autoFW'), fw),
    err);
  openModal(t(existing ? 'prof.editTitle' : 'prof.new'), body, [
    { label: t('common.cancel') },
    { label: t('prof.save'), kind: 'primary', onClick: async () => {
      err.textContent = '';
      try {
        const input = inputOf(f, existing?.id);
        if (existing) await profilesApi.update(existing.id, input); else await profilesApi.create(input);
      } catch (e) { err.textContent = message(e); return false; }
      saved();
    } },
  ]);
}

export async function profilesSettings(body: HTMLElement): Promise<void> {
  const box = h('div', { class: 'profiles' });
  body.append(h('p', {}, t('prof.intro')), box);
  const draw = async (): Promise<void> => {
    let list: ProfileView[];
    try { list = await profilesApi.list(); } catch (e) { box.replaceChildren(h('p', { class: 'sec-error' }, message(e))); return; }
    const items = list.map((p) => h('div', { class: 'card prof-item' },
      h('div', { class: 'row' }, h('strong', {}, p.name), h('div', { class: 'spacer' }),
        h('button', { class: 'btn', onclick: () => editor(p, () => void draw()) }, t('prof.edit')),
        h('button', { class: 'btn', onclick: async () => {
          if (!(await confirmDialog(t('prof.delete'), t('prof.deleteConfirm', { name: p.name }), t('prof.delete')))) return;
          try { await profilesApi.remove(p.id); void draw(); } catch (e) { toast(message(e)); }
        } }, t('prof.delete'))),
      h('ul', { class: 'prof-steps' },
        ...(describe(p).map(([s, v]) => h('li', {}, stepText(s, v)))),
        p.wifiSSID ? h('li', { class: 'muted' }, `${t('prof.wifi')}: ${p.wifiSSID}`) : null)));
    box.replaceChildren(...(items.length ? items : [h('p', { class: 'muted' }, t('prof.none'))]),
      h('div', { class: 'row' }, h('button', { class: 'btn primary', onclick: () => editor(null, () => void draw()) }, t('prof.new'))));
  };
  await draw();
}
