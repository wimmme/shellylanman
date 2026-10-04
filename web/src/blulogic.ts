// The pure parts of the "Identify BLU devices" wizard (DECISIONS P14-4).
import type { Device } from './api';

/**
 * How the device enters pairing mode: one button held > 10 s, or two buttons
 * for the four-button models (RC Button 4, Wall Switch 4 — firmware 1.0.22+,
 * docs-ble/Devices/BLU/BluRCButton4.md). Unknown device: say both.
 */
export function pairingHint(d: Device | undefined): 'one' | 'two' | 'any' {
  if (!d) return 'any';
  if (/^BLU(6|7)$/.test(d.typeId) || /Switch 4|Button 4/.test(d.typeName)) return 'two';
  return 'one';
}

/** The gateway to scan with: the device's own, else the first that can. */
export function defaultGateway(gateways: Device[], d: Device | undefined): string {
  if (d?.parent && gateways.some((g) => g.id === d.parent)) return d.parent;
  return gateways[0]?.id ?? '';
}
