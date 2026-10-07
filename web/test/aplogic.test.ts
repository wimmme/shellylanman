import { test } from 'node:test';
import assert from 'node:assert/strict';
import type { APGuide, Device } from '../src/api';
import { apIsOff, canGoOn, cameBack, FIRMWARE_STEPS, neighbour, startWaiting, targetModel, watchedId } from '../src/aplogic';

const guide = (extra: Partial<APGuide> = {}): APGuide => ({ ssid: 'ShellyPlugSG3-54320467CBD4', recognised: true, gen: '2', key: 'PlugSG3', pageURL: 'http://192.168.33.1', pageQR: 'x', ...extra });
const dev = (extra: Partial<Device> = {}): Device => ({
  id: '54320467CBD4', mac: '54320467CBD4', gen: '3', typeId: 'PlugSG3', typeName: 'Plug S G3', hostname: '', name: '', ip: '10.0.0.5', port: 80,
  status: 'online', managed: true, battery: false, lastSeen: 0, rebootRequired: false,
  rssi: 0, cloudEnabled: false, cloudConnected: false, mqttEnabled: false, mqttConnected: false, uptime: 100000, logMode: 'NONE', ...extra,
});

test('the model comes from the name, else from the pick', () => {
  assert.deepEqual(targetModel({ guide: guide() }), { gen: '2', key: 'PlugSG3' });
  const unknown = guide({ recognised: false, gen: undefined, key: undefined });
  assert.equal(targetModel({ guide: unknown }), null);
  assert.deepEqual(targetModel({ guide: unknown, picked: { gen: '1', key: 'SHPLG-S', name: 'PlugS' } }), { gen: '1', key: 'SHPLG-S' });
  assert.equal(targetModel({}), null);
});

test('the first step needs a model, the others may always go on', () => {
  assert.equal(canGoOn('which', {}), false);
  assert.equal(canGoOn('which', { guide: guide({ recognised: false }) }), false);
  assert.equal(canGoOn('which', { guide: guide() }), true);
  assert.equal(canGoOn('file', {}), true);
});

test('the steps run in order and stop at both ends', () => {
  assert.deepEqual([...FIRMWARE_STEPS], ['which', 'file', 'join', 'page', 'wait']);
  assert.equal(neighbour(FIRMWARE_STEPS, 'which', 1), 'file');
  assert.equal(neighbour(FIRMWARE_STEPS, 'wait', -1), 'page');
  assert.equal(neighbour(FIRMWARE_STEPS, 'which', -1), null);
  assert.equal(neighbour(FIRMWARE_STEPS, 'wait', 1), null);
});

test('the device to watch: the one in the list, else the MAC of the name', () => {
  assert.equal(watchedId(guide({ id: 'AABBCC000001', mac: 'AABBCC000001' })), 'AABBCC000001');
  assert.equal(watchedId(guide({ mac: '54320467CBD4' })), '54320467CBD4');
  assert.equal(watchedId(guide()), '');
  assert.equal(watchedId(undefined), '');
});

test('back after the update: online and restarted, or online when it was not before', () => {
  const w = startWaiting(dev({ uptime: 100000 }), 1000);
  assert.equal(w.baseUptime, 100000);
  assert.equal(cameBack(dev({ uptime: 100050 }), w), false, 'still the same run');
  assert.equal(cameBack(dev({ uptime: 20 }), w), true, 'restarted');
  assert.equal(cameBack(dev({ uptime: 20, status: 'offline' }), w), false);
  assert.equal(cameBack(undefined, w), false);
  // not online when the wait began (offline, or not in the list): online is enough
  for (const before of [undefined, dev({ status: 'offline' }), dev({ status: 'ghost' })]) {
    const w2 = startWaiting(before, 1000);
    assert.equal(w2.baseUptime, undefined);
    assert.equal(cameBack(dev({ uptime: 5000 }), w2), true);
    assert.equal(cameBack(dev({ status: 'error' }), w2), false);
  }
});

test('the access point is off only for a device in the list that says so', () => {
  assert.equal(apIsOff(guide({ id: 'X', apEnabled: false })), true);
  assert.equal(apIsOff(guide({ id: 'X', apEnabled: true })), false);
  assert.equal(apIsOff(guide({ id: 'X' })), false, 'unknown');
  assert.equal(apIsOff(guide({ apEnabled: false })), false, 'not in the list: nothing to switch');
  assert.equal(apIsOff(undefined), false);
});
