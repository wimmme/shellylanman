// The wizard that updates a device's firmware through the device's own access point
// (DECISIONS §29, docs/phase-20-ap-wizards.md): the phone does the work at the device, the
// wizard on this screen shows what to scan. It starts from a device in the list or from the name
// of an access point, so it also serves a Shelly ShellyLanMan does not know.
import { ApiError, apApi, configApi, firmwareApi, localFwApi, type APGuide, type LocalLink, type ModelChoice, type PhoneLink } from '../api';
import {
  apIsOff, apLabel, canGoOn, canSwitchAPOn, cameBack, FIRMWARE_STEPS, neighbour, startWaiting, targetModel, watchedId,
  type WizardState, type WizardStep, type Waiting,
} from '../aplogic';
import { copyText } from '../clipboard';
import { allDevices, loadDevices, onDevicesChanged } from '../devices';
import { reconnectEvents } from '../socket';
import { h } from '../dom';
import { fetchWithProgress, localPath, megabytes, progressText, saveBlob } from '../download';
import { dateTime } from '../format';
import { t, type Key } from '../i18n';
import { openFrame } from '../modal';
import { toast } from '../toast';

export const msg = (e: unknown): string => (e instanceof ApiError || e instanceof Error ? e.message : String(e));
const k = (key: string): Key => key as Key;

export const loading = (): HTMLElement => h('div', { class: 'state' }, h('div', { class: 'spinner', role: 'status', 'aria-label': t('state.loading') }), t('state.loading'));
export const fail = (e: unknown): HTMLElement => h('div', { class: 'banner warn', role: 'alert' }, msg(e));
export const qr = (src: string, alt: Key, small = false): HTMLElement => h('img', { class: 'lfw-qr' + (small ? ' small' : ''), src, alt: t(alt), width: 320, height: 320 });
/** Text in paragraphs, one per line of the catalogue entry. */
const paras = (text: string, cls = 'muted'): HTMLElement[] => text.split('\n').map((l) => h('p', { class: cls }, l));
/** A link warning (lfw.warn.*): ingress is a tip (the address is a guess), the others are errors. */
const linkWarning = (w: string | undefined): HTMLElement | null => !w ? null : w === 'ingress'
  ? h('div', { class: 'banner', role: 'note' }, t('lfw.warn.ingress'))
  : h('div', { class: 'banner warn', role: 'alert' }, t(k('lfw.warn.' + w)));
export const copyBtn = (text: string): HTMLElement => h('button', { class: 'btn small', onclick: async () => {
  if (await copyText(text)) toast(t('lfw.copied'), 'info'); // else: the text is shown, select it
} }, t('common.copy'));
export const badge = (label: Key, v: string): HTMLElement => h('div', { class: 'lfw-badge' }, h('span', { class: 'muted' }, t(label)), h('strong', {}, v));
// The device's page as a link: on the phone itself it opens in a tab next to the wizard.
export const pageBadge = (url: string): HTMLElement => h('div', { class: 'lfw-badge' }, h('span', { class: 'muted' }, t('apw.page.url')),
  h('strong', {}, h('a', { href: url, target: '_blank', rel: 'noopener noreferrer' }, url)));

/** Open the firmware wizard, for a device in the list (id) or from the name of an access point. */
export function openFirmwareWizard(opts: { id?: string } = {}): void {
  const st: WizardState = {};
  let step: WizardStep = 'which';
  let link: LocalLink | null = null;
  let waiting: Waiting | null = null;
  let unsub: (() => void) | null = null;
  let models: ModelChoice[] | null = null;
  let phoneLink: PhoneLink | null | undefined;
  let gone = false;
  const opened = Date.now();

  const frame = openFrame(t('apw.title'), { onClose: () => { gone = true; unsub?.(); } });
  const stepper = h('ol', { class: 'apw-steps' });
  const content = h('div', { class: 'apw-content' });
  frame.body.append(stepper, content);

  const back = h('button', { class: 'btn', onclick: () => go(-1) }, t('apw.back'));
  const next = h('button', { class: 'btn primary', onclick: () => go(1) }, t('apw.next'));
  const close = h('button', { class: 'btn', onclick: () => frame.close() }, t('apw.close'));
  frame.footer.append(close, back, next);

  const go = (dir: 1 | -1): void => {
    const to = neighbour(FIRMWARE_STEPS, step, dir);
    if (!to || (dir === 1 && !canGoOn(step, st))) return;
    unsub?.(); unsub = null;
    step = to;
    void show();
  };

  const drawSteps = (): void => {
    stepper.replaceChildren(...FIRMWARE_STEPS.map((s, i) => h('li', {
      class: s === step ? 'current' : FIRMWARE_STEPS.indexOf(step) > i ? 'done' : undefined,
      'aria-current': s === step ? 'step' : undefined,
    }, t(k('apw.step.' + s)))));
    back.classList.toggle('hidden', step === 'which'); // the hidden attribute loses against .btn's display
    next.classList.toggle('hidden', step === 'wait');
    next.toggleAttribute('disabled', !canGoOn(step, st));
  };

  async function show(): Promise<void> {
    drawSteps();
    const mine = step;
    content.replaceChildren(loading());
    let node: Node;
    try {
      node = await render(mine);
    } catch (e) {
      node = fail(e);
    }
    if (gone || step !== mine) return;
    content.replaceChildren(node);
    drawSteps();
  }

  async function render(s: WizardStep): Promise<Node> {
    switch (s) {
      case 'which': return which();
      case 'file': return file();
      case 'join': return join();
      case 'page': return page();
      case 'wait': return wait();
    }
  }

  // ---- 1: which device ----
  async function which(): Promise<Node> {
    if (opts.id && !st.guide) { // from a device in the list: nothing to ask
      st.guide = await apApi.guide({ id: opts.id });
      link = null;
    }
    const result = h('div', { class: 'apw-result' });
    const input = h('input', { type: 'text', value: st.guide && !opts.id ? st.guide.ssid : '', placeholder: 'ShellyPlus2PM-A8032AB636EC', size: 34, spellcheck: false, 'aria-label': t('apw.which.name') });
    const showResult = (): void => {
      const g = st.guide;
      if (!g) { result.replaceChildren(); return; }
      const rows: Node[] = [];
      if (g.id) rows.push(h('p', { class: 'muted' }, t('apw.which.inList', { name: allDevices().find((d) => d.id === g.id)?.name || g.id })));
      if (g.recognised) rows.push(h('div', { class: 'row' }, badge('lfw.model', g.model || g.key || ''), g.mac ? badge('apw.mac', g.mac) : null));
      else {
        rows.push(h('p', {}, t('apw.which.unknown')));
        const sel = h('select', { 'aria-label': t('apw.which.pick') }, h('option', { value: '' }, t('apw.which.pick')),
          ...(models ?? []).map((m) => h('option', { value: `${m.gen}/${m.key}` }, m.name)));
        sel.addEventListener('change', () => {
          st.picked = (models ?? []).find((m) => `${m.gen}/${m.key}` === sel.value);
          link = null;
          drawSteps();
        });
        if (st.picked) sel.value = `${st.picked.gen}/${st.picked.key}`;
        rows.push(sel);
      }
      result.replaceChildren(...rows);
      drawSteps();
    };
    const find = async (): Promise<void> => {
      const name = input.value.trim();
      if (!name) return;
      result.replaceChildren(loading());
      try {
        models ??= await apApi.models();
        st.guide = await apApi.guide({ name });
        st.picked = undefined;
        link = null;
        showResult();
      } catch (e) {
        st.guide = undefined;
        result.replaceChildren(fail(e));
        drawSteps();
      }
    };
    input.addEventListener('keydown', (e) => { if (e.key === 'Enter') void find(); });
    // The wizard is easiest on the phone itself: a code that opens it there (hidden on a phone).
    phoneLink ??= await apApi.phone(opts.id).catch(() => null);
    const phone = h('div', { class: 'apw-phone' }, h('p', { class: 'muted' }, t('apw.which.phone')),
      phoneLink ? h('div', { class: 'apw-phone-qr' }, linkWarning(phoneLink.warning), qr(phoneLink.qr, 'apw.which.qrAlt', true), h('p', { class: 'muted' }, t('apw.which.pc'))) : null);
    if (opts.id) {
      showResult();
      return h('div', {}, h('p', {}, t('apw.which.fromList')), phone, result);
    }
    if (st.guide) { models ??= await apApi.models(); showResult(); }
    return h('div', {}, h('p', { class: 'muted' }, t('apw.which.intro')), phone,
      h('div', { class: 'row' }, h('label', { class: 'row' }, t('apw.which.name'), input), h('button', { class: 'btn', onclick: () => void find() }, t('apw.which.find'))),
      result);
  }

  // ---- 2: the firmware file onto the phone ----
  async function file(): Promise<Node> {
    const intro = h('div', {}, ...paras(t('apw.file.intro')));
    if (!link) {
      const m = targetModel(st);
      if (!m) return fail(t('apw.which.unknown'));
      // A device in the list is compared with its own version ("already up to date"); a model is not.
      link = st.guide?.id ? await localFwApi.link(st.guide.id) : await localFwApi.modelLink(m.gen, m.key);
    }
    const l = link;
    const url = h('input', { readonly: true, value: l.url, 'aria-label': t('lfw.link') });
    // On the phone itself the QR code cannot be scanned: the same file as a download. It is fetched
    // in the page, with progress, because a plain link shows nothing in an installed app (PWA).
    const dlStatus = h('div', { class: 'apw-dl', role: 'status' });
    const download: HTMLButtonElement = h('button', { class: 'btn primary', onclick: () => void fetchFile() }, t('apw.file.download'));
    const fetchFile = async (): Promise<void> => {
      const path = localPath(l.url);
      if (!path) { window.open(l.url, '_blank', 'noopener'); return; }
      download.disabled = true;
      const bar = h('progress', { max: 1 });
      const text = h('span', {}, t('apw.file.downloading', { progress: progressText(0, 0) }));
      dlStatus.replaceChildren(bar, text);
      try {
        const blob = await fetchWithProgress(path, (done, total) => {
          if (total > 0) { bar.max = total; bar.value = done; }
          text.textContent = t('apw.file.downloading', { progress: progressText(done, total) });
        });
        saveBlob(blob, l.fileName);
        dlStatus.replaceChildren(h('span', {}, t('apw.file.done', { file: l.fileName, size: megabytes(blob.size) })));
      } catch (e) {
        dlStatus.replaceChildren(h('div', { class: 'banner warn', role: 'alert' }, t('apw.file.failed', { error: msg(e) }), ' ',
          h('a', { href: l.url, target: '_blank', rel: 'noopener noreferrer' }, t('apw.file.openLink'))));
      } finally {
        download.disabled = false;
      }
    };
    return h('div', { class: 'lfw' }, intro, linkWarning(l.warning),
      h('div', { class: 'lfw-grid' }, qr(l.qr, 'lfw.qrAlt', true),
        h('div', { class: 'lfw-info' },
          badge('lfw.model', l.model), badge('lfw.version', l.current ? `${l.current} → ${l.version}` : l.version),
          badge('lfw.source', t(l.source === 'shelly-tools' ? 'lfw.srcArchive' : 'lfw.srcShelly')),
          badge('lfw.file', l.fileName), badge('lfw.expires', dateTime(new Date(l.expires), true)))),
      h('div', { class: 'row' }, url, copyBtn(l.url), download), dlStatus);
  }

  // ---- 3: join the access point ----
  async function join(): Promise<Node> {
    let g = st.guide!;
    const box = h('div', {});
    const draw = (): void => {
      const parts: Node[] = [h('p', {}, t('apw.join.intro'))];
      if (apIsOff(g) && !canSwitchAPOn(g)) { // Gen1: only the way (DECISIONS P20-10)
        parts.push(h('div', { class: 'banner warn', role: 'status' }, t('apw.join.offGen1')));
      } else if (apIsOff(g)) {
        const on = h('button', { class: 'btn primary', onclick: async () => {
          on.setAttribute('disabled', '');
          try {
            const res = await configApi.checklistAction([g.id!], 'ap', true);
            if (res.errors.length) throw new Error(res.errors[0]!.message || res.errors[0]!.result);
            toast(t('apw.join.switchedOn'), 'info');
            g = st.guide = await apApi.guide({ id: g.id });
            draw();
          } catch (e) { // the device's own words are English: its refusal of an AP without a password is explained
            toast(msg(e).includes("'pass'") ? t('apw.join.needsPass') : msg(e));
            on.removeAttribute('disabled');
          }
        } }, t('apw.join.switchOn'));
        parts.push(h('div', { class: 'banner warn', role: 'status' }, t('apw.join.off')), h('div', { class: 'row' }, on));
      }
      if (g.assumed) parts.push(h('p', { class: 'muted' }, t('apw.join.assumed')));
      parts.push(h('div', { class: 'lfw-grid' },
        g.joinQR ? qr(g.joinQR, 'apw.join.qrAlt') : null,
        h('div', { class: 'lfw-info' }, badge('apw.join.ssid', apLabel(g)), copyBtn(apLabel(g)),
          h('p', { class: 'muted' }, g.joinQR ? t('apw.join.open') : t('apw.join.protected')),
          g.joinQR ? h('p', { class: 'muted' }, t('apw.join.onPhone')) : null)));
      box.replaceChildren(...parts);
    };
    draw();
    return box;
  }

  // ---- 4: open the device's page ----
  async function page(): Promise<Node> {
    const g = st.guide!;
    // The address in the sentence, as a link: on the phone itself it opens in a tab next to the wizard.
    const [before, after] = t('apw.page.intro', { url: '\u0000' }).split('\u0000');
    return h('div', { class: 'lfw' },
      h('p', {}, before ?? '', h('a', { href: g.pageURL, target: '_blank', rel: 'noopener noreferrer' }, g.pageURL), after ?? ''),
      h('p', { class: 'muted' }, t('apw.page.scan')), qr(g.pageQR, 'apw.page.qrAlt', true),
      ...paras(t('apw.page.steps'), ''));
  }

  // ---- 5: wait until the device is back ----
  async function wait(): Promise<Node> {
    const g = st.guide!;
    const id = watchedId(g);
    const name = allDevices().find((d) => d.id === id)?.name || g.model || id || apLabel(g);
    const status = h('p', { role: 'status' });
    const detail = h('div', {});
    if (!id) { // a short MAC (older Gen1): the device cannot be told from the list by the name
      return h('div', {}, h('p', {}, t('apw.wait.noId')));
    }
    // The phone may still be on the device's access point: the list it has is the starting point, and
    // nothing here waits for ShellyLanMan. The list is read again every few seconds: a phone that
    // changes network often loses the event connection without noticing.
    waiting = startWaiting(allDevices().find((d) => d.id === id), Date.now(), opened);
    const w = waiting;
    status.textContent = t('apw.wait.waiting', { name });
    const spinner = h('div', { class: 'spinner', 'aria-hidden': 'true' });
    // Still waiting while the phone is back home: read again now (not pull to refresh, it closes the wizard).
    const again = h('button', { class: 'btn small' }, t('apw.wait.again'));
    const stuck = h('p', { class: 'muted' }, t('apw.wait.stuck'), ' ', again);
    let reported = false;
    const check = async (): Promise<void> => {
      if (reported || gone || step !== 'wait' || !cameBack(allDevices().find((d) => d.id === id), w)) return;
      reported = true;
      spinner.remove();
      stuck.remove();
      status.textContent = t('apw.wait.back', { name });
      try {
        const row = (await firmwareApi.rows([id]))[0];
        if (row?.valid && row.current) detail.replaceChildren(h('p', {}, t('apw.wait.version', { current: row.current, target: link?.version ?? '' })));
      } catch { /* the version is a bonus */ }
    };
    const off = onDevicesChanged(() => void check());
    // A read that hangs (the phone still on the access point, or just off it) is ended after 8 s: hanging
    // reads hold the browser's few connections to ShellyLanMan, and new ones would wait behind them.
    let reading = false;
    const read = async (): Promise<void> => {
      if (reported || reading) return;
      reading = true;
      try { await loadDevices(AbortSignal.timeout(8000)); } catch { /* next time */ }
      reading = false;
      void check();
    };
    const poll = setInterval(() => void read(), 4000);
    unsub = () => { off(); clearInterval(poll); };
    again.addEventListener('click', () => { reconnectEvents(); reading = false; void read(); });
    const box = h('div', {}, h('p', { class: 'muted' }, t('apw.wait.intro')), h('div', { class: 'row' }, spinner, status), detail, stuck,
      g.id ? null : h('p', { class: 'muted' }, t('apw.wait.notListed')));
    void check(); // after the step is built: a device back already takes the spinner and the button out of it
    return box;
  }

  void show();
}
