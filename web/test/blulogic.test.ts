import { test } from 'node:test';
import assert from 'node:assert/strict';
import type { Device } from '../src/api';
import { defaultGateway, pairingHint } from '../src/blulogic';

const dev = (over: Partial<Device>): Device => ({
  id: 'X', mac: 'x', gen: 'bth', typeId: 'BLU', typeName: 'Blu', hostname: '', name: '', ip: '', port: 80,
  status: 'online', managed: true, battery: false, lastSeen: 0, rebootRequired: false,
  rssi: 0, cloudEnabled: false, cloudConnected: false, mqttEnabled: false, mqttConnected: false, uptime: 0, logMode: 'NONE', ...over,
});

test('pairing mode: two buttons for the four-button models', () => {
  assert.equal(pairingHint(undefined), 'any');
  assert.equal(pairingHint(dev({ typeId: 'BLU7', typeName: 'Blu RC Button 4' })), 'two');
  assert.equal(pairingHint(dev({ typeName: 'Blu Wall Switch 4 / RC Button 4 ?' })), 'two');
  assert.equal(pairingHint(dev({ typeId: 'BLU3', typeName: 'Blu H&T' })), 'one');
});

test('the gateway: the device\'s own when it can scan', () => {
  const gws = [dev({ id: 'G1' }), dev({ id: 'G2' })];
  assert.equal(defaultGateway(gws, dev({ parent: 'G2' })), 'G2');
  assert.equal(defaultGateway(gws, dev({ parent: 'OTHER' })), 'G1');
  assert.equal(defaultGateway([], undefined), '');
});
