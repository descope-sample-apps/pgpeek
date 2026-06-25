// DataTab component — paged table rows with search, sort, filters, FK links,
// and CSV export. Calls onStateChange(state) whenever URL-trackable state
// changes so App can replaceState in the browser history.
import { html, useState, useEffect, useCallback } from "./vendor/preact-htm.js";
import { getJSON, tablePath, tableKey, appendDataParams, dbUrl } from "./api.js";

// Allowlisted filter operators.
const OPS = [
  ["", "—"], ["eq", "="], ["ne", "≠"], ["lt", "<"], ["lte", "≤"],
  ["gt", ">"], ["gte", "≥"], ["ilike", "ILIKE"], ["like", "LIKE"],
  ["is_null", "IS NULL"], ["is_not_null", "NOT NULL"],
];

function filterFor(filters, column) {
  return filters.find((f) => f.column === column) || {};
}

function setFilterValue(filters, column, patch) {
  const next = filters.filter((f) => f.column !== column);
  next.push({ column, ...patch });
  return next;
}

function cellText(v) {
  if (v === null || v === undefined) return null;
  return typeof v === "object" ? JSON.stringify(v) : String(v);
}

function Cell({ value, fkRef, onNavigate }) {
  const text = cellText(value);
  if (text === null) return html`<td class="null">NULL</td>`;
  if (fkRef) {
    return html`<td><button class="fk"
      title=${"→ " + fkRef.schema + "." + fkRef.table + "." + fkRef.column}
      onClick=${() => onNavigate(fkRef, value)}>${text}</button></td>`;
  }
  return html`<td>${text}</td>`;
}

function BodyRows({ rows, fkByCol, onNavigate }) {
  return rows.map((row) =>
    html`<tr>${row.map((v, i) =>
      html`<${Cell} value=${v} fkRef=${fkByCol && fkByCol[i]} onNavigate=${onNavigate} />`)}</tr>`);
}

export function DataTab({
  table, pageSize, dbId,
  initialFilters, initialOffset, initialSearch, initialSort,
  onNavigate, setStatus, onStateChange,
}) {
  const [offset, setOffset] = useState(initialOffset || 0);
  const [search, setSearch] = useState(initialSearch || "");
  const [searchBox, setSearchBox] = useState(initialSearch || "");
  const [filters, setFilters] = useState(initialFilters || []);
  const [draft, setDraft] = useState(initialFilters || []);
  const [sort, setSort] = useState(initialSort || null);
  const [data, setData] = useState(null);
  const [fks, setFks] = useState({});

  // Notify App of URL-trackable state so it can replaceState.
  const notify = useCallback((upd) => {
    onStateChange(upd);
  }, [onStateChange]);

  useEffect(() => {
    let live = true;
    (async () => {
      try {
        const list = await getJSON(tablePath(table) + "/fks", dbId);
        if (!live) return;
        const m = {};
        for (const fk of list) m[fk.column] = { schema: fk.refSchema, table: fk.refTable, column: fk.refColumn };
        setFks(m);
      } catch { /* no FK links */ }
    })();
    return () => { live = false; };
  }, [table, dbId]);

  useEffect(() => {
    let live = true;
    setStatus({ text: "Loading " + tableKey(table) + "…", cls: "ok" });
    const p = new URLSearchParams();
    p.set("limit", pageSize);
    p.set("offset", offset);
    appendDataParams(p, search, sort, filters);
    (async () => {
      try {
        const d = await getJSON(tablePath(table) + "/data?" + p.toString(), dbId);
        if (!live) return;
        setData(d);
        setStatus({ text: "✓ " + d.rowCount + " row" + (d.rowCount === 1 ? "" : "s") + " in " + d.elapsedMs + " ms", cls: "ok" });
      } catch (e) {
        if (live) setStatus({ text: "✗ " + e.message, cls: "error" });
      }
    })();
    return () => { live = false; };
  }, [table, dbId, offset, search, JSON.stringify(filters), sort && sort.col, sort && sort.dir, pageSize]);

  const applyDraft = useCallback((next) => {
    const clean = next.filter((f) => f.op);
    setFilters(clean);
    setOffset(0);
    notify({ schema: table.schema, table: table.name, offset: 0, search, sort, filters: clean });
  }, [search, sort, table, notify]);

  const toggleSort = (col) => {
    setOffset(0);
    const s = sort && sort.col === col
      ? { col, dir: sort.dir === "asc" ? "desc" : "asc" }
      : { col, dir: "asc" };
    setSort(s);
    notify({ schema: table.schema, table: table.name, offset: 0, search, sort: s, filters });
  };

  const exportURL = () => {
    const p = new URLSearchParams();
    p.set("format", "csv");
    appendDataParams(p, search, sort, filters);
    return dbUrl(tablePath(table) + "/data?" + p.toString(), dbId);
  };

  let grid;
  if (!data || data.columns === undefined) {
    grid = html`<div class="empty">Loading…</div>`;
  } else if (!data.columns.length) {
    grid = html`<div class="empty">No columns.</div>`;
  } else {
    const fkByCol = data.columns.map((c) => fks[c] || null);
    grid = html`
      <table>
        <thead>
          <tr>${data.columns.map((c) => html`<th class="sortable" key=${c} onClick=${() => toggleSort(c)}>
            ${c}${sort && sort.col === c ? (sort.dir === "desc" ? " ▼" : " ▲") : ""}</th>`)}</tr>
          <tr class="filter-row">${data.columns.map((c) => {
            const d = filterFor(draft, c);
            return html`<td key=${c}>
              <select class="f-op" data-col=${c} value=${d.op || ""} onChange=${(e) => {
                const next = setFilterValue(draft, c, { op: e.target.value, value: d.value || "" });
                setDraft(next); applyDraft(next);
              }}>${OPS.map(([k, label]) => html`<option value=${k}>${label}</option>`)}</select>
              <input class="f-val" data-col=${c} placeholder="filter…" value=${d.value || ""}
                onInput=${(e) => setDraft(setFilterValue(draft, c, { op: d.op || "", value: e.target.value }))}
                onKeyDown=${(e) => {
                  if (e.key === "Enter") applyDraft(setFilterValue(draft, c, { op: d.op || "", value: e.target.value }));
                }} />
            </td>`;
          })}</tr>
        </thead>
        <tbody><${BodyRows} rows=${data.rows} fkByCol=${fkByCol} onNavigate=${onNavigate} /></tbody>
      </table>
      ${data.rows.length ? "" : html`<div class="empty">0 rows.</div>`}`;
  }

  const rowCount = data && data.rowCount ? data.rowCount : 0;
  const from = rowCount ? offset + 1 : 0;

  const goOffset = (n) => {
    setOffset(n);
    notify({ schema: table.schema, table: table.name, offset: n, search, sort, filters });
  };

  const doSearch = (s) => {
    setOffset(0); setSearch(s);
    notify({ schema: table.schema, table: table.name, offset: 0, search: s, sort, filters });
  };

  return html`
    <div class="toolbar">
      <input id="data-search" type="search" placeholder="Search all columns…" autocomplete="off"
        value=${searchBox} onInput=${(e) => setSearchBox(e.target.value)}
        onKeyDown=${(e) => {
          if (e.key === "Enter") doSearch(searchBox.trim());
          else if (e.key === "Escape") doSearch("");
        }} />
      <button class="ghost" id="data-clear" onClick=${() => {
        setSearch(""); setSearchBox(""); setFilters([]); setDraft([]); setSort(null); setOffset(0);
        notify({ schema: table.schema, table: table.name, offset: 0, search: "", sort: null, filters: [] });
      }}>Clear</button>
      <span class="sep"></span>
      <button class="ghost" id="prev-btn" disabled=${offset === 0}
        onClick=${() => goOffset(Math.max(0, offset - pageSize))}>◀ Prev</button>
      <button class="ghost" id="next-btn" disabled=${rowCount < pageSize}
        onClick=${() => goOffset(offset + pageSize)}>Next ▶</button>
      <span class="page-info" id="page-info">${from}–${offset + rowCount}</span>
      <a class="action secondary" id="data-export-btn" role="button"
        href=${exportURL()} download=${table.name + ".csv"}>Export CSV</a>
    </div>
    <div class="results" id="data-results">${grid}</div>`;
}
