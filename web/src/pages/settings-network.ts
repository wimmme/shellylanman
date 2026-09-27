// Settings → Network and Archive (ShellyScanner: PanelNetwork, PanelStore).
import { devicesApi, type IPRange, type ScanSettings } from '../api';
import { h } from '../dom';
import { t, type Key } from '../i18n';
import { confirmDialog } from '../modal';

const MAX_RANGES = 10; // ShellyScanner: up to 10 IP ranges

function field(id: string, label: string, control: HTMLElement, help?: string): HTMLElement {
  control.id = id;
  return h('div', { class: 'field' }, h('label', { for: id }, label), control, help ? h('small', { class: 'muted' }, help) : null);
}

function result(): { el: HTMLElement; ok: () => void; fail: (e: unknown) => void } {
  const el = h('span', { class: 'muted', role: 'status' });
  return {
    el,
    ok: () => { el.textContent = t('settings.saved'); },
    fail: (e) => { el.textContent = e instanceof Error ? e.message : String(e); },
  };
}

export async function network(body: HTMLElement): Promise<void> {
  const [settings, ifaces, creds] = await Promise.all([devicesApi.settings(), devicesApi.interfaces(), devicesApi.credentials()]);
  const scan: ScanSettings = { ...settings.scan, ranges: [...(settings.scan.ranges ?? [])] };

  // Scan mode
  const modes: [string, Key][] = [['full', 'scan.mode.full'], ['local', 'scan.mode.local'], ['ip', 'scan.mode.ip'], ['offline', 'scan.mode.offline']];
  const modeBox = h('div', { class: 'field', role: 'radiogroup', 'aria-label': t('scan.mode') }, h('span', { class: 'muted' }, t('scan.mode')));
  const extra = h('div');
  for (const [value, key] of modes) {
    const input = h('input', { type: 'radio', name: 'scanMode', value, checked: scan.mode === value });
    input.addEventListener('change', () => { scan.mode = value; drawExtra(); });
    modeBox.append(h('label', { class: 'row' }, input, t(key)));
  }

  const drawExtra = (): void => {
    extra.replaceChildren();
    if (scan.mode === 'local') {
      const sel = h('select', {}, ...ifaces.map((i) => h('option', { value: i.name }, `${i.name} (${i.addrs.join(', ')})`)));
      sel.value = scan.interface || ifaces[0]?.name || '';
      scan.interface = sel.value;
      sel.addEventListener('change', () => { scan.interface = sel.value; });
      extra.append(field('scanIface', t('scan.interface'), sel));
    }
    if (scan.mode === 'ip') {
      if (scan.ranges!.length === 0) scan.ranges!.push({ base: '192.168.1', first: 1, last: 254 });
      const list = h('div');
      const drawRanges = (): void => {
        list.replaceChildren(...scan.ranges!.map((r, i) => rangeRow(r, () => { scan.ranges!.splice(i, 1); drawRanges(); })));
        add.toggleAttribute('disabled', scan.ranges!.length >= MAX_RANGES);
      };
      const add = h('button', { class: 'btn', onclick: () => { scan.ranges!.push({ base: '', first: 1, last: 254 }); drawRanges(); } }, t('scan.addRange'));
      extra.append(h('div', { class: 'field' }, h('span', { class: 'muted' }, t('scan.ranges')), list, h('div', {}, add)));
      drawRanges();
    }
  };
  drawExtra();

  const refresh = h('input', { type: 'number', min: 1, max: 3600, value: scan.refreshSeconds });
  const tics = h('input', { type: 'number', min: 1, max: 1000, value: scan.configTics });
  const res = result();
  const save = h('button', { class: 'btn primary', onclick: async () => {
    scan.refreshSeconds = Number(refresh.value);
    scan.configTics = Number(tics.value);
    if (scan.mode !== 'ip') scan.ranges = settings.scan.ranges ?? [];
    try { await devicesApi.updateSettings({ scan }); res.ok(); } catch (e) { res.fail(e); }
  } }, t('common.saveRescan'));

  // Default credentials (write-only password)
  const user = h('input', { autocomplete: 'username', value: creds.globalUser || 'admin' });
  const pass = h('input', { type: 'password', autocomplete: 'new-password', placeholder: creds.globalSet ? t('cred.unchanged') : '' });
  const cres = result();
  const csave = h('button', { class: 'btn', onclick: async () => {
    if (!pass.value) { cres.fail(new Error(t('login.needPassword'))); return; }
    try { await devicesApi.setCredentials(user.value, pass.value); pass.value = ''; pass.placeholder = t('cred.unchanged'); cres.ok(); } catch (e) { cres.fail(e); }
  } }, t('common.save'));
  const cclear = h('button', { class: 'btn', onclick: async () => {
    try { await devicesApi.setCredentials('', ''); pass.placeholder = ''; cres.ok(); } catch (e) { cres.fail(e); }
  } }, t('cred.clear'));

  // MQTT delay (ShellyScanner's -slow option)
  const slow = h('input', { type: 'number', min: 0, max: 600, value: settings.mqttSlow ?? 0 });
  const sres = result();
  const ssave = h('button', { class: 'btn', onclick: async () => {
    try { await devicesApi.updateSettings({ mqttSlow: Number(slow.value) }); sres.ok(); } catch (e) { sres.fail(e); }
  } }, t('common.save'));

  // Backups kept per device
  const keep = h('input', { type: 'number', min: 0, max: 1000, value: settings.backupKeep ?? 10 });
  const kres = result();
  const ksave = h('button', { class: 'btn', onclick: async () => {
    try { await devicesApi.updateSettings({ backupKeep: Number(keep.value) }); kres.ok(); } catch (e) { kres.fail(e); }
  } }, t('common.save'));

  // Address of this server for phones (local firmware download)
  const phone = h('input', { type: 'url', placeholder: location.origin, value: settings.phoneBaseURL ?? '', size: 32 });
  const pres = result();
  const psave = h('button', { class: 'btn', onclick: async () => {
    try { await devicesApi.updateSettings({ phoneBaseURL: phone.value }); pres.ok(); } catch (e) { pres.fail(e); }
  } }, t('common.save'));

  body.append(
    modeBox, extra,
    field('scanRefresh', t('scan.refresh'), refresh),
    field('scanTics', t('scan.tics'), tics, t('scan.ticsHelp')),
    h('div', { class: 'row' }, save, res.el),
    h('h3', { class: 'card-title', style: 'margin-top:24px' }, t('cred.title')),
    h('p', { class: 'muted' }, t('cred.text')),
    field('credUser', t('cred.user'), user),
    field('credPass', t('login.password'), pass),
    h('div', { class: 'row' }, csave, cclear, cres.el),
    h('h3', { class: 'card-title', style: 'margin-top:24px' }, t('slow.title')),
    field('mqttSlow', t('slow.label'), slow, t('slow.help')),
    h('div', { class: 'row' }, ssave, sres.el),
    h('h3', { class: 'card-title', style: 'margin-top:24px' }, t('backupKeep.title')),
    field('backupKeep', t('backupKeep.label'), keep, t('backupKeep.help')),
    h('div', { class: 'row' }, ksave, kres.el),
    h('h3', { class: 'card-title', style: 'margin-top:24px' }, t('phone.title')),
    field('phoneBase', t('phone.label'), phone, t('phone.help')),
    h('div', { class: 'row' }, psave, pres.el),
  );
}

function rangeRow(r: IPRange, remove: () => void): HTMLElement {
  const base = h('input', { value: r.base, placeholder: '192.168.1', size: 12, 'aria-label': t('scan.base') });
  const first = h('input', { type: 'number', min: 0, max: 255, value: r.first, 'aria-label': t('scan.first'), style: 'width:5em' });
  const last = h('input', { type: 'number', min: 0, max: 255, value: r.last, 'aria-label': t('scan.last'), style: 'width:5em' });
  base.addEventListener('input', () => { r.base = base.value.trim(); });
  first.addEventListener('input', () => { r.first = Number(first.value); });
  last.addEventListener('input', () => { r.last = Number(last.value); });
  return h('div', { class: 'row', style: 'margin-bottom:6px' }, base, '.', first, '–', last,
    h('button', { class: 'btn', onclick: remove, 'aria-label': t('scan.removeRange') }, '×'));
}

export async function archive(body: HTMLElement): Promise<void> {
  const settings = await devicesApi.settings();
  const arc = { ...settings.archive };
  const use = h('input', { type: 'checkbox', checked: arc.use });
  const auto = h('input', { type: 'checkbox', checked: arc.autoReload });
  use.addEventListener('change', () => { arc.use = use.checked; });
  auto.addEventListener('change', () => { arc.autoReload = auto.checked; });
  const res = result();
  body.append(
    h('p', { class: 'muted' }, t('archive.text')),
    h('label', { class: 'row field' }, use, t('archive.use')),
    h('label', { class: 'row field' }, auto, t('archive.autoReload')),
    h('div', { class: 'row' },
      h('button', { class: 'btn primary', onclick: async () => {
        try { await devicesApi.updateSettings({ archive: arc }); res.ok(); } catch (e) { res.fail(e); }
      } }, t('common.save')),
      h('button', { class: 'btn danger', onclick: async () => {
        if (await confirmDialog(t('archive.clear'), t('archive.clearConfirm'), t('archive.clear'))) {
          try { await devicesApi.clearArchive(); res.ok(); } catch (e) { res.fail(e); }
        }
      } }, t('archive.clear')),
      res.el),
  );
}
