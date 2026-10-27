// Copy text. The Clipboard API needs a secure context (https or localhost); ShellyLanMan is
// mostly opened over plain http on the LAN, where the old way still works.
import { h } from './dom';

export async function copyText(text: string): Promise<boolean> {
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(text);
      return true;
    }
  } catch { /* the old way */ }
  const area = h('textarea', { class: 'offscreen', readonly: true, 'aria-hidden': 'true' });
  area.value = text;
  document.body.append(area);
  area.select();
  let ok = false;
  try { ok = document.execCommand('copy'); } catch { /* not allowed */ }
  area.remove();
  return ok;
}
