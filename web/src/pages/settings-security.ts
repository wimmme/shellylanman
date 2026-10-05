// Settings → Security: the optional UI password (DECISIONS §24). Set, change
// or switch off; the rules and a strength indicator while it is chosen (P15-7).
import { ApiError, api, authApi } from '../api';
import { h } from '../dom';
import { t, type Key } from '../i18n';
import { LEVELS, problems, strength } from '../passwordlogic';
import { toast } from '../toast';

function pwInput(id: string, autocomplete: string): HTMLInputElement {
  return h('input', { type: 'password', id, autocomplete });
}

function field(label: string, control: HTMLElement, ...extra: HTMLElement[]): HTMLElement {
  return h('div', { class: 'field' }, h('label', { for: control.id }, label), control, ...extra);
}

/** The new password twice, the rules ticked off and the strength meter; ok() tells whether it may be saved. */
function newPassword(onChange: () => void): { part: HTMLElement[]; value: () => string; ok: () => boolean; clear: () => void } {
  const pw = pwInput('secNew', 'new-password');
  const again = pwInput('secAgain', 'new-password');
  const meter = h('meter', { id: 'secMeter', min: 0, max: 4, low: 2, high: 3, optimum: 4, value: 0, 'aria-label': t('security.strength') });
  const word = h('span', { class: 'sec-strength', 'aria-live': 'polite' });
  const ruleLen = h('li', {}, t('security.ruleLength'));
  const ruleCap = h('li', {}, t('security.ruleCapital'));
  const ruleMore = h('li', { class: 'muted' }, t('security.ruleMore'));
  const match = h('p', { class: 'sec-match', 'aria-live': 'polite' });
  const update = (): void => {
    const p = problems(pw.value);
    ruleLen.classList.toggle('ok', !p.includes('tooShort'));
    ruleCap.classList.toggle('ok', !p.includes('noCapital'));
    const level = strength(pw.value);
    meter.value = level;
    word.textContent = pw.value ? t(('security.level.' + LEVELS[level]) as Key) : '';
    match.textContent = again.value && again.value !== pw.value ? t('security.mismatch') : '';
    onChange();
  };
  pw.addEventListener('input', update);
  again.addEventListener('input', update);
  return {
    part: [
      field(t('security.new'), pw, h('div', { class: 'sec-meter' }, meter, word)),
      h('ul', { class: 'sec-rules' }, ruleLen, ruleCap, ruleMore),
      field(t('security.again'), again, match),
    ],
    value: () => pw.value,
    ok: () => problems(pw.value).length === 0 && pw.value === again.value,
    clear: () => { pw.value = ''; again.value = ''; update(); },
  };
}

function message(err: unknown): string {
  if (err instanceof ApiError) {
    if (err.retryAfter) return t('login.wait', { s: err.retryAfter });
    if (err.code === 'wrong') return t('security.wrongCurrent');
    if (err.code === 'tooShort') return t('security.ruleLength');
    if (err.code === 'noCapital') return t('security.ruleCapital');
  }
  return err instanceof Error ? err.message : String(err);
}

export async function securitySettings(body: HTMLElement, onChanged: () => void): Promise<void> {
  const status = await api.status();
  const box = h('div', { class: 'security' });
  const intro: HTMLElement[] = [h('p', {}, t('security.intro'))];
  if (status.ingress) intro.push(h('p', { class: 'banner', role: 'note' }, t('security.ingress')));

  if (!status.authEnabled) {
    const save = h('button', { class: 'btn primary', disabled: true }, t('security.switchOn'));
    const np = newPassword(() => { save.disabled = !np.ok(); });
    const err = h('p', { class: 'sec-error', role: 'alert' });
    save.addEventListener('click', async () => {
      save.disabled = true;
      try {
        await authApi.setPassword('', np.value());
        toast(t('security.on'), 'info');
        onChanged();
      } catch (e) {
        err.textContent = message(e);
        save.disabled = !np.ok();
      }
    });
    box.append(...intro, h('h3', {}, t('security.setTitle')), ...np.part, h('div', { class: 'row' }, save), err);
  } else {
    // Under Home Assistant ingress the current password is not needed (P15-9):
    // the user is logged in to Home Assistant, and a forgotten one cannot be reset otherwise.
    const needCurrent = !status.ingress;
    const current = pwInput('secCurrent', 'current-password');
    const haveCurrent = (): boolean => !needCurrent || current.value !== '';
    const change = h('button', { class: 'btn primary', disabled: true }, t('security.change'));
    const np = newPassword(() => { change.disabled = !np.ok() || !haveCurrent(); });
    current.addEventListener('input', () => { change.disabled = !np.ok() || !haveCurrent(); off.disabled = !haveCurrent(); });
    const off = h('button', { class: 'btn danger', disabled: needCurrent }, t('security.switchOff'));
    const err = h('p', { class: 'sec-error', role: 'alert' });
    change.addEventListener('click', async () => {
      change.disabled = true;
      try {
        await authApi.setPassword(current.value, np.value());
        current.value = '';
        np.clear();
        err.textContent = '';
        toast(t('security.changed'), 'info');
      } catch (e) {
        err.textContent = message(e);
        change.disabled = !np.ok() || !haveCurrent();
      }
    });
    off.addEventListener('click', async () => {
      off.disabled = true;
      try {
        await authApi.setPassword(current.value, '');
        toast(t('security.off'), 'info');
        onChanged();
      } catch (e) {
        err.textContent = message(e);
        off.disabled = !haveCurrent();
      }
    });
    box.append(...intro,
      h('p', { class: 'sec-state' }, t('security.isOn')),
      needCurrent ? field(t('security.current'), current) : h('p', { class: 'muted' }, t('security.noCurrent')),
      h('h3', {}, t('security.changeTitle')), ...np.part, h('div', { class: 'row' }, change),
      h('h3', {}, t('security.offTitle')), h('p', { class: 'muted' }, t(needCurrent ? 'security.offText' : 'security.offTextHa')), h('div', { class: 'row' }, off),
      err,
      ...(needCurrent ? [h('p', { class: 'muted' }, t('security.reset'))] : []));
  }
  body.append(box);
}
