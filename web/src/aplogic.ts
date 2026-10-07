// The logic of the access-point wizards (DECISIONS §29): which model a step works on, whether
// a step may go on, and when a device is "back" after its update. Pure functions, tested without a browser.
import type { APGuide, Device, ModelChoice } from './api';

export type WizardStep = 'which' | 'file' | 'join' | 'page' | 'wait';

/** The steps of the firmware wizard, in order. */
export const FIRMWARE_STEPS: readonly WizardStep[] = ['which', 'file', 'join', 'page', 'wait'];

export interface WizardState {
  guide?: APGuide;
  /** The model the user picked when the access point's name did not say. */
  picked?: ModelChoice;
}

/** The model the firmware is for: read from the name or picked, else none. */
export function targetModel(s: WizardState): { gen: string; key: string } | null {
  const g = s.guide;
  if (g?.recognised && g.gen && g.key) return { gen: g.gen, key: g.key };
  if (s.picked) return { gen: s.picked.gen, key: s.picked.key };
  return null;
}

/** Whether the wizard may leave `step` (the first step needs a model; the others only need to have been shown). */
export function canGoOn(step: WizardStep, s: WizardState): boolean {
  if (step === 'which') return !!s.guide && targetModel(s) !== null;
  return true;
}

export function neighbour(steps: readonly WizardStep[], cur: WizardStep, dir: 1 | -1): WizardStep | null {
  const i = steps.indexOf(cur) + dir;
  return steps[i] ?? null;
}

/** The device to watch after the update: the one in the list, else the MAC of the access point's name. */
export function watchedId(g: APGuide | undefined): string {
  return g?.id || g?.mac || '';
}

export interface Waiting {
  since: number;
  /** Uptime of the device when the wait began; absent when it was not online then (or not in the list). */
  baseUptime?: number;
}

export function startWaiting(d: Device | undefined, now: number): Waiting {
  return { since: now, baseUptime: d && d.status === 'online' && d.uptime >= 0 ? d.uptime : undefined };
}

/**
 * The device is back after its update: it is online and either it was not before, or it restarted
 * since the wait began (its uptime is lower than it was). A device that is online with the same
 * long uptime has not restarted yet.
 */
export function cameBack(d: Device | undefined, w: Waiting): boolean {
  if (!d || d.status !== 'online') return false;
  if (w.baseUptime === undefined) return true;
  return d.uptime >= 0 && d.uptime < w.baseUptime;
}

/** The AP's name for display and for the join QR: the Wi-Fi network, trimmed. */
export function apLabel(g: APGuide): string {
  return g.ssid.trim();
}

/** The access point of a device that ShellyLanMan can still reach is off: the wizard offers to switch it on. */
export function apIsOff(g: APGuide | undefined): boolean {
  return !!g?.id && g.apEnabled === false;
}

/** The wizard may switch the access point on for Gen2 and newer; for Gen1 it only says how (DECISIONS P20-10). */
export function canSwitchAPOn(g: APGuide | undefined): boolean {
  return apIsOff(g) && g?.gen !== '1';
}
