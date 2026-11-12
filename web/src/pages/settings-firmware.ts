// Settings → Firmware (DECISIONS §34): who looks for firmware updates, the address
// devices fetch it from in local mode, and the address phones use (the wizard's QR code and
// download); both addresses are checked (P32-1).
import { devicesApi, type AddressCheck, type FirmwareSettings } from '../api';
import { h } from '../dom';
import { t, type Key } from '../i18n';
import { field, result } from './settings-network';

/** What the check found on an address (DECISIONS P32-1): a wrong port is easy to miss. */
function addressNote(el: HTMLElement, c: AddressCheck | undefined): void {
  if (!c) { el.replaceChildren(); return; }
  const port = c.port ? ' ' + t('addr.port', { port: String(c.port) }) : '';
  el.replaceChildren(c.result === 'ok' ? h('small', { class: 'muted addr-ok' }, t('addr.ok'))
    : c.result === 'other' ? h('div', { class: 'banner warn addr-other', role: 'alert' }, t('addr.other') + port)
      : h('div', { class: 'banner addr-unreachable', role: 'note' }, t('addr.unreachable', { error: c.error ?? '' }) + port));
}

export async function firmwareSettings(body: HTMLElement): Promise<void> {
  let settings = await devicesApi.settings();
  const fw: FirmwareSettings = { mode: settings.firmware?.mode === 'local' ? 'local' : 'device', deviceBaseURL: settings.firmware?.deviceBaseURL ?? '' };

  const modes: ['device' | 'local', Key, Key][] = [
    ['device', 'fwset.mode.device', 'fwset.mode.deviceHelp'],
    ['local', 'fwset.mode.local', 'fwset.mode.localHelp']];
  const modeBox = h('div', { class: 'field', role: 'radiogroup', 'aria-label': t('fwset.mode') }, h('span', { class: 'muted' }, t('fwset.mode')));
  const extra = h('div');
  const deviceNote = h('div', { class: 'addr-note', 'data-for': 'device' });
  const phoneNote = h('div', { class: 'addr-note', 'data-for': 'phone' });
  // The saved addresses are checked when the page opens and after each save.
  const check = async (): Promise<void> => {
    const busy = (el: HTMLElement, set: string | undefined): void => {
      el.replaceChildren(...(set ? [h('small', { class: 'muted' }, t('addr.checking'))] : []));
    };
    busy(phoneNote, settings.phoneBaseURL);
    busy(deviceNote, settings.firmware?.deviceBaseURL);
    const c = await devicesApi.checkAddresses().catch(() => null);
    addressNote(phoneNote, c?.phone);
    addressNote(deviceNote, c?.device);
  };
  for (const [value, key, help] of modes) {
    const input = h('input', { type: 'radio', name: 'fwMode', value, checked: fw.mode === value });
    input.addEventListener('change', () => { fw.mode = value; drawExtra(); });
    modeBox.append(h('label', { class: 'row' }, input, t(key)), h('small', { class: 'muted' }, t(help)));
  }

  // The address devices fetch from: only in local mode.
  const device = h('input', { type: 'url', placeholder: t('fwset.deviceAuto'), value: fw.deviceBaseURL ?? '', size: 32 });
  const drawExtra = (): void => {
    extra.replaceChildren(...(fw.mode === 'local' ? [field('fwDeviceBase', t('fwset.deviceLabel'), device, t('fwset.deviceHelp')), deviceNote] : []));
  };
  drawExtra();
  const res = result();
  const save = h('button', { class: 'btn', onclick: async () => {
    try { settings = await devicesApi.updateSettings({ firmware: { mode: fw.mode, deviceBaseURL: device.value.trim() } }); res.ok(); void check(); } catch (e) { res.fail(e); }
  } }, t('common.save'));

  // Address of this server for phones (the wizard's download through the device's access point)
  const phone = h('input', { type: 'url', placeholder: location.origin, value: settings.phoneBaseURL ?? '', size: 32 });
  const pres = result();
  const psave = h('button', { class: 'btn', onclick: async () => {
    try { settings = await devicesApi.updateSettings({ phoneBaseURL: phone.value }); pres.ok(); void check(); } catch (e) { pres.fail(e); }
  } }, t('common.save'));

  body.append(
    h('h3', { class: 'card-title' }, t('fwset.title')),
    modeBox, extra,
    h('div', { class: 'row' }, save, res.el),
    h('h3', { class: 'card-title', style: 'margin-top:24px' }, t('phone.title')),
    field('phoneBase', t('phone.label'), phone, t('phone.help')), phoneNote,
    h('div', { class: 'row' }, psave, pres.el),
  );
  void check();
}
