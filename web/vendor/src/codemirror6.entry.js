// pgpeek SQL editor — CodeMirror 6 adapter.
//
// This is the source for the committed web/vendor/codemirror6.js bundle. The
// formatter is kept in web/vendor/sql-formatter.js and loaded on first use.
import { EditorView, GutterMarker, gutter, keymap } from "@codemirror/view";
import { basicSetup } from "codemirror";
import { Prec, Compartment, EditorState } from "@codemirror/state";
import { sql, PostgreSQL } from "@codemirror/lang-sql";
import { HighlightStyle, syntaxHighlighting, syntaxTree } from "@codemirror/language";
import { setDiagnostics, setDiagnosticsEffect } from "@codemirror/lint";
import { tags as t } from "@lezer/highlight";

const highlight = Prec.highest(
  syntaxHighlighting(
    HighlightStyle.define([
      { tag: t.keyword, color: "var(--cm-keyword)" },
      { tag: [t.string, t.special(t.string)], color: "var(--cm-string)" },
      { tag: [t.number, t.bool, t.null], color: "var(--cm-number)" },
      { tag: [t.comment, t.lineComment, t.blockComment], color: "var(--linenum)", fontStyle: "italic" },
    ]),
  ),
);

class RunStatementMarker extends GutterMarker {
  constructor(from) {
    super();
    this.from = from;
  }

  eq(other) { return this.from === other.from; }

  toDOM() {
    const marker = document.createElement("button");
    marker.type = "button";
    marker.tabIndex = -1;
    marker.className = "cm-run-statement";
    marker.dataset.from = String(this.from);
    marker.title = "Run statement";
    marker.setAttribute("aria-label", "Run statement");

    const svg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
    svg.setAttribute("viewBox", "0 0 24 24");
    svg.setAttribute("aria-hidden", "true");
    const path = document.createElementNS("http://www.w3.org/2000/svg", "path");
    path.setAttribute("d", "M6.906 4.537A.6.6 0 0 0 6 5.053v13.894a.6.6 0 0 0 .906.516l11.723-6.947a.6.6 0 0 0 0-1.032L6.906 4.537Z");
    path.setAttribute("fill", "color-mix(in srgb, var(--sql-console-gutter-run-button, #0fdfb9), transparent 50%)");
    path.setAttribute("stroke", "currentColor");
    path.setAttribute("stroke-linecap", "round");
    path.setAttribute("stroke-linejoin", "round");
    path.setAttribute("stroke-width", "1.5");
    svg.append(path);
    marker.append(svg);
    return marker;
  }
}

function runStatementGutter(onRun) {
  return gutter({
    class: "cm-run-gutter",
    lineMarker: (view, line) => {
      const statement = statementAt(view.state, line.from);
      return statement && view.state.doc.lineAt(statement.from).from === line.from
        ? new RunStatementMarker(statement.from)
        : null;
    },
    lineMarkerChange: (update) => update.docChanged,
    domEventHandlers: {
      click(view, _line, event) {
        const marker = event.target.closest?.(".cm-run-statement");
        if (!marker) return false;
        view.dispatch({ selection: { anchor: Number(marker.dataset.from) }, scrollIntoView: true });
        view.focus();
        onRun();
        return true;
      },
    },
  });
}

let formatter;
function loadFormatter() {
  const entry = import.meta.url.endsWith(".entry.js") ? "./sql-formatter.entry.js" : "./sql-formatter.js";
  return formatter ||= import(new URL(entry, import.meta.url).href);
}
function trimmedRange(state, from, to, kind) {
  const text = state.sliceDoc(from, to);
  const leading = text.length - text.trimStart().length;
  const trailing = text.length - text.trimEnd().length;
  from += leading;
  to -= trailing;
  return { sql: state.sliceDoc(from, to), from, to, kind };
}

function statementAt(state, pos) {
  const tree = syntaxTree(state);
  const statements = [];
  for (let node = tree.topNode.firstChild; node; node = node.nextSibling) {
    if (node.name === "Statement") statements.push(node);
  }

  for (const node of statements) {
    if (node.from <= pos && node.to >= pos) return node;
  }
  if (!statements.length) return null;

  let nearest = statements[0];
  let distance = Math.abs(pos - nearest.from);
  for (const node of statements) {
    const nodeDistance = pos < node.from ? node.from - pos : pos - node.to;
    if (nodeDistance < distance) {
      nearest = node;
      distance = nodeDistance;
    }
  }
  return nearest;
}

export function runnableRange(state) {
  const selection = state.selection.main;
  if (!selection.empty) return trimmedRange(state, selection.from, selection.to, "selection");

  const statement = statementAt(state, selection.head);
  if (statement) return trimmedRange(state, statement.from, statement.to, "statement");

  return trimmedRange(state, 0, state.doc.length, "document");
}

// mount(parent, doc, onRun, onChange) retains the original signature. SQL
// configuration is reconfigured as a unit so CodeMirror's native completion
// source receives the same nested namespace as the parser support.
function mount(parent, doc, onRun, onChange) {
  const sqlLang = new Compartment();
  let reportChanges = true;
  const view = new EditorView({
    doc,
    parent,
    extensions: [
      basicSetup,
      runStatementGutter(onRun),
      sqlLang.of(sql({ dialect: PostgreSQL, upperCaseKeywords: true })),
      EditorView.lineWrapping,
      EditorView.contentAttributes.of({ "aria-label": "SQL query" }),
      EditorState.transactionExtender.of((transaction) => transaction.docChanged
        ? { effects: setDiagnosticsEffect.of([]) }
        : null),
      EditorView.updateListener.of((update) => {
        if (update.docChanged && reportChanges) onChange(update.state.doc.toString());
      }),
      highlight,
      Prec.highest(keymap.of([
        { key: "Mod-Enter", preventDefault: true, run: () => { onRun(); return true; } },
        { key: "Shift-Alt-f", preventDefault: true, run: () => { void formatRange(); return true; } },
      ])),
    ],
  });

  function getRunnable() {
    return runnableRange(view.state);
  }

  async function formatRange() {
    const state = view.state;
    const range = runnableRange(state);
    if (!range.sql) return;
    const oldSelection = state.selection.main;
    const { formatPostgreSQL } = await loadFormatter();
    if (view.state.doc !== state.doc) return;
    const formatted = formatPostgreSQL(range.sql);
    if (formatted === range.sql) return;

    const selection = range.kind === "selection"
      ? { anchor: range.from, head: range.from + formatted.length }
      : { anchor: range.from + Math.min(Math.max(oldSelection.head - range.from, 0), formatted.length) };
    view.dispatch({
      changes: { from: range.from, to: range.to, insert: formatted },
      selection,
      scrollIntoView: true,
    });
  }

  return {
    getValue: () => view.state.doc.toString(),
    setValue: (value, notify = true) => {
      reportChanges = notify;
      try {
        view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: value } });
      } finally {
        reportChanges = true;
      }
    },
    refresh: () => view.requestMeasure(),
    setSQLConfig: ({ schema, defaultSchema } = {}) => view.dispatch({
      effects: sqlLang.reconfigure(sql({
        dialect: PostgreSQL,
        schema,
        defaultSchema,
        upperCaseKeywords: true,
      })),
    }),
    getRunnable,
    format: formatRange,
    setDiagnostics: (diagnostics) => view.dispatch(setDiagnostics(view.state, diagnostics)),
    clearDiagnostics: () => view.dispatch(setDiagnostics(view.state, [])),
    focusRange: (from, to) => {
      const length = view.state.doc.length;
      from = Math.max(0, Math.min(from, length));
      to = Math.max(from, Math.min(to, length));
      view.dispatch({ selection: { anchor: from, head: to }, scrollIntoView: true });
      view.focus();
    },
  };
}

window.cm6 = { mount };
