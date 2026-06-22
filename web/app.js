// pgpeek UI. Kept in a separate file so the Content-Security-Policy can forbid
// inline scripts (script-src 'self' + the CodeMirror CDN, no 'unsafe-inline').
(function () {
  "use strict";
  const $ = (id) => document.getElementById(id);
  const PAGE_SIZE = 100;

  const statusEl = $("status");
  let editor = null;
  let savedQueries = [];
  let tables = [];
  let current = null; // {schema, name, type}
  let offset = 0;
  let lastSQL = "";
  let pageSize = PAGE_SIZE; // narrowed to the server row cap once /api/meta loads
  let dataSeq = 0; // request tokens: ignore responses superseded by a newer click
  let structSeq = 0;
  let searchTerm = "";
  let filters = {}; // column -> {op, value}
  let sortCol = null;
  let sortDir = "asc";
  let currentFKs = {}; // column -> {schema, table, column} for the open table

  // Allowlisted filter operators (key sent to the server, label shown in the UI).
  const OPS = [
    ["", "—"], ["eq", "="], ["ne", "≠"], ["lt", "<"], ["lte", "≤"],
    ["gt", ">"], ["gte", "≥"], ["ilike", "ILIKE"], ["like", "LIKE"],
    ["is_null", "IS NULL"], ["is_not_null", "NOT NULL"],
  ];
  const opNeedsValue = (op) => op !== "" && op !== "is_null" && op !== "is_not_null";

  function setStatus(msg, cls) { statusEl.className = "status " + cls; statusEl.textContent = msg; }
  function setStatusHTML(html, cls) { statusEl.className = "status " + cls; statusEl.innerHTML = html; }
  function empty(text) { const d = document.createElement("div"); d.className = "empty"; d.textContent = text; return d; }

  function cellEl(cell, ref) {
    const td = document.createElement("td");
    if (cell === null || cell === undefined) { td.className = "null"; td.textContent = "NULL"; return td; }
    const text = typeof cell === "object" ? JSON.stringify(cell) : String(cell);
    if (ref) {
      const link = document.createElement("button");
      link.className = "fk";
      link.textContent = text;
      link.title = "→ " + ref.schema + "." + ref.table + "." + ref.column;
      link.addEventListener("click", () => openRef(ref, cell));
      td.append(link);
    } else {
      td.textContent = text;
    }
    return td;
  }

  // fkByCol maps a column index to its FK ref (or null) so cells become links.
  function bodyRows(res, fkByCol) {
    const tbody = document.createElement("tbody");
    for (const row of res.rows) {
      const tr = document.createElement("tr");
      row.forEach((cell, i) => tr.append(cellEl(cell, fkByCol && fkByCol[i])));
      tbody.append(tr);
    }
    return tbody;
  }

  // Plain tabular renderer for SQL query results.
  function renderGrid(el, res) {
    el.replaceChildren();
    if (!res.columns.length) { el.append(empty("Query ran. No columns returned.")); return; }
    if (!res.rows.length) { el.append(empty("0 rows.")); return; }
    const table = document.createElement("table");
    const thead = document.createElement("thead");
    const htr = document.createElement("tr");
    for (const c of res.columns) { const th = document.createElement("th"); th.textContent = c; htr.append(th); }
    thead.append(htr);
    table.append(thead, bodyRows(res));
    el.append(table);
  }

  // Data renderer: sortable headers + a per-column filter row.
  function renderDataGrid(res) {
    const el = $("data-results");
    el.replaceChildren();
    if (!res.columns.length) { el.append(empty("No columns.")); return; }

    const table = document.createElement("table");
    const thead = document.createElement("thead");

    const htr = document.createElement("tr");
    for (const c of res.columns) {
      const th = document.createElement("th");
      th.className = "sortable";
      th.textContent = c === sortCol ? c + (sortDir === "desc" ? " ▼" : " ▲") : c;
      th.addEventListener("click", () => toggleSort(c));
      htr.append(th);
    }
    thead.append(htr);

    const ftr = document.createElement("tr");
    ftr.className = "filter-row";
    for (const c of res.columns) {
      const td = document.createElement("td");
      const sel = document.createElement("select");
      sel.className = "f-op";
      sel.dataset.col = c;
      for (const [k, label] of OPS) sel.append(new Option(label, k));
      const inp = document.createElement("input");
      inp.className = "f-val";
      inp.dataset.col = c;
      inp.placeholder = "filter…";
      const cur = filters[c];
      if (cur) { sel.value = cur.op; inp.value = cur.value || ""; }
      sel.addEventListener("change", applyFilters);
      inp.addEventListener("keydown", (e) => { if (e.key === "Enter") { e.preventDefault(); applyFilters(); } });
      td.append(sel, inp);
      ftr.append(td);
    }
    thead.append(ftr);

    const fkByCol = res.columns.map((c) => currentFKs[c] || null);
    table.append(thead, bodyRows(res, fkByCol));
    el.append(table);
    if (!res.rows.length) el.append(empty("0 rows."));
  }

  function toggleSort(col) {
    if (sortCol === col) sortDir = sortDir === "asc" ? "desc" : "asc";
    else { sortCol = col; sortDir = "asc"; }
    offset = 0;
    loadData();
  }

  function applyFilters() {
    filters = {};
    for (const sel of $("data-results").querySelectorAll(".f-op")) {
      if (!sel.value) continue;
      const inp = sel.parentElement.querySelector(".f-val");
      filters[sel.dataset.col] = { op: sel.value, value: inp.value };
    }
    offset = 0;
    loadData();
  }

  // appendDataParams adds search/sort/filter params shared by browse + export.
  function appendDataParams(p) {
    if (searchTerm) p.set("search", searchTerm);
    if (sortCol) { p.set("sort", sortCol); p.set("dir", sortDir); }
    for (const col of Object.keys(filters)) {
      const f = filters[col];
      if (opNeedsValue(f.op)) p.append("f", col + ":" + f.op + ":" + (f.value || ""));
      else p.append("f", col + ":" + f.op);
    }
  }

  // ---- tabs ----
  const TABS = ["data", "structure", "sql"];
  function switchTab(name) {
    for (const t of TABS) {
      $("panel-" + t).hidden = t !== name;
      $("tab-" + t).classList.toggle("active", t === name);
    }
  }
  $("tab-data").addEventListener("click", () => switchTab("data"));
  $("tab-structure").addEventListener("click", () => { switchTab("structure"); loadStructure(); });
  $("tab-sql").addEventListener("click", () => {
    switchTab("sql");
    // CodeMirror was created while this panel was hidden (zero size); it must be
    // refreshed once visible or it renders blank.
    if (editor) editor.refresh();
  });

  // ---- sidebar ----
  async function loadTables() {
    const r = await fetch("/api/tables");
    tables = await r.json();
    renderSidebar();
  }

  function renderSidebar() {
    const filter = $("tbl-filter").value.toLowerCase();
    const list = $("tables");
    list.replaceChildren();
    let schema = null;
    for (const t of tables) {
      const label = t.schema + "." + t.name;
      if (filter && !label.toLowerCase().includes(filter)) continue;
      if (t.schema !== schema) {
        schema = t.schema;
        const h = document.createElement("div"); h.className = "schema"; h.textContent = schema;
        list.append(h);
      }
      const item = document.createElement("button");
      item.className = "tbl" + (t.type === "view" ? " view" : "");
      item.textContent = t.name;
      item.dataset.key = label;
      item.title = label + (t.estRows >= 0 ? " (~" + t.estRows + " rows)" : "");
      item.addEventListener("click", () => activateTable(t));
      list.append(item);
    }
    if (!list.children.length) list.append(empty("No tables match."));
  }
  $("tbl-filter").addEventListener("input", renderSidebar);

  function highlightSidebar(t) {
    for (const b of document.querySelectorAll(".tbl.active")) b.classList.remove("active");
    const key = t.schema + "." + t.name;
    for (const b of document.querySelectorAll("#tables .tbl")) {
      if (b.dataset.key === key) b.classList.add("active");
    }
  }

  // activateTable opens a table in the Data tab, optionally with preset filters
  // (used by FK click-through to land on a specific referenced row).
  async function activateTable(t, initialFilters) {
    current = t; offset = 0;
    searchTerm = ""; sortCol = null; sortDir = "asc";
    filters = initialFilters || {};
    $("data-search").value = "";
    $("tab-title").textContent = t.schema + "." + t.name;
    highlightSidebar(t);
    switchTab("data");
    await loadFKs(t);
    loadData();
  }

  // openRef navigates to the row referenced by a foreign-key cell.
  function openRef(ref, value) {
    const target = tables.find((x) => x.schema === ref.schema && x.name === ref.table);
    if (!target) { setStatus("✗ referenced table " + ref.schema + "." + ref.table + " is not browsable", "error"); return; }
    activateTable(target, { [ref.column]: { op: "eq", value: String(value) } });
  }

  async function loadFKs(t) {
    currentFKs = {};
    try {
      const r = await fetch(tablePath(t) + "/fks");
      const list = await r.json();
      if (r.ok) for (const f of list) currentFKs[f.column] = { schema: f.refSchema, table: f.refTable, column: f.refColumn };
    } catch {
      /* no FK links if introspection fails */
    }
  }

  function tablePath(t) {
    return "/api/tables/" + encodeURIComponent(t.schema) + "/" + encodeURIComponent(t.name);
  }

  async function loadData() {
    if (!current) return;
    const seq = ++dataSeq;
    const tbl = current, off = offset;
    setStatus("Loading " + tbl.schema + "." + tbl.name + "…", "ok");
    try {
      const p = new URLSearchParams();
      p.set("limit", pageSize);
      p.set("offset", off);
      appendDataParams(p);
      const r = await fetch(tablePath(tbl) + "/data?" + p.toString());
      const data = await r.json();
      if (seq !== dataSeq) return; // a newer table/page selection superseded this
      if (!r.ok) { setStatus("✗ " + (data.error || r.statusText), "error"); return; }
      renderDataGrid(data);
      $("prev-btn").disabled = off === 0;
      $("next-btn").disabled = data.rowCount < pageSize;
      const from = data.rowCount ? off + 1 : 0;
      $("page-info").textContent = from + "–" + (off + data.rowCount);
      $("data-export-btn").disabled = data.rowCount === 0;
      setStatus("✓ " + data.rowCount + " row" + (data.rowCount === 1 ? "" : "s") + " in " + data.elapsedMs + " ms", "ok");
    } catch (e) {
      setStatus("✗ " + e.message, "error");
    }
  }
  $("prev-btn").addEventListener("click", () => { if (offset > 0) { offset = Math.max(0, offset - pageSize); loadData(); } });
  $("next-btn").addEventListener("click", () => { offset += pageSize; loadData(); });
  $("data-export-btn").addEventListener("click", () => {
    if (!current) return;
    // Export the whole filtered/sorted table (server caps at the row limit).
    const p = new URLSearchParams();
    p.set("format", "csv");
    appendDataParams(p);
    const a = document.createElement("a");
    a.href = tablePath(current) + "/data?" + p.toString();
    a.download = current.name + ".csv";
    a.click();
  });

  function doSearch() {
    searchTerm = $("data-search").value.trim();
    offset = 0;
    loadData();
  }
  $("data-search").addEventListener("keydown", (e) => { if (e.key === "Enter") doSearch(); });
  $("data-clear").addEventListener("click", () => {
    searchTerm = ""; filters = {}; sortCol = null; sortDir = "asc"; offset = 0;
    $("data-search").value = "";
    if (current) loadData();
  });

  async function loadStructure() {
    if (!current) { $("structure-results").replaceChildren(empty("Select a table to see its structure.")); return; }
    const seq = ++structSeq;
    const tbl = current;
    try {
      const r = await fetch(tablePath(tbl) + "/columns");
      const cols = await r.json();
      if (seq !== structSeq) return; // superseded by a newer selection
      if (!r.ok) { setStatus("✗ " + (cols.error || r.statusText), "error"); return; }
      renderColumns($("structure-results"), cols);
    } catch (e) {
      setStatus("✗ " + e.message, "error");
    }
  }

  function renderColumns(el, cols) {
    el.replaceChildren();
    if (!cols.length) { el.append(empty("No columns.")); return; }
    const table = document.createElement("table");
    const thead = document.createElement("tr");
    for (const h of ["Column", "Type", "Nullable", "Default"]) { const th = document.createElement("th"); th.textContent = h; thead.append(th); }
    table.append(thead);
    for (const c of cols) {
      const tr = document.createElement("tr");
      const cells = [c.name, c.type, c.nullable ? "YES" : "NO", c.default == null ? "" : c.default];
      for (const v of cells) { const td = document.createElement("td"); td.textContent = v; tr.append(td); }
      table.append(tr);
    }
    el.append(table);
  }

  // ---- SQL tab ----
  if (window.CodeMirror) {
    editor = CodeMirror.fromTextArea($("sql"), {
      mode: "text/x-pgsql", lineNumbers: true, theme: "default",
      lineWrapping: true,
    });
    editor.setOption("extraKeys", { "Cmd-Enter": runQuery, "Ctrl-Enter": runQuery });
  } else {
    $("sql").addEventListener("keydown", (e) => {
      if ((e.metaKey || e.ctrlKey) && e.key === "Enter") { e.preventDefault(); runQuery(); }
    });
  }
  const getSQL = () => (editor ? editor.getValue() : $("sql").value).trim();
  const setSQL = (v) => editor ? editor.setValue(v) : ($("sql").value = v);

  async function runQuery() {
    const sql = getSQL();
    if (!sql) return;
    setStatus("Running…", "ok");
    $("run-btn").disabled = true; $("sql-export-btn").disabled = true;
    try {
      const r = await fetch("/api/query", {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ sql }),
      });
      const data = await r.json();
      if (!r.ok) { setStatus("✗ " + (data.error || r.statusText), "error"); $("sql-results").replaceChildren(); return; }
      lastSQL = sql;
      renderGrid($("sql-results"), data);
      const msg = "✓ " + data.rowCount + " row" + (data.rowCount === 1 ? "" : "s") + " in " + data.elapsedMs + " ms";
      if (data.truncated) setStatusHTML(msg + ' <span class="warn">· capped (more rows available — add LIMIT or refine)</span>', "ok");
      else setStatus(msg, "ok");
      $("sql-export-btn").disabled = data.rowCount === 0;
    } catch (e) {
      setStatus("✗ " + e.message, "error");
    } finally {
      $("run-btn").disabled = false;
    }
  }

  async function exportSQL() {
    const sql = lastSQL || getSQL();
    if (!sql) return;
    const r = await fetch("/api/export", {
      method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ sql }),
    });
    if (!r.ok) { const d = await r.json().catch(() => ({})); setStatus("✗ " + (d.error || "export failed"), "error"); return; }
    const blob = await r.blob();
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url; a.download = "pgpeek-export.csv"; a.click();
    URL.revokeObjectURL(url);
  }

  async function loadSaved() {
    const r = await fetch("/api/queries");
    savedQueries = await r.json();
    const presetsEl = $("presets");
    presetsEl.replaceChildren(new Option("Saved queries…", ""));
    const addGroup = (lbl, items) => {
      if (!items.length) return;
      const g = document.createElement("optgroup"); g.label = lbl;
      for (const q of items) g.append(new Option(q.name, q.id));
      presetsEl.append(g);
    };
    addGroup("Presets", savedQueries.filter((q) => q.isPreset));
    addGroup("Saved", savedQueries.filter((q) => !q.isPreset));
  }

  $("presets").addEventListener("change", () => {
    const id = $("presets").value;
    const q = savedQueries.find((x) => String(x.id) === id);
    $("delete-btn").disabled = !(q && !q.isPreset);
    if (q) { setSQL(q.sql); setStatus("Loaded “" + q.name + "”. Press Run.", "ok"); }
  });

  $("run-btn").addEventListener("click", runQuery);
  $("sql-export-btn").addEventListener("click", exportSQL);

  $("save-btn").addEventListener("click", async () => {
    const sql = getSQL();
    if (!sql) return;
    const name = prompt("Name for this saved query:");
    if (!name) return;
    const description = prompt("Description (optional):") || "";
    const r = await fetch("/api/queries", {
      method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ name, description, sql }),
    });
    const d = await r.json();
    if (!r.ok) { setStatus("✗ " + (d.error || "save failed"), "error"); return; }
    await loadSaved(); $("presets").value = d.id; $("delete-btn").disabled = false;
    setStatus("✓ Saved “" + d.name + "”.", "ok");
  });

  $("delete-btn").addEventListener("click", async () => {
    const id = $("presets").value;
    if (!id) return;
    const q = savedQueries.find((x) => String(x.id) === id);
    if (!q || !confirm("Delete saved query “" + q.name + "”?")) return;
    const r = await fetch("/api/queries/" + id, { method: "DELETE" });
    if (!r.ok && r.status !== 204) { setStatus("✗ delete failed", "error"); return; }
    await loadSaved(); $("delete-btn").disabled = true;
    setStatus("✓ Deleted.", "ok");
  });

  // Learn the server's row cap so paging never asks for more than a page the
  // server will actually return (otherwise Next could never enable).
  async function loadMeta() {
    try {
      const r = await fetch("/api/meta");
      const m = await r.json();
      if (m && m.rowCap > 0) pageSize = Math.min(PAGE_SIZE, m.rowCap);
    } catch {
      /* keep the default page size */
    }
  }

  loadMeta();
  loadTables().catch((e) => setStatus("✗ failed to load tables: " + e.message, "error"));
  loadSaved().catch((e) => setStatus("✗ failed to load saved queries: " + e.message, "error"));
})();
