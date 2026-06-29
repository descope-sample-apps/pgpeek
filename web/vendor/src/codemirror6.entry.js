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
  const view = new EditorView({
    doc,
    parent,
    extensions: [
      basicSetup,
      sqlLang.of(sql({ dialect: PostgreSQL })),
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
      effects: sqlLang.reconfigure(sql({ dialect: PostgreSQL, ...config })),
    }),
  };
}

window.cm6 = { mount };
