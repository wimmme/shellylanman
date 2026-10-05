// The login page of the optional UI password (DECISIONS §24): one password,
// "Stay logged in", the wait after wrong tries. Shown instead of the app.
import { ApiError, authApi } from '../api';
import { h } from '../dom';
import { t } from '../i18n';

export function showLogin(app: HTMLElement): void {
  const pw = h('input', { type: 'password', id: 'loginPw', autocomplete: 'current-password', required: true });
  const remember = h('input', { type: 'checkbox', id: 'loginRemember' });
  const msg = h('p', { class: 'login-msg', role: 'alert' });
  const submit = h('button', { class: 'btn primary', type: 'submit' }, t('login.submit'));
  let timer = 0;

  const wait = (secs: number): void => {
    window.clearInterval(timer);
    let left = secs;
    submit.disabled = true;
    msg.textContent = t('login.wait', { s: left });
    timer = window.setInterval(() => {
      left--;
      if (left > 0) { msg.textContent = t('login.wait', { s: left }); return; }
      window.clearInterval(timer);
      submit.disabled = false;
      msg.textContent = '';
    }, 1000);
  };

  const form = h('form', { class: 'login-card', onsubmit: async (e: Event) => {
    e.preventDefault();
    submit.disabled = true;
    msg.textContent = '';
    try {
      await authApi.login(pw.value, remember.checked);
      location.reload(); // start the app with the new session
    } catch (err) {
      submit.disabled = false;
      pw.select();
      if (err instanceof ApiError && err.retryAfter) wait(err.retryAfter);
      else if (err instanceof ApiError && err.code === 'wrong') msg.textContent = t('login.wrong');
      else msg.textContent = err instanceof Error ? err.message : String(err);
    }
  } },
    h('img', { src: 'logo.png', alt: '', width: 56, height: 56 }),
    h('h1', {}, t('app.name')),
    h('div', { class: 'field' }, h('label', { for: 'loginPw' }, t('login.password')), pw),
    h('label', { class: 'row' }, remember, t('login.remember')),
    submit, msg);
  app.replaceChildren(h('main', { class: 'login' }, form));
  document.title = `${t('login.title')} · ${t('app.name')}`;
  pw.focus();
}
