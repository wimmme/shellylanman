// Live device list: loaded over REST, kept current by WebSocket events
// (device.upsert, device.removed, devices.reset, scan.state).
import { devicesApi, type Device, type ScanState } from './api';
import type { EventSocket } from './socket';

type Listener = () => void;

const devices = new Map<string, Device>();
let scan: ScanState | null = null;
let useArchive = true;

/** Settings → Archive "use archive" (notes live in the archive). */
export function archiveInUse(): boolean { return useArchive; }
export function setArchiveInUse(v: boolean): void { useArchive = v; changed(); }
const listeners = new Set<Listener>();
let scheduled = false;

/** Coalesce bursts of events (a scan finds dozens of devices) into one repaint per frame. */
function changed(): void {
  if (scheduled) return;
  scheduled = true;
  const run = (): void => {
    scheduled = false;
    listeners.forEach((l) => l());
  };
  if (typeof requestAnimationFrame === 'function') requestAnimationFrame(run);
  else setTimeout(run, 0);
}

export function onDevicesChanged(l: Listener): () => void {
  listeners.add(l);
  return () => listeners.delete(l);
}

export function allDevices(): Device[] {
  return [...devices.values()];
}

export function scanState(): ScanState | null {
  return scan;
}

export async function loadDevices(): Promise<void> {
  const [list, st] = await Promise.all([devicesApi.list(), devicesApi.scan()]);
  devices.clear();
  for (const d of list) devices.set(d.id, d);
  scan = st;
  changed();
}

export function wireDeviceEvents(socket: EventSocket): void {
  socket.on('hello', () => { void loadDevices(); }); // (re)connected: resync
  socket.on('device.upsert', (ev) => {
    const d = ev.data as Device;
    devices.set(d.id, d);
    changed();
  });
  socket.on('device.removed', (ev) => {
    devices.delete((ev.data as { id: string }).id);
    changed();
  });
  socket.on('devices.reset', (ev) => {
    devices.clear();
    for (const d of (ev.data as Device[]) || []) devices.set(d.id, d);
    changed();
  });
  socket.on('scan.state', (ev) => {
    scan = ev.data as ScanState;
    changed();
  });
}

/** Numeric IPv4 then port order (ShellyScanner's default sort, InetAddressAndPort). */
export function compareAddress(a: Device, b: Device): number {
  const pa = a.ip.split('.').map(Number);
  const pb = b.ip.split('.').map(Number);
  for (let i = 0; i < 4; i++) {
    const d = (pa[i] ?? 0) - (pb[i] ?? 0);
    if (d !== 0) return d;
  }
  return a.port - b.port || a.id.localeCompare(b.id);
}

/** "ip" or "ip:port" when not 80, as ShellyScanner shows addresses. */
export function addressText(d: Device): string {
  return d.port === 80 ? d.ip : `${d.ip}:${d.port}`;
}
