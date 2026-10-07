import { test } from 'node:test';
import assert from 'node:assert/strict';
import type { Device } from '../src/api';
import { addressText, compareAddress, statusMarks } from '../src/devices';

const dev = (ip: string, port = 80, id = ip): Device => ({
  id, mac: id, gen: '1', typeId: '', typeName: '', hostname: '', name: '', ip, port,
  status: 'online', managed: true, battery: false, lastSeen: 0, rebootRequired: false,
  rssi: 0, cloudEnabled: false, cloudConnected: false, mqttEnabled: false, mqttConnected: false, uptime: 0, logMode: 'NONE',
});

test('addresses sort numerically, then by port (ShellyScanner default order)', () => {
  const list = [dev('192.168.0.100'), dev('192.168.0.9'), dev('192.168.0.9', 8081), dev('10.0.0.1')];
  const sorted = [...list].sort(compareAddress).map(addressText);
  assert.deepEqual(sorted, ['10.0.0.1', '192.168.0.9', '192.168.0.9:8081', '192.168.0.100']);
});

test('status marks: ↻ for a restart, ↑ for a newer stable firmware, only when online', () => {
  assert.equal(statusMarks(dev('10.0.0.1')), '');
  assert.equal(statusMarks({ ...dev('10.0.0.1'), rebootRequired: true }), ' ↻');
  assert.equal(statusMarks({ ...dev('10.0.0.1'), updateAvailable: true, updateVersion: '1.4.4' }), ' ↑');
  assert.equal(statusMarks({ ...dev('10.0.0.1'), rebootRequired: true, updateAvailable: true }), ' ↻ ↑');
  assert.equal(statusMarks({ ...dev('10.0.0.1'), status: 'offline', rebootRequired: true, updateAvailable: true }), '');
});
