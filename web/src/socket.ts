// Server events over WebSocket, reconnecting with capped exponential backoff.
// The socket is receive-only; actions go through the REST API.
import { wsURL } from './api';

export interface ServerEvent { type: string; data?: unknown }
export type ConnState = 'connecting' | 'ok' | 'down';

type Handler = (ev: ServerEvent) => void;

export class EventSocket {
  private ws: WebSocket | null = null;
  private handlers = new Map<string, Set<Handler>>();
  private retry = 0;
  private timer: ReturnType<typeof setTimeout> | undefined;

  constructor(private onState: (s: ConnState) => void) {}

  connect(): void {
    this.onState('connecting');
    const ws = new WebSocket(wsURL('ws'));
    this.ws = ws;
    ws.onopen = () => {
      this.retry = 0;
      this.onState('ok');
    };
    ws.onmessage = (m) => {
      let ev: ServerEvent;
      try { ev = JSON.parse(String(m.data)) as ServerEvent; } catch { return; }
      this.handlers.get(ev.type)?.forEach((h) => h(ev));
      this.handlers.get('*')?.forEach((h) => h(ev));
    };
    ws.onclose = () => {
      this.onState('down');
      clearTimeout(this.timer);
      this.timer = setTimeout(() => this.connect(), backoff(this.retry++));
    };
  }

  /** Subscribe to one event type, or '*' for all. Returns an unsubscribe function. */
  on(type: string, h: Handler): () => void {
    let set = this.handlers.get(type);
    if (!set) this.handlers.set(type, (set = new Set()));
    set.add(h);
    return () => set!.delete(h);
  }
}

/** The server that answers after a reconnect is another version than the one this page came from:
 * its API may have changed, so the page must be loaded again (main.ts asks, it does not reload by itself:
 * a script being edited would be lost). */
export function newServer(seen: string | undefined, now: string | undefined): boolean {
  return !!seen && !!now && seen !== now;
}

/** 1 s, 2 s, 4 s … capped at 30 s. */
export function backoff(attempt: number): number {
  return Math.min(30_000, 1000 * 2 ** Math.min(attempt, 5));
}
