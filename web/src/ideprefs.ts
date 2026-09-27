// Script editor settings (ShellyScanner: Settings → IDE, view/appsettings/PanelIDE),
// kept per browser.
export type Indent = 'NO' | 'STD' | 'SMART';

export interface IDEPrefs {
  tabSize: number;   // IDE_TAB_SIZE, default 4
  fontSize: number;  // IDE_FONT_SIZE, default 12
  indent: Indent;    // IDE_INDENT, default SMART
  closeCurly: boolean; closeBracket: boolean; closeSquare: boolean; closeString: boolean; // CL_*, default off
  dark: boolean;     // IDE_DARK, default off
}

export const IDE_DEFAULTS: IDEPrefs = { tabSize: 4, fontSize: 12, indent: 'SMART', closeCurly: false, closeBracket: false, closeSquare: false, closeString: false, dark: false };

const KEY = 'sl_ide';

export function ideprefs(): IDEPrefs {
  try {
    const raw = localStorage.getItem(KEY);
    if (raw) return { ...IDE_DEFAULTS, ...(JSON.parse(raw) as Partial<IDEPrefs>) };
  } catch { /* defaults */ }
  return { ...IDE_DEFAULTS };
}

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
