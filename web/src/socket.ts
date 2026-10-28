// Server events over WebSocket, reconnecting with capped exponential backoff.
// The socket is receive-only; actions go through the REST API.
import { wsURL } from './api';

export interface ServerEvent { type: string; data?: unknown }
export type ConnState = 'connecting' | 'ok' | 'down';

type Handler = (ev: ServerEvent) => void;

let current: EventSocket | null = null;

/** Connect the page's event socket again at once (the AP wizard's "Look again"). */
export function reconnectEvents(): void {
  current?.reconnect();
}

export class EventSocket {
  private ws: WebSocket | null = null;
  private handlers = new Map<string, Set<Handler>>();
  private retry = 0;
  private timer: ReturnType<typeof setTimeout> | undefined;
  private state: ConnState = 'connecting';
  private hiddenAt = 0;

  constructor(private onState: (s: ConnState) => void) {
    current = this;
  }

  private set(s: ConnState): void {
    this.state = s;
    this.onState(s);
  }

  connect(): void {
    this.set('connecting');
    const ws = new WebSocket(wsURL('ws'));
    this.ws = ws;
    // A phone on a network that does not reach the server may leave the attempt hanging for minutes,
    // holding one of the browser's few connections to it: give up after 10 s and try again.
    const opening = setTimeout(() => { if (this.ws === ws && ws.readyState === WebSocket.CONNECTING) this.reconnect(); }, 10_000);
    ws.addEventListener('open', () => clearTimeout(opening));
    ws.addEventListener('close', () => clearTimeout(opening));
    ws.onopen = () => {
      this.retry = 0;
      this.set('ok');
    };
    ws.onmessage = (m) => {
      let ev: ServerEvent;
      try { ev = JSON.parse(String(m.data)) as ServerEvent; } catch { return; }
      this.handlers.get(ev.type)?.forEach((h) => h(ev));
      this.handlers.get('*')?.forEach((h) => h(ev));
    };
    ws.onclose = () => {
      if (this.ws !== ws) return; // replaced by reconnect()
      this.set('down');
      clearTimeout(this.timer);
      this.timer = setTimeout(() => this.connect(), backoff(this.retry++));
    };
  }

  /** Drop the connection, whatever state it seems to be in, and connect again at once. */
  reconnect(): void {
    clearTimeout(this.timer);
    this.retry = 0;
    const old = this.ws;
    this.ws = null;
    if (old) {
      old.onmessage = null;
      old.close();
    }
    this.connect();
  }

  /**
   * A phone that changes network (a Shelly's access point and back) or puts the tab away often
   * keeps a dead connection without a close event, or waits out a long backoff: connect again
   * when the network comes back, and when the page is shown again after a while or while down.
   */
  watchPage(): void {
    window.addEventListener('online', () => this.reconnect());
    document.addEventListener('visibilitychange', () => {
      if (document.hidden) { this.hiddenAt = Date.now(); return; }
      if (this.state !== 'ok' || Date.now() - this.hiddenAt > 10_000) this.reconnect();
    });
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
