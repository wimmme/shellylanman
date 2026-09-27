import { api } from '../api';
import { h, ICONS } from '../dom';
import { t, type Key } from '../i18n';
import { card, type Page } from './common';

const link = (href: string, text: string): HTMLAnchorElement =>
  h('a', { href, target: '_blank', rel: 'noopener noreferrer' }, text);

export const aboutPage: Page = {
  id: 'about',
  title: 'nav.about',
  icon: ICONS.about,
  async render(main) {
    const a = await api.about();
    main.append(
      card(t('help.title'), null, ...(['devices', 'settings', 'checklist', 'firmware', 'backup', 'scheduler', 'scripts', 'charts'] as const).map((k) =>
        h('details', { class: 'help-item' }, h('summary', {}, t(`help.${k}.title` as Key)), ...t(`help.${k}` as Key).split('\n').map((l) => h('p', {}, l))))),
      card(a.name, a.version,
        h('dl', { class: 'kv' },
          h('dt', {}, t('about.version')), h('dd', { class: 'mono' }, a.version + (a.commit ? ` (${a.commit})` : '')),
          h('dt', {}, t('about.license')), h('dd', {}, a.license),
          h('dt', {}, t('about.source')), h('dd', {}, link(a.source, a.source)),
        ),
        h('p', { class: 'muted' }, t('about.notice'))),
      card(t('about.basedOn'), null,
        h('p', {}, t('about.basedOnText', { name: a.basedOn.name, author: a.basedOn.author })),
        h('p', {}, link(a.basedOn.url, a.basedOn.url), ' · ', link('https://www.usna.it/shellyscanner/', 'usna.it/shellyscanner'), ` · ${a.basedOn.license}`)),
      card(t('about.credits'), null,
        h('dl', { class: 'kv' }, ...a.credits.flatMap((c) => [
          h('dt', {}, link(c.url, c.name)), h('dd', {}, `${c.what} — ${c.license}`),
        ]))),
    );
  },
};
