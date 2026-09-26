import { test } from 'node:test';
import assert from 'node:assert/strict';
import type { Device } from '../src/api';
import { addressText, compareAddress } from '../src/devices';

const dev = (ip: string, port = 80, id = ip): Device => ({
  id, mac: id, gen: '1', typeId: '', typeName: '', hostname: '', name: '', ip, port,
  status: 'online', managed: true, battery: false, lastSeen: 0, rebootRequired: false,
});

test('addresses sort numerically, then by port (ShellyScanner default order)', () => {
  const list = [dev('192.168.0.100'), dev('192.168.0.9'), dev('192.168.0.9', 8081), dev('10.0.0.1')];
  const sorted = [...list].sort(compareAddress).map(addressText);
  assert.deepEqual(sorted, ['10.0.0.1', '192.168.0.9', '192.168.0.9:8081', '192.168.0.100']);
});
