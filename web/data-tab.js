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

function cellFullText(v) {
  return typeof v === "object" ? JSON.stringify(v, null, 2) : String(v);
}

const PREVIEW_CHARS = 220;
const MATCH_CONTEXT = 90;

function cleanTerm(term) {
  return String(term || "").trim().replace(/^%+/, "").replace(/%+$/, "");
}

function filterTerm(filter) {
  if (!filter || !filter.op || filter.op === "is_null" || filter.op === "is_not_null") return "";
  return cleanTerm(filter.value);
}

function previewText(text, term) {
  const needle = cleanTerm(term);
  const match = needle ? text.toLowerCase().indexOf(needle.toLowerCase()) : -1;
  if (match >= 0 && text.length > PREVIEW_CHARS) {
    const start = Math.max(0, match - MATCH_CONTEXT);
    const end = Math.min(text.length, match + needle.length + MATCH_CONTEXT);
    return (start ? "…" : "") + text.slice(start, end) + (end < text.length ? "…" : "");
  }
  if (text.length <= PREVIEW_CHARS) return text;
  return text.slice(0, PREVIEW_CHARS) + "…";
}

function highlightText(text, term) {
  const needle = cleanTerm(term);
  if (!needle) return text;
  const match = text.toLowerCase().indexOf(needle.toLowerCase());
  if (match < 0) return text;
  return [
    text.slice(0, match),
    html`<mark>${text.slice(match, match + needle.length)}</mark>`,
    text.slice(match + needle.length),
  ];
}

function Cell({ value, column, term, fkRef, onNavigate }) {
  const text = cellText(value);
  if (text === null) return html`<td class="null cell">NULL</td>`;
  const fullText = cellFullText(value);
  const preview = previewText(text, term);
  const body = highlightText(preview, term);
  const long = text.length > PREVIEW_CHARS || fullText.length > PREVIEW_CHARS || fullText.includes("\n");
  if (fkRef) {
    return html`<td class="cell" title=${text}><button class="fk"
      title=${"→ " + fkRef.schema + "." + fkRef.table + "." + fkRef.column}
      onClick=${() => onNavigate(fkRef, value)}>${body}</button></td>`;
  }
  if (!long) return html`<td class="cell" title=${text}><span class="cell-preview">${body}</span></td>`;
  return html`<td class="cell cell-long" title="Expand to read full value">
    <details class="cell-detail">
      <summary aria-label=${"Show full value for " + column}>
        <span class="cell-preview">${body}</span>
      </summary>
      <pre>${fullText}</pre>
    </details>
  </td>`;
}

function BodyRows({ rows, columns, fkByCol, termsByCol, onNavigate }) {
  return rows.map((row) =>
    html`<tr>${row.map((v, i) =>
      html`<${Cell} value=${v} column=${columns[i]} term=${termsByCol && termsByCol[i]} fkRef=${fkByCol && fkByCol[i]} onNavigate=${onNavigate} />`)}</tr>`);
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
    const globalTerm = cleanTerm(search);
    const termsByCol = data.columns.map((c) => filterTerm(filterFor(filters, c)) || globalTerm);
    grid = html`
      <table>
        <thead>
          <tr>${data.columns.map((c) => html`<th class="sortable" key=${c} title=${c} tabindex="0"
            aria-sort=${sort && sort.col === c ? (sort.dir === "desc" ? "descending" : "ascending") : "none"}
            onClick=${() => toggleSort(c)} onKeyDown=${(e) => {
              if (e.key === "Enter" || e.key === " ") { e.preventDefault(); toggleSort(c); }
            }}>
            ${c}${sort && sort.col === c ? (sort.dir === "desc" ? " ▼" : " ▲") : ""}</th>`)}</tr>
          <tr class="filter-row">${data.columns.map((c) => {
            const d = filterFor(draft, c);
            return html`<td key=${c}>
              <select class="f-op" data-col=${c} aria-label=${"Filter operator for " + c} value=${d.op || ""} onChange=${(e) => {
                const next = setFilterValue(draft, c, { op: e.target.value, value: d.value || "" });
                setDraft(next); applyDraft(next);
              }}>${OPS.map(([k, label]) => html`<option value=${k}>${label}</option>`)}</select>
              <input class="f-val" data-col=${c} aria-label=${"Filter value for " + c} placeholder="filter…" value=${d.value || ""}
                onInput=${(e) => setDraft(setFilterValue(draft, c, { op: d.op || "", value: e.target.value }))}
                onKeyDown=${(e) => {
                  if (e.key === "Enter") applyDraft(setFilterValue(draft, c, { op: d.op || "", value: e.target.value }));
                }} />
            </td>`;
          })}</tr>
        </thead>
        <tbody><${BodyRows} rows=${data.rows} columns=${data.columns} fkByCol=${fkByCol} termsByCol=${termsByCol} onNavigate=${onNavigate} /></tbody>
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
