// Device logs (ShellyScanner: DialogDeviceLogsG1 / DialogDeviceLogsG2).
// Gen1: the /debug/log and /debug/log1 files. Gen2+: the live /debug/log
// WebSocket, relayed by the server; for a BLU device, its gateway's log.
import { devicesApi, wsURL, type Device } from '../api';
import { allDevices } from '../devices';
import { h } from '../dom';
import { t } from '../i18n';
import { openModal } from '../modal';

const LEVELS = ['0 - error', '1 - warn', '2 - info', '3 - debug', '4 - verbose'];
const MAX_LINES = 5000;

export function openLogs(id: string): void {
  const dev = allDevices().find((d) => d.id === id);
  if (!dev) return;
  if (dev.gen === '1') gen1(dev);
  else gen2(dev);
}

function gen1(dev: Device): void {
  const tabs = h('div', { class: 'tabs', role: 'tablist' });
  const pre = h('pre', { class: 'log', tabindex: 0 });
  let file = 0;
  const load = async (f: number): Promise<void> => {
    file = f;
    tabs.querySelectorAll('button').forEach((b, j) => b.setAttribute('aria-selected', String(j === f)));
    pre.textContent = t('state.loading');
    try { pre.textContent = await devicesApi.logText(dev.id, f); } catch (e) { pre.textContent = t('state.error', { msg: e instanceof Error ? e.message : String(e) }); }
  };
  ['debug/log', 'debug/log1'].forEach((name, i) => tabs.append(h('button', { role: 'tab', onclick: () => void load(i) }, name)));
  openModal(`${t('action.logs')} — ${dev.hostname}`, h('div', {}, tabs,
    h('div', { class: 'row' }, h('button', { class: 'btn', onclick: () => void load(file) }, t('action.refresh')),
      h('button', { class: 'btn', onclick: () => void navigator.clipboard?.writeText(pre.textContent ?? '') }, t('common.copy'))),
    pre), [{ label: t('common.close') }], undefined, 'wide');
  void load(0);
}

function gen2(dev: Device): void {
  const pre = h('pre', { class: 'log', tabindex: 0, 'aria-live': 'polite' });
  const level = h('select', { 'aria-label': t('logs.level') }, ...LEVELS.map((l, i) => h('option', { value: i }, l)));
  level.value = '4'; // AbstractG2Device.LOG_VERBOSE
  let ws: WebSocket | null = null;
  let paused = !!dev.paused;
  const on = h('button', { class: 'btn primary' }, t('logs.on'));
  const offBtn = h('button', { class: 'btn', disabled: true }, t('logs.off'));
  const pauseBtn = h('button', { class: 'btn', 'aria-pressed': String(paused) }, t('logs.pauseRefresh'));

  const append = (text: string, cls = ''): void => {
    const line = h('div', { class: cls }, text);
    const atBottom = pre.scrollTop + pre.clientHeight >= pre.scrollHeight - 20;
    pre.append(line);
    while (pre.childElementCount > MAX_LINES) pre.firstElementChild?.remove();
    if (atBottom) pre.scrollTop = pre.scrollHeight;
  };
  const connect = (): void => {
    ws = new WebSocket(wsURL(`ws/log/${encodeURIComponent(dev.id)}`));
    on.setAttribute('disabled', '');
    offBtn.removeAttribute('disabled');
    ws.onopen = () => append('>>>> Open', 'log-meta');
    ws.onclose = (e) => {
      append(`>>>> Close${e.reason ? ': ' + e.reason : ''} (${e.code})`, 'log-meta');
      on.removeAttribute('disabled');
      offBtn.setAttribute('disabled', '');
    };
    ws.onmessage = (m) => {
      let msg: { ts?: number; level?: number; fd?: number | string; data?: string; error?: string };
      try { msg = JSON.parse(String(m.data)); } catch { append(String(m.data)); return; }
      if (msg.error) { append('>>>> ' + msg.error, 'log-meta'); return; }
      const lvl = msg.level ?? 0;
      if (lvl <= Number(level.value)) append(`${Math.trunc(msg.ts ?? 0)} - L${lvl} - fd${msg.fd ?? ''}: ${(msg.data ?? '').trim()}`);
    };
  };
  on.addEventListener('click', connect);
  offBtn.addEventListener('click', () => ws?.close(1000, 'bye'));
  pauseBtn.addEventListener('click', async () => {
    paused = !paused;
    await devicesApi.pause(dev.id, paused);
    pauseBtn.setAttribute('aria-pressed', String(paused));
    if (paused) append('>>>> ' + t('logs.paused'), 'log-meta');
  });

  const title = dev.gen === 'blu' || dev.gen === 'bth' ? t('logs.gatewayOf', { device: dev.hostname }) : dev.hostname;
  openModal(`${t('action.logs')} — ${title}`, h('div', {},
    h('div', { class: 'row' }, on, offBtn, pauseBtn, h('label', { class: 'row' }, t('logs.level'), level),
      h('button', { class: 'btn', onclick: () => pre.replaceChildren() }, t('logs.clear')),
      h('button', { class: 'btn', onclick: () => void navigator.clipboard?.writeText(pre.innerText) }, t('common.copy'))),
    h('p', { class: 'muted' }, t('logs.hint')),
    pre), [{ label: t('common.close') }], () => {
    ws?.close(1000, 'bye');
    if (paused) void devicesApi.pause(dev.id, false); // resume refresh when the dialog closes
  }, 'wide');
  connect();
}
