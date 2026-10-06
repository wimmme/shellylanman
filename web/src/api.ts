// REST client for /api/v1. Types mirror internal/httpapi.

export interface Status { firstRunDone: boolean; authEnabled: boolean; loggedIn?: boolean; clients: number; ingress?: boolean }

// URLs are relative to the page, so ShellyLanMan also works under a path prefix
// (Home Assistant ingress: /api/hassio_ingress/<token>/).
export function wsURL(path: string): string {
  const u = new URL(path, location.href);
  u.protocol = u.protocol === 'https:' ? 'wss:' : 'ws:';
  u.hash = '';
  return u.href;
}
export interface Settings { firstRunDone: boolean; language: string; updateCheck?: string; skipVersion?: string }
export interface UpdateStatus { mode: string; current: string; latest?: string; url?: string; newer: boolean; skipped?: boolean; checked?: number; error?: string }
export interface Credit { name: string; author: string; url: string; license: string; what: string }
export interface Dep { name: string; version: string; license: string }
export interface About {
  name: string; version: string; commit?: string; license: string; source: string; issues: string; started: number;
  runtime: { go: string; os: string; arch: string; kernel?: string; memMB: number; data: string };
  basedOn: Credit[]; credits: Credit[]; deps: Dep[]; notice: string; languages: string[]; donate: { name: string; url: string }[];
}
/** Where ShellyLanMan listens and where that is set (DECISIONS P17-1); shown, not changed. */
export interface ServerInfo {
  port: number; source: 'default' | 'env' | 'app';
  app: boolean; ingress?: string; mcpLocal?: string;
}
export interface MCPInfo { enabled: boolean; access: 'read' | 'control' | 'configure'; hasToken: boolean; token?: string }

export class ApiError extends Error {
  constructor(public status: number, message: string, public code?: string, public retryAfter?: number) {
    super(message);
  }
}

// A session that ended (logged out elsewhere, password changed): main.ts shows the login.
let loginRequired: () => void = () => {};
export function onLoginRequired(fn: () => void): void { loginRequired = fn; }

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const init: RequestInit = { method, headers: {} };
  if (body !== undefined) {
    init.body = JSON.stringify(body);
    (init.headers as Record<string, string>)['Content-Type'] = 'application/json';
  }
  const resp = await fetch('api/v1' + path, init);
  const data = resp.status === 204 ? {} : await resp.json().catch(() => ({}));
  if (!resp.ok) {
    const e = data as { error?: string; code?: string; retryAfter?: number };
    if (resp.status === 401 && e.error === 'login required') loginRequired();
    throw new ApiError(resp.status, e.error || resp.statusText, e.code, e.retryAfter);
  }
  return data as T;
}

export const api = {
  status: () => request<Status>('GET', '/status'),
  about: () => request<About>('GET', '/about'),
  settings: () => request<Settings>('GET', '/settings'),
  updateSettings: (patch: Partial<Settings>) => request<Settings>('PUT', '/settings', patch),
  update: () => request<UpdateStatus>('GET', '/update'),
  server: () => request<ServerInfo>('GET', '/server'),
  mcp: () => request<MCPInfo>('GET', '/mcp'),
  setMCP: (patch: { enabled?: boolean; access?: MCPInfo['access'] }) => request<MCPInfo>('PUT', '/mcp', patch),
  newMCPToken: () => request<MCPInfo>('POST', '/mcp/token', {}),
};

// ---- UI password (DECISIONS §24) ----
export const authApi = {
  login: (password: string, remember: boolean) => request<void>('POST', '/auth/login', { password, remember }),
  logout: () => request<void>('POST', '/auth/logout', {}),
  /** Set or change (current needed once one is set); an empty password switches it off. */
  setPassword: (current: string, password: string) => request<{ authEnabled: boolean }>('PUT', '/auth/password', { current, password }),
};

// ---- devices (Phase 2) ----

export type DeviceStatus = 'online' | 'offline' | 'login' | 'reading' | 'error' | 'ghost' | 'searching';

export interface Device {
  id: string; mac: string; gen: string; typeId: string; typeName: string;
  hostname: string; name: string; ip: string; port: number; status: DeviceStatus;
  managed: boolean; battery: boolean; error?: string; lastSeen: number; ssid?: string;
  rebootRequired: boolean; parent?: string; parents?: string[]; note?: string; keyword?: string;
  /** A BLU device a gateway only relays to the Shelly Cloud: read only (DECISIONS P14-1). */
  relay?: boolean; protected?: boolean;
  rssi: number; cloudEnabled: boolean; cloudConnected: boolean; mqttEnabled: boolean; mqttConnected: boolean;
  uptime: number; logMode: string; internalTemp?: number; meters?: MeterSet[]; modules?: Module[]; paused?: boolean;
  layout?: Layout;
}

/** How the Command cell draws the modules: the array type of ShellyScanner's getModules(). */
export type Layout = 'relay' | 'roller' | 'rgbcct' | 'rgbw' | 'rgb' | 'thermostat' | 'trvg1' | 'cb' | 'mixed';

export interface MeterValue { type: string; value: number; name?: string }
export interface MeterSet { label?: string; values: MeterValue[]; total?: boolean }
export interface Module {
  kind: string; index: number; key?: string; label: string; on?: boolean; brightness?: number; gain?: number; position?: number;
  calibrated?: boolean; target?: number; inputOn?: boolean; inputOn1?: boolean; rgb?: number[]; white?: number; tempK?: number;
  colorMode?: boolean; state?: string; source?: string;
  min?: number; max?: number; div?: number; tMin?: number; tMax?: number;
  enabled?: boolean; running?: boolean; schedule?: boolean; locked?: boolean; motion?: boolean; events?: InputEvent[];
}
export interface InputEvent { event: string; enabled: boolean }
/** One action on one module (service.Command). */
export interface Command {
  key: string; action: string; value?: number; rgb?: number[]; white?: number; event?: number; confirm?: boolean;
}
export interface InfoRequest { name: string; path: string }
export interface InfoResult { data: unknown; stored: boolean }

export interface ScanState {
  mode: string; scanning: boolean; mdnsActive: boolean; mdnsError?: string; mdnsInstances: number; startedAt: number;
}

export interface IPRange { base: string; first: number; last: number }
export interface ScanSettings { mode: string; interface?: string; ranges?: IPRange[]; refreshSeconds: number; configTics: number }
export interface ArchiveSettings { use: boolean; autoReload: boolean }
export interface FullSettings extends Settings { scan: ScanSettings; archive: ArchiveSettings; mqttSlow?: number; backupKeep?: number; phoneBaseURL?: string }
export interface CredentialsInfo { globalSet: boolean; globalUser: string }
export interface NetInterface { name: string; addrs: string[] }

export const devicesApi = {
  list: () => request<Device[]>('GET', '/devices'),
  scan: () => request<ScanState>('GET', '/scan'),
  rescan: () => request<unknown>('POST', '/scan'),
  refresh: (ids?: string[]) => request<unknown>('POST', '/devices/refresh', ids ? { ids } : {}),
  reload: (id: string) => request<unknown>('POST', `/devices/${encodeURIComponent(id)}/reload`),
  remove: (id: string) => request<unknown>('DELETE', `/devices/${encodeURIComponent(id)}`),
  setDeviceCredentials: (id: string, user: string, password: string) =>
    request<unknown>('PUT', `/devices/${encodeURIComponent(id)}/credentials`, { user, password }),
  credentials: () => request<CredentialsInfo>('GET', '/credentials'),
  setCredentials: (user: string, password: string) => request<CredentialsInfo>('PUT', '/credentials', { user, password }),
  interfaces: () => request<NetInterface[]>('GET', '/network/interfaces'),
  settings: () => request<FullSettings>('GET', '/settings'),
  updateSettings: (patch: Partial<FullSettings>) => request<FullSettings>('PUT', '/settings', patch),
  clearArchive: () => request<unknown>('DELETE', '/archive'),
  infoRequests: (id: string) => request<InfoRequest[]>('GET', `/devices/${encodeURIComponent(id)}/info`),
  info: (id: string, index: number) => request<InfoResult>('GET', `/devices/${encodeURIComponent(id)}/info/${index}`),
  setNote: (id: string, note: string, keyword: string, name?: string) =>
    request<unknown>('PUT', `/devices/${encodeURIComponent(id)}/note`, name === undefined ? { note, keyword } : { note, keyword, name }),
  pause: (id: string, paused: boolean) => request<unknown>('PUT', `/devices/${encodeURIComponent(id)}/pause`, { paused }),
  command: (id: string, cmd: Command) => request<unknown>('POST', `/devices/${encodeURIComponent(id)}/command`, cmd),
  reboot: (ids: string[]) => request<unknown>('POST', '/devices/reboot', { ids, confirm: true }),
  logText: async (id: string, file: number): Promise<string> => {
    const r = await fetch(`api/v1/devices/${encodeURIComponent(id)}/log?file=${file}`);
    const text = await r.text();
    if (!r.ok) throw new ApiError(r.status, text);
    return text;
  },
};

// ---- configuration (Phase 5) ----

export interface DeviceRef { id: string; name: string }
export interface ResultLine extends DeviceRef { result: 'ok' | 'fail' | 'queued' | 'excluded' | 'stored' | 'cancel'; message?: string }
export interface WiFiForm { enabled: boolean; ssid: string; static: boolean | null; ip: string; netmask: string; gateway: string; dns: string }
export interface LoginForm { enabled: boolean; user: string }
export interface MQTTForm {
  enabled: boolean; server: string; user: string; prefix: string; noPassword: boolean;
  reconnectMax?: number; reconnectMin?: number; cleanSession?: boolean; keepAlive?: number; qos?: number; retain?: boolean; updatePeriod?: number;
  control?: boolean; rpc?: boolean; rpcNtf?: boolean; statusNtf?: boolean;
}
export interface OthersForm { ntp: string; cloud: boolean | null; reset: boolean | null }
export interface ConfigForm {
  section: string; variant?: 'g1' | 'g2' | 'mix'; devices: number; excluded: DeviceRef[];
  wifi?: WiFiForm; login?: LoginForm; mqtt?: MQTTForm; others?: OthersForm;
}
export type Cell = boolean | string | number | BLEItem[] | null;
export interface BLEItem { id?: string; name?: string; mac?: string; address?: string; lastSeen?: number }
export interface ChecklistRow {
  id: string; host: string; address: string; status: DeviceStatus; gen: string;
  eco: Cell; led: Cell; logs: Cell; ble: Cell; ap: Cell; roaming: Cell; wifi1: Cell; wifi2: Cell; extender: Cell; scripts: Cell; autoFW: Cell;
}
export interface DeferredTask {
  id: string; time: number; deviceId: string; deviceName: string; type: string; description: string;
  status: 'WAITING' | 'CANCELLED' | 'RUNNING' | 'SUCCESS' | 'FAIL'; message?: string;
}

const idsQuery = (ids: string[]): string => 'ids=' + ids.map(encodeURIComponent).join(',');

export const configApi = {
  form: (section: string, ids: string[]) => request<ConfigForm>('GET', `/config/${section}?${idsQuery(ids)}`),
  apply: (section: string, ids: string[], body: Record<string, unknown>) =>
    request<{ results: ResultLine[] }>('POST', `/config/${section}`, { ids, ...body }),
  checklist: (ids: string[]) => request<ChecklistRow[]>('GET', `/checklist${ids.length ? '?' + idsQuery(ids) : ''}`),
  checklistAction: (ids: string[], action: string, value: boolean, mode?: string) =>
    request<{ errors: ResultLine[]; rows: ChecklistRow[] }>('POST', '/checklist/action', { ids, action, value, mode }),
  deferred: () => request<DeferredTask[]>('GET', '/deferred'),
  cancelDeferred: (id: string) => request<unknown>('DELETE', `/deferred/${encodeURIComponent(id)}`),
};

// ---- backup and restore (Phase 6) ----

export interface BackupFile { deviceId: string; name: string; hostname: string; time: number; size: number }
/** A stored backup (of any device) or an uploaded .sbk (base64). */
export interface RestoreSource { deviceId?: string; file?: string; upload?: string }
export interface RestoreItem { key: string; type: 'pre' | 'error' | 'warn' | 'ask'; value?: string; args?: string[] }
export interface RestorePlan { items: RestoreItem[]; queue: boolean }
export interface RestoreResult { result: 'ok' | 'queued' | 'fail'; problems?: string[]; reboot?: boolean }

export const backupApi = {
  backup: (ids: string[]) => request<{ results: ResultLine[] }>('POST', '/backup', { ids }),
  list: (id?: string) => request<BackupFile[]>('GET', '/backups' + (id ? '?id=' + encodeURIComponent(id) : '')),
  downloadURL: (f: BackupFile): string => `api/v1/devices/${encodeURIComponent(f.deviceId)}/backups/${encodeURIComponent(f.name)}`,
  check: (id: string, source: RestoreSource) => request<RestorePlan>('POST', `/devices/${encodeURIComponent(id)}/restore/check`, { source }),
  restore: (id: string, source: RestoreSource, answers: Record<string, string>) =>
    request<RestoreResult>('POST', `/devices/${encodeURIComponent(id)}/restore`, { source, answers, confirm: true }),
};

// ---- firmware (Phase 7) ----

export interface FirmwareRow {
  id: string; name: string; status: DeviceStatus; gen: string; known: boolean; valid: boolean;
  current?: string; currentBuild?: string; stable?: string; stableBuild?: string; beta?: string; betaBuild?: string;
  preselect?: boolean; updating?: boolean; progress: number; rebooting?: boolean; queued?: boolean;
}

export const firmwareApi = {
  rows: (ids: string[]) => request<FirmwareRow[]>('GET', '/firmware' + (ids.length ? '?' + idsQuery(ids) : '')),
  update: (items: { id: string; stage: string }[]) => request<{ results: ResultLine[] }>('POST', '/firmware/update', { items, confirm: true }),
};
export interface IndexRow { id: string; key?: string; current?: string; latest?: string; source?: string; newer: boolean; error?: string }
export interface LocalLink {
  url: string; qr: string; name: string; model: string; current: string; version: string; source: string; fileName: string; expires: number; warning?: string;
}
export const localFwApi = {
  index: (ids: string[]) => request<IndexRow[]>('GET', '/firmware/index' + (ids.length ? '?' + idsQuery(ids) : '')),
  link: (id: string) => request<LocalLink>('POST', `/firmware/${encodeURIComponent(id)}/local`),
};

// ---- scripts and KVS (Phase 8) ----

export interface ScriptInfo { id: number; name: string; enable: boolean; running: boolean }
export interface KVItem { key: string; etag: string; value: string }
export interface ScriptsView { scripts: ScriptInfo[] | null; kvs: KVItem[] | null }

const dev = (id: string): string => `/devices/${encodeURIComponent(id)}`;
export const scriptsApi = {
  list: (id: string) => request<ScriptsView>('GET', `${dev(id)}/scripts`),
  create: (id: string) => request<ScriptInfo>('POST', `${dev(id)}/scripts`),
  update: (id: string, sid: number, patch: { name?: string; enable?: boolean }) => request<unknown>('PATCH', `${dev(id)}/scripts/${sid}`, patch),
  remove: (id: string, sid: number) => request<unknown>('DELETE', `${dev(id)}/scripts/${sid}`),
  run: (id: string, sid: number, run: boolean, log: boolean) => request<unknown>('POST', `${dev(id)}/scripts/${sid}/${run ? 'start' : 'stop'}${log ? '?log=true' : ''}`),
  code: async (id: string, sid: number): Promise<string> => (await request<{ code: string }>('GET', `${dev(id)}/scripts/${sid}/code`)).code,
  putCode: (id: string, sid: number, code: string) => request<unknown>('PUT', `${dev(id)}/scripts/${sid}/code`, { code }),
  kvsSet: (id: string, key: string, value: string) => request<KVItem>('POST', `${dev(id)}/kvs`, { key, value }),
  kvsDelete: (id: string, key: string) => request<unknown>('DELETE', `${dev(id)}/kvs?key=${encodeURIComponent(key)}`),
  logOn: (id: string) => request<unknown>('POST', `${dev(id)}/scripts/log`),
  backupScripts: (upload: string) => request<{ name: string; code: string }[]>('POST', '/sbk/scripts', { upload }),
};

// ---- schedulers (Phase 8) ----

export interface MethodHint { name: string; method?: string; params?: string }
export const scheduleApi = {
  rpc: async (id: string, method: string, params: unknown): Promise<unknown> =>
    (await request<{ result: unknown }>('POST', `/devices/${encodeURIComponent(id)}/rpc`, { method, params })).result,
  hints: (id: string) => request<MethodHint[]>('GET', `/devices/${encodeURIComponent(id)}/schedule/hints`),
  backupJSON: (upload: string) => request<Record<string, unknown>>('POST', '/sbk/json', { upload }),
};

// ---- charts (Phase 8) ----

export interface Sample { t: number; rssi: number; temp?: number; meters?: MeterSet[] }
export interface EMSeries { meter: string; lines: number; data: [number, number][] }
// ---- BLU identification (DECISIONS P14-4) ----

export interface BLUDiscovered { id: string; mac: string; localName: string; modelId: number; model: string; rssi: number; listed: boolean }
export interface BLUIdentifyEvent { state: 'started' | 'done' | 'error'; gateway: string; duration?: number; found?: number; error?: string }
export const bluApi = {
  gateways: () => request<Device[]>('GET', '/blu/gateways'),
  identify: (gateway: string, duration: number) => request<unknown>('POST', '/blu/identify', { gateway, duration }),
};

export const chartsApi = {
  samples: (ids: string[]) => request<Record<string, Sample[]>>('GET', `/samples?${idsQuery(ids)}`),
  clear: (ids: string[]) => request<unknown>('DELETE', `/samples?${idsQuery(ids)}`),
  emdata: (id: string, start: number, end: number) => request<EMSeries[]>('GET', `/devices/${encodeURIComponent(id)}/emdata?start=${start}&end=${end}`),
};
