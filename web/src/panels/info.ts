// Device info (ShellyScanner: DialogDeviceInfo): one tab per info request,
// raw JSON, refreshed when the device comes back on line.
import { devicesApi, type InfoRequest } from '../api';
import { allDevices, onDevicesChanged } from '../devices';
import { h } from '../dom';
import { t } from '../i18n';
import { openModal } from '../modal';

export function openInfo(id: string): void {
  const dev = allDevices().find((d) => d.id === id);
  if (!dev) return;
  const tabs = h('div', { class: 'tabs', role: 'tablist' });
  const badge = h('span', { class: 'card-badge hidden' }, t('info.stored'));
  const pre = h('pre', { class: 'json', tabindex: 0 }, t('state.loading'));
  let reqs: InfoRequest[] = [];
  let current = 0;
  let lastStatus = dev.status;

  const show = async (i: number): Promise<void> => {
    current = i;
    tabs.querySelectorAll('button').forEach((b, j) => b.setAttribute('aria-selected', String(j === i)));
    pre.textContent = t('state.loading');
    badge.classList.add('hidden');
    try {
      const res = await devicesApi.info(id, i);
      if (current !== i) return;
      pre.textContent = typeof res.data === 'string' ? res.data : JSON.stringify(res.data, null, 2);
      badge.classList.toggle('hidden', !res.stored);
    } catch (e) {
      if (current === i) pre.textContent = t('state.error', { msg: e instanceof Error ? e.message : String(e) });
    }
  };

  const onKey = (e: KeyboardEvent): void => {
    if (!(e.ctrlKey || e.metaKey) || reqs.length === 0) return;
    if (e.key === 'ArrowRight') { e.preventDefault(); void show((current + 1) % reqs.length); }
    if (e.key === 'ArrowLeft') { e.preventDefault(); void show((current - 1 + reqs.length) % reqs.length); }
  };
  document.addEventListener('keydown', onKey);
  const off = onDevicesChanged(() => { // DialogDeviceInfo auto-updates when the device goes on line
    const d = allDevices().find((x) => x.id === id);
    if (d && d.status === 'online' && lastStatus !== 'online') void show(current);
    if (d) lastStatus = d.status;
  });

  const body = h('div', { class: 'info-panel' },
    tabs,
    h('div', { class: 'row' },
      h('button', { class: 'btn', onclick: () => void show(current) }, t('action.refresh')),
      h('button', { class: 'btn', onclick: () => void navigator.clipboard?.writeText(pre.textContent ?? '') }, t('common.copy')),
      badge),
    pre);
  openModal(`${t('action.info')} — ${dev.hostname || dev.ip}`, body, [{ label: t('common.close') }], () => {
    off();
    document.removeEventListener('keydown', onKey);
  }, 'wide');

  void devicesApi.infoRequests(id).then((list) => {
    reqs = list;
    list.forEach((r, i) => tabs.append(h('button', { role: 'tab', title: r.path, onclick: () => void show(i) }, r.name)));
    if (list.length > 0) void show(0);
  }).catch((e) => { pre.textContent = t('state.error', { msg: e instanceof Error ? e.message : String(e) }); });
}
