// Portions derived from ShellyScanner (https://github.com/usnasoft/shellyscanner),
// Copyright (C) Antonio Flaccomio / usnasoft, licensed under GPL-3.0:
// controller/RestoreAction.restoreDevice (order of the questions, script
// answers, message arguments) and erroreMsg.
//
// The decisions of the restore dialog, without DOM, so they can be tested.
import type { RestoreItem } from './api';

export const Q_OVERRIDE = 'QUESTION_RESTORE_SCRIPTS_OVERRIDE';
export const Q_ENABLE = 'QUESTION_RESTORE_SCRIPTS_ENABLE_LIKE_BACKED_UP';
export const Q_SKIP = 'QUESTION_RESTORE_SCRIPTS_SKIP';

/** The password dialogs, in the order of the original. */
export const PASSWORD_ASKS = ['RESTORE_LOGIN', 'RESTORE_WI_FI1', 'RESTORE_WI_FI2', 'RESTORE_WI_FI_AP', 'RESTORE_MQTT'] as const;
export type PasswordAsk = typeof PASSWORD_ASKS[number];

/** What each password dialog shows: the name field (read only) and whether the password is typed twice. */
export const PASSWORD_DIALOG: Record<PasswordAsk, { title: string; user: 'user' | 'ssid' | null; confirm: boolean }> = {
  RESTORE_LOGIN: { title: 'restore.dlg.login', user: 'user', confirm: true },     // user shown for Gen1 only
  RESTORE_WI_FI1: { title: 'restore.dlg.wifi1', user: 'ssid', confirm: true },
  RESTORE_WI_FI2: { title: 'restore.dlg.wifi2', user: 'ssid', confirm: true },
  RESTORE_WI_FI_AP: { title: 'restore.dlg.ap', user: null, confirm: true },
  RESTORE_MQTT: { title: 'restore.dlg.mqtt', user: 'user', confirm: false },
};

export interface Plan {
  pre: RestoreItem[]; error: RestoreItem | null; warn: RestoreItem[];
  passwords: RestoreItem[]; override: RestoreItem | null; enable: RestoreItem | null;
}

/** Split the check result like restoreDevice walks it. */
export function planOf(items: RestoreItem[]): Plan {
  const by = (k: string): RestoreItem | null => items.find((i) => i.key === k) ?? null;
  return {
    pre: items.filter((i) => i.type === 'pre'),
    error: items.find((i) => i.type === 'error') ?? null,
    warn: items.filter((i) => i.type === 'warn'),
    passwords: PASSWORD_ASKS.map(by).filter((i): i is RestoreItem => i !== null),
    override: by(Q_OVERRIDE),
    enable: by(Q_ENABLE),
  };
}

export type ScriptChoice = 'rename' | 'skip' | 'overwrite';

/** Answers of the scripts questions; enableAsked tells whether the "enable" question is to be asked. */
export function scriptAnswers(plan: Plan, choice: ScriptChoice | null): { answers: Record<string, string>; enableAsked: boolean } {
  const answers: Record<string, string> = {};
  if (plan.override && choice === 'overwrite') answers[Q_OVERRIDE] = 'true';
  if (plan.override && choice === 'skip') answers[Q_SKIP] = 'true';
  // Asked when there is no name conflict, or the user chose to overwrite (not on rename or skip).
  const enableAsked = plan.enable !== null && choice !== 'skip' && (answers[Q_OVERRIDE] !== undefined || plan.override === null);
  return { answers, enableAsked };
}

/** Answers of a multiple restore: scripts overwritten and enabled like the backup, no passwords. */
export const MULTI_ANSWERS: Record<string, string> = { [Q_OVERRIDE]: 'true', [Q_ENABLE]: 'true' };

/** Labels of the lights profiles (lbl_<type><profile>). */
const PROFILE_LABELS: Record<string, string> = {
  PlusRGBWPMlight: 'restore.profile.lightx4', PlusRGBWPMrgb: 'restore.profile.rgb', PlusRGBWPMrgbw: 'restore.profile.rgbw',
  ProRGBWWPMlight: 'restore.profile.lightx5', ProRGBWWPMrgbx2light: 'restore.profile.rgbx2light',
  ProRGBWWPMrgbcct: 'restore.profile.rgbcct', ProRGBWWPMcctx2: 'restore.profile.cctx2',
};

/** The message of a check item: an i18n key and its variables (profile names translated when a label exists). */
export function itemMessage(item: RestoreItem, typeId: string, tr: (k: string) => string): { key: string; vars: Record<string, string> } {
  const label = (v: string): string => (PROFILE_LABELS[typeId + v] ? tr(PROFILE_LABELS[typeId + v]!) : v);
  const args = item.args ?? [];
  return { key: 'restore.msg.' + item.key, vars: { value: item.value ?? '', current: label(args[0] ?? ''), backup: label(args[1] ?? '') } };
}

/** errRestore<text> when the original has a translation, else the text itself. */
const PROBLEM_KEYS = new Set(['ERR_RESTORE_MODE_COVER', 'ERR_UNKNOWN', 'ERR_RESTORE_POWER_BASE']);
export function problemText(p: string, tr: (k: string) => string): string {
  return PROBLEM_KEYS.has(p) ? tr('restore.err.' + p) : p;
}

/** Multi restore result: the reason is shown when it is short (dlgSetMultiMsgFailReason). */
export function shortReason(text: string): string | undefined {
  return text.length < 50 ? text : undefined;
}
