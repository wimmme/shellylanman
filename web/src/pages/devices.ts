import { h, ICONS } from '../dom';
import { t, type Key } from '../i18n';
import { card, emptyState, type Page } from './common';

// Column headings from ShellyScanner's device table (MainView / DevicesTable),
// kept verbatim as terminology; they get translations when the table is built.
const COLUMNS = ['', 'Type', 'Device', 'Name', 'IP', 'RSSI (dBm)', 'Cloud (En/Con)', 'MQTT (En/Con)', 'Uptime', 'Temp', 'Measurements', 'Source', 'Command'];

const SUMMARY: { key: Key; cls: string }[] = [
  { key: 'devices.summary.total', cls: '' },
  { key: 'devices.summary.online', cls: 'ok' },
  { key: 'devices.summary.offline', cls: 'err' },
  { key: 'devices.summary.login', cls: 'warn' },
  { key: 'devices.summary.reboot', cls: 'warn' },
  { key: 'devices.summary.updates', cls: '' },
  { key: 'devices.summary.stored', cls: '' },
];

export const devicesPage: Page = {
  id: 'devices',
  title: 'nav.devices',
  icon: ICONS.devices,
  render(main) {
    main.append(h('div', { class: 'summary' },
      ...SUMMARY.map((s) => h('div', { class: 'stat ' + s.cls }, h('div', { class: 'k' }, t(s.key)), h('div', { class: 'v' }, '0')))));
    const table = h('table', { class: 'data' },
      h('thead', {}, h('tr', {}, ...COLUMNS.map((c) => h('th', { scope: 'col' }, c)))),
      h('tbody'));
    main.append(card(t('devices.table.title'), '0',
      h('div', { class: 'table-wrap' }, table),
      emptyState(t('devices.empty.title'), t('devices.empty.text'), ICONS.devices)));
  },
};
