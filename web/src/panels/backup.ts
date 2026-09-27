// Portions derived from ShellyScanner (https://github.com/usnasoft/shellyscanner),
// Copyright (C) Antonio Flaccomio / usnasoft, licensed under GPL-3.0:
// controller/BackupAction and controller/RestoreAction (dialogs, result
// lines, reboot offer), view/DialogAuthentication.
//
// Backups live on the server (/data/backups); a restore picks one of them
// or a .sbk uploaded from this computer.
import { ApiError, backupApi, devicesApi, type BackupFile, type Device, type RestoreItem, type RestoreSource, type ResultLine } from '../api';
import { h } from '../dom';
import { dateTime } from '../format';
import { t, type Key } from '../i18n';
import { confirmDialog, openModal } from '../modal';
import {
  itemMessage, MULTI_ANSWERS, PASSWORD_DIALOG, planOf, problemText, Q_ENABLE, scriptAnswers, shortReason,
  type PasswordAsk, type ScriptChoice,
} from '../restorelogic';
import { toast } from '../toast';
import { showResults } from './devsettings';

const tr = (k: string, vars?: Record<string, string | number>): string => t(k as Key, vars);
const errText = (e: unknown): string => (e instanceof ApiError || e instanceof Error ? e.message : String(e));
const hostOf = (d: Device): string => d.hostname || d.ip;

// ---- backup ---------------------------------------------------------------------

/** BackupAction: one result line per device (ok, stored data used, queued, fail). */
export async function backupDevices(list: Device[]): Promise<void> {
  toast(t('backup.running', { n: list.length }), 'info');
  try {
    const res = await backupApi.backup(list.map((d) => d.id));
    showResults(t('backup.done'), res.results);
  } catch (e) {
    toast(errText(e));
  }
}

// ---- small dialogs ----------------------------------------------------------------

/** A message with buttons; resolves to the index of the button pressed, or -1 when closed. */
function ask(title: string, message: string, buttons: { label: string; kind?: 'primary' | 'danger' }[]): Promise<number> {
  return new Promise((resolve) => {
    let answer = -1;
    const body = h('div', { class: 'restore-msg' }, ...message.split('\n').map((l) => h('p', {}, l)));
    openModal(title, body, buttons.map((b, i) => ({ ...b, onClick: () => { answer = i; } })), () => resolve(answer));
  });
}

/** DialogAuthentication: the name is shown read only; resolves to the password, or null to skip that setting. */
function passwordDialog(item: RestoreItem, gen1: boolean): Promise<string | null> {
  const cfg = PASSWORD_DIALOG[item.key as PasswordAsk];
  return new Promise((resolve) => {
    let answer: string | null = null;
    const pass = h('input', { id: 'rsPass', type: 'password', autocomplete: 'new-password' });
    const pass2 = h('input', { id: 'rsPass2', type: 'password', autocomplete: 'new-password' });
    const name = h('input', { id: 'rsName', readonly: true, value: item.value ?? '' });
    const err = h('p', { class: 'muted', role: 'alert' });
    const showName = cfg.user === 'ssid' || (cfg.user === 'user' && (item.key !== 'RESTORE_LOGIN' || gen1));
    openModal(tr(cfg.title), h('div', {},
      h('p', {}, tr('restore.msg.' + item.key)),
      showName ? h('div', { class: 'field' }, h('label', { for: 'rsName' }, t(cfg.user === 'ssid' ? 'restore.ssid' : 'login.user')), name) : null,
      h('div', { class: 'field' }, h('label', { for: 'rsPass' }, t('login.password')), pass),
      cfg.confirm ? h('div', { class: 'field' }, h('label', { for: 'rsPass2' }, t('restore.confirmPassword')), pass2) : null,
      err), [
      { label: t('restore.skip') },
      { label: t('common.ok'), kind: 'primary', onClick: () => {
        if (cfg.confirm && pass.value !== pass2.value) { err.textContent = t('restore.passwordMismatch'); return false; }
        answer = pass.value;
      } },
    ], () => resolve(answer));
  });
}

function fileToBase64(f: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const r = new FileReader();
    r.onload = () => resolve(String(r.result).replace(/^data:[^,]*,/, ''));
    r.onerror = () => reject(r.error);
    r.readAsDataURL(f);
  });
}

function backupLabel(b: BackupFile, withHost: boolean): string {
  const when = dateTime(new Date(b.time), true);
  return withHost ? `${b.hostname} — ${when}` : when;
}

// ---- restore of one device ------------------------------------------------------------

/** Pick the backup to restore: one of this device, one of another device, or a file. */
async function pickSource(d: Device): Promise<RestoreSource | null> {
  let all: BackupFile[] = [];
  try { all = await backupApi.list(); } catch (e) { toast(errText(e)); return null; }
  const own = all.filter((b) => b.deviceId === d.id);
  const others = all.filter((b) => b.deviceId !== d.id);
  return new Promise((resolve) => {
    let result: RestoreSource | null = null;
    const radio = (value: string, checked: boolean): HTMLInputElement => h('input', { type: 'radio', name: 'rsSrc', value, checked });
    const rOwn = radio('own', own.length > 0), rOther = radio('other', own.length === 0 && others.length > 0), rFile = radio('file', own.length === 0 && others.length === 0);
    const selOwn = h('select', { 'aria-label': t('restore.src.own') }, ...own.map((b, i) => h('option', { value: i }, backupLabel(b, false))));
    const selOther = h('select', { 'aria-label': t('restore.src.other') }, ...others.map((b, i) => h('option', { value: i }, backupLabel(b, true))));
    const file = h('input', { type: 'file', accept: '.sbk', 'aria-label': t('restore.src.file') });
    const dl = h('a', { class: 'btn', download: '' }, t('restore.download'));
    const syncLink = (): void => {
      const b = rOwn.checked ? own[Number(selOwn.value)] : rOther.checked ? others[Number(selOther.value)] : undefined;
      if (b) { dl.setAttribute('href', backupApi.downloadURL(b)); dl.classList.remove('hidden'); } else dl.classList.add('hidden');
    };
    for (const el of [rOwn, rOther, rFile, selOwn, selOther]) el.addEventListener('change', syncLink);
    selOwn.addEventListener('focus', () => { rOwn.checked = true; syncLink(); });
    selOther.addEventListener('focus', () => { rOther.checked = true; syncLink(); });
    file.addEventListener('change', () => { rFile.checked = true; syncLink(); });
    if (!own.length) { rOwn.disabled = true; selOwn.disabled = true; }
    if (!others.length) { rOther.disabled = true; selOther.disabled = true; }
    const err = h('p', { class: 'muted', role: 'alert' });
    syncLink();
    openModal(t('restore.title', { host: hostOf(d) }), h('div', { class: 'restore-src' },
      h('label', { class: 'row' }, rOwn, t('restore.src.own')), own.length ? selOwn : h('p', { class: 'muted' }, t('restore.src.none')),
      h('label', { class: 'row' }, rOther, t('restore.src.other')), others.length ? selOther : null,
      h('label', { class: 'row' }, rFile, t('restore.src.file')), file,
      h('div', { class: 'row' }, dl), err), [
      { label: t('common.cancel') },
      { label: t('restore.next'), kind: 'primary', onClick: async () => {
        if (rOwn.checked && own.length) { const b = own[Number(selOwn.value)]!; result = { deviceId: b.deviceId, file: b.name }; return; }
        if (rOther.checked && others.length) { const b = others[Number(selOther.value)]!; result = { deviceId: b.deviceId, file: b.name }; return; }
        const f = file.files?.[0];
        if (!f) { err.textContent = t('restore.src.pick'); return false; }
        result = { upload: await fileToBase64(f) };
      } },
    ], () => resolve(result), 'wide');
  });
}

/** RestoreAction.restoreDevice for one device, interactive. */
export async function restoreDevice(d: Device): Promise<void> {
  const source = await pickSource(d);
  if (!source) return;
  const title = t('restore.titleShort') + ' - ' + hostOf(d);
  let plan;
  try { plan = await backupApi.check(d.id, source); } catch (e) { toast(errText(e)); return; }
  const p = planOf(plan.items);
  for (const pre of p.pre) {
    const m = itemMessage(pre, d.typeId, tr);
    if ((await ask(title, tr(m.key, m.vars), [{ label: t('common.no') }, { label: t('common.yes'), kind: 'primary' }])) !== 1) return;
  }
  if (p.error) {
    const m = itemMessage(p.error, d.typeId, tr);
    await ask(title, p.error.key === 'ERR_RESTORE_MSG' ? p.error.value ?? '' : tr(m.key, m.vars), [{ label: t('common.close') }]);
    return;
  }
  if (p.warn.length) await ask(title, p.warn.map((w) => tr('restore.msg.' + w.key)).join('\n\n'), [{ label: t('common.ok'), kind: 'primary' }]);

  const answers: Record<string, string> = {};
  for (const item of p.passwords) {
    const pwd = await passwordDialog(item, d.gen === '1');
    if (pwd !== null) answers[item.key] = pwd;
  }
  let choice: ScriptChoice | null = null;
  if (p.override) {
    const i = await ask(title, tr('restore.msg.' + p.override.key, { value: p.override.value ?? '' }),
      [{ label: t('restore.rename'), kind: 'primary' }, { label: t('restore.skip') }, { label: t('restore.overwrite') }]);
    choice = i === 2 ? 'overwrite' : i === 1 ? 'skip' : 'rename';
  }
  const sa = scriptAnswers(p, choice);
  Object.assign(answers, sa.answers);
  if (sa.enableAsked && p.enable &&
    (await ask(title, tr('restore.msg.' + Q_ENABLE, { value: p.enable.value ?? '' }), [{ label: t('common.no') }, { label: t('common.yes'), kind: 'primary' }])) === 1) {
    answers[Q_ENABLE] = 'true';
  }
  if (!(await confirmDialog(title, t(plan.queue ? 'restore.confirmQueued' : 'restore.confirm', { host: hostOf(d) }), t('restore.action')))) return;

  toast(t('restore.running', { host: hostOf(d) }), 'info');
  try {
    const res = await backupApi.restore(d.id, source, answers);
    if (res.result === 'queued') {
      await ask(hostOf(d), t('restore.queued'), [{ label: t('common.ok'), kind: 'primary' }]);
    } else if (res.result === 'fail') {
      await ask(title, (res.problems ?? []).map((x) => problemText(x, tr)).join('\n'), [{ label: t('common.close') }]);
    } else if (res.reboot) {
      if ((await ask(title, t('restore.successReboot'), [{ label: t('common.ok'), kind: 'primary' }, { label: t('action.reboot') }])) === 1) {
        await devicesApi.reboot([d.id]).catch((e) => toast(errText(e)));
      }
    } else {
      showResults(t('restore.done'), [{ id: d.id, name: hostOf(d), result: 'ok' }]);
    }
  } catch (e) {
    toast(errText(e));
  }
}

// ---- restore of several devices ---------------------------------------------------------

/**
 * Multiple restore: every device from its newest backup, with the
 * questions of the original (other host, warnings) but no passwords;
 * scripts are overwritten and enabled like the backup.
 */
export async function restoreDevices(list: Device[]): Promise<void> {
  if (!(await confirmDialog(t('restore.titleShort'), t('restore.multiConfirm', { n: list.length }), t('restore.action')))) return;
  let all: BackupFile[] = [];
  try { all = await backupApi.list(); } catch (e) { toast(errText(e)); return; }
  const lines: ResultLine[] = [];
  for (const d of list) {
    const name = hostOf(d);
    const line = (result: ResultLine['result'], message?: string): void => { lines.push({ id: d.id, name, result, message }); };
    const newest = all.find((b) => b.deviceId === d.id); // the list is newest first
    if (!newest) { line('fail', t('restore.noBackup')); continue; }
    const source = { deviceId: d.id, file: newest.name };
    const title = t('restore.titleShort') + ' - ' + name;
    try {
      const p = planOf((await backupApi.check(d.id, source)).items);
      let cancelled = false;
      for (const pre of p.pre) {
        const m = itemMessage(pre, d.typeId, tr);
        if ((await ask(title, tr(m.key, m.vars), [{ label: t('common.no') }, { label: t('common.yes'), kind: 'primary' }])) !== 1) { cancelled = true; break; }
      }
      if (cancelled) { line('cancel'); continue; }
      if (p.error) {
        const m = itemMessage(p.error, d.typeId, tr);
        line('fail', shortReason(p.error.key === 'ERR_RESTORE_MSG' ? p.error.value ?? '' : tr(m.key, m.vars)));
        continue;
      }
      if (p.warn.length) await ask(title, p.warn.map((w) => tr('restore.msg.' + w.key)).join('\n\n'), [{ label: t('common.ok'), kind: 'primary' }]);
      const res = await backupApi.restore(d.id, source, MULTI_ANSWERS);
      if (res.result === 'fail') line('fail', shortReason((res.problems ?? []).map((x) => problemText(x, tr)).join('\n')));
      else line(res.result);
    } catch (e) {
      line('fail', shortReason(errText(e)));
    }
  }
  showResults(t('restore.done'), lines);
}
