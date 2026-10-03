// Script editor settings (ShellyScanner: Settings → IDE, view/appsettings/PanelIDE),
// kept per browser.
export type Indent = 'NO' | 'STD' | 'SMART';
/** Editor colours: follow the app's light/dark theme, or always dark / light. */
export type IDETheme = 'auto' | 'dark' | 'light';

export interface IDEPrefs {
  tabSize: number;   // IDE_TAB_SIZE, default 4
  fontSize: number;  // IDE_FONT_SIZE, default 12
  indent: Indent;    // IDE_INDENT, default SMART
  closeCurly: boolean; closeBracket: boolean; closeSquare: boolean; closeString: boolean; // CL_*, default off
  // IDE_DARK (on/off) in ShellyScanner; here the app's theme by default (ShellyLanMan's own).
  theme: IDETheme;
}

export const IDE_DEFAULTS: IDEPrefs = { tabSize: 4, fontSize: 12, indent: 'SMART', closeCurly: false, closeBracket: false, closeSquare: false, closeString: false, theme: 'auto' };

const KEY = 'sl_ide';

/**
 * Stored settings with defaults. Before 0.6.3 the colours were `dark: true|false`
 * (false the default): a dark choice stays dark, the default now follows the app.
 */
export function fromStored(raw: string | null): IDEPrefs {
  if (!raw) return { ...IDE_DEFAULTS };
  try {
    const { dark, ...rest } = JSON.parse(raw) as Partial<IDEPrefs> & { dark?: boolean };
    const p = { ...IDE_DEFAULTS, ...rest };
    if (!['auto', 'dark', 'light'].includes(p.theme)) p.theme = 'auto';
    if (!('theme' in rest) && dark === true) p.theme = 'dark';
    return p;
  } catch { return { ...IDE_DEFAULTS }; }
}

export function ideprefs(): IDEPrefs {
  let raw: string | null = null;
  try { raw = localStorage.getItem(KEY); } catch { /* defaults */ }
  return fromStored(raw);
}

/** Whether the editor is dark, given the app's current theme. */
export const editorDark = (p: IDEPrefs, appTheme: 'dark' | 'light'): boolean => (p.theme === 'auto' ? appTheme === 'dark' : p.theme === 'dark');

export function setIdeprefs(p: IDEPrefs): void {
  try { localStorage.setItem(KEY, JSON.stringify(p)); } catch { /* private mode */ }
}

/** The brackets closed automatically. */
export function closingBrackets(p: IDEPrefs): string[] {
  return [p.closeBracket && '(', p.closeSquare && '[', p.closeCurly && '{', p.closeString && '"'].filter((x): x is string => !!x);
}

/** EditorPanel's autocompletion words besides the language's own. */
export const SHELLY_WORDS = ['Shelly', 'JSON', 'Timer', 'MQTT', 'BLE', 'HTTPServer', 'Virtual', 'AES',
  'String', 'Number', 'Function', 'Array', 'Math', 'Date', 'Object', 'Exceptions', 'ArrayBuffer', 'print(', 'console.log('];
