// The logic of the profiles (DECISIONS P20-11): the form's state to the API's input and back,
// the lines that describe a profile, and which devices have just come onto the network.
// Pure functions, tested without a browser.
import type { Device, Profile, ProfileInput } from './api';

/** A switch the profile may leave alone. */
export type Tri = '' | 'on' | 'off';

export const triOf = (b: boolean | undefined): Tri => (b === undefined ? '' : b ? 'on' : 'off');
export const boolOf = (t: Tri): boolean | undefined => (t === '' ? undefined : t === 'on');

/** What the editor shows. Passwords are only here when the user typed one. */
export interface ProfileForm {
  name: string;
  namePattern: string;
  wifiSSID: string;
  login: Tri; user: string; loginPassword: string;
  mqtt: Tri; mqttServer: string; mqttUser: string; mqttPassword: string;
  ntp: string;
  cloud: Tri; eco: Tri; ledOff: Tri; ap: Tri; roaming: Tri;
  autoFW: '' | 'stable' | 'beta' | 'none';
}

export function emptyForm(): ProfileForm {
  return {
    name: '', namePattern: '', wifiSSID: '', login: '', user: 'admin', loginPassword: '',
    mqtt: '', mqttServer: '', mqttUser: '', mqttPassword: '', ntp: '', cloud: '', eco: '', ledOff: '', ap: '', roaming: '', autoFW: '',
  };
}

export function formOf(p: Profile): ProfileForm {
  return {
    name: p.name, namePattern: p.namePattern ?? '', wifiSSID: p.wifiSSID ?? '',
    login: p.login ? (p.login.enabled ? 'on' : 'off') : '', user: p.login?.user || 'admin', loginPassword: '',
    mqtt: p.mqtt ? (p.mqtt.enabled ? 'on' : 'off') : '', mqttServer: p.mqtt?.server ?? '', mqttUser: p.mqtt?.user ?? '', mqttPassword: '',
    ntp: p.ntp ?? '', cloud: triOf(p.cloud), eco: triOf(p.eco), ledOff: triOf(p.ledOff), ap: triOf(p.ap), roaming: triOf(p.roaming),
    autoFW: (p.autoFW as ProfileForm['autoFW']) ?? '',
  };
}

/**
 * The API's input. A password is sent only when typed (an empty one leaves the stored one alone);
 * a login or MQTT switched off carries nothing else.
 */
export function inputOf(f: ProfileForm, id?: string): ProfileInput {
  const out: ProfileInput = { id: id ?? '', name: f.name.trim() };
  const text = (v: string): string | undefined => (v.trim() === '' ? undefined : v.trim());
  out.namePattern = text(f.namePattern);
  out.wifiSSID = text(f.wifiSSID);
  if (f.login === 'on') out.login = { enabled: true, user: text(f.user) };
  else if (f.login === 'off') out.login = { enabled: false };
  if (f.login === 'on' && f.loginPassword !== '') out.loginPassword = f.loginPassword;
  if (f.mqtt === 'on') {
    const user = text(f.mqttUser);
    out.mqtt = { enabled: true, server: text(f.mqttServer), user, noPassword: user === undefined ? true : undefined };
    if (user !== undefined && f.mqttPassword !== '') out.mqttPassword = f.mqttPassword;
  } else if (f.mqtt === 'off') out.mqtt = { enabled: false };
  out.ntp = text(f.ntp);
  out.cloud = boolOf(f.cloud);
  out.eco = boolOf(f.eco);
  out.ledOff = boolOf(f.ledOff);
  out.ap = boolOf(f.ap);
  out.roaming = boolOf(f.roaming);
  if (f.autoFW !== '') out.autoFW = f.autoFW;
  return JSON.parse(JSON.stringify(out)) as ProfileInput; // without the undefined fields
}

/** The things a profile sets, as [step, value] in the order they run (the name's pattern as written). */
export function describe(p: Profile): [string, string][] {
  const out: [string, string][] = [];
  const onOff = (b: boolean): string => (b ? 'on' : 'off');
  if (p.namePattern) out.push(['name', p.namePattern]);
  if (p.eco !== undefined) out.push(['eco', onOff(p.eco)]);
  if (p.ledOff !== undefined) out.push(['ledOff', onOff(p.ledOff)]);
  if (p.ap !== undefined) out.push(['ap', onOff(p.ap)]);
  if (p.roaming !== undefined) out.push(['roaming', onOff(p.roaming)]);
  if (p.autoFW) out.push(['autoFW', p.autoFW]);
  if (p.ntp) out.push(['ntp', p.ntp]);
  if (p.cloud !== undefined) out.push(['cloud', onOff(p.cloud)]);
  if (p.mqtt) out.push(['mqtt', p.mqtt.enabled ? p.mqtt.server || '' : 'off']);
  if (p.login) out.push(['login', p.login.enabled ? p.login.user || 'admin' : 'off']);
  return out;
}

export interface Seen { status: string; uptime: number }

/** What was on the network when the wait began: id → how it was. */
export function snapshot(list: Device[]): Map<string, Seen> {
  return new Map(list.map((d) => [d.id, { status: d.status, uptime: d.uptime }]));
}

/**
 * Devices that have come onto the network since the snapshot: online now and either not online
 * before (new, or archived and now back) or restarted since (uptime dropped). MAC, when known
 * from the access point's name, puts that device first.
 */
export function newlyOnline(list: Device[], before: Map<string, Seen>, mac = ''): Device[] {
  const out = list.filter((d) => {
    if (d.status !== 'online') return false;
    const b = before.get(d.id);
    return !b || b.status !== 'online' || (d.uptime >= 0 && b.uptime >= 0 && d.uptime < b.uptime);
  });
  return out.sort((a, b) => Number(b.id === mac) - Number(a.id === mac));
}
