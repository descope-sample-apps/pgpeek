import { dbUrl } from "./api.js";
import { responseError } from "./sql-helpers.js";
import { html, useEffect, useRef, useState } from "./vendor/preact-htm.js";
export { DEFAULT_SQL, queryStatusText, responseError } from "./sql-helpers.js";

function cellText(value) {
  return typeof value === "object" ? JSON.stringify(value) : String(value);
}

export function LazyCell({ value, truncated, columnName, loadValue, onFullValue }) {
  const [full, setFull] = useState(null);
  const [loading, setLoading] = useState(false);
  const [loadError, setLoadError] = useState("");
  const [open, setOpen] = useState(false);
  const requestRef = useRef(null);
  useEffect(() => () => requestRef.current?.abort(), []);
  if (value === null) return html`<td class="cell"><span class="null">NULL</span></td>`;
  if (!truncated) return html`<td class="cell">${cellText(value)}</td>`;

  const load = async (event) => {
    const expanded = event.currentTarget.open;
    setOpen(expanded);
    if (!expanded || full !== null || loading) return;
    const controller = new AbortController();
    requestRef.current = controller;
    setLoading(true); setLoadError("");
    try {
      const data = await loadValue(controller.signal);
      if (controller.signal.aborted) return;
      setFull(data.value);
    } catch (error) {
      if (controller.signal.aborted) return;
      setLoadError(error instanceof Error ? error.message : "cell fetch failed");
    } finally {
      if (!controller.signal.aborted) setLoading(false);
    }
  };

  return html`<td class="cell cell-long">
    <details class="cell-detail" onToggle=${load}>
      <summary aria-label=${(open ? "Hide" : "Show") + " full value for " + columnName}>
        <span class="cell-preview">${value}</span>
        <span class="cell-toggle" aria-hidden="true">${open ? "show less" : "show more..."}</span>
      </summary>
      <pre>${loading
        ? html`<span role="status" aria-live="polite">Loading…</span>`
        : loadError
          ? html`<span class="cell-load-error" role="alert">${loadError}</span>`
          : (full === null ? "" : cellText(full))}</pre>
      ${full !== null && onFullValue ? html`<button class="fk cell-full-action" onClick=${() => onFullValue(full)}>Open referenced row</button>` : null}
    </details>
  </td>`;
}

export function SqlResults({ result, resultKey, sql, dbId }) {
  if (!result) return html`<div class="empty">Preview a query to see results.</div>`;
  if (result.columns.length === 0) return html`<div class="empty">Query ran. No columns returned.</div>`;
  if (result.rows.length === 0) return html`<div class="empty">0 rows.</div>`;
  const truncated = new Map((result.truncatedCells || []).map((cell) => [cell.row + ":" + cell.column, cell]));
  return html`<table>
    <thead><tr>${result.columns.map((column) => html`<th key=${column}>${column}</th>`)}</tr></thead>
    <tbody>${result.rows.map((row, rowIndex) =>
      html`<tr key=${resultKey + ":" + rowIndex}>${row.map((value, columnIndex) =>
        html`<${LazyCell} key=${resultKey + ":" + columnIndex} value=${value} truncated=${truncated.has(rowIndex + ":" + columnIndex)} columnName=${result.columns[columnIndex]}
          loadValue=${async (signal) => {
            const response = await fetch(dbUrl("/api/query/cell", dbId), {
              method: "POST",
              headers: { "Content-Type": "application/json" },
              body: JSON.stringify({ sql, row: rowIndex, column: columnIndex, hash: truncated.get(rowIndex + ":" + columnIndex)?.hash || "" }),
              signal,
            });
            if (!response.ok) throw new Error(await responseError(response, "cell fetch failed"));
            return response.json();
          }} />`)}</tr>`)}</tbody>
  </table>`;
}
