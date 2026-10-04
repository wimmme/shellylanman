// The wizard "Identify BLU devices" (ShellyLanMan's own, DECISIONS P14-4): a
// gateway scans actively for a while; the user puts the BLU device in pairing
// mode; every device that answers is shown with its model, and a listed device
// gets the model on its row. Nothing changes on the device or the gateway.
import { ApiError, bluApi, type BLUDiscovered, type BLUIdentifyEvent, type Device } from '../api';
import { defaultGateway, pairingHint } from '../blulogic';
import { h } from '../dom';
import { t } from '../i18n';
import { openModal } from '../modal';
import type { EventSocket } from '../socket';

const DURATION = 90;

const discoveredListeners = new Set<(d: BLUDiscovered) => void>();
const stateListeners = new Set<(e: BLUIdentifyEvent) => void>();

export function wireBLUEvents(socket: EventSocket): void {
  socket.on('blu.discovered', (ev) => { const d = ev.data as BLUDiscovered; discoveredListeners.forEach((l) => l(d)); });
  socket.on('blu.identify', (ev) => { const e = ev.data as BLUIdentifyEvent; stateListeners.forEach((l) => l(e)); });
}

/** Open the wizard, for one device (its gateway and its pairing hint) or for any; gatewayId preselects a gateway. */
export async function openIdentify(target?: Device, gatewayId?: string): Promise<void> {
  let gateways: Device[] = [];
  try { gateways = await bluApi.gateways(); } catch { /* shown below */ }
  gateways.sort((a, b) => (a.name || a.hostname).localeCompare(b.name || b.hostname));

  const gwSel = h('select', { id: 'bluGw' }, ...gateways.map((g) => h('option', { value: g.id }, `${g.name || g.hostname} (${g.ip})`)));
  gwSel.value = gatewayId && gateways.some((g) => g.id === gatewayId) ? gatewayId : defaultGateway(gateways, target);
  const hint = pairingHint(target);
  const status = h('p', { class: 'blu-status', role: 'status' });
  const tbody = h('tbody');
  const table = h('table', { class: 'data blu-found hidden' },
    h('thead', {}, h('tr', {}, ...(['blu.col.model', 'blu.col.name', 'col.mac', 'col.rssi', 'blu.col.list'] as const).map((k) => h('th', {}, t(k))))), tbody);
  const start = h('button', { class: 'btn primary', disabled: gateways.length === 0 }, t('blu.start'));

  let timer = 0;
  let found = 0;
  const stop = (): void => { window.clearInterval(timer); timer = 0; };
  const onFound = (d: BLUDiscovered): void => {
    found++;
    table.classList.remove('hidden');
    tbody.append(h('tr', { class: d.id === target?.id ? 'selected' : '' },
      h('td', {}, d.model || `#${d.modelId}`), h('td', { class: 'mono' }, d.localName), h('td', { class: 'mono' }, d.mac),
      h('td', {}, String(d.rssi)), h('td', {}, t(d.listed ? 'blu.listed' : 'blu.notListed'))));
  };
  const onState = (e: BLUIdentifyEvent): void => {
    if (e.gateway !== gwSel.value || e.state === 'started') return;
    stop();
    start.disabled = false;
    gwSel.disabled = false;
    status.textContent = e.state === 'error' ? t('blu.failed', { error: e.error ?? '' }) : t(found ? 'blu.done' : 'blu.none');
  };

  start.addEventListener('click', async () => {
    start.disabled = true;
    gwSel.disabled = true;
    tbody.replaceChildren();
    table.classList.add('hidden');
    found = 0;
    try {
      await bluApi.identify(gwSel.value, DURATION);
    } catch (e) {
      start.disabled = false;
      gwSel.disabled = false;
      status.textContent = e instanceof ApiError ? e.message : String(e);
      return;
    }
    let left = DURATION;
    status.textContent = t('blu.scanning', { s: left });
    timer = window.setInterval(() => {
      left--;
      if (left > 0) status.textContent = t('blu.scanning', { s: left });
      else stop();
    }, 1000);
  });

  discoveredListeners.add(onFound);
  stateListeners.add(onState);
  const steps = h('ol', { class: 'blu-steps' },
    h('li', {}, t('blu.step1')),
    h('li', {}, t(hint === 'two' ? 'blu.step2two' : hint === 'one' ? 'blu.step2one' : 'blu.step2any')),
    h('li', {}, t('blu.step3')));
  const body = h('div', { class: 'blu-identify' },
    h('p', {}, t('blu.intro')),
    gateways.length
      ? h('div', { class: 'field' }, h('label', { for: 'bluGw' }, t('blu.gateway')), gwSel)
      : h('p', { class: 'banner warn' }, t('blu.noGateway')),
    steps, h('div', { class: 'row' }, start), status, table,
    h('p', { class: 'muted' }, t('blu.note')));
  openModal(target ? t('blu.titleFor', { device: target.name || target.hostname }) : t('blu.title'), body, [{ label: t('common.close') }], () => {
    stop();
    discoveredListeners.delete(onFound);
    stateListeners.delete(onState);
  }, 'wide');
}
