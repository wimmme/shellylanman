import { CHART_TYPES } from '../chartlogic';
import { ideprefs, setIdeprefs, type Indent } from '../ideprefs';
import { api } from '../api';
import {
  APPEAR_DEFAULT, FONTS, FONT_SIZES, PALETTES, SLIDER_MAX, applyFont, applyFontSize, applyLevel, applyPalette,
  applyTheme, currentPalette, currentTheme, resetAppearance, storedFont, storedFontSize,
} from '../appearance';
import { h, ICONS } from '../dom';
import { prefs, type DblClick, type TempUnit, type UptimeMode } from '../format';
import { LANGUAGES, isLang, lang, setLang, t, type Key } from '../i18n';
import { card, type Page } from './common';
import { archive, network } from './settings-network';

export function settingsPage(onLanguageChange: () => void): Page {
  let tab: 'general' | 'network' | 'archive' | 'ide' | 'appearance' = 'general';
  return {
    id: 'settings',
    title: 'nav.settings',
    icon: ICONS.settings,
    async render(main) {
      const body = h('div');
      const tabs = h('div', { class: 'tabs', role: 'tablist' });
      const show = (which: typeof tab): void => {
        tab = which;
        tabs.querySelectorAll('button').forEach((b) => b.setAttribute('aria-selected', String(b.dataset.tab === which)));
        body.replaceChildren();
        if (which === 'general') void general(body, onLanguageChange);
        else if (which === 'network') void network(body);
        else if (which === 'archive') void archive(body);
        else if (which === 'ide') ide(body);
        else appearance(body);
      };
      for (const [id, key] of [['general', 'settings.tab.general'], ['network', 'settings.tab.network'], ['archive', 'settings.tab.archive'], ['ide', 'settings.tab.ide'], ['appearance', 'settings.tab.appearance']] as const) {
        const b = h('button', { role: 'tab', 'data-tab': id, onclick: () => show(id) }, t(key));
        tabs.append(b);
      }
      main.append(card(t('nav.settings'), null, tabs, body));
      show(tab);
    },
  };
}

function select(id: string, options: { value: string; label: string }[], value: string, onChange: (v: string) => void): HTMLSelectElement {
  const s = h('select', { id, onchange: () => onChange(s.value) }, ...options.map((o) => h('option', { value: o.value }, o.label)));
  s.value = value;
  return s;
}

function field(label: string, control: HTMLElement): HTMLElement {
  return h('div', { class: 'field' }, h('label', { for: control.id }, label), control);
}

function csvInput(): HTMLInputElement {
  const i = h('input', { id: 'prefCsv', size: 3, maxlength: 3, value: prefs.csvSeparator() });
  i.addEventListener('change', () => prefs.setCsvSeparator(i.value));
  return i;
}

async function general(body: HTMLElement, onLanguageChange: () => void): Promise<void> {
  // ShellyScanner's General tab display options, kept per browser here.
  body.append(
    field(t('prefs.uptime'), select('prefUptime', [
      { value: 'SEC', label: t('prefs.uptime.SEC') }, { value: 'DAY', label: t('prefs.uptime.DAY') }, { value: 'FROM', label: t('prefs.uptime.FROM') },
    ], prefs.uptime(), (v) => prefs.setUptime(v as UptimeMode))),
    field(t('prefs.temp'), select('prefTemp', [{ value: 'C', label: t('prefs.temp.C') }, { value: 'F', label: t('prefs.temp.F') }],
      prefs.temp(), (v) => prefs.setTemp(v as TempUnit))),
    field(t('prefs.dblclick'), select('prefDbl', [{ value: 'DET', label: t('prefs.dblclick.DET') }, { value: 'WEB', label: t('prefs.dblclick.WEB') }],
      prefs.dblClick(), (v) => prefs.setDblClick(v as DblClick))),
    field(t('prefs.filter'), select('prefFilter', [
      { value: '0', label: t('filter.all') }, { value: '1', label: t('col.type') }, { value: '2', label: t('col.device') },
      { value: '3', label: t('col.name') }, { value: '4', label: t('col.keyword') },
    ], String(prefs.defaultFilter()), (v) => prefs.setDefaultFilter(Number(v)))),
    field(t('prefs.csv'), csvInput()),
    field(t('prefs.chart'), select('prefChart', CHART_TYPES.map((c) => ({ value: c.type, label: t(('chart.t.' + c.type) as Key) })), prefs.chartDefault(), (v) => prefs.setChartDefault(v))),
    field(t('prefs.chartExport'), select('prefChartExp', [{ value: 'H', label: t('prefs.chartExport.H') }, { value: 'V', label: t('prefs.chartExport.V') }],
      prefs.chartExport(), (v) => prefs.setChartExport(v as 'H' | 'V'))),
    h('p', { class: 'muted' }, t('prefs.note')),
  );
  const upd = await api.settings();
  body.append(field(t('update.setting'), select('updCheck', [{ value: 'never', label: t('update.never') }, { value: 'stable', label: t('update.stable') }, { value: 'all', label: t('update.all') }],
    upd.updateCheck || 'never', (v) => void api.updateSettings({ updateCheck: v }).catch(() => {}))), h('p', { class: 'muted' }, t('update.help')));
  const langs = LANGUAGES.map((l) => ({ value: l.id, label: l.label }));
  body.append(field(t('settings.language.browser'), select('langBrowser', langs, lang(), (v) => {
    if (isLang(v)) {
      setLang(v, true);
      onLanguageChange();
    }
  })));
  const settings = await api.settings();
  const saved = h('span', { class: 'muted' });
  body.append(field(t('settings.language.default'), select('langDefault', langs, settings.language, async (v) => {
    await api.updateSettings({ language: v });
    saved.textContent = t('settings.saved');
  })), saved);
}

/** PanelIDE: script editor settings, per browser. */
function ide(body: HTMLElement): void {
  const p = ideprefs();
  const save = (): void => setIdeprefs(p);
  const num = (id: string, v: number, min: number, max: number, set: (n: number) => void): HTMLInputElement => {
    const i = h('input', { id, type: 'number', min, max, value: v });
    i.addEventListener('change', () => { const n = Number(i.value); if (n >= min && n <= max) { set(n); save(); } });
    return i;
  };
  const chk = (label: string, v: boolean, set: (b: boolean) => void): HTMLElement => {
    const c = h('input', { type: 'checkbox', checked: v });
    c.addEventListener('change', () => { set(c.checked); save(); });
    return h('label', { class: 'row' }, c, label);
  };
  body.append(
    field(t('ide.tab'), num('ideTab', p.tabSize, 1, 16, (n) => { p.tabSize = n; })),
    field(t('ide.font'), num('ideFont', p.fontSize, 8, 32, (n) => { p.fontSize = n; })),
    field(t('ide.indent'), select('ideIndent', [{ value: 'NO', label: t('ide.indent.NO') }, { value: 'STD', label: t('ide.indent.STD') }, { value: 'SMART', label: t('ide.indent.SMART') }],
      p.indent, (v) => { p.indent = v as Indent; save(); })),
    h('div', { class: 'field' }, h('span', { class: 'cfg-label' }, t('ide.close')),
      chk('{ }', p.closeCurly, (b) => { p.closeCurly = b; }), chk('( )', p.closeBracket, (b) => { p.closeBracket = b; }),
      chk('[ ]', p.closeSquare, (b) => { p.closeSquare = b; }), chk('" "', p.closeString, (b) => { p.closeString = b; })),
    chk(t('ide.dark'), p.dark, (b) => { p.dark = b; }),
    h('p', { class: 'muted' }, t('ide.note')),
  );
}

function appearance(body: HTMLElement): void {
  const swatches = h('div', { class: 'swatches' });
  const drawSwatches = (): void => {
    swatches.replaceChildren();
    for (const p of PALETTES) {
      for (const mode of ['dark', 'light'] as const) {
        if (mode === 'light' && !p.light) continue;
        const active = currentPalette() === p.id && currentTheme() === mode;
        swatches.append(h('button', {
          class: 'swatch' + (active ? ' active' : ''),
          'aria-pressed': String(active),
          onclick: () => { applyPalette(p.id, mode); drawSwatches(); },
        }, h('i', { style: `background:linear-gradient(135deg,${mode === 'light' ? '#eee' : p.swatch[0]} 55%,${p.swatch[1]} 55%)` }),
        `${p.label} · ${t(mode === 'light' ? 'appearance.light' : 'appearance.dark')}`));
      }
    }
  };
  drawSwatches();

  const themeBtns = h('div', { class: 'row' },
    ...(['dark', 'light'] as const).map((m) => h('button', { class: 'btn', onclick: () => { applyTheme(m); drawSwatches(); } },
      t(m === 'light' ? 'appearance.light' : 'appearance.dark'))));

  const slider = (id: string, key: Key, attr: 'data-contrast' | 'data-text-bright' | 'data-bg-bright'): HTMLElement => {
    const input = h('input', {
      id, type: 'range', min: 1, max: SLIDER_MAX, step: 1,
      value: document.documentElement.getAttribute(attr) || String(APPEAR_DEFAULT),
    });
    input.addEventListener('input', () => applyLevel(attr, Number(input.value)));
    return field(t(key), input);
  };

  body.append(
    h('div', { class: 'field' }, h('span', { class: 'muted' }, t('appearance.theme')), themeBtns),
    h('div', { class: 'muted' }, t('appearance.palette')), swatches,
    slider('apContrast', 'appearance.contrast', 'data-contrast'),
    slider('apTextBright', 'appearance.textBright', 'data-text-bright'),
    slider('apBgBright', 'appearance.bgBright', 'data-bg-bright'),
    field(t('appearance.font'), select('apFont', FONTS.map((f) => ({ value: f.id, label: f.label })), storedFont(), applyFont)),
    field(t('appearance.fontSize'), select('apFontSize', FONT_SIZES.map((s) => ({ value: s.id, label: t(`size.${s.id}`) })), storedFontSize(), applyFontSize)),
    h('p', { class: 'muted' }, t('appearance.note')),
    h('button', { class: 'btn', onclick: () => { resetAppearance(); body.replaceChildren(); appearance(body); } }, t('appearance.reset')),
  );
}
