// StructureTab — column metadata for the selected table.
import { html, useState, useEffect } from "./vendor/preact-htm.js";
import { getJSON, tablePath } from "./api.js";
import { shuniColumns, SHUNI_STATUS } from "./easter-eggs.js";

export function StructureTab({ table, dbId, setStatus }) {
  const [cols, setCols] = useState(null);
  useEffect(() => {
    setStatus({ text: "Loading structure for " + table.schema + "." + table.name + "…", cls: "ok" });
    // Easter egg: the shuni view is fictional — serve its columns locally.
    const egg = shuniColumns(table);
    if (egg) {
      setCols(egg);
      setStatus({ text: SHUNI_STATUS, cls: "ok" });
      return;
    }
    let live = true;
    (async () => {
      try {
        const c = await getJSON(tablePath(table) + "/columns", dbId);
        if (live) {
          setCols(c);
          setStatus({ text: "✓ " + c.length + " column" + (c.length === 1 ? "" : "s") + " loaded", cls: "ok" });
        }
      } catch (e) {
        if (live) setStatus({ text: "✗ " + e.message, cls: "error" });
      }
    })();
    return () => { live = false; };
  }, [table, dbId]);

  let body;
  if (cols === null) body = html`<div class="empty">Loading…</div>`;
  else if (!cols.length) body = html`<div class="empty">No columns.</div>`;
  else body = html`<table>
    <thead><tr><th>Column</th><th>Type</th><th>Nullable</th><th>Default</th></tr></thead>
    <tbody>${cols.map((c) => html`<tr key=${c.name}><td>${c.name}</td><td>${c.type}</td>
      <td>${c.nullable ? "YES" : "NO"}</td><td>${c.default == null ? "" : c.default}</td></tr>`)}</tbody>
  </table>`;
  return html`<div class="results" id="structure-results">${body}</div>`;
}
