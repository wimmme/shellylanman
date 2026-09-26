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
