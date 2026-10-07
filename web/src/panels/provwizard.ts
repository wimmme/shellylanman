// The wizard "Set up a new Shelly" (DECISIONS §29, docs/phase-20-ap-wizards.md): the phone joins the new
// device's own access point and opens its page, where the home network is entered; once the device is on
// the network, ShellyLanMan shows what the chosen profile would do to it and, after the user's go, does it.
import { apApi, profilesApi, type APGuide, type Device, type PlanStep, type ProfileStep, type ProfileView } from '../api';
import { allDevices, loadDevices, onDevicesChanged } from '../devices';
import { h } from '../dom';
import { t, type Key } from '../i18n';
import { confirmDialog, openFrame } from '../modal';
import { neighbour, type WizardStep } from '../aplogic';
import { describe, newlyOnline, snapshot, type Seen } from '../profilelogic';
import { stepText } from '../pages/settings-profiles';
import { badge, copyBtn, fail, loading, msg, qr } from './apwizard';

type Step = Extract<WizardStep, 'join' | 'page'> | 'profile' | 'apply';
const STEPS: readonly Step[] = ['profile', 'join', 'page', 'apply'];
const k = (key: string): Key => key as Key;

const LAST = 'sl_profile_last';
const lastProfile = (): string => { try { return localStorage.getItem(LAST) ?? ''; } catch { return ''; } };
const rememberProfile = (id: string): void => { try { localStorage.setItem(LAST, id); } catch { /* private mode */ } };

export function openProvisionWizard(): void {
  let step: Step = 'profile';
  let profiles: ProfileView[] = [];
  let profile: ProfileView | undefined;
  let guide: APGuide | undefined;
  let before: Map<string, Seen> = new Map();
  let unsub: (() => void) | null = null;
  let gone = false;

  const frame = openFrame(t('prov.title'), () => { gone = true; unsub?.(); });
  const stepper = h('ol', { class: 'apw-steps' });
  const content = h('div', { class: 'apw-content' });
  frame.body.append(stepper, content);
  const back = h('button', { class: 'btn', onclick: () => go(-1) }, t('apw.back'));
  const next = h('button', { class: 'btn primary', onclick: () => go(1) }, t('apw.next'));
  frame.footer.append(h('button', { class: 'btn', onclick: () => frame.close() }, t('apw.close')), back, next);

  const canGo = (): boolean => step !== 'profile' || !!profile;
  const drawSteps = (): void => {
    stepper.replaceChildren(...STEPS.map((s, i) => h('li', {
      class: s === step ? 'current' : STEPS.indexOf(step) > i ? 'done' : undefined,
      'aria-current': s === step ? 'step' : undefined,
    }, t(k('prov.step.' + s)))));
    back.classList.toggle('hidden', step === 'profile');
    next.classList.toggle('hidden', step === 'apply');
    next.toggleAttribute('disabled', !canGo());
  };
  const go = (dir: 1 | -1): void => {
    const to = neighbour(STEPS as readonly WizardStep[], step as WizardStep, dir) as Step | null;
    if (!to || (dir === 1 && !canGo())) return;
    unsub?.(); unsub = null;
    step = to;
    void show();
  };

  async function show(): Promise<void> {
    drawSteps();
    const mine = step;
    content.replaceChildren(loading());
    let node: Node;
    try {
      node = mine === 'profile' ? await chooseProfile() : mine === 'join' ? join() : mine === 'page' ? page() : await apply();
    } catch (e) {
      node = fail(e);
    }
    if (gone || step !== mine) return;
    content.replaceChildren(node);
    drawSteps();
  }

  // ---- 1: the profile, and optionally the access point's name ----
  async function chooseProfile(): Promise<Node> {
    profiles = await profilesApi.list();
    if (!profiles.length) return h('div', {}, h('p', {}, t('prov.noProfiles')), h('a', { href: '#/settings', onclick: () => frame.close() }, t('prov.makeOne')));
    profile ??= profiles.find((p) => p.id === lastProfile()) ?? profiles[0];
    const sel = h('select', { 'aria-label': t('prov.profile') }, ...profiles.map((p) => h('option', { value: p.id }, p.name)));
    sel.value = profile!.id;
    const summary = h('ul', { class: 'prof-steps' });
    const drawSummary = (): void => {
      profile = profiles.find((p) => p.id === sel.value);
      if (profile) rememberProfile(profile.id);
      summary.replaceChildren(...(profile ? stepsOf(profile) : []));
      drawSteps();
    };
    sel.addEventListener('change', drawSummary);
    const name = h('input', { type: 'text', value: guide?.ssid ?? '', placeholder: 'ShellyPlus2PM-A8032AB636EC', size: 34, spellcheck: false, 'aria-label': t('prov.apName') });
    const known = h('p', { class: 'muted' });
    const find = async (): Promise<void> => {
      const v = name.value.trim();
      known.textContent = '';
      if (!v) { guide = undefined; return; }
      try {
        guide = await apApi.guide({ name: v });
        known.textContent = guide.recognised ? t('apw.which.recognised', { model: guide.model || guide.key || '' }) : '';
      } catch (e) { guide = undefined; known.textContent = msg(e); }
    };
    name.addEventListener('change', () => void find());
    drawSummary();
    return h('div', {}, h('p', { class: 'muted' }, t('prov.intro')),
      h('div', { class: 'row' }, h('label', { class: 'row' }, t('prov.profile'), sel)), summary,
      h('p', { class: 'muted' }, t('prov.apName.intro')),
      h('div', { class: 'row' }, h('label', { class: 'row' }, t('prov.apName'), name)), known);
  }

  function stepsOf(p: ProfileView): HTMLElement[] {
    const out: HTMLElement[] = [];
    for (const [s, v] of describe(p)) out.push(h('li', {}, stepText(s, v)));
    return out;
  }

  // ---- 2: join the access point ----
  function join(): Node {
    if (!guide || !guide.ssid) {
      return h('div', {}, h('p', { class: 'muted' }, t('apw.join.intro')), h('p', {}, t('prov.join.generic')));
    }
    const g = guide;
    return h('div', {}, h('p', { class: 'muted' }, t('apw.join.intro')),
      h('div', { class: 'lfw-grid' }, g.joinQR ? qr(g.joinQR, 'apw.join.qrAlt') : null,
        h('div', { class: 'lfw-info' }, badge('apw.join.ssid', g.ssid.trim()), copyBtn(g.ssid.trim()), h('p', { class: 'muted' }, t('apw.join.open')))));
  }

  // ---- 3: open the page and enter the home network ----
  function page(): Node {
    const pageURL = guide?.pageURL || 'http://192.168.33.1';
    const pageQR = guide?.pageQR;
    const ssid = profile?.wifiSSID;
    return h('div', {}, h('p', { class: 'muted' }, t('apw.page.intro')),
      h('div', { class: 'lfw-grid' }, pageQR ? qr(pageQR, 'apw.page.qrAlt') : null,
        h('div', { class: 'lfw-info' }, badge('apw.page.url', pageURL), h('p', {}, t('prov.page.steps')),
          ssid ? h('div', { class: 'row' }, badge('prov.page.network', ssid), copyBtn(ssid)) : null,
          h('p', { class: 'muted' }, t('prov.page.password')))));
  }

  // ---- 4: the device is on the network: show the plan, ask, apply ----
  async function apply(): Promise<Node> {
    await loadDevices().catch(() => {});
    before = snapshot(allDevices());
    const box = h('div', {});
    const status = h('p', { role: 'status' }, t('prov.wait.waiting'));
    const spinner = h('div', { class: 'spinner', 'aria-hidden': 'true' });
    const found = h('div', {});
    box.append(h('p', { class: 'muted' }, t('prov.wait.intro')), h('div', { class: 'row' }, spinner, status), found);
    let chosen: string | null = null;
    const draw = (): void => {
      if (chosen) return;
      const list = newlyOnline(allDevices(), before, guide?.mac ?? '');
      if (!list.length) return;
      spinner.remove();
      status.textContent = t('prov.wait.found', { n: list.length });
      found.replaceChildren(...list.map((d) => h('div', { class: 'card' }, h('div', { class: 'row' },
        h('strong', {}, label(d)), h('span', { class: 'muted' }, `${d.typeName} · ${d.ip}`), h('div', { class: 'spacer' }),
        h('button', { class: 'btn primary', onclick: () => void choose(d) }, t('prov.use'))))));
    };
    const choose = async (d: Device): Promise<void> => {
      chosen = d.id;
      unsub?.(); unsub = null;
      status.textContent = '';
      found.replaceChildren(loading());
      try {
        const plan = await profilesApi.plan(profile!.id, d.id);
        found.replaceChildren(planView(d, plan));
      } catch (e) { found.replaceChildren(fail(e)); }
    };
    const planView = (d: Device, plan: PlanStep[]): HTMLElement => {
      const go = h('button', { class: 'btn primary' }, t('prov.apply'));
      const out = h('div', {});
      go.addEventListener('click', async () => {
        if (!(await confirmDialog(t('prov.confirmTitle'), t('prov.confirm', { profile: profile!.name, device: label(d) }), t('prov.apply'), false))) return;
        go.setAttribute('disabled', '');
        try {
          out.replaceChildren(results(await profilesApi.apply(profile!.id, d.id)));
        } catch (e) { out.replaceChildren(fail(e)); go.removeAttribute('disabled'); }
      });
      return h('div', { class: 'card' }, h('strong', {}, label(d)), h('p', { class: 'muted' }, t('prov.plan')),
        plan.length ? h('ul', { class: 'prof-steps' }, ...plan.map((s) => h('li', {}, stepText(s.step, s.value)))) : h('p', { class: 'muted' }, t('prov.plan.empty')),
        h('div', { class: 'row' }, go), out);
    };
    unsub = onDevicesChanged(draw);
    draw();
    return box;
  }

  void show();
}

const label = (d: Device): string => d.name || d.hostname || d.id;

function results(steps: ProfileStep[]): HTMLElement {
  const bad = steps.filter((s) => s.result === 'fail').length;
  return h('div', { class: 'prov-results', role: 'status' },
    h('p', { class: bad ? 'sec-error' : '' }, t(bad ? 'prov.result.problems' : 'prov.result.ok', { n: bad })),
    h('ul', { class: 'prof-steps' }, ...steps.map((s) => h('li', { class: 'res-' + s.result },
      `${t(k('prof.step.' + s.step))}: ${t(k('prof.res.' + s.result))}${s.message ? ' — ' + s.message : ''}`))));
}

