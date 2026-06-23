// pgpeek UI — Preact + htm, no build step. Vendored Preact/htm keeps the app a
// set of static files embedded in the Go binary, CSP-safe (no eval), with the
// reactivity that the imperative version was outgrowing.
import {
  html, render, useState, useEffect, useRef, useCallback,
} from "./vendor/preact-htm.js";

const PAGE_SIZE = 100;

const THEME_KEY = "pgpeek-theme";
// Switchable color themes. id "" = built-in default (the :root palette).
const THEMES = [
  ["", "Default"],
  ["dark-plus", "Dark+"],
  ["light-plus", "Light+"],
  ["monokai", "Monokai"],
  ["dracula", "Dracula"],
  ["one-dark", "One Dark Pro"],
  ["nord", "Nord"],
  ["solarized-dark", "Solarized Dark"],
  ["solarized-light", "Solarized Light"],
  ["github-dark", "GitHub Dark"],
  ["github-light", "GitHub Light"],
  ["catppuccin-mocha", "Catppuccin Mocha"],
  ["catppuccin-latte", "Catppuccin Latte"],
  ["tokyo-night", "Tokyo Night"],
  ["ayu-dark", "Ayu Dark"],
  ["ayu-mirage", "Ayu Mirage"],
  ["night-owl", "Night Owl"],
  ["houston", "Houston"],
  ["matcha", "Matcha"],
  ["dainty", "Dainty"],
];

function getStoredTheme() {
  try { return localStorage.getItem(THEME_KEY) || ""; } catch { return ""; }
}

// applyTheme sets (or clears) the data-theme attribute that selects a palette.
function applyTheme(id) {
  const root = document.documentElement;
  if (id) root.setAttribute("data-theme", id);
  else root.removeAttribute("data-theme");
}

// Apply the saved theme at import time to avoid a flash of the default palette.
applyTheme(getStoredTheme());

// Allowlisted filter operators (key sent to the server, label shown in the UI).
const OPS = [
  ["", "—"], ["eq", "="], ["ne", "≠"], ["lt", "<"], ["lte", "≤"],
  ["gt", ">"], ["gte", "≥"], ["ilike", "ILIKE"], ["like", "LIKE"],
  ["is_null", "IS NULL"], ["is_not_null", "NOT NULL"],
];
const opNeedsValue = (op) => op !== "" && op !== "is_null" && op !== "is_not_null";

const tablePath = (t) => "/api/tables/" + encodeURIComponent(t.schema) + "/" + encodeURIComponent(t.name);
const tableKey = (t) => t.schema + "." + t.name;

async function getJSON(url) {
  const r = await fetch(url);
  const body = await r.json();
  if (!r.ok) throw new Error(body.error || r.statusText);
  return body;
}

// appendDataParams adds search/sort/filter params shared by browse + export.
function appendDataParams(p, search, sort, filters) {
  if (search) p.set("search", search);
  if (sort) { p.set("sort", sort.col); p.set("dir", sort.dir); }
  for (const col of Object.keys(filters)) {
    const f = filters[col];
    if (!f.op) continue;
    if (opNeedsValue(f.op)) p.append("f", col + ":" + f.op + ":" + (f.value || ""));
    else p.append("f", col + ":" + f.op);
  }
}

function cellText(v) {
  if (v === null || v === undefined) return null;
  return typeof v === "object" ? JSON.stringify(v) : String(v);
}

// Cell renders one value, as an FK link when fkRef is set.
function Cell({ value, fkRef, onNavigate }) {
  const text = cellText(value);
  if (text === null) return html`<td class="null">NULL</td>`;
  if (fkRef) {
    return html`<td><button class="fk" title=${"→ " + fkRef.schema + "." + fkRef.table + "." + fkRef.column}
      onClick=${() => onNavigate(fkRef, value)}>${text}</button></td>`;
  }
  return html`<td>${text}</td>`;
}

function BodyRows({ rows, fkByCol, onNavigate }) {
  return rows.map((row) => html`<tr>${row.map((v, i) => html`<${Cell} value=${v} fkRef=${fkByCol && fkByCol[i]} onNavigate=${onNavigate} />`)}</tr>`);
}

// ---- Sidebar ----
function Sidebar({ tables, loaded, currentKey, onSelect }) {
  const [filter, setFilter] = useState("");
  const f = filter.toLowerCase();
  const items = [];
  let schema = null;
  for (const t of tables) {
    const label = tableKey(t);
    if (f && !label.toLowerCase().includes(f)) continue;
    if (t.schema !== schema) {
      schema = t.schema;
      items.push(html`<div class="schema" key=${"s:" + schema}>${schema}</div>`);
    }
    const cls = "tbl" + (t.type === "view" ? " view" : "") + (label === currentKey ? " active" : "");
    items.push(html`<button class=${cls} key=${label}
      title=${label + (t.estRows >= 0 ? " (~" + t.estRows + " rows)" : "")}
      onClick=${() => onSelect(t)}>${t.name}</button>`);
  }
  return html`
    <aside class="sidebar">
      <input id="tbl-filter" type="search" placeholder="Filter tables…" autocomplete="off"
        value=${filter} onInput=${(e) => setFilter(e.target.value)} />
      <div id="tables">${items.length ? items : html`<div class="empty">${tables.length ? "No tables match." : (loaded ? "No tables." : "Loading tables…")}</div>`}</div>
    </aside>`;
}

// ---- Tabs ----
function Tabs({ tab, setTab, title }) {
  const btn = (id, label) => html`<button id=${"tab-" + id} class=${tab === id ? "active" : ""} onClick=${() => setTab(id)}>${label}</button>`;
  return html`
    <div class="tabs">
      ${btn("data", "Data")} ${btn("structure", "Structure")} ${btn("sql", "SQL")}
      <span class="title" id="tab-title">${title}</span>
    </div>`;
}

// ---- Data tab ----
function DataTab({ table, pageSize, initialFilters, onNavigate, setStatus }) {
  const [offset, setOffset] = useState(0);
  const [search, setSearch] = useState("");
  const [searchBox, setSearchBox] = useState("");
  const [filters, setFilters] = useState(initialFilters || {});
  const [draft, setDraft] = useState(initialFilters || {});
  const [sort, setSort] = useState(null);
  const [data, setData] = useState(null);
  const [fks, setFks] = useState({});

  useEffect(() => {
    let live = true;
    (async () => {
      try {
        const list = await getJSON(tablePath(table) + "/fks");
        if (!live) return;
        const m = {};
        for (const fk of list) m[fk.column] = { schema: fk.refSchema, table: fk.refTable, column: fk.refColumn };
        setFks(m);
      } catch { /* no FK links */ }
    })();
    return () => { live = false; };
  }, [table]);

  useEffect(() => {
    let live = true;
    setStatus({ text: "Loading " + tableKey(table) + "…", cls: "ok" });
    const p = new URLSearchParams();
    p.set("limit", pageSize);
    p.set("offset", offset);
    appendDataParams(p, search, sort, filters);
    (async () => {
      try {
        const d = await getJSON(tablePath(table) + "/data?" + p.toString());
        if (!live) return;
        setData(d);
        setStatus({ text: "✓ " + d.rowCount + " row" + (d.rowCount === 1 ? "" : "s") + " in " + d.elapsedMs + " ms", cls: "ok" });
      } catch (e) {
        if (live) setStatus({ text: "✗ " + e.message, cls: "error" });
      }
    })();
    return () => { live = false; };
  }, [table, offset, search, JSON.stringify(filters), sort && sort.col, sort && sort.dir, pageSize]);

  const applyDraft = useCallback((next) => {
    const clean = {};
    for (const c of Object.keys(next)) if (next[c] && next[c].op) clean[c] = next[c];
    setFilters(clean);
    setOffset(0);
  }, []);

  const toggleSort = (col) => {
    setOffset(0);
    setSort((s) => (s && s.col === col ? { col, dir: s.dir === "asc" ? "desc" : "asc" } : { col, dir: "asc" }));
  };

  const exportURL = () => {
    const p = new URLSearchParams();
    p.set("format", "csv");
    appendDataParams(p, search, sort, filters);
    return tablePath(table) + "/data?" + p.toString();
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
            const d = draft[c] || {};
            return html`<td key=${c}>
              <select class="f-op" data-col=${c} value=${d.op || ""} onChange=${(e) => {
                const next = { ...draft, [c]: { op: e.target.value, value: d.value || "" } };
                setDraft(next); applyDraft(next);
              }}>${OPS.map(([k, label]) => html`<option value=${k}>${label}</option>`)}</select>
              <input class="f-val" data-col=${c} placeholder="filter…" value=${d.value || ""}
                onInput=${(e) => setDraft({ ...draft, [c]: { op: d.op || "", value: e.target.value } })}
                onKeyDown=${(e) => { if (e.key === "Enter") applyDraft({ ...draft, [c]: { op: d.op || "", value: e.target.value } }); }} />
            </td>`;
          })}</tr>
        </thead>
        <tbody><${BodyRows} rows=${data.rows} fkByCol=${fkByCol} onNavigate=${onNavigate} /></tbody>
      </table>
      ${data.rows.length ? "" : html`<div class="empty">0 rows.</div>`}`;
  }

  const rowCount = data && data.rowCount ? data.rowCount : 0;
  const from = rowCount ? offset + 1 : 0;

  return html`
    <div class="toolbar">
      <input id="data-search" type="search" placeholder="Search all columns…" autocomplete="off"
        value=${searchBox} onInput=${(e) => setSearchBox(e.target.value)}
        onKeyDown=${(e) => { if (e.key === "Enter") { setOffset(0); setSearch(searchBox.trim()); } }} />
      <button class="ghost" id="data-clear" onClick=${() => {
        setSearch(""); setSearchBox(""); setFilters({}); setDraft({}); setSort(null); setOffset(0);
      }}>Clear</button>
      <span class="sep"></span>
      <button class="ghost" id="prev-btn" disabled=${offset === 0} onClick=${() => setOffset(Math.max(0, offset - pageSize))}>◀ Prev</button>
      <button class="ghost" id="next-btn" disabled=${rowCount < pageSize} onClick=${() => setOffset(offset + pageSize)}>Next ▶</button>
      <span class="page-info" id="page-info">${from}–${offset + rowCount}</span>
      <a class="ghost btn" id="data-export-btn" role="button" href=${exportURL()} download=${table.name + ".csv"}>Export CSV</a>
    </div>
    <div class="results" id="data-results">${grid}</div>`;
}

// ---- Structure tab ----
function StructureTab({ table, setStatus }) {
  const [cols, setCols] = useState(null);
  useEffect(() => {
    let live = true;
    (async () => {
      try {
        const c = await getJSON(tablePath(table) + "/columns");
        if (live) setCols(c);
      } catch (e) {
        if (live) setStatus({ text: "✗ " + e.message, cls: "error" });
      }
    })();
    return () => { live = false; };
  }, [table]);

  let body;
  if (cols === null) body = html`<div class="empty">Loading…</div>`;
  else if (!cols.length) body = html`<div class="empty">No columns.</div>`;
  else body = html`<table>
    <tr><th>Column</th><th>Type</th><th>Nullable</th><th>Default</th></tr>
    ${cols.map((c) => html`<tr key=${c.name}><td>${c.name}</td><td>${c.type}</td>
      <td>${c.nullable ? "YES" : "NO"}</td><td>${c.default == null ? "" : c.default}</td></tr>`)}
  </table>`;
  return html`<div class="results" id="structure-results">${body}</div>`;
}

// ---- SQL tab ----
function SqlTab({ active, saved, reloadSaved, setStatus }) {
  const wrapRef = useRef();
  const taRef = useRef();
  const cmRef = useRef();
  const [result, setResult] = useState(null);
  const [lastSQL, setLastSQL] = useState("");
  const [selected, setSelected] = useState("");
  const [running, setRunning] = useState(false);
  const runningRef = useRef(false);

  const getSQL = () => (cmRef.current ? cmRef.current.getValue() : taRef.current.value).trim();
  const setSQL = (v) => { if (cmRef.current) cmRef.current.setValue(v); else taRef.current.value = v; };

  const run = useCallback(async () => {
    const sql = getSQL();
    if (!sql) return;
    if (runningRef.current) return;
    runningRef.current = true; setRunning(true);
    setStatus({ text: "Running…", cls: "ok" });
    try {
      const r = await fetch("/api/query", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ sql }) });
      const d = await r.json();
      if (!r.ok) { setStatus({ text: "✗ " + (d.error || r.statusText), cls: "error" }); setResult(null); return; }
      setLastSQL(sql); setResult(d);
      const base = "✓ " + d.rowCount + " row" + (d.rowCount === 1 ? "" : "s") + " in " + d.elapsedMs + " ms";
      setStatus(d.truncated ? { text: base, cls: "ok", warn: "· capped (more rows available — add LIMIT or refine)" } : { text: base, cls: "ok" });
    } catch (e) {
      setStatus({ text: "✗ " + e.message, cls: "error" });
    } finally {
      runningRef.current = false; setRunning(false);
    }
  }, []);

  // Init CodeMirror once, into a Preact-stable wrapper it fully owns.
  useEffect(() => {
    const ta = document.createElement("textarea");
    ta.id = "sql";
    ta.value = "SELECT now();";
    wrapRef.current.appendChild(ta);
    taRef.current = ta;
    if (window.CodeMirror) {
      cmRef.current = window.CodeMirror.fromTextArea(ta, { mode: "text/x-pgsql", lineNumbers: true, lineWrapping: true });
      cmRef.current.setOption("extraKeys", { "Cmd-Enter": run, "Ctrl-Enter": run });
    } else {
      ta.addEventListener("keydown", (e) => { if ((e.metaKey || e.ctrlKey) && e.key === "Enter") { e.preventDefault(); run(); } });
    }
  }, []);

  // CodeMirror was created while hidden (zero size); refresh when shown.
  useEffect(() => { if (active && cmRef.current) cmRef.current.refresh(); }, [active]);

  const exportCSV = async () => {
    const sql = lastSQL || getSQL();
    if (!sql) return;
    const r = await fetch("/api/export", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ sql }) });
    if (!r.ok) { const d = await r.json().catch(() => ({})); setStatus({ text: "✗ " + (d.error || "export failed"), cls: "error" }); return; }
    const url = URL.createObjectURL(await r.blob());
    const a = document.createElement("a"); a.href = url; a.download = "pgpeek-export.csv"; a.click();
    URL.revokeObjectURL(url);
  };

  const onPick = (e) => {
    const id = e.target.value; setSelected(id);
    const q = saved.find((x) => String(x.id) === id);
    if (q) { setSQL(q.sql); setStatus({ text: "Loaded “" + q.name + "”. Press Run.", cls: "ok" }); }
  };
  const selectedQ = saved.find((x) => String(x.id) === selected);

  const onSave = async () => {
    const sql = getSQL();
    if (!sql) return;
    const name = prompt("Name for this saved query:");
    if (!name) return;
    const description = prompt("Description (optional):") || "";
    const r = await fetch("/api/queries", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ name, description, sql }) });
    const d = await r.json();
    if (!r.ok) { setStatus({ text: "✗ " + (d.error || "save failed"), cls: "error" }); return; }
    await reloadSaved(); setSelected(String(d.id));
    setStatus({ text: "✓ Saved “" + d.name + "”.", cls: "ok" });
  };

  const onDelete = async () => {
    if (!selectedQ) return;
    if (!confirm("Delete saved query “" + selectedQ.name + "”?")) return;
    const r = await fetch("/api/queries/" + selectedQ.id, { method: "DELETE" });
    if (!r.ok && r.status !== 204) { setStatus({ text: "✗ delete failed", cls: "error" }); return; }
    await reloadSaved(); setSelected("");
    setStatus({ text: "✓ Deleted.", cls: "ok" });
  };

  const presets = saved.filter((q) => q.isPreset);
  const mine = saved.filter((q) => !q.isPreset);

  return html`
    <div class="editor-wrap" ref=${wrapRef}></div>
    <div class="toolbar">
      <button class="primary" id="run-btn" disabled=${running} onClick=${run}>Run ▶</button>
      <button class="ghost" id="sql-export-btn" disabled=${running || !result || result.rowCount === 0} onClick=${exportCSV}>Export CSV</button>
      <select id="presets" title="Saved & preset queries" value=${selected} onChange=${onPick}>
        <option value="">Saved queries…</option>
        ${presets.length ? html`<optgroup label="Presets">${presets.map((q) => html`<option key=${q.id} value=${q.id}>${q.name}</option>`)}</optgroup>` : ""}
        ${mine.length ? html`<optgroup label="Saved">${mine.map((q) => html`<option key=${q.id} value=${q.id}>${q.name}</option>`)}</optgroup>` : ""}
      </select>
      <button class="ghost" id="save-btn" onClick=${onSave}>Save</button>
      <button class="ghost" id="delete-btn" disabled=${!(selectedQ && !selectedQ.isPreset)} onClick=${onDelete}>Delete</button>
      <span class="hint">Ctrl/Cmd\u00a0+\u00a0Enter to run · single SELECT/WITH only</span>
    </div>
    <div class="results" id="sql-results">
      ${result ? (result.columns.length === 0 ? html`<div class="empty">Query ran. No columns returned.</div>`
        : (result.rows.length === 0 ? html`<div class="empty">0 rows.</div>`
          : html`<table><thead><tr>${result.columns.map((c) => html`<th key=${c}>${c}</th>`)}</tr></thead>
              <tbody><${BodyRows} rows=${result.rows} /></tbody></table>`))
        : html`<div class="empty">Run a query to see results.</div>`}
    </div>`;
}

// ---- Theme switcher ----
function ThemeSelect() {
  const [theme, setTheme] = useState(getStoredTheme);
  useEffect(() => {
    applyTheme(theme);
    try { localStorage.setItem(THEME_KEY, theme); } catch { /* persistence is best-effort */ }
  }, [theme]);
  return html`
    <label class="theme-select" title="Color theme">Theme
      <select id="theme-select" value=${theme} onChange=${(e) => setTheme(e.target.value)}>
        ${THEMES.map(([id, label]) => html`<option value=${id}>${label}</option>`)}
      </select>
    </label>`;
}

// ---- App ----
function App() {
  const [tables, setTables] = useState([]);
  const [tablesLoaded, setTablesLoaded] = useState(false);
  const [rowCap, setRowCap] = useState(PAGE_SIZE);
  const [saved, setSaved] = useState([]);
  const [tab, setTab] = useState("data");
  const [current, setCurrent] = useState(null);
  const [navKey, setNavKey] = useState(0);
  const [pendingFilters, setPendingFilters] = useState(null);
  const [status, setStatus] = useState({ text: "Ready.", cls: "ok" });

  const reloadSaved = useCallback(async () => {
    try { setSaved(await getJSON("/api/queries")); }
    catch (e) { setStatus({ text: "✗ failed to load saved queries: " + e.message, cls: "error" }); }
  }, []);

  useEffect(() => {
    (async () => { try { setTables(await getJSON("/api/tables")); } catch (e) { setStatus({ text: "✗ failed to load tables: " + e.message, cls: "error" }); } finally { setTablesLoaded(true); } })();
    (async () => { try { const m = await getJSON("/api/meta"); if (m && m.rowCap > 0) setRowCap(m.rowCap); } catch { /* default */ } })();
    reloadSaved();
  }, []);

  const open = (t, initialFilters) => {
    setCurrent(t); setPendingFilters(initialFilters || null);
    setNavKey((n) => n + 1); setTab("data");
  };
  const onNavigate = (ref, value) => {
    const target = tables.find((x) => x.schema === ref.schema && x.name === ref.table);
    if (!target) { setStatus({ text: "✗ referenced table " + ref.schema + "." + ref.table + " is not browsable", cls: "error" }); return; }
    open(target, { [ref.column]: { op: "eq", value: String(value) } });
  };

  const pageSize = Math.min(PAGE_SIZE, rowCap);
  const title = current ? tableKey(current) : "Pick a table";

  return html`
    <header><h1>pgpeek</h1><span class="badge">read-only</span><${ThemeSelect} /></header>
    <div class="body">
      <${Sidebar} tables=${tables} loaded=${tablesLoaded} currentKey=${current && tableKey(current)} onSelect=${(t) => open(t)} />
      <main>
        <${Tabs} tab=${tab} setTab=${setTab} title=${title} />
        <section class="panel" id="panel-data" hidden=${tab !== "data"}>
          ${current
            ? html`<${DataTab} key=${navKey} table=${current} pageSize=${pageSize}
                initialFilters=${pendingFilters} onNavigate=${onNavigate} setStatus=${setStatus} />`
            : html`<div class="results"><div class="empty">Select a table to browse its rows.</div></div>`}
        </section>
        <section class="panel" id="panel-structure" hidden=${tab !== "structure"}>
          ${current && tab === "structure"
            ? html`<${StructureTab} key=${"s" + navKey} table=${current} setStatus=${setStatus} />`
            : html`<div class="results"><div class="empty">Select a table to see its structure.</div></div>`}
        </section>
        <section class="panel" id="panel-sql" hidden=${tab !== "sql"}>
          <${SqlTab} active=${tab === "sql"} saved=${saved} reloadSaved=${reloadSaved} setStatus=${setStatus} />
        </section>
      </main>
    </div>
    <div class=${"status " + status.cls} id="status">${status.text}${status.warn ? html`<span class="warn"> ${status.warn}</span>` : ""}</div>`;
}

render(html`<${App} />`, document.getElementById("app"));
