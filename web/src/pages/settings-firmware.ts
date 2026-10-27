// Settings → Firmware (DECISIONS §34): who looks for firmware updates, the address
// devices fetch it from in local mode, and the address phones use (the wizard's download).
import { devicesApi, type FirmwareSettings } from '../api';
import { h } from '../dom';
import { t, type Key } from '../i18n';
import { field, result } from './settings-network';

export async function firmwareSettings(body: HTMLElement): Promise<void> {
  const settings = await devicesApi.settings();
  const fw: FirmwareSettings = { mode: settings.firmware?.mode === 'local' ? 'local' : 'device', deviceBaseURL: settings.firmware?.deviceBaseURL ?? '' };

  const modes: ['device' | 'local', Key, Key][] = [
    ['device', 'fwset.mode.device', 'fwset.mode.deviceHelp'],
    ['local', 'fwset.mode.local', 'fwset.mode.localHelp']];
  const modeBox = h('div', { class: 'field', role: 'radiogroup', 'aria-label': t('fwset.mode') }, h('span', { class: 'muted' }, t('fwset.mode')));
  const extra = h('div');
  for (const [value, key, help] of modes) {
    const input = h('input', { type: 'radio', name: 'fwMode', value, checked: fw.mode === value });
    input.addEventListener('change', () => { fw.mode = value; drawExtra(); });
    modeBox.append(h('label', { class: 'row' }, input, t(key)), h('small', { class: 'muted' }, t(help)));
  }

  // The address devices fetch from: only in local mode.
  const device = h('input', { type: 'url', placeholder: t('fwset.deviceAuto'), value: fw.deviceBaseURL ?? '', size: 32 });
  const drawExtra = (): void => {
    extra.replaceChildren(...(fw.mode === 'local' ? [field('fwDeviceBase', t('fwset.deviceLabel'), device, t('fwset.deviceHelp'))] : []));
  };
  drawExtra();
  const res = result();
  const save = h('button', { class: 'btn', onclick: async () => {
    try { await devicesApi.updateSettings({ firmware: { mode: fw.mode, deviceBaseURL: device.value.trim() } }); res.ok(); } catch (e) { res.fail(e); }
  } }, t('common.save'));

  // Address of this server for phones (the wizard's download through the device's access point)
  const phone = h('input', { type: 'url', placeholder: location.origin, value: settings.phoneBaseURL ?? '', size: 32 });
  const pres = result();
  const psave = h('button', { class: 'btn', onclick: async () => {
    try { await devicesApi.updateSettings({ phoneBaseURL: phone.value }); pres.ok(); } catch (e) { pres.fail(e); }
  } }, t('common.save'));

  body.append(
    h('h3', { class: 'card-title' }, t('fwset.title')),
    modeBox, extra,
    h('div', { class: 'row' }, save, res.el),
    h('h3', { class: 'card-title', style: 'margin-top:24px' }, t('phone.title')),
    field('phoneBase', t('phone.label'), phone, t('phone.help')),
    h('div', { class: 'row' }, psave, pres.el),
  );
}
