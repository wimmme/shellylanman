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
}

export interface ScanState {
  mode: string; scanning: boolean; mdnsActive: boolean; mdnsError?: string; mdnsInstances: number; startedAt: number;
}

export interface IPRange { base: string; first: number; last: number }
export interface ScanSettings { mode: string; interface?: string; ranges?: IPRange[]; refreshSeconds: number; configTics: number }
export interface ArchiveSettings { use: boolean; autoReload: boolean }
export interface FullSettings extends Settings { scan: ScanSettings; archive: ArchiveSettings }
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
};
