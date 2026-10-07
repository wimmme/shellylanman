import { test } from 'node:test';
import assert from 'node:assert/strict';
import type { Device, Profile } from '../src/api';
import { boolOf, describe, emptyForm, formOf, inputOf, newlyOnline, snapshot, triOf } from '../src/profilelogic';

const dev = (id: string, extra: Partial<Device> = {}): Device => ({
  id, mac: id, gen: '3', typeId: 'PlugSG3', typeName: 'Plug S G3', hostname: '', name: '', ip: '10.0.0.5', port: 80,
  status: 'online', managed: true, battery: false, lastSeen: 0, rebootRequired: false,
  rssi: 0, cloudEnabled: false, cloudConnected: false, mqttEnabled: false, mqttConnected: false, uptime: 5000, logMode: 'NONE', ...extra,
});

test('a switch is left alone, on or off', () => {
  assert.equal(triOf(undefined), '');
  assert.equal(triOf(true), 'on');
  assert.equal(triOf(false), 'off');
  assert.equal(boolOf(''), undefined);
  assert.equal(boolOf('on'), true);
  assert.equal(boolOf('off'), false);
});

test('an empty form sends only the name', () => {
  const f = emptyForm();
  f.name = '  Home ';
  assert.deepEqual(inputOf(f), { id: '', name: 'Home' });
});

test('the form becomes the API input: left alone is absent, passwords only when typed', () => {
  const f = emptyForm();
  Object.assign(f, {
    name: 'Home', namePattern: '{model} {mac4}', wifiSSID: ' HomeNet ', login: 'on', user: 'me', loginPassword: 'secret1',
    mqtt: 'on', mqttServer: 'broker:1883', mqttUser: 'mq', mqttPassword: 'mqpw', ntp: 'pool.ntp.org', cloud: 'off', eco: 'on', ledOff: 'off', autoFW: 'stable',
  });
  assert.deepEqual(inputOf(f, 'abc'), {
    id: 'abc', name: 'Home', namePattern: '{model} {mac4}', wifiSSID: 'HomeNet',
    login: { enabled: true, user: 'me' }, loginPassword: 'secret1',
    mqtt: { enabled: true, server: 'broker:1883', user: 'mq' }, mqttPassword: 'mqpw',
    ntp: 'pool.ntp.org', cloud: false, eco: true, ledOff: false, autoFW: 'stable',
  });
  // editing without typing the passwords again keeps the stored ones
  f.loginPassword = ''; f.mqttPassword = '';
  const keep = inputOf(f, 'abc');
  assert.equal('loginPassword' in keep, false);
  assert.equal('mqttPassword' in keep, false);
  // off carries nothing else; MQTT without a user has no password
  f.login = 'off'; f.mqtt = 'on'; f.mqttUser = '';
  const off = inputOf(f, 'abc');
  assert.deepEqual(off.login, { enabled: false });
  assert.deepEqual(off.mqtt, { enabled: true, server: 'broker:1883', noPassword: true });
});

test('a profile fills the form and the lines that describe it', () => {
  const p: Profile = { id: 'x', name: 'Home', namePattern: '{model}', login: { enabled: true, user: 'admin' }, mqtt: { enabled: false }, cloud: false, eco: true, ap: false, autoFW: 'beta', ntp: 'pool.ntp.org' };
  const f = formOf(p);
  assert.equal(f.login, 'on');
  assert.equal(f.mqtt, 'off');
  assert.equal(f.cloud, 'off');
  assert.equal(f.eco, 'on');
  assert.equal(f.ledOff, '');
  assert.equal(f.loginPassword, '', 'a password is never filled in');
  assert.deepEqual(describe(p), [['name', '{model}'], ['eco', 'on'], ['ap', 'off'], ['autoFW', 'beta'], ['ntp', 'pool.ntp.org'], ['cloud', 'off'], ['mqtt', 'off'], ['login', 'admin']]);
  assert.deepEqual(describe({ id: 'y', name: 'Empty' }), []);
});

test('devices that came onto the network since the wait began', () => {
  const before = snapshot([dev('OLD', { uptime: 100000 }), dev('GONE', { status: 'offline' }), dev('GHOST', { status: 'ghost' })]);
  const now = [
    dev('OLD', { uptime: 100100 }),        // still the same run: not new
    dev('GONE', { uptime: 30 }),           // was offline, is online: back
    dev('GHOST', { uptime: 40 }),          // archived, now on the network again
    dev('NEW', { uptime: 20 }),            // never seen
    dev('RESTARTED', { uptime: 10 }),      // not in the snapshot at all: new
    dev('LOGIN', { status: 'login' }),     // not online: not yet
  ];
  assert.deepEqual(newlyOnline(now, before).map((d) => d.id).sort(), ['GHOST', 'GONE', 'NEW', 'RESTARTED']);
  // an online device that restarted shows up again
  const b2 = snapshot([dev('OLD', { uptime: 100000 })]);
  assert.deepEqual(newlyOnline([dev('OLD', { uptime: 12 })], b2).map((d) => d.id), ['OLD']);
  // the MAC from the access point's name comes first
  assert.deepEqual(newlyOnline(now, before, 'NEW').map((d) => d.id)[0], 'NEW');
});
