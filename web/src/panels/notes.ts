// Portions derived from ShellyScanner (https://github.com/usnasoft/shellyscanner),
// Copyright (C) Antonio Flaccomio / usnasoft, licensed under GPL-3.0:
// view/NotesEditor.
//
// Notes and keyword of a device, kept in the archive. Cut/copy/paste, undo
// and find are the browser's own; Ctrl+S saves, Ctrl+K goes to the keyword.
import { ApiError, devicesApi, type Device } from '../api';
import { h } from '../dom';
import { t } from '../i18n';
import { openModal } from '../modal';

export const MAX_KEYWORD = 32;

export function openNotes(d: Device): void {
  const note = h('textarea', { id: 'ntNote', rows: 12, class: 'notes-text', 'aria-label': t('notes.note') });
  note.value = d.note ?? '';
  const keyword = h('input', { id: 'ntKey', maxlength: MAX_KEYWORD, size: MAX_KEYWORD, value: d.keyword ?? '' });
  // A relayed BLU device has no name of its own outside the Shelly Cloud: it is
  // named here, in ShellyLanMan's archive (DECISIONS P14-3).
  const name = d.relay ? h('input', { id: 'ntName', size: MAX_KEYWORD, value: d.name ?? '' }) : null;
  const err = h('p', { class: 'muted', role: 'alert' });
  let close = (): void => {};
  const save = async (): Promise<boolean> => {
    try {
      await devicesApi.setNote(d.id, note.value, keyword.value.slice(0, MAX_KEYWORD), name ? name.value : undefined);
      return true;
    } catch (e) {
      err.textContent = e instanceof ApiError ? e.message : String(e);
      return false;
    }
  };
  const onKey = (e: KeyboardEvent): void => {
    if (!(e.ctrlKey || e.metaKey)) return;
    const k = e.key.toLowerCase();
    if (k === 's') { e.preventDefault(); void save().then((ok) => { if (ok) close(); }); }
    if (k === 'k') { e.preventDefault(); keyword.focus(); }
  };
  const body = h('div', { class: 'notes' },
    name && h('div', { class: 'field row' }, h('label', { for: 'ntName' }, t('notes.name')), name),
    note,
    h('div', { class: 'field row' }, h('label', { for: 'ntKey' }, t('notes.keyword')), keyword), err);
  body.addEventListener('keydown', onKey);
  close = openModal(t('notes.title', { device: d.name || d.hostname }), body, [
    { label: t('common.save'), kind: 'primary', onClick: save },
    { label: t('common.close') },
  ], undefined, 'wide');
  note.focus();
}
