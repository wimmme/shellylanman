import { api } from '../api';
import {
  APPEAR_DEFAULT, FONTS, FONT_SIZES, PALETTES, SLIDER_MAX, applyFont, applyFontSize, applyLevel, applyPalette,
  applyTheme, currentPalette, currentTheme, resetAppearance, storedFont, storedFontSize,
} from '../appearance';
import { h, ICONS } from '../dom';
import { LANGUAGES, isLang, lang, setLang, t, type Key } from '../i18n';
import { card, type Page } from './common';

export function settingsPage(onLanguageChange: () => void): Page {
  let tab: 'general' | 'appearance' = 'general';
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
        else appearance(body);
      };
      for (const [id, key] of [['general', 'settings.tab.general'], ['appearance', 'settings.tab.appearance']] as const) {
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

async function general(body: HTMLElement, onLanguageChange: () => void): Promise<void> {
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
