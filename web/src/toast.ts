// Short notices at the bottom of the screen (errors of device commands).
import { h } from './dom';

let box: HTMLElement | null = null;

export function toast(message: string, kind: 'error' | 'info' = 'error'): void {
  if (!box || !box.isConnected) {
    box = h('div', { class: 'toasts', role: 'status', 'aria-live': 'polite' });
    document.body.append(box);
  }
  const item = h('div', { class: 'toast ' + kind }, message);
  box.append(item);
  setTimeout(() => item.remove(), kind === 'error' ? 6000 : 3000);
}
