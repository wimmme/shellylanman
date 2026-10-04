// Portions derived from ShellyScanner (https://github.com/usnasoft/shellyscanner),
// Copyright (C) Antonio Flaccomio / usnasoft, licensed under GPL-3.0:
// view/scripts/ide/ScriptFrame.
//
// The script editor window: open a file (.js, or a script from a .sbk), save
// to a file, upload to the device, run/stop, upload and run, undo/redo, find,
// go to line, caret position, help, and the script's log (the device's debug
// websocket, "info" lines of this script only).
import { ApiError, scriptsApi, wsURL, type Device, type ScriptInfo } from '../api';
import { download } from '../csv';
import { h } from '../dom';
import { t } from '../i18n';
import { currentTheme } from '../appearance';
import type { Editor } from '../editor/codemirror';
import { editorDark, ideprefs } from '../ideprefs';
import { openModal } from '../modal';
import { toast } from '../toast';

const msg = (e: unknown): string => (e instanceof ApiError || e instanceof Error ? e.message : String(e));

export function readFileText(buf: Uint8Array): string {
  return new TextDecoder().decode(buf).replace(/\r+\n/g, '\n');
}

function toBase64(buf: Uint8Array): string {
  let s = '';
  for (let i = 0; i < buf.length; i += 0x8000) s += String.fromCharCode(...buf.subarray(i, i + 0x8000));
  return btoa(s);
}

/** Choose one script of a backup file (scrSelectionMsg); null when cancelled or none. */
export async function pickBackupScript(buf: Uint8Array): Promise<string | null> {
  let list;
  try { list = await scriptsApi.backupScripts(toBase64(buf)); } catch (e) { toast(msg(e)); return null; }
  if (list.length === 0) { toast(t('scr.noneInBackup')); return null; }
  return new Promise((resolve) => {
    let code: string | null = null;
    const sel = h('select', { id: 'scrPick' }, ...list.map((s, i) => h('option', { value: i }, s.name)));
    openModal(t('scr.pickTitle'), h('div', { class: 'field' }, h('label', { for: 'scrPick' }, t('scr.pickText')), sel), [
      { label: t('common.cancel') },
      { label: t('common.ok'), kind: 'primary', onClick: () => { code = list[Number(sel.value)]!.code; } },
    ], () => resolve(code));
  });
}

/** Open the editor; resolves when it is closed. onRun reports the running state. */
export async function openScriptEditor(d: Device, s: ScriptInfo, onRun: (running: boolean) => void): Promise<void> {
  // The editor window opens at once and says it is reading: a device on weak
  // Wi-Fi can take seconds to send its code. Everything that needs the code
  // (and above all the uploads) stays off until it has arrived; when reading
  // fails the window says so and offers Retry, never an empty editor whose
  // upload would wipe the script on the device.
  const prefs = ideprefs();
  const dark = editorDark(prefs, currentTheme());
  const host = h('div', { class: 'ide-editor' + (dark ? ' dark' : '') });
  const caret = h('span', { class: 'muted ide-caret' }, '1 : 1');
  const logs = h('pre', { class: 'ide-log' + (dark ? ' dark' : ''), 'aria-live': 'polite' });
  let editor: Editor | null = null;
  let closed = false;
  const needCode: HTMLButtonElement[] = [];
  const load = async (): Promise<void> => {
    host.replaceChildren(h('div', { class: 'ide-state', role: 'status' }, h('div', { class: 'spinner' }), t('scr.loadingCode')));
    try {
      const [code, { createEditor }] = await Promise.all([scriptsApi.code(d.id, s.id), import('../editor/codemirror')]);
      if (closed) return;
      host.replaceChildren();
      editor = createEditor(host, code, prefs, dark, (l, c) => { caret.textContent = `${l} : ${c}`; });
      for (const b of needCode) b.disabled = false;
      status(running); // the uploads also depend on the running state
      editor.focus();
    } catch (e) {
      if (closed) return;
      host.replaceChildren(h('div', { class: 'ide-state error', role: 'alert' },
        h('strong', {}, t('scr.loadFailed')), h('span', { class: 'muted' }, msg(e)),
        h('button', { class: 'btn', onclick: () => void load() }, t('scr.retry'))));
      toast(t('scr.loadFailed'));
    }
  };
  let fileName = s.name.endsWith('.js') ? s.name : s.name + '.js';
  let running = s.running;
  let ws: WebSocket | null = null;

  const log = (line: string): void => {
    logs.append(line + '\n');
    logs.scrollTop = logs.scrollHeight;
  };
  const connectLog = async (): Promise<void> => { // LogWebSocketDeviceListener: level 2 (info), fd = 100 + script id
    if (ws) return;
    await scriptsApi.logOn(d.id).catch(() => { /* the connection will report it */ });
    if (ws || !running) return;
    ws = new WebSocket(wsURL(`ws/log/${encodeURIComponent(d.id)}`));
    ws.onmessage = (m) => {
      try {
        const j = JSON.parse(String(m.data)) as { level?: number; fd?: number; data?: string; error?: string };
        if (j.error) log('ShellyLanMan error: ' + j.error);
        else if (j.level === 2 && (j.fd ?? 100 + s.id) === 100 + s.id) log((j.data ?? '').replace(/\n$/, ''));
      } catch { /* not JSON */ }
    };
    ws.onclose = () => { ws = null; };
  };
  const disconnectLog = (): void => { ws?.close(); ws = null; };

  const runBtn = h('button', { class: 'btn' });
  const upBtn = h('button', { class: 'btn', title: t('scr.uploadDeviceTip') }, t('scr.uploadDevice'));
  const upRunBtn = h('button', { class: 'btn' }, t('scr.uploadRun'));
  const status = (r: boolean): void => { // runningStatus
    running = r;
    runBtn.textContent = r ? '■ ' + t('scr.stop') : '▶ ' + t('scr.run');
    runBtn.title = t(r ? 'scr.runningTip' : 'scr.stoppedTip');
    upBtn.disabled = upRunBtn.disabled = r || !editor;
    if (r) void connectLog(); else disconnectLog();
    onRun(r);
  };
  const upload = async (): Promise<boolean> => {
    if (!editor) return false; // the code was not read: never upload over the device's script
    try { await scriptsApi.putCode(d.id, s.id, editor.text()); return true; } catch (e) { toast(msg(e)); return false; }
  };
  const toggleRun = async (): Promise<void> => {
    try {
      if (running) { await scriptsApi.run(d.id, s.id, false, true); status(false); } else { status(true); await scriptsApi.run(d.id, s.id, true, true); }
    } catch (e) {
      toast(e instanceof ApiError && e.status >= 500 ? t('scr.exeError') : msg(e));
      status(false);
    }
  };
  runBtn.addEventListener('click', () => void toggleRun());
  upBtn.addEventListener('click', async () => { if (await upload()) toast(t('scr.uploaded'), 'info'); });
  upRunBtn.addEventListener('click', async () => { if (await upload()) await toggleRun(); });

  const file = h('input', { type: 'file', accept: '.js,.mjs,.sbk,.txt', class: 'hidden' });
  file.addEventListener('change', async () => {
    const f = file.files?.[0];
    file.value = '';
    if (!f) return;
    const buf = new Uint8Array(await f.arrayBuffer());
    const text = buf[0] === 0x50 && buf[1] === 0x4b ? await pickBackupScript(buf) : readFileText(buf);
    if (text === null || !editor) return;
    editor.setText(text);
    editor.focus();
    if (!f.name.endsWith('.sbk')) fileName = f.name;
  });
  const gotoLine = (): void => {
    const input = h('input', { id: 'ideLine', type: 'number', min: 1, size: 8 });
    openModal(t('scr.gotoTitle'), h('div', { class: 'field' }, h('label', { for: 'ideLine' }, t('scr.gotoLabel')), input), [
      { label: t('common.cancel') },
      { label: t('common.ok'), kind: 'primary', onClick: () => { const n = parseInt(input.value, 10); if (n > 0) editor?.gotoLine(n); } },
    ]);
  };
  const help = (): void => {
    openModal(t('scr.editorTitle'), h('div', { class: 'ide-help' }, ...t('scr.help').split('\n').map((l) => h('p', {}, l))), [{ label: t('common.close') }]);
  };
  // Buttons that need the code: off until it has been read (load()).
  const codeBtn = (label: string, onclick: () => void, title?: string): HTMLButtonElement => {
    const b = h('button', { class: 'btn', disabled: true, onclick, title });
    b.append(label);
    needCode.push(b);
    return b;
  };
  const save = (): void => { if (editor) download(fileName, editor.text(), 'text/javascript'); };
  const body = h('div', { class: 'ide' },
    h('div', { class: 'toolbar ide-toolbar' },
      codeBtn(t('scr.open'), () => file.click()),
      codeBtn(t('scr.save'), save, t('scr.saveTip')),
      upBtn, runBtn, upRunBtn,
      codeBtn(t('scr.undo'), () => editor?.undo(), 'Ctrl+Z'),
      codeBtn(t('scr.redo'), () => editor?.redo(), 'Ctrl+Y'),
      codeBtn(t('scr.find'), () => editor?.find(), 'Ctrl+F'),
      codeBtn(t('scr.goto'), gotoLine, 'Ctrl+G'),
      h('div', { class: 'spacer' }), caret,
      h('button', { class: 'btn', onclick: help, title: t('scr.helpTip') }, '?'), file),
    host,
    h('div', { class: 'ide-logbar' }, h('span', { class: 'muted' }, t('scr.log')), h('button', { class: 'btn small', onclick: () => { logs.textContent = ''; } }, t('scr.clear'))),
    logs);
  body.addEventListener('keydown', (e) => {
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'g' && editor) { e.preventDefault(); gotoLine(); }
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 's') { e.preventDefault(); save(); }
  });
  status(running);
  await new Promise<void>((resolve) => {
    openModal(`${t('scr.editorTitle')} - ${s.name}`, body, [{ label: t('common.close') }], () => {
      closed = true;
      disconnectLog();
      editor?.destroy();
      resolve();
    }, 'full');
    void load();
  });
}
