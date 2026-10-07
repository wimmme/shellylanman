// Portions derived from ShellyScanner (https://github.com/usnasoft/shellyscanner),
// Copyright (C) Antonio Flaccomio / usnasoft, licensed under GPL-3.0:
// view/scheduler/* (AbstractCronPanel, CronValuesDialog, gen2plus/G2SchedulerDialog,
// G2SchedulerPanel, G2JobPanel, pareditor/*, walldisplay/WDSchedulerDialog,
// WDThermSchedulerPanel, ProfilesPanel, ThermJobPanel, blutrv/TRVSchedulerDialog,
// TRVJobPanel) and the schedule managers they call (Schedule.*,
// Thermostat.Schedule.*, TRV.*ScheduleRule through BluTrv.Call).
import { ApiError, scheduleApi, type Device, type MethodHint } from '../api';
import {
  canEditParams, CRON_TH_WD, daysOfWeekAsString, DEF_CRON, DEF_TH_CRON, expression, fragStrToNum, fragmentToInt, hmCompare, invalidFields, isValid,
  PARAM_PATTERNS, PARAM_RANGES, parseCron, parseThCron, replaceParam, setMode, thExpression, thExtTimespec, valuesToField, type Cron, type Mode,
} from '../cronlogic';
import { h } from '../dom';
import { t, type Key } from '../i18n';
import { confirmDialog, openModal } from '../modal';
import { toast } from '../toast';

type Json = Record<string, unknown>;
const msg = (e: unknown): string => (e instanceof ApiError || e instanceof Error ? e.message : String(e));
const DAYS_EN = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'];
const MONTHS_EN = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];

function iconBtn(label: string, tip: Key, onClick: () => void, disabled = false): HTMLButtonElement {
  return h('button', { class: 'btn small', title: t(tip), 'aria-label': t(tip), disabled, onclick: onClick }, label);
}

/** The enable toggle of a line (Standby icons in the original). */
function enableToggle(on: boolean, onChange: (on: boolean) => Promise<boolean>): { el: HTMLButtonElement; value: () => boolean } {
  let state = on;
  const el = h('button', { class: 'btn small sch-enable', 'aria-pressed': String(on), title: t(on ? 'sch.enabled' : 'sch.disabled') }, '●');
  el.addEventListener('click', async () => {
    if (await onChange(!state)) {
      state = !state;
      el.setAttribute('aria-pressed', String(state));
      el.title = t(state ? 'sch.enabled' : 'sch.disabled');
    }
  });
  return { el, value: () => state };
}

// ---- CronValuesDialog --------------------------------------------------------------------------

function valuesDialog(field: HTMLInputElement, min: number, max: number, onDone: () => void): void {
  const selected = new Set(fragmentToInt(field.value));
  const box = h('div', { class: 'sch-values' }, ...Array.from({ length: max - min }, (_, i) => {
    const v = min + i;
    const c = h('input', { type: 'checkbox', checked: selected.has(v) });
    c.addEventListener('change', () => { if (c.checked) selected.add(v); else selected.delete(v); });
    return h('label', { class: 'row' }, c, String(v));
  }));
  openModal(t('sch.title'), box, [
    { label: t('common.cancel') },
    { label: t('common.ok'), kind: 'primary', onClick: () => { field.value = valuesToField([...selected], min, max); onDone(); } },
  ]);
}

// ---- AbstractCronPanel -------------------------------------------------------------------------

interface CronEditor { el: HTMLElement; exp(): string; set(line: string): void }

function cronEditor(initial: string): CronEditor {
  let c: Cron = parseCron(initial);
  const input = (): HTMLInputElement => h('input', { class: 'sch-frag', size: 8 });
  const hours = input(), minutes = input(), seconds = input(), days = input();
  const exp = h('input', { class: 'sch-exp', 'aria-label': t('sch.expression') });
  const modes: [Mode, Key][] = [['time', 'sch.mode.time'], ['beforeRise', 'sch.mode.beforeRise'], ['afterRise', 'sch.mode.afterRise'], ['beforeSet', 'sch.mode.beforeSet'], ['afterSet', 'sch.mode.afterSet']];
  const name = `m${Math.random().toString(36).slice(2)}`;
  const radios = modes.map(([m]) => h('input', { type: 'radio', name, value: m }));
  const wd = DAYS_EN.map(() => h('input', { type: 'checkbox' }));
  const mo = MONTHS_EN.map(() => h('input', { type: 'checkbox' }));

  const draw = (): void => {
    hours.value = c.hours; minutes.value = c.minutes; seconds.value = c.seconds; days.value = c.days;
    radios.forEach((r) => { r.checked = r.value === c.mode; });
    seconds.disabled = pickS.disabled = c.mode !== 'time';
    wd.forEach((b, i) => { b.checked = c.wdays[i]!; b.disabled = !!c.wdaysRaw; });
    mo.forEach((b, i) => { b.checked = c.months[i]!; b.disabled = !!c.monthsRaw; });
    generate();
  };
  const generate = (): void => { // generateExpression
    const bad = invalidFields(c);
    for (const [f, el] of [['hours', hours], ['minutes', minutes], ['seconds', seconds], ['days', days]] as const) el.classList.toggle('invalid', bad.has(f));
    exp.value = expression(c);
    exp.classList.toggle('invalid', !isValid(exp.value));
  };
  const fragChange = (el: HTMLInputElement, key: 'hours' | 'minutes' | 'seconds' | 'days') => (): void => {
    el.value = el.value.replace(/\s/g, '') || '*';
    c = { ...c, [key]: el.value };
    generate();
  };
  hours.addEventListener('change', fragChange(hours, 'hours'));
  minutes.addEventListener('change', fragChange(minutes, 'minutes'));
  seconds.addEventListener('change', fragChange(seconds, 'seconds'));
  days.addEventListener('change', fragChange(days, 'days'));
  radios.forEach((r) => r.addEventListener('change', () => { if (r.checked) { c = setMode(c, r.value as Mode); draw(); } }));
  wd.forEach((b, i) => b.addEventListener('change', () => { c.wdays[i] = b.checked; generate(); }));
  mo.forEach((b, i) => b.addEventListener('change', () => { c.months[i] = b.checked; generate(); }));
  exp.addEventListener('focus', () => exp.classList.remove('invalid'));
  exp.addEventListener('change', () => { // focusLost
    const v = fragStrToNum(exp.value.trim());
    if (isValid(v)) { c = parseCron(v); draw(); } else { exp.value = v; exp.classList.add('invalid'); }
  });
  const picker = (el: HTMLInputElement, min: number, max: number, key: 'hours' | 'minutes' | 'seconds' | 'days'): HTMLButtonElement =>
    iconBtn('✎', 'sch.pick', () => valuesDialog(el, min, max, () => fragChange(el, key)()));
  const pickS = picker(seconds, 0, 60, 'seconds');
  const all = (boxes: HTMLInputElement[], arr: 'wdays' | 'months', on: boolean) => (): void => {
    boxes.forEach((b, i) => { if (!b.disabled) { b.checked = on; c[arr][i] = on; } });
    generate();
  };
  const frag = (label: Key, el: HTMLInputElement, pick: HTMLButtonElement): HTMLElement => h('label', { class: 'sch-fraglabel' }, h('span', {}, t(label)), h('span', { class: 'row' }, el, pick));
  const el = h('div', { class: 'sch-cron' },
    h('div', { class: 'row sch-frags' },
      frag('sch.hours', hours, picker(hours, 0, 24, 'hours')), frag('sch.minutes', minutes, picker(minutes, 0, 60, 'minutes')),
      frag('sch.seconds', seconds, pickS), frag('sch.days', days, picker(days, 1, 32, 'days'))),
    h('div', { class: 'row sch-checks' }, ...wd.map((b, i) => h('label', { class: 'sch-check' }, b, DAYS_EN[i]!)),
      iconBtn('✓', 'sch.addAll', all(wd, 'wdays', true)), iconBtn('✗', 'sch.removeAll', all(wd, 'wdays', false))),
    h('div', { class: 'row sch-checks' }, ...mo.map((b, i) => h('label', { class: 'sch-check' }, b, MONTHS_EN[i]!)),
      iconBtn('✓', 'sch.addAll', all(mo, 'months', true)), iconBtn('✗', 'sch.removeAll', all(mo, 'months', false))),
    h('div', { class: 'row' }, ...radios.map((r, i) => h('label', { class: 'row' }, r, t(modes[i]![1])))),
    exp);
  draw();
  return { el, exp: () => exp.value, set: (line) => { c = parseCron(line); draw(); } };
}

// ---- parameter editor (ParamEditorDialog) ---------------------------------------------------------

function paramEditor(field: HTMLInputElement): void {
  const par = field.value;
  if (!canEditParams(par)) return;
  const edits: (() => void)[] = [];
  const body = h('div', { class: 'sch-params' });
  const on = PARAM_PATTERNS.on.exec(par);
  if (on) {
    const c = h('input', { type: 'checkbox', checked: on[1] === 'true' });
    body.append(h('label', { class: 'row' }, c, t('sch.par.on')));
    edits.push(() => { field.value = replaceParam(field.value, 'on', String(c.checked)); });
  }
  const slider = (key: 'brightness' | 'white' | 'ct' | 'pos' | 'slat_pos', label: Key): void => {
    const m = PARAM_PATTERNS[key].exec(par);
    if (!m) return;
    const [min, max] = PARAM_RANGES[key];
    const s = h('input', { type: 'range', min, max, value: Math.min(max, Math.max(min, Number(m[1]))) });
    const v = h('span', {}, s.value);
    s.addEventListener('input', () => { v.textContent = s.value; });
    body.append(h('label', { class: 'sch-slider' }, h('span', {}, t(label)), s, v));
    edits.push(() => { field.value = replaceParam(field.value, key, s.value); });
  };
  slider('brightness', 'sch.par.brightness');
  const rgb = PARAM_PATTERNS.rgb.exec(par);
  if (rgb) {
    const cols = ([1, 2, 3] as const).map((i, j) => {
      const s = h('input', { type: 'range', min: 0, max: 255, value: Math.min(255, Number(rgb[i])) });
      const v = h('span', {}, s.value);
      s.addEventListener('input', () => { v.textContent = s.value; });
      body.append(h('label', { class: 'sch-slider' }, h('span', {}, t((['sch.par.red', 'sch.par.green', 'sch.par.blue'] as Key[])[j]!)), s, v));
      return s;
    });
    edits.push(() => { field.value = replaceParam(field.value, 'rgb', `[${cols.map((s) => s.value).join(',')}]`); });
    slider('white', 'sch.par.white'); // RGBPanel includes white when present
  } else {
    slider('white', 'sch.par.white');
  }
  slider('ct', 'sch.par.ct');
  slider('pos', 'sch.par.pos');
  slider('slat_pos', 'sch.par.pos');
  openModal(t('sch.par.title'), body, [
    { label: t('common.cancel') },
    { label: t('common.ok'), kind: 'primary', onClick: () => { edits.forEach((f) => f()); } },
  ]);
}

// ---- Gen2+ jobs (G2SchedulerPanel / G2JobPanel) -----------------------------------------------------

interface G2Line {
  el: HTMLElement;
  id: number;          // -1: not on the device
  orig: Json | null;
  system: boolean;     // a call with "origin" (e.g. the automatic firmware update): calls read only
  enable: () => boolean;
  cron: CronEditor;
  calls: () => { method: string; params: string }[];
  isNull: () => boolean;
  json: () => Json | null;
}

function paramsText(p: unknown): string {
  if (p === undefined || p === null) return '';
  const s = JSON.stringify(p);
  return s.length > 2 ? s.slice(1, -1) : '';
}

function g2Panel(d: Device): { el: HTMLElement; apply(): Promise<boolean>; refresh(): Promise<void>; load(files: Record<string, Json>): void } {
  const box = h('div', { class: 'sch-lines' });
  let lines: G2Line[] = [];
  let removed: number[] = [];
  let hints: MethodHint[] | null = null;

  const makeLine = (job: Json | null): G2Line => {
    const cron = cronEditor((job?.timespec as string) || DEF_CRON);
    const callsBox = h('div', { class: 'sch-calls' });
    const calls = (job?.calls as Json[] | undefined) ?? [];
    const system = calls.some((c) => c.origin !== undefined && c.origin !== null);
    const rows: { method: HTMLInputElement; params: HTMLInputElement; row: HTMLElement }[] = [];
    const addCall = (method: string, params: string, at: number): void => {
      const m = h('input', { class: 'sch-method', value: method, size: 20, 'aria-label': t('sch.method'), disabled: system });
      const p = h('input', { class: 'sch-par', value: params, size: 40, 'aria-label': t('sch.params'), disabled: system });
      p.addEventListener('dblclick', () => { if (!system) paramEditor(p); });
      const hintMenu = h('div', { class: 'menu hidden', role: 'menu' });
      const hintBtn = iconBtn('▾', 'sch.hint', async () => {
        if (!hints) { try { hints = await scheduleApi.hints(d.id); } catch (e) { toast(msg(e)); return; } }
        const items: HTMLElement[] = [];
        if (canEditParams(p.value)) items.push(h('button', { role: 'menuitem', onclick: () => { hintMenu.classList.add('hidden'); paramEditor(p); } }, t('sch.par.title')), h('hr'));
        for (const x of hints) items.push(h('button', { role: 'menuitem', onclick: () => { if (x.method) m.value = x.method; if (x.params !== undefined) p.value = x.params; hintMenu.classList.add('hidden'); } }, x.name));
        hintMenu.replaceChildren(...items);
        hintMenu.classList.toggle('hidden');
      }, system);
      const test = iconBtn('▶', 'sch.test', async () => {
        if (!m.value.trim()) return;
        let params: unknown = {};
        try { params = JSON.parse('{' + p.value + '}'); } catch { toast(t('sch.err.params')); return; }
        try { await scheduleApi.rpc(d.id, m.value, params); toast(t('sch.testOk'), 'info'); } catch (e) {
          if (!(e instanceof ApiError && e.status === 428)) { toast(msg(e)); return; }
          // A method that restarts, updates or deletes: ask first (DECISIONS P19-5).
          if (!(await confirmDialog(t('sch.testConfirmTitle'), t('sch.testConfirm', { method: m.value.trim(), device: d.name || d.hostname }), t('sch.testRun')))) return;
          try { await scheduleApi.rpc(d.id, m.value, params, true); toast(t('sch.testOk'), 'info'); } catch (e2) { toast(msg(e2)); }
        }
      }, system);
      const row = h('div', { class: 'row sch-call' }, m, p,
        iconBtn('+', 'sch.addMethod', () => addCall('', '', rows.findIndex((r) => r.row === row) + 1), system),
        iconBtn('−', 'sch.removeMethod', () => { if (rows.length > 1) { const i = rows.findIndex((r) => r.row === row); rows.splice(i, 1); row.remove(); } }, system),
        h('div', { class: 'dropdown' }, hintBtn, hintMenu), test);
      rows.splice(at, 0, { method: m, params: p, row });
      callsBox.insertBefore(row, callsBox.children[at] ?? null);
    };
    if (calls.length) calls.forEach((c, i) => addCall(String(c.method ?? ''), paramsText(c.params), i));
    else addCall('', '', 0);

    const line: G2Line = {
      el: h('div'), id: typeof job?.id === 'number' ? job.id : -1, orig: null, system, cron,
      enable: () => false,
      calls: () => rows.map((r) => ({ method: r.method.value, params: r.params.value })),
      isNull: () => cron.exp() === DEF_CRON && rows.length === 1 && !rows[0]!.method.value.trim() && !rows[0]!.params.value.trim(),
      json: () => {
        const out: Json = { timespec: cron.exp() };
        const cs: Json[] = [];
        for (const r of rows) {
          const call: Json = { method: r.method.value };
          if (r.params.value.trim()) {
            try { call.params = JSON.parse('{' + r.params.value + '}'); } catch { toast(t('sch.err.params')); r.params.focus(); return null; }
          }
          if (r.method.value.trim() || r.params.value.trim()) cs.push(call);
        }
        out.calls = cs;
        return out;
      },
    };
    const en = enableToggle(job?.enable === true, async (on) => {
      if (line.id < 0) return true;
      try { await scheduleApi.rpc(d.id, 'Schedule.Update', { id: line.id, enable: on }); return true; } catch (e) { toast(msg(e)); return false; }
    });
    line.enable = en.value;
    const ops = h('div', { class: 'sch-ops' },
      iconBtn('+', 'sch.add', () => insert(makeLine(null), lines.indexOf(line) + 1)),
      system ? null : iconBtn('⧉', 'sch.duplicate', () => insert(makeLine(line.json()), lines.indexOf(line) + 1)),
      iconBtn('✕', 'sch.remove', () => remove(line)),
      iconBtn('⎘', 'sch.copy', () => { void navigator.clipboard?.writeText(JSON.stringify(line.json())); }),
      system ? null : iconBtn('📋', 'sch.paste', async () => {
        try {
          const j = JSON.parse(await navigator.clipboard.readText()) as Json;
          cron.set(String(j.timespec ?? ''));
          if (Array.isArray(j.calls)) {
            // setCalls: an empty last row is replaced
            const last = rows[rows.length - 1];
            if (last && !last.method.value && !last.params.value) { rows.pop(); last.row.remove(); }
            (j.calls as Json[]).forEach((c) => addCall(String(c.method ?? ''), paramsText(c.params), rows.length));
          }
        } catch { toast(t('sch.err.paste')); }
      }));
    line.el = h('div', { class: 'sch-line' }, h('div', { class: 'sch-job' }, cron.el,
      h('div', { class: 'row sch-callhead' }, h('span', {}, t('sch.method')), h('span', {}, t('sch.params'))), callsBox), en.el, ops);
    line.orig = line.json();
    return line;
  };
  const insert = (l: G2Line, at: number): void => {
    lines.splice(at, 0, l);
    box.insertBefore(l.el, box.children[at] ?? null);
  };
  const remove = (l: G2Line): void => {
    const i = lines.indexOf(l);
    lines.splice(i, 1);
    l.el.remove();
    if (l.id >= 0) removed.push(l.id);
    if (lines.length === 0) insert(makeLine(null), 0);
  };
  const refresh = async (): Promise<void> => {
    box.replaceChildren(h('div', { class: 'spinner' }));
    lines = []; removed = [];
    let jobs: Json[] = [];
    try { jobs = ((await scheduleApi.rpc(d.id, 'Schedule.List', {})) as { jobs?: Json[] }).jobs ?? []; } catch (e) { toast(msg(e)); }
    box.replaceChildren();
    for (const j of jobs) insert(makeLine(j), lines.length);
    if (lines.length === 0) insert(makeLine(null), 0);
  };
  const apply = async (): Promise<boolean> => { // G2SchedulerPanel.apply
    if (lines.length === 1 && lines[0]!.id < 0) {
      const l = lines[0]!;
      if (!l.isNull() && !validateG2(l)) return false;
    } else {
      for (const l of lines) if (!validateG2(l)) { l.el.scrollIntoView({ block: 'nearest' }); return false; }
    }
    try {
      for (const id of removed) await scheduleApi.rpc(d.id, 'Schedule.Delete', { id });
      removed = [];
      for (const l of lines) {
        if (l.isNull()) continue;
        const job = l.json();
        if (!job) return false;
        const o = l.orig ?? {};
        if (l.id < 0) {
          const res = (await scheduleApi.rpc(d.id, 'Schedule.Create', { ...job, enable: l.enable() })) as { id?: number };
          if (typeof res.id !== 'number' || res.id < 0) { toast(t('sch.err.create')); return false; }
          l.id = res.id; l.orig = job;
        } else if (l.system && job.timespec !== o.timespec) {
          delete job.calls; // the system calls are not rewritten
          await scheduleApi.rpc(d.id, 'Schedule.Update', { ...job, id: l.id });
          l.orig = { ...o, timespec: job.timespec };
        } else if (!l.system && (job.timespec !== o.timespec || JSON.stringify(job.calls) !== JSON.stringify(o.calls))) {
          await scheduleApi.rpc(d.id, 'Schedule.Update', { ...job, id: l.id });
          l.orig = job;
        }
      }
      return true;
    } catch (e) { toast(msg(e)); return false; }
  };
  const load = (files: Record<string, Json>): void => { // loadFromBackup
    const jobs = (files['Schedule.List.json']?.jobs as Json[] | undefined);
    if (!jobs) { toast(t('sch.err.file')); return; }
    for (const j of jobs) {
      if (!j.timespec || !j.calls) { toast(t('sch.err.file')); return; }
      const { id: _id, ...rest } = j;
      void _id;
      if (lines.length === 1 && lines[0]!.isNull()) { lines[0]!.el.remove(); lines = []; }
      insert(makeLine(rest), lines.length);
    }
  };
  void refresh();
  return { el: box, apply, refresh, load };
}

function validateG2(l: G2Line): boolean { // G2JobPanel.validateData
  if (!isValid(l.cron.exp())) { toast(t('sch.err.expression')); return false; }
  for (const c of l.calls()) {
    if (!c.method.trim()) { toast(t('sch.err.method')); return false; }
    if (c.params.trim()) { try { JSON.parse('{' + c.params + '}'); } catch { toast(t('sch.err.params')); return false; } }
  }
  return true;
}

// ---- BLU TRV rules (TRVSchedulerDialog / TRVJobPanel) --------------------------------------------------

const TRV_MIN = 4, TRV_MAX = 30;

function trvPanel(d: Device): { el: HTMLElement; apply(): Promise<boolean>; refresh(): Promise<void>; load(files: Record<string, Json>): Promise<void> } {
  const box = h('div', { class: 'sch-lines' });
  type Line = { el: HTMLElement; id: number; orig: Json; cron: CronEditor; json(): Json; isNull(): boolean; target(): string; enable(): boolean };
  let lines: Line[] = [];
  let removed: number[] = [];
  const makeLine = (rule: Json | null): Line => {
    const cron = cronEditor((rule?.timespec as string) || DEF_CRON);
    const name = `t${Math.random().toString(36).slice(2)}`;
    const rTemp = h('input', { type: 'radio', name, checked: rule?.pos === undefined || rule?.target_C !== undefined });
    const rPos = h('input', { type: 'radio', name, checked: rule?.target_C === undefined && rule?.pos !== undefined });
    const target = h('input', { type: 'number', class: 'sch-target', step: 0.1, min: TRV_MIN, max: TRV_MAX,
      value: rule?.target_C !== undefined ? String(rule.target_C) : rule?.pos !== undefined ? String(rule.pos) : '' });
    const limits = (): void => {
      if (rTemp.checked) {
        target.step = '0.1'; target.min = String(TRV_MIN); target.max = String(TRV_MAX);
        if (target.value) target.value = String(Math.min(TRV_MAX, Math.max(TRV_MIN, Number(target.value))));
      } else { target.step = '1'; target.min = '0'; target.max = '100'; }
    };
    rTemp.addEventListener('change', limits);
    rPos.addEventListener('change', limits);
    limits();
    const line: Line = {
      el: h('div'), id: typeof rule?.rule_id === 'number' ? rule.rule_id : -1, orig: {}, cron,
      target: () => target.value,
      json: () => {
        const out: Json = { timespec: cron.exp() };
        if (target.value !== '') { if (rTemp.checked) out.target_C = Number(target.value); else out.pos = Math.round(Number(target.value)); }
        return out;
      },
      isNull: () => cron.exp() === DEF_CRON && target.value === '',
      enable: () => false,
    };
    const en = enableToggle(rule?.enable === true, async (on) => {
      if (line.id < 0) return true;
      try { await scheduleApi.rpc(d.id, 'Trv.UpdateScheduleRule', { id: 0, rule_id: line.id, rule: { enable: on } }); return true; } catch (e) { toast(msg(e)); return false; }
    });
    line.enable = en.value;
    const ops = h('div', { class: 'sch-ops' },
      iconBtn('+', 'sch.add', () => insert(makeLine(null), lines.indexOf(line) + 1)),
      iconBtn('⧉', 'sch.duplicate', () => insert(makeLine(line.json()), lines.indexOf(line) + 1)),
      iconBtn('✕', 'sch.remove', () => {
        const i = lines.indexOf(line); lines.splice(i, 1); line.el.remove();
        if (line.id >= 0) removed.push(line.id);
        if (lines.length === 0) insert(makeLine(null), 0);
      }),
      iconBtn('⎘', 'sch.copy', () => { void navigator.clipboard?.writeText(JSON.stringify(line.json())); }),
      iconBtn('📋', 'sch.paste', async () => {
        try {
          const j = JSON.parse(await navigator.clipboard.readText()) as Json;
          cron.set(String(j.timespec ?? ''));
          if (j.target_C !== undefined) { rTemp.checked = true; target.value = String(j.target_C); } else if (j.pos !== undefined) { rPos.checked = true; target.value = String(j.pos); }
          limits();
        } catch { toast(t('sch.err.paste')); }
      }));
    line.el = h('div', { class: 'sch-line' }, h('div', { class: 'sch-job' }, cron.el,
      h('div', { class: 'row' }, h('label', { class: 'row' }, rTemp, t('sch.targetTemp')), h('label', { class: 'row' }, rPos, t('sch.targetPos')), target)), en.el, ops);
    line.orig = line.json();
    return line;
  };
  const insert = (l: Line, at: number): void => { lines.splice(at, 0, l); box.insertBefore(l.el, box.children[at] ?? null); };
  const refresh = async (): Promise<void> => {
    box.replaceChildren(h('div', { class: 'spinner' }));
    lines = []; removed = [];
    let rules: Json[] = [];
    try { rules = ((await scheduleApi.rpc(d.id, 'TRV.ListScheduleRules', { id: 0 })) as { rules?: Json[] }).rules ?? []; } catch (e) { toast(msg(e)); }
    box.replaceChildren();
    for (const r of rules) insert(makeLine(r), lines.length);
    if (lines.length === 0) insert(makeLine(null), 0);
  };
  const validate = (l: Line): boolean => {
    if (!isValid(l.cron.exp())) { toast(t('sch.err.expression')); return false; }
    if (l.target() === '') { toast(t('sch.err.target')); return false; }
    return true;
  };
  const apply = async (): Promise<boolean> => {
    if (lines.length === 1 && lines[0]!.id < 0) { const l = lines[0]!; if (!l.isNull() && !validate(l)) return false; } else {
      for (const l of lines) if (!validate(l)) { l.el.scrollIntoView({ block: 'nearest' }); return false; }
    }
    try {
      for (const id of removed) await scheduleApi.rpc(d.id, 'Trv.RemoveScheduleRule', { id: 0, rule_id: id });
      removed = [];
      for (const l of lines) {
        if (l.isNull()) continue;
        const j = l.json();
        if (l.id < 0) {
          const res = (await scheduleApi.rpc(d.id, 'Trv.AddScheduleRule', { id: 0, rule: { ...j, enable: l.enable() } })) as { rule_id?: number };
          if (typeof res.rule_id !== 'number' || res.rule_id < 0) { toast(t('sch.err.create')); return false; }
          l.id = res.rule_id; l.orig = j;
        } else if (j.timespec !== l.orig.timespec || j.pos !== l.orig.pos || j.target_C !== l.orig.target_C) {
          await scheduleApi.rpc(d.id, 'Trv.UpdateScheduleRule', { id: 0, rule_id: l.id, rule: j });
          l.orig = j;
        }
      }
      return true;
    } catch (e) { toast(msg(e)); return false; }
  };
  const add = (rules: Json[]): void => {
    for (const r of rules) {
      if (lines.length === 1 && lines[0]!.isNull()) { lines[0]!.el.remove(); lines = []; }
      insert(makeLine(r), lines.length);
    }
  };
  const load = async (files: Record<string, Json>): Promise<void> => {
    if (files['Thermostat.Schedule.ListProfiles.json']) { // Wall Display backup: pick a profile
      const rules = await pickProfileRules(files);
      if (rules) add(rules);
    } else if (files['TRV.ListScheduleRules.json']) {
      add(((files['TRV.ListScheduleRules.json'].rules as Json[]) ?? []).map(({ rule_id: _r, ...rest }) => { void _r; return rest; }));
    } else toast(t('sch.err.file'));
  };
  void refresh();
  return { el: box, apply, refresh, load };
}

/** dlgProfileSelection: the rules of one profile of a Wall Display backup. */
function pickProfileRules(files: Record<string, Json>): Promise<Json[] | null> {
  const profiles = ((files['Thermostat.Schedule.ListProfiles.json']?.profiles as Json[]) ?? []);
  if (profiles.length === 0) { toast(t('sch.noProfiles')); return Promise.resolve(null); }
  return new Promise((resolve) => {
    let out: Json[] | null = null;
    const sel = h('select', { id: 'schProf' }, ...profiles.map((p, i) => h('option', { value: i }, String(p.name ?? p.id))));
    openModal(t('sch.profileSelect'), h('div', { class: 'field' }, h('label', { for: 'schProf' }, t('sch.profileSelectText')), sel), [
      { label: t('common.cancel') },
      { label: t('common.ok'), kind: 'primary', onClick: () => {
        const p = profiles[Number(sel.value)]!;
        out = ((files[`Thermostat.Schedule.ListRules_profile_id-${p.id}.json`]?.rules as Json[]) ?? []).map((r) => ({ timespec: r.timespec, target_C: r.target_C }));
      } },
    ], () => resolve(out));
  });
}

// ---- Wall Display thermostat (WDThermSchedulerPanel / ProfilesPanel / ThermJobPanel) ------------------------

const WD_MIN = 5, WD_MAX = 35;

interface Rule { ruleId: string | null; target: number | null; timespec: string | null; enabled: boolean }

function wdThermPanel(d: Device): { el: HTMLElement; apply(): Promise<boolean>; refresh(): Promise<void>; load(files: Record<string, Json>): Promise<void> } {
  const rpc = (method: string, params: Json): Promise<unknown> => scheduleApi.rpc(d.id, method, params);
  type Profile = { id: number; name: string };
  let profiles: Profile[] = [];
  let current: Profile | null = null;
  let selected = -1;
  const rules = new Map<number, Rule[]>();
  let removed: { ruleId: string; profileId: number }[] = [];
  type Line = { el: HTMLElement; rule: Rule; exp(): string; ext(): string; target(): number | null; isNull(): boolean; valid(): boolean; enable(): boolean };
  let lines: Line[] = [];

  const profBox = h('div', { class: 'sch-profiles' });
  const rulesBox = h('div', { class: 'sch-lines' });
  const enableProfiles = enableToggle(false, async (on) => {
    try { await rpc('Thermostat.Schedule.SetConfig', { id: 0, config: { enable: on } }); } catch (e) { toast(msg(e)); }
    current = on ? await currentProfile() : null;
    drawProfiles();
    return true;
  });
  const currentProfile = async (): Promise<Profile | null> => {
    try {
      const st = (await rpc('Thermostat.GetStatus', { id: 0 })) as { schedules?: { enable?: boolean; profile_id?: number; profile_name?: string } };
      return st.schedules?.enable ? { id: st.schedules.profile_id ?? 0, name: st.schedules.profile_name ?? '' } : null;
    } catch { return null; }
  };

  const makeLine = (rule: Rule): Line => {
    const r = parseThCron(rule.timespec ?? DEF_TH_CRON);
    const hh = h('input', { type: 'number', min: 0, max: 23, value: r.h, class: 'sch-frag' });
    const mm = h('input', { type: 'number', min: 0, max: 59, value: r.m, class: 'sch-frag' });
    const wd = DAYS_EN.map((_, i) => h('input', { type: 'checkbox', checked: r.wdays[i] }));
    const exp = h('input', { class: 'sch-exp' });
    const target = h('input', { type: 'number', class: 'sch-target', min: WD_MIN, max: WD_MAX, step: 0.1, value: rule.target ?? '' });
    const days = (): boolean[] => wd.map((b) => b.checked);
    const gen = (): void => { exp.value = thExpression(Number(hh.value), Number(mm.value), days()); exp.classList.toggle('invalid', !CRON_TH_WD.test(exp.value)); };
    [hh, mm, ...wd].forEach((e) => e.addEventListener('change', gen));
    exp.addEventListener('change', () => {
      const v = fragStrToNum(exp.value.trim());
      if (CRON_TH_WD.test(v)) { const p = parseThCron(v); hh.value = String(p.h); mm.value = String(p.m); wd.forEach((b, i) => { b.checked = p.wdays[i]!; }); gen(); } else exp.classList.add('invalid');
    });
    gen();
    const line: Line = {
      el: h('div'), rule,
      exp: () => exp.value,
      ext: () => thExtTimespec(Number(hh.value), Number(mm.value), days()),
      target: () => (target.value === '' ? null : Math.round(Number(target.value) * 10) / 10),
      isNull: () => exp.value === DEF_TH_CRON && target.value === '',
      valid: () => {
        if (!CRON_TH_WD.test(exp.value)) { toast(t('sch.err.expression')); return false; }
        if (target.value === '') { toast(t('sch.err.target')); return false; }
        return true;
      },
      enable: () => false,
    };
    const en = enableToggle(rule.enabled, async (on) => {
      if (rule.ruleId === null) return true;
      try { await rpc('Thermostat.Schedule.UpdateRule', { id: 0, profile_id: selected, config: { rule_id: rule.ruleId, enable: on } }); rule.enabled = on; return true; } catch (e) { toast(msg(e)); return false; }
    });
    line.enable = en.value;
    const newRule = (ts: string | null, tg: number | null): Rule => ({ ruleId: null, target: tg, timespec: ts, enabled: false });
    const ops = h('div', { class: 'sch-ops' },
      iconBtn('+', 'sch.add', () => insert(makeLine(newRule(null, null)), lines.indexOf(line) + 1)),
      iconBtn('⧉', 'sch.duplicate', () => insert(makeLine(newRule(line.exp(), line.target())), lines.indexOf(line) + 1)),
      iconBtn('✕', 'sch.remove', () => {
        const i = lines.indexOf(line); lines.splice(i, 1); rules.get(selected)!.splice(i, 1); line.el.remove();
        if (rule.ruleId !== null) removed.push({ ruleId: rule.ruleId, profileId: selected });
        if (lines.length === 0) insert(makeLine(newRule(null, null)), 0);
      }),
      iconBtn('⎘', 'sch.copy', () => { void navigator.clipboard?.writeText(JSON.stringify({ timespec: line.exp(), target_C: line.target() })); }),
      iconBtn('📋', 'sch.paste', async () => {
        try {
          const j = JSON.parse(await navigator.clipboard.readText()) as Json;
          const p = parseThCron(String(j.timespec));
          hh.value = String(p.h); mm.value = String(p.m); wd.forEach((b, i) => { b.checked = p.wdays[i]!; });
          target.value = String(j.target_C ?? '');
          gen();
        } catch { toast(t('sch.err.paste')); }
      }));
    line.el = h('div', { class: 'sch-line' }, h('div', { class: 'sch-job' },
      h('div', { class: 'row sch-frags' }, h('label', { class: 'sch-fraglabel' }, h('span', {}, t('sch.hours')), hh), h('label', { class: 'sch-fraglabel' }, h('span', {}, t('sch.minutes')), mm),
        h('label', { class: 'sch-fraglabel' }, h('span', {}, t('sch.targetTemp')), target)),
      h('div', { class: 'row sch-checks' }, ...wd.map((b, i) => h('label', { class: 'sch-check' }, b, DAYS_EN[i]!)),
        iconBtn('✓', 'sch.addAll', () => { wd.forEach((b) => { b.checked = true; }); gen(); }), iconBtn('✗', 'sch.removeAll', () => { wd.forEach((b) => { b.checked = false; }); gen(); })),
      exp), en.el, ops);
    return line;
  };
  const insert = (l: Line, at: number): void => {
    const list = rules.get(selected)!;
    if (!list.includes(l.rule)) list.splice(at, 0, l.rule);
    lines.splice(at, 0, l);
    rulesBox.insertBefore(l.el, rulesBox.children[at] ?? null);
  };
  const showRules = async (): Promise<void> => {
    lines = [];
    rulesBox.replaceChildren();
    if (selected < 0) return;
    let list = rules.get(selected);
    if (!list) {
      try {
        const res = (await rpc('Thermostat.Schedule.ListRules', { id: 0, profile_id: selected })) as { rules?: Json[] };
        list = (res.rules ?? []).map((r) => ({ ruleId: String(r.rule_id ?? ''), target: Number(r.target_C), timespec: fragStrToNum(String(r.timespec ?? '')), enabled: r.enable === true }));
        list.sort((a, b) => hmCompare(a.timespec!, b.timespec!));
      } catch (e) { toast(msg(e)); list = []; }
      rules.set(selected, list);
    }
    if (list.length === 0) list.push({ ruleId: null, target: null, timespec: null, enabled: false });
    for (const r of list) { const l = makeLine(r); lines.push(l); rulesBox.append(l.el); }
  };
  const drawProfiles = (): void => {
    const rows = [...profiles].sort((a, b) => a.name.localeCompare(b.name)).map((p) => {
      const name = h('input', { value: p.name, class: 'sch-profname', 'aria-label': t('sch.profiles') });
      name.addEventListener('change', async () => {
        if (!name.value) { name.value = p.name; return; }
        try { await rpc('Thermostat.Schedule.RenameProfile', { id: 0, profile_id: p.id, name: name.value }); p.name = name.value; } catch (e) { toast(msg(e)); name.value = p.name; }
      });
      const tr = h('tr', { class: p.id === selected ? 'selected' : '' }, h('td', {}, name, current?.id === p.id ? ' ✓' : ''));
      tr.addEventListener('click', () => { if (selected !== p.id) { selected = p.id; drawProfiles(); void showRules(); } });
      return tr;
    });
    const sel = profiles.find((p) => p.id === selected);
    profBox.replaceChildren(h('table', { class: 'data sch-proftable' }, h('thead', {}, h('tr', {}, h('th', {}, t('sch.profiles')))), h('tbody', {}, ...rows)),
      h('div', { class: 'row sch-profbtns' },
        h('button', { class: 'btn', onclick: addProfile }, t('sch.addProfile')),
        h('button', { class: 'btn', disabled: !sel, onclick: () => void duplicateProfile() }, t('sch.duplicateProfile')),
        h('button', { class: 'btn', disabled: !sel, onclick: () => void deleteProfile() }, t('sch.delProfile')),
        h('button', { class: 'btn', disabled: !sel || !current || current.id === selected, onclick: async () => {
          try { await rpc('Thermostat.Schedule.SetConfig', { id: 0, config: { profile_id: selected } }); current = sel ?? null; } catch (e) { toast(msg(e)); current = null; }
          drawProfiles();
        } }, t('sch.selectProfile')),
        enableProfiles.el));
  };
  const addProfile = (): void => {
    const name = h('input', { id: 'schNewProf', value: t('sch.defaultProfile') });
    openModal(t('sch.addProfile'), h('div', { class: 'field' }, h('label', { for: 'schNewProf' }, t('sch.profiles')), name), [
      { label: t('common.cancel') },
      { label: t('common.ok'), kind: 'primary', onClick: async () => {
        if (!name.value) return false;
        try {
          const res = (await rpc('Thermostat.Schedule.AddProfile', { id: 0, name: name.value })) as { profile_id?: number };
          profiles.push({ id: res.profile_id ?? 0, name: name.value });
          selected = res.profile_id ?? 0;
          current = await currentProfile();
          drawProfiles(); void showRules();
        } catch (e) { toast(e instanceof ApiError && /precondition/i.test(e.message) ? t('sch.addProfileError') : msg(e)); }
      } },
    ]);
    name.select();
  };
  const duplicateProfile = async (): Promise<void> => {
    const src = profiles.find((p) => p.id === selected);
    if (!src) { toast(t('sch.selectFirst')); return; }
    const name = src.name + '-new';
    try {
      const res = (await rpc('Thermostat.Schedule.AddProfile', { id: 0, name })) as { profile_id?: number };
      const newId = res.profile_id ?? 0;
      profiles.push({ id: newId, name });
      for (const r of rules.get(src.id) ?? []) { // DUPLICATE_EVENT: the rules are created on the device at once
        if (!r.timespec) continue;
        const f = r.timespec.split(' ');
        const ts = `${f[0]} ${f[1]} ${f[2]} ${f[3]} ${f[4]} ${daysOfWeekAsString(f[5] ?? '*')}`;
        await rpc('Thermostat.Schedule.CreateRule', { id: 0, config: { profile_id: newId, target_C: r.target, timespec: ts, enable: r.enabled } });
      }
      drawProfiles();
    } catch (e) { toast(e instanceof ApiError && /precondition/i.test(e.message) ? t('sch.addProfileError') : msg(e)); }
  };
  const deleteProfile = async (): Promise<void> => {
    const p = profiles.find((x) => x.id === selected);
    if (!p) return;
    try {
      const res = (await rpc('Thermostat.Schedule.ListRules', { id: 0, profile_id: p.id })) as { rules?: unknown[] };
      if ((res.rules ?? []).length > 0 && !(await confirmDialog(t('sch.delProfile'), t('sch.delProfileConfirm'), t('sch.delProfile')))) return;
      await rpc('Thermostat.Schedule.DeleteProfile', { id: 0, profile_id: p.id });
      rules.delete(p.id);
      await refresh();
    } catch (e) { toast(msg(e)); }
  };
  const refresh = async (): Promise<void> => {
    rules.clear(); removed = [];
    try {
      const res = (await rpc('Thermostat.Schedule.ListProfiles', { id: 0 })) as { profiles?: Json[] };
      profiles = (res.profiles ?? []).map((p) => ({ id: Number(p.id), name: String(p.name ?? '') }));
      current = await currentProfile();
      enableProfiles.el.setAttribute('aria-pressed', String(current !== null));
    } catch (e) { toast(msg(e)); }
    if (!profiles.some((p) => p.id === selected)) selected = -1;
    drawProfiles();
    await showRules();
  };
  const apply = async (): Promise<boolean> => { // WDThermSchedulerPanel.apply
    if (lines.length === 1 && lines[0]!.rule.ruleId === null) { const l = lines[0]!; if (!l.isNull() && !l.valid()) return false; } else {
      for (const l of lines) if (!l.valid()) { l.el.scrollIntoView({ block: 'nearest' }); return false; }
    }
    try {
      for (const r of removed) await rpc('Thermostat.Schedule.DeleteRule', { id: 0, profile_id: r.profileId, rule_id: r.ruleId });
      removed = [];
      for (const l of lines) {
        if (l.isNull()) continue;
        const r = l.rule;
        if (r.ruleId === null) {
          const res = (await rpc('Thermostat.Schedule.CreateRule', { id: 0, config: { profile_id: selected, target_C: l.target(), timespec: l.ext(), enable: l.enable() } })) as { new_rule?: { rule_id?: string } };
          if (!res.new_rule?.rule_id) { toast(t('sch.err.create')); return false; }
          r.ruleId = String(res.new_rule.rule_id); r.target = l.target(); r.timespec = l.exp(); r.enabled = l.enable();
        } else if (r.target !== l.target() || r.timespec !== l.exp()) {
          await rpc('Thermostat.Schedule.UpdateRule', { id: 0, profile_id: selected, config: { rule_id: r.ruleId, target_C: l.target(), timespec: l.ext(), enable: r.enabled } });
          r.target = l.target(); r.timespec = l.exp();
        }
      }
      return true;
    } catch (e) { toast(msg(e)); return false; }
  };
  const load = async (files: Record<string, Json>): Promise<void> => {
    if (selected < 0) { toast(t('sch.selectFirst')); return; }
    let list: Json[] | null = null;
    if (files['Thermostat.Schedule.ListProfiles.json']) list = await pickProfileRules(files);
    else if (files['TRV.ListScheduleRules.json']) {
      list = ((files['TRV.ListScheduleRules.json'].rules as Json[]) ?? []).filter((r) => r.target_C !== undefined && r.target_C !== null && !String(r.timespec ?? '').startsWith('@'));
    } else { toast(t('sch.err.file')); return; }
    for (const r of list ?? []) {
      if (lines.length === 1 && lines[0]!.isNull()) { lines[0]!.el.remove(); lines = []; rules.get(selected)!.length = 0; }
      insert(makeLine({ ruleId: null, target: Number(r.target_C), timespec: String(r.timespec), enabled: false }), lines.length);
    }
  };
  void refresh();
  return { el: h('div', { class: 'sch-wd' }, profBox, rulesBox), apply, refresh, load };
}

// ---- the dialogs ------------------------------------------------------------------------------------

interface Tab { label: Key; panel: { el: HTMLElement; apply(): Promise<boolean>; refresh(): Promise<void>; load(files: Record<string, Json>): void | Promise<void> } }

async function readBackup(): Promise<Record<string, Json> | null> {
  return new Promise((resolve) => {
    const f = h('input', { type: 'file', accept: '.sbk', class: 'hidden' });
    f.addEventListener('change', async () => {
      const file = f.files?.[0];
      f.remove();
      if (!file) { resolve(null); return; }
      const buf = new Uint8Array(await file.arrayBuffer());
      let s = '';
      for (let i = 0; i < buf.length; i += 0x8000) s += String.fromCharCode(...buf.subarray(i, i + 0x8000));
      try { resolve((await scheduleApi.backupJSON(btoa(s))) as Record<string, Json>); } catch { toast(t('sch.err.file')); resolve(null); }
    });
    document.body.append(f);
    f.click();
  });
}

export function schedulerKind(d: Device): 'wd' | 'g2' | 'trv' | null {
  if (d.status === 'ghost' || d.battery) return null;
  if (d.gen === 'blu') return 'trv';
  if (!['2', '3', '4'].includes(d.gen)) return null;
  return d.typeId.startsWith('WallDisplay') && (d.modules ?? []).some((m) => m.kind === 'thermostat') ? 'wd' : 'g2';
}

export function openScheduler(d: Device): void {
  const kind = schedulerKind(d);
  if (!kind) return;
  const tabs: Tab[] = kind === 'trv' ? [{ label: 'sch.title', panel: trvPanel(d) }]
    : kind === 'wd' ? [{ label: 'sch.jobs', panel: g2Panel(d) }, { label: 'sch.thermostat', panel: wdThermPanel(d) }]
      : [{ label: 'sch.title', panel: g2Panel(d) }];
  let cur = 0;
  const bar = h('div', { class: 'tabs', role: 'tablist' });
  const content = h('div', { class: 'sch-body' });
  const show = (i: number): void => {
    cur = i;
    bar.replaceChildren(...(tabs.length > 1 ? tabs.map((x, j) => h('button', { class: 'tab' + (i === j ? ' active' : ''), role: 'tab', onclick: () => show(j) }, t(x.label))) : []));
    content.replaceChildren(tabs[i]!.panel.el);
  };
  show(0);
  const help = (): void => { openModal(t('sch.title'), h('div', {}, ...t('sch.help').split('\n').map((l) => h('p', {}, l))), [{ label: t('common.close') }]); };
  openModal(`${t('sch.title')} - ${d.name && d.name !== d.hostname ? `${d.name} (${d.hostname})` : d.hostname}`, h('div', {}, bar, content), [
    { label: t('cfg.apply'), kind: 'primary', onClick: async () => { await tabs[cur]!.panel.apply(); return false; } },
    { label: t('cfg.applyClose'), onClick: () => tabs[cur]!.panel.apply() },
    { label: t('sch.refresh'), onClick: async () => { for (const x of tabs) await x.panel.refresh(); return false; } },
    { label: t('sch.load'), onClick: async () => { const f = await readBackup(); if (f) await tabs[cur]!.panel.load(f); return false; } },
    { label: '?', onClick: () => { help(); return false; } },
    { label: t('common.close') },
  ], undefined, 'full');
}
