// Modal dialogs: a generic one and a confirmation that resolves to true/false.
// Escape and the backdrop cancel; focus moves into the dialog.
import { h } from './dom';
import { t } from './i18n';

export interface ModalButton {
  label: string;
  kind?: 'primary' | 'danger';
  /** Return false to keep the dialog open (e.g. validation failed). */
  onClick?: () => boolean | void | Promise<boolean | void>;
}

let seq = 0;

export function openModal(title: string, body: Node, buttons: ModalButton[], onClose?: () => void, size?: 'wide'): () => void {
  const id = `modal${++seq}`;
  const close = (): void => {
    back.remove();
    document.removeEventListener('keydown', onKey);
    onClose?.();
  };
  const onKey = (e: KeyboardEvent): void => {
    if (e.key === 'Escape') close();
  };
  const footer = h('footer', {}, ...buttons.map((b) => {
    const btn = h('button', { class: 'btn' + (b.kind ? ' ' + b.kind : '') }, b.label);
    btn.addEventListener('click', async () => {
      btn.setAttribute('disabled', '');
      try {
        if ((await b.onClick?.()) !== false) close();
      } finally {
        btn.removeAttribute('disabled');
      }
    });
    return btn;
  }));
  const dialog = h('div', { class: size === 'wide' ? 'modal wide' : 'modal', role: 'dialog', 'aria-modal': 'true', 'aria-labelledby': id },
    h('header', { id }, title), h('div', { class: 'body' }, body), footer);
  const back = h('div', { class: 'modal-back' }, dialog);
  back.addEventListener('mousedown', (e) => { if (e.target === back) close(); });
  document.addEventListener('keydown', onKey);
  document.body.append(back);
  (dialog.querySelector('input,select,textarea,button') as HTMLElement | null)?.focus();
  return close;
}

/** Ask before something destructive; the confirm button carries the action's name. */
export function confirmDialog(title: string, message: string, action: string, danger = true): Promise<boolean> {
  return new Promise((resolve) => {
    let answered = false;
    openModal(title, h('p', {}, message), [
      { label: t('common.cancel') },
      { label: action, kind: danger ? 'danger' : 'primary', onClick: () => { answered = true; } },
    ], () => resolve(answered));
  });
}
