// Portions derived from ShellyScanner (https://github.com/usnasoft/shellyscanner),
// Copyright (C) Antonio Flaccomio / usnasoft, licensed under GPL-3.0:
// view/scripts/DialogDeviceScripts, ScriptsPanel and KVSPanel.
//
// "Scripts" of a Gen2+ device: a Scripts tab (name, enabled, run/stop; new,
// delete, download, upload from a .js or a .sbk, edit) and a KVS tab (key,
// etag, value; new, edit, delete). A tab the device does not offer is left out.
import { ApiError, scriptsApi, type Device, type KVItem, type ScriptInfo } from '../api';
import { h } from '../dom';
import { t, type Key } from '../i18n';
import { confirmDialog, openModal } from '../modal';
import { toast } from '../toast';
import { download } from '../csv';
import { openScriptEditor, pickBackupScript, readFileText } from './scripteditor';

const msg = (e: unknown): string => (e instanceof ApiError || e instanceof Error ? e.message : String(e));

/** The code of a file: a .js as text, a .sbk by choosing one of its scripts. */
export async function codeFromFile(f: File): Promise<string | null> {
  const buf = new Uint8Array(await f.arrayBuffer());
  if (buf[0] === 0x50 && buf[1] === 0x4b) return pickBackupScript(buf); // "PK": a zip (backup)
  return readFileText(buf);
}

export async function openScripts(d: Device): Promise<void> {
  let view;
  try { view = await scriptsApi.list(d.id); } catch (e) { toast(e instanceof ApiError && e.status === 409 ? t('scr.notSupported') : msg(e)); return; }
  const title = t('scr.title', { device: d.name && d.name !== d.hostname ? `${d.name} (${d.hostname})` : d.hostname });
  const tabs: { label: Key; body: HTMLElement }[] = [];
  if (view.scripts) tabs.push({ label: 'scr.tab.scripts', body: scriptsTab(d, view.scripts) });
  if (view.kvs) tabs.push({ label: 'scr.tab.kvs', body: kvsTab(d, view.kvs) });
  const bar = h('div', { class: 'tabs', role: 'tablist' });
  const content = h('div', { class: 'scr-body' });
  const show = (i: number): void => {
    bar.replaceChildren(...tabs.map((x, j) => h('button', { class: 'tab' + (i === j ? ' active' : ''), role: 'tab', 'aria-selected': String(i === j), onclick: () => show(j) }, t(x.label))));
    content.replaceChildren(tabs[i]!.body);
  };
  show(0);
  openModal(title, h('div', {}, bar, content), [{ label: t('common.close') }], undefined, 'wide');
}

// ---- Scripts tab ------------------------------------------------------------------------

function scriptsTab(d: Device, initial: ScriptInfo[]): HTMLElement {
  let list = initial;
  let sel: number | null = null;
  const editing = new Set<number>(); // scripts with an open editor: name and run button locked
  const tbody = h('tbody');
  const btns = {
    del: h('button', { class: 'btn' }, t('scr.delete')),
    add: h('button', { class: 'btn' }, t('scr.new')),
    down: h('button', { class: 'btn' }, t('scr.download')),
    up: h('button', { class: 'btn', title: t('scr.uploadTip') }, t('scr.upload')),
    edit: h('button', { class: 'btn' }, t('scr.edit')),
  };
  const file = h('input', { type: 'file', accept: '.js,.mjs,.sbk,.txt', class: 'hidden' });
  const current = (): ScriptInfo | undefined => list.find((s) => s.id === sel);

  const draw = (): void => {
    tbody.replaceChildren(...list.map((s) => {
      const name = h('input', { class: 'scr-name', value: s.name, 'aria-label': t('scr.col.name'), disabled: editing.has(s.id) });
      name.addEventListener('change', async () => {
        try { await scriptsApi.update(d.id, s.id, { name: name.value }); s.name = name.value; } catch (e) { toast(msg(e)); name.value = s.name; }
      });
      const en = h('input', { type: 'checkbox', checked: s.enable, 'aria-label': t('scr.col.enabled') });
      en.addEventListener('change', async () => {
        try { await scriptsApi.update(d.id, s.id, { enable: en.checked }); s.enable = en.checked; } catch (e) { toast(msg(e)); en.checked = s.enable; }
      });
      const run = h('button', { class: 'btn small', disabled: editing.has(s.id), title: t(s.running ? 'scr.stop' : 'scr.run') }, s.running ? '■' : '▶');
      run.addEventListener('click', async (ev) => {
        ev.stopPropagation();
        try { await scriptsApi.run(d.id, s.id, !s.running, false); s.running = !s.running; draw(); } catch (e) { toast(msg(e)); }
      });
      const tr = h('tr', { class: s.id === sel ? 'selected' : '' }, h('td', {}, name), h('td', {}, en), h('td', {}, run));
      tr.addEventListener('click', () => { sel = s.id; draw(); });
      tr.addEventListener('dblclick', () => { sel = s.id; void edit(); });
      return tr;
    }));
    const none = sel === null || !current();
    btns.del.disabled = btns.down.disabled = btns.up.disabled = none;
    btns.edit.disabled = none || editing.has(sel!); // already open: one editor per script
  };

  const edit = async (): Promise<void> => {
    const s = current();
    if (!s || editing.has(s.id)) return;
    editing.add(s.id);
    draw();
    await openScriptEditor(d, s, (running) => { s.running = running; draw(); });
    editing.delete(s.id);
    draw();
  };

  btns.del.addEventListener('click', async () => {
    const s = current();
    if (!s || !(await confirmDialog(t('scr.delete'), t('scr.deleteConfirm'), t('scr.delete')))) return;
    try { await scriptsApi.remove(d.id, s.id); list = list.filter((x) => x !== s); sel = null; draw(); } catch (e) { toast(msg(e)); }
  });
  btns.add.addEventListener('click', async () => {
    try { const s = await scriptsApi.create(d.id); list = [...list, s]; sel = s.id; draw(); } catch (e) { toast(msg(e)); }
  });
  btns.down.addEventListener('click', async () => {
    const s = current();
    if (!s) return;
    try {
      const code = await scriptsApi.code(d.id, s.id);
      download(s.name.endsWith('.js') ? s.name : s.name + '.js', code, 'text/javascript');
    } catch { toast(t('scr.noCode')); }
  });
  btns.up.addEventListener('click', () => file.click());
  file.addEventListener('change', async () => {
    const s = current(), f = file.files?.[0];
    file.value = '';
    if (!s || !f) return;
    try {
      const code = await codeFromFile(f);
      if (code === null) return;
      await scriptsApi.putCode(d.id, s.id, code);
      toast(t('scr.uploaded'), 'info');
    } catch (e) { toast(msg(e)); }
  });
  btns.edit.addEventListener('click', () => void edit());
  draw();
  return h('div', { class: 'scr-tab' },
    h('div', { class: 'table-wrap' }, h('table', { class: 'data scr' },
      h('thead', {}, h('tr', {}, ...(['scr.col.name', 'scr.col.enabled', 'scr.col.running'] as Key[]).map((k) => h('th', { scope: 'col' }, t(k))))), tbody)),
    h('div', { class: 'row scr-buttons' }, btns.del, btns.add, btns.down, btns.up, btns.edit, file));
}

// ---- KVS tab ------------------------------------------------------------------------------

function kvsTab(d: Device, initial: KVItem[]): HTMLElement {
  let list = initial;
  let sel: string | null = null;
  const tbody = h('tbody');
  const del = h('button', { class: 'btn' }, t('scr.delete'));
  const add = h('button', { class: 'btn' }, t('scr.new'));

  const draw = (): void => {
    tbody.replaceChildren(...list.map((it) => {
      const value = h('input', { class: 'kvs-value', value: it.value, 'aria-label': t('kvs.col.value') });
      value.addEventListener('change', async () => {
        if (value.value === it.value) return;
        try { const n = await scriptsApi.kvsSet(d.id, it.key, value.value); Object.assign(it, n); draw(); } catch (e) { toast(msg(e)); value.value = it.value; }
      });
      const tr = h('tr', { class: it.key === sel ? 'selected' : '' }, h('td', {}, it.key), h('td', { class: 'muted' }, it.etag), h('td', {}, value));
      tr.addEventListener('click', () => { if (sel !== it.key) { sel = it.key; draw(); } });
      return tr;
    }));
    del.disabled = sel === null;
  };
  del.addEventListener('click', async () => {
    if (sel === null || !(await confirmDialog(t('scr.delete'), t('scr.deleteConfirm'), t('scr.delete')))) return;
    try { await scriptsApi.kvsDelete(d.id, sel); list = list.filter((x) => x.key !== sel); sel = null; draw(); } catch (e) { toast(msg(e)); }
  });
  add.addEventListener('click', () => {
    const key = h('input', { id: 'kvsKey', value: t('kvs.col.key').toLowerCase() });
    const err = h('p', { class: 'muted', role: 'alert' });
    openModal(t('kvs.new'), h('div', {}, h('div', { class: 'field' }, h('label', { for: 'kvsKey' }, t('kvs.col.key')), key), err), [
      { label: t('common.cancel') },
      { label: t('common.ok'), kind: 'primary', onClick: async () => {
        const k = key.value;
        if (!k) return false;
        if (list.some((x) => x.key === k)) { err.textContent = t('kvs.exists'); return false; }
        try { const n = await scriptsApi.kvsSet(d.id, k, ''); list = [...list, n]; sel = k; draw(); return true; } catch (e) { err.textContent = msg(e); return false; }
      } },
    ]);
    key.select();
  });
  draw();
  return h('div', { class: 'scr-tab' },
    h('div', { class: 'table-wrap' }, h('table', { class: 'data kvs' },
      h('thead', {}, h('tr', {}, ...(['kvs.col.key', 'kvs.col.etag', 'kvs.col.value'] as Key[]).map((k) => h('th', { scope: 'col' }, t(k))))), tbody)),
    h('div', { class: 'row scr-buttons' }, del, add));
}
