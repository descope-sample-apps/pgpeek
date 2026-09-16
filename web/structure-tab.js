// StructureTab — column metadata for the selected table.
import { html, useState, useEffect } from "./vendor/preact-htm.js";
import { getJSON, tablePath } from "./api.js";
import { shuniColumns, SHUNI_STATUS, SHUNI_VIEW_QUERY } from "./easter-eggs.js";

export function StructureTab({ table, dbId, setStatus }) {
  const [structure, setStructure] = useState(null);
  useEffect(() => {
    setStatus({ text: "Loading structure for " + table.schema + "." + table.name + "…", cls: "ok" });
    // Easter egg: the shuni view is fictional — serve its columns locally.
    const egg = shuniColumns(table);
    if (egg) {
      setStructure({ columns: egg, query: SHUNI_VIEW_QUERY });
      setStatus({ text: SHUNI_STATUS, cls: "ok" });
      return;
    }
    let live = true;
    (async () => {
      try {
        const path = tablePath(table);
        const definitionPromise = table.type === "view"
          ? getJSON(path + "/definition", dbId).then(
            (definition) => ({ query: definition.query, error: null }),
            (error) => ({ query: null, error: error.message }),
          )
          : Promise.resolve({ query: null, error: null });
        const [columns, definition] = await Promise.all([
          getJSON(path + "/columns", dbId),
          definitionPromise,
        ]);
        if (live) {
          setStructure({ columns, query: definition.query, definitionError: definition.error });
          setStatus(definition.error
            ? { text: "✗ " + definition.error, cls: "error" }
            : { text: "✓ " + columns.length + " column" + (columns.length === 1 ? "" : "s") + " loaded", cls: "ok" });
        }
      } catch (e) {
        if (live) setStatus({ text: "✗ " + e.message, cls: "error" });
      }
    })();
    return () => { live = false; };
  }, [table, dbId]);

  const cols = structure?.columns ?? null;
  let columns;
  if (cols === null) columns = html`<div class="empty">Loading…</div>`;
  else if (!cols.length) columns = html`<div class="empty">No columns.</div>`;
  else columns = html`<table>
    <thead><tr><th>Column</th><th>Type</th><th>Nullable</th><th>Default</th></tr></thead>
    <tbody>${cols.map((c) => html`<tr key=${c.name}><td>${c.name}</td><td>${c.type}</td>
      <td>${c.nullable ? "YES" : "NO"}</td><td>${c.default == null ? "" : c.default}</td></tr>`)}</tbody>
  </table>`;
  const definition = structure?.definitionError
    ? html`<section class="view-definition"><h2>View query</h2>
        <div class="view-definition-error">Query unavailable: ${structure.definitionError}</div></section>`
    : structure?.query == null ? null : html`<section class="view-definition">
        <h2>View query</h2>
        <pre><code>${structure.query}</code></pre>
      </section>`;
  return html`<div class="results" id="structure-results">${definition}${columns}</div>`;
}
