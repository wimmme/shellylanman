// REST client for /api/v1. Types mirror internal/httpapi.

export interface Status { firstRunDone: boolean; authEnabled: boolean; clients: number }
export interface Settings { firstRunDone: boolean; language: string }
export interface Credit { name: string; author: string; url: string; license: string; what: string }
export interface About {
  name: string; version: string; commit?: string; license: string; source: string;
  basedOn: Credit; credits: Credit[]; notice: string; languages: string[];
}

export class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message);
  }
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const init: RequestInit = { method, headers: {} };
  if (body !== undefined) {
    init.body = JSON.stringify(body);
    (init.headers as Record<string, string>)['Content-Type'] = 'application/json';
  }
  const resp = await fetch('/api/v1' + path, init);
  const data = await resp.json().catch(() => ({}));
  if (!resp.ok) throw new ApiError(resp.status, (data as { error?: string }).error || resp.statusText);
  return data as T;
}

export const api = {
  status: () => request<Status>('GET', '/status'),
  about: () => request<About>('GET', '/about'),
  settings: () => request<Settings>('GET', '/settings'),
  updateSettings: (patch: Partial<Settings>) => request<Settings>('PUT', '/settings', patch),
};

// ---- devices (Phase 2) ----

export type DeviceStatus = 'online' | 'offline' | 'login' | 'reading' | 'error' | 'ghost';

export interface Device {
  id: string; mac: string; gen: string; typeId: string; typeName: string;
  hostname: string; name: string; ip: string; port: number; status: DeviceStatus;
  managed: boolean; battery: boolean; error?: string; lastSeen: number; ssid?: string;
  rebootRequired: boolean; parent?: string; parents?: string[]; note?: string; keyword?: string;
  rssi: number; cloudEnabled: boolean; cloudConnected: boolean; mqttEnabled: boolean; mqttConnected: boolean;
  uptime: number; logMode: string; internalTemp?: number; meters?: MeterSet[]; modules?: Module[]; paused?: boolean;
  layout?: Layout;
}

/** How the Command cell draws the modules: the array type of ShellyScanner's getModules(). */
export type Layout = 'relay' | 'roller' | 'rgbcct' | 'rgbw' | 'rgb' | 'thermostat' | 'trvg1' | 'cb' | 'mixed';

export interface MeterValue { type: string; value: number; name?: string }
export interface MeterSet { label?: string; values: MeterValue[] }
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
export interface FullSettings extends Settings { scan: ScanSettings; archive: ArchiveSettings; mqttSlow?: number; backupKeep?: number }
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
  pause: (id: string, paused: boolean) => request<unknown>('PUT', `/devices/${encodeURIComponent(id)}/pause`, { paused }),
  command: (id: string, cmd: Command) => request<unknown>('POST', `/devices/${encodeURIComponent(id)}/command`, cmd),
  reboot: (ids: string[]) => request<unknown>('POST', '/devices/reboot', { ids, confirm: true }),
  logText: async (id: string, file: number): Promise<string> => {
    const r = await fetch(`/api/v1/devices/${encodeURIComponent(id)}/log?file=${file}`);
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
  downloadURL: (f: BackupFile): string => `/api/v1/devices/${encodeURIComponent(f.deviceId)}/backups/${encodeURIComponent(f.name)}`,
  check: (id: string, source: RestoreSource) => request<RestorePlan>('POST', `/devices/${encodeURIComponent(id)}/restore/check`, { source }),
  restore: (id: string, source: RestoreSource, answers: Record<string, string>) =>
    request<RestoreResult>('POST', `/devices/${encodeURIComponent(id)}/restore`, { source, answers, confirm: true }),
};
