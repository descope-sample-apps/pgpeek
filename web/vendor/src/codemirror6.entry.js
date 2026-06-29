// pgpeek SQL editor — CodeMirror 6 adapter.
//
// This is the SOURCE for the committed bundle web/vendor/codemirror6.js.
// It is NOT loaded directly by the browser; it is bundled (with its
// @codemirror/* dependencies) into a single minified ESM file.
//
// Regenerate after any @codemirror/* or codemirror bump:
//     npm install && npm run vendor
// (npm run vendor === esbuild --bundle … → web/vendor/codemirror6.js)
//
// Design: the bundle exposes ONE global, `window.cm6`, mirroring how the app
// previously consumed the CM5 CDN global. This keeps web/app.js import-free and
// lets it degrade to a plain <textarea> when this bundle is absent.
import { EditorView, keymap } from "@codemirror/view";
import { basicSetup } from "codemirror";
import { Prec, Compartment } from "@codemirror/state";
import { sql, PostgreSQL } from "@codemirror/lang-sql";
import { HighlightStyle, syntaxHighlighting } from "@codemirror/language";
import { tags as t } from "@lezer/highlight";

const ident = String.raw`(?:"[^"]+"|[A-Za-z_][\w$]*)`;
const relationPattern = new RegExp(String.raw`\b(?:from|join)\s+(${ident}(?:\.${ident})?)(?:\s+(?:as\s+)?(${ident}))?`, "gi");
const relationTailPattern = new RegExp(String.raw`\b(?:from|join)\s+${ident}?$`, "i");

const cleanIdent = (s) => s && s.replace(/^"|"$/g, "");

export function columnCompletionSource(columnsByRelation = {}) {
  return (context) => {
    const before = context.state.sliceDoc(0, context.pos);
    if (relationTailPattern.test(before)) return null;
    const word = context.matchBefore(/[A-Za-z_][\w$]*/);
    if (!word && !context.explicit) return null;

    const options = [];
    const seen = new Set();
    for (const match of context.state.doc.toString().matchAll(relationPattern)) {
      const relation = cleanIdent(match[1]).split(".").map(cleanIdent).join(".");
      for (const column of columnsByRelation[relation] || []) {
        if (seen.has(column)) continue;
        seen.add(column);
        options.push({ label: column, type: "property", detail: relation, boost: 50 });
      }
    }
    return options.length ? { from: word ? word.from : context.pos, options, validFor: /^[A-Za-z_][\w$]*$/ } : null;
  };
}

// Token colors map to the same CSS custom properties the CM5 themes used, so
// every existing color theme keeps working through the cascade. Prec.highest
// ensures these win over basicSetup's bundled defaultHighlightStyle.
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

// mount(parent, doc, onRun) builds an editor inside `parent` and returns the
// tiny imperative surface web/app.js relies on (getValue / setValue / refresh).
function mount(parent, doc, onRun) {
  const sqlLang = new Compartment();
  const completion = new Compartment();
  const view = new EditorView({
    doc,
    parent,
    extensions: [
      basicSetup,
      sqlLang.of(sql({ dialect: PostgreSQL })),
      completion.of([]),
      EditorView.lineWrapping,
      highlight,
      // Prec.highest so Mod-Enter beats basicSetup's defaultKeymap, which
      // otherwise binds Mod-Enter to insertBlankLine and swallows the event.
      Prec.highest(keymap.of([
        { key: "Mod-Enter", preventDefault: true, run: () => { onRun(); return true; } },
      ])),
    ],
  });
  return {
    getValue: () => view.state.doc.toString(),
    setValue: (v) => view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: v } }),
    refresh: () => view.requestMeasure(),
    setSQLConfig: (config) => view.dispatch({
      effects: [
        sqlLang.reconfigure(sql({ dialect: PostgreSQL, ...config })),
        completion.reconfigure(PostgreSQL.language.data.of({ autocomplete: columnCompletionSource(config.columnsByRelation) })),
      ],
    }),
  };
}

window.cm6 = { mount };
