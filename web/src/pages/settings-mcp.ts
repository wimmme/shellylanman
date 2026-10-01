// Settings → MCP: the Model Context Protocol server for AI assistants
// (internal/mcp, DECISIONS.md §18). Not in ShellyScanner; asked for by Wim.
import { api, type MCPInfo } from '../api';
import { h } from '../dom';
import { t } from '../i18n';
import { confirmDialog } from '../modal';
import { toast } from '../toast';

function copyRow(text: string, label: string): HTMLElement {
  const pre = h('pre', { class: 'json', 'aria-label': label }, text);
  const btn = h('button', { class: 'btn', onclick: async () => {
    try { await navigator.clipboard.writeText(text); toast(t('mcp.copied'), 'info'); } catch { /* select by hand */ }
  } }, t('common.copy'));
  return h('div', { class: 'mcp-snippet' }, h('div', { class: 'row' }, h('strong', {}, label), btn), pre);
}

export async function mcpSettings(body: HTMLElement): Promise<void> {
  let info = await api.mcp();
  const url = `${location.origin}/mcp`;
  const box = h('div');

  const draw = (token?: string): void => {
    const enabled = h('input', { type: 'checkbox', id: 'mcpOn', checked: info.enabled });
    enabled.addEventListener('change', async () => {
      const next = await api.setMCP({ enabled: enabled.checked });
      info = next;
      draw(next.token);
    });
    const access = h('select', { id: 'mcpAccess' },
      h('option', { value: 'read' }, t('mcp.access.read')), h('option', { value: 'control' }, t('mcp.access.control')),
      h('option', { value: 'configure' }, t('mcp.access.configure')));
    access.value = info.access;
    access.disabled = !info.enabled;
    access.addEventListener('change', async () => { info = await api.setMCP({ access: access.value as MCPInfo['access'] }); toast(t('settings.saved'), 'info'); });

    const newToken = h('button', { class: 'btn', disabled: !info.enabled, onclick: async () => {
      if (info.hasToken && !(await confirmDialog(t('mcp.tokenNew'), t('mcp.tokenConfirm'), t('mcp.tokenNew')))) return;
      info = await api.newMCPToken();
      draw(info.token);
    } }, t('mcp.tokenNew'));

    const tokenPart: HTMLElement[] = token
      ? [h('div', { class: 'banner warn', role: 'status' }, t('mcp.tokenShown')),
        copyRow(token, t('mcp.token')),
        copyRow(`claude mcp add --transport http shellylanman ${url} --header "Authorization: Bearer ${token}"`, t('mcp.claude')),
        copyRow(JSON.stringify({ mcpServers: { shellylanman: { type: 'http', url, headers: { Authorization: `Bearer ${token}` } } } }, null, 2), t('mcp.json'))]
      : info.hasToken ? [h('p', { class: 'muted' }, t('mcp.tokenSet'))] : [];

    box.replaceChildren(
      h('p', {}, t('mcp.intro')),
      h('label', { class: 'row' }, enabled, t('mcp.enable')),
      h('div', { class: 'field' }, h('label', { for: 'mcpAccess' }, t('mcp.access')), access),
      h('p', { class: 'muted' }, t('mcp.accessHelp')),
      h('div', { class: 'field' }, h('label', {}, t('mcp.url')), h('input', { readonly: true, value: url, 'aria-label': t('mcp.url') })),
      h('div', { class: 'row' }, newToken),
      ...tokenPart,
      h('p', { class: 'muted' }, t('mcp.warn')),
    );
  };
  draw();
  body.append(box);
}
