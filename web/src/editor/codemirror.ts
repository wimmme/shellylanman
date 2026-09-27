// The script editor on CodeMirror 6 (loaded on demand). Features of
// ShellyScanner's EditorPanel/ScriptFrame: syntax colours, tab size, font
// size, auto-indent (none / keep / smart), optional auto-closing of ( [ { ",
// bracket matching, line comments (Ctrl+/), upper/lower case (Ctrl+Shift+U/L),
// find/replace, go to line, completion on Ctrl+Space, dark mode.
import { autocompletion, closeBrackets, closeBracketsKeymap, completeFromList, completionKeymap } from '@codemirror/autocomplete';
import { defaultKeymap, history, historyKeymap, indentWithTab, insertNewline, insertNewlineKeepIndent, redo, undo } from '@codemirror/commands';
import { javascript, javascriptLanguage } from '@codemirror/lang-javascript';
import { bracketMatching, defaultHighlightStyle, foldGutter, foldKeymap, indentOnInput, indentUnit, syntaxHighlighting } from '@codemirror/language';
import { highlightSelectionMatches, openSearchPanel, searchKeymap } from '@codemirror/search';
import { EditorSelection, EditorState, type Extension } from '@codemirror/state';
import { oneDark } from '@codemirror/theme-one-dark';
import { drawSelection, EditorView, highlightActiveLine, highlightActiveLineGutter, highlightSpecialChars, keymap, lineNumbers } from '@codemirror/view';
import { closingBrackets, SHELLY_WORDS, type IDEPrefs } from '../ideprefs';

export interface Editor {
  text(): string;
  setText(s: string): void;
  focus(): void;
  gotoLine(n: number): void;
  undo(): void;
  redo(): void;
  find(): void;
  destroy(): void;
}

const changeCase = (upper: boolean) => (view: EditorView): boolean => {
  view.dispatch(view.state.changeByRange((r) => {
    const t = view.state.sliceDoc(r.from, r.to);
    const s = upper ? t.toUpperCase() : t.toLowerCase();
    return { changes: { from: r.from, to: r.to, insert: s }, range: EditorSelection.range(r.from, r.from + s.length) };
  }));
  return true;
};

export function createEditor(parent: HTMLElement, doc: string, p: IDEPrefs, onCursor: (line: number, col: number) => void): Editor {
  const enter = p.indent === 'NO' ? insertNewline : p.indent === 'STD' ? insertNewlineKeepIndent : null;
  const ext: Extension[] = [
    lineNumbers(), highlightActiveLineGutter(), highlightSpecialChars(), history(), foldGutter(), drawSelection(),
    EditorState.allowMultipleSelections.of(true), bracketMatching(), highlightActiveLine(), highlightSelectionMatches(),
    javascript(), syntaxHighlighting(defaultHighlightStyle, { fallback: true }),
    javascriptLanguage.data.of({ autocomplete: completeFromList(SHELLY_WORDS.map((w) => ({ label: w, type: 'variable' }))) }),
    autocompletion({ activateOnTyping: false }), // on Ctrl+Space, like the original
    EditorState.tabSize.of(p.tabSize), indentUnit.of(' '.repeat(Math.max(1, p.tabSize))),
    EditorView.theme({ '&': { fontSize: p.fontSize + 'px', height: '100%' }, '.cm-scroller': { fontFamily: 'var(--font-mono, monospace)' } }),
    keymap.of([
      ...(enter ? [{ key: 'Enter', run: enter }] : []),
      { key: 'Mod-Shift-u', run: changeCase(true) }, { key: 'Mod-Shift-l', run: changeCase(false) },
      ...closeBracketsKeymap, ...defaultKeymap, ...searchKeymap, ...historyKeymap, ...foldKeymap, ...completionKeymap, indentWithTab,
    ]),
    EditorView.updateListener.of((u) => {
      if (u.selectionSet || u.docChanged) {
        const pos = u.state.selection.main.head;
        const line = u.state.doc.lineAt(pos);
        onCursor(line.number, pos - line.from + 1);
      }
    }),
  ];
  if (p.indent === 'SMART') ext.push(indentOnInput());
  const brackets = closingBrackets(p);
  if (brackets.length) ext.push(closeBrackets(), EditorState.languageData.of(() => [{ closeBrackets: { brackets } }]));
  if (p.dark) ext.push(oneDark);
  const view = new EditorView({ state: EditorState.create({ doc, extensions: ext }), parent });
  return {
    text: () => view.state.doc.toString(),
    setText: (s) => view.setState(EditorState.create({ doc: s, extensions: ext })),
    focus: () => view.focus(),
    gotoLine: (n) => {
      const line = view.state.doc.line(Math.min(Math.max(1, n), view.state.doc.lines));
      view.dispatch({ selection: { anchor: line.from }, scrollIntoView: true });
      view.focus();
    },
    undo: () => { undo(view); view.focus(); },
    redo: () => { redo(view); view.focus(); },
    find: () => { openSearchPanel(view); },
    destroy: () => view.destroy(),
  };
}
