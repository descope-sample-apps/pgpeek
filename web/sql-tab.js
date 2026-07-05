// SqlTab — CodeMirror SQL editor with run, export, and saved-query CRUD.
import { html, useState, useEffect, useRef, useCallback } from "./vendor/preact-htm.js";
import { dbUrl, getJSON, tablePath } from "./api.js";

async function responseError(r, fallback) {
  const body = await r.json().catch(() => null);
  return (body && body.error) || r.statusText || fallback;
}

export function SqlTab({ active, saved, reloadSaved, dbId, setStatus, tables }) {
  const wrapRef = useRef();
  const taRef = useRef();
  const editorRef = useRef();
  const [result, setResult] = useState(null);
  const [lastSQL, setLastSQL] = useState("");
  const [selected, setSelected] = useState("");
  const [running, setRunning] = useState(false);
  const [error, setError] = useState("");
  const runningRef = useRef(false);
  const actionRef = useRef(0);

  const getSQL = () => (editorRef.current ? editorRef.current.getValue() : taRef.current.value).trim();
  const setSQL = (v) => { if (editorRef.current) editorRef.current.setValue(v); else taRef.current.value = v; };

  const run = useCallback(async () => {
    const sql = getSQL();
    if (!sql) return;
    if (runningRef.current) return;
    const action = actionRef.current + 1;
    actionRef.current = action;
    runningRef.current = true; setRunning(true);
    setError("");
    setStatus({ text: "Running…", cls: "ok" });
    try {
      const r = await fetch(dbUrl("/api/query", dbId), {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ sql }),
      });
      if (!r.ok) {
        const message = await responseError(r, "query failed");
        if (action !== actionRef.current) return;
        setError(message);
        setStatus({ text: "✗ " + message, cls: "error" });
        setResult(null);
        return;
      }
      const d = await r.json();
      if (action !== actionRef.current) return;
      setLastSQL(sql); setResult(d);
      const base = "✓ " + d.rowCount + " row" + (d.rowCount === 1 ? "" : "s") + " in " + d.elapsedMs + " ms";
      setStatus(d.truncated
        ? { text: base, cls: "ok", warn: "· capped (more rows available — add LIMIT or refine)" }
        : { text: base, cls: "ok" });
    } catch (e) {
      if (action === actionRef.current) {
        setError(e.message);
        setStatus({ text: "✗ " + e.message, cls: "error" });
      }
    } finally {
      runningRef.current = false; setRunning(false);
    }
  }, [dbId]);

  const runRef = useRef(run);
  useEffect(() => { runRef.current = run; }, [run]);

  // Init CodeMirror once into a Preact-stable wrapper it fully owns.
  useEffect(() => {
    if (window.cm6) {
      editorRef.current = window.cm6.mount(wrapRef.current, "SELECT now();", () => runRef.current());
      return;
    }
    const ta = document.createElement("textarea");
    ta.id = "sql";
    ta.value = "SELECT now();";
    wrapRef.current.appendChild(ta);
    taRef.current = ta;
    ta.addEventListener("keydown", (e) => {
      if ((e.metaKey || e.ctrlKey) && e.key === "Enter") { e.preventDefault(); runRef.current(); }
    });
  }, []);

  // CM6 autocomplete: fetch columns for each table and wire up schema config.
  // Textarea mode skips this entirely (no window.cm6 → no column fetches).
  useEffect(() => {
    if (!active) return;
    if (!window.cm6) return;
    if (!editorRef.current?.setSQLConfig) return;
    if (!tables || tables.length === 0) {
      editorRef.current.setSQLConfig({ schema: {} });
      return;
    }
    let live = true;
    // Build schema → table-names entries up front (sync).
    const schema = {};
    const tableNameCounts = {};
    for (const tbl of tables) {
      if (!schema[tbl.schema]) schema[tbl.schema] = [];
      schema[tbl.schema].push(tbl.name);
      tableNameCounts[tbl.name] = (tableNameCounts[tbl.name] || 0) + 1;
    }
    const columnsByRelation = {};
    const baseConfig = { schema, columnsByRelation, defaultSchema: schema.public ? "public" : tables[0].schema };
    editorRef.current.setSQLConfig(baseConfig);
    // Fetch columns async; populate qualified-table → column-names entries.
    (async () => {
      await Promise.all(tables.map(async (tbl) => {
        try {
          const cols = await getJSON(tablePath(tbl) + "/columns", dbId);
          if (!live) return;
          const names = cols.map((c) => c.name);
          const qualified = tbl.schema + "." + tbl.name;
          schema[qualified] = names;
          columnsByRelation[qualified] = names;
          if (tableNameCounts[tbl.name] === 1) columnsByRelation[tbl.name] = names;
        } catch { /* partial autocomplete ok */ }
      }));
      if (!live) return;
      editorRef.current?.setSQLConfig(baseConfig);
    })();
    return () => { live = false; };
  }, [active, tables, dbId]);

  // CodeMirror was created while hidden (zero size); refresh when shown.
  useEffect(() => { if (active && editorRef.current) editorRef.current.refresh(); }, [active]);

  const exportCSV = async () => {
    const sql = lastSQL || getSQL();
    if (!sql) return;
    const action = actionRef.current + 1;
    actionRef.current = action;
    setError("");
    try {
      const r = await fetch(dbUrl("/api/export", dbId), {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ sql }),
      });
      if (!r.ok) {
        const message = await responseError(r, "export failed");
        if (action !== actionRef.current) return;
        setError(message);
        setStatus({ text: "✗ " + message, cls: "error" });
        return;
      }
      const blob = await r.blob();
      if (action !== actionRef.current) return;
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a"); a.href = url; a.download = "pgpeek-export.csv"; a.click();
      setTimeout(() => URL.revokeObjectURL(url), 0);
    } catch (e) {
      if (action === actionRef.current) {
        setError(e.message);
        setStatus({ text: "✗ " + e.message, cls: "error" });
      }
    }
  };

  const onPick = (e) => {
    const id = e.target.value; setSelected(id);
    const q = saved.find((x) => String(x.id) === id);
    if (q) {
      actionRef.current += 1;
      setSQL(q.sql); setError(""); setStatus({ text: "Loaded \u201c" + q.name + "\u201d. Press Run.", cls: "ok" });
    }
  };
  const selectedQ = saved.find((x) => String(x.id) === selected);

  const onSave = async () => {
    const sql = getSQL();
    if (!sql) return;
    const name = prompt("Name for this saved query:");
    if (!name) return;
    const description = prompt("Description (optional):") || "";
    const action = actionRef.current + 1;
    actionRef.current = action;
    const r = await fetch("/api/queries", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ name, description, sql }),
    });
    const d = await r.json();
    if (!r.ok) {
      if (action === actionRef.current) setStatus({ text: "✗ " + (d.error || "save failed"), cls: "error" });
      return;
    }
    await reloadSaved();
    if (action !== actionRef.current) return;
    setSelected(String(d.id));
    setError("");
    setStatus({ text: "\u2713 Saved \u201c" + d.name + "\u201d.", cls: "ok" });
  };

  const onDelete = async () => {
    if (!selectedQ) return;
    if (!confirm("Delete saved query \u201c" + selectedQ.name + "\u201d?")) return;
    const action = actionRef.current + 1;
    actionRef.current = action;
    const r = await fetch("/api/queries/" + selectedQ.id, { method: "DELETE" });
    if (!r.ok && r.status !== 204) {
      if (action === actionRef.current) setStatus({ text: "✗ delete failed", cls: "error" });
      return;
    }
    await reloadSaved();
    if (action !== actionRef.current) return;
    setSelected("");
    setError("");
    setStatus({ text: "✓ Deleted.", cls: "ok" });
  };

  const presets = saved.filter((q) => q.isPreset);
  const mine = saved.filter((q) => !q.isPreset);

  return html`
    <div class="editor-wrap" ref=${wrapRef}></div>
    <div class="toolbar">
      <button class="primary" id="run-btn" disabled=${running} onClick=${run}>Run ▶</button>
      <button class="ghost" id="sql-export-btn"
        disabled=${running || !result || result.rowCount === 0}
        onClick=${exportCSV}>Export CSV</button>
      <select id="presets" title="Saved & preset queries" value=${selected} onChange=${onPick}>
        <option value="">Saved queries…</option>
        ${presets.length ? html`<optgroup label="Presets">${presets.map((q) =>
          html`<option key=${q.id} value=${q.id}>${q.name}</option>`)}</optgroup>` : ""}
        ${mine.length ? html`<optgroup label="Saved">${mine.map((q) =>
          html`<option key=${q.id} value=${q.id}>${q.name}</option>`)}</optgroup>` : ""}
      </select>
      <button class="ghost" id="save-btn" onClick=${onSave}>Save</button>
      <button class="ghost" id="delete-btn"
        disabled=${!(selectedQ && !selectedQ.isPreset)} onClick=${onDelete}>Delete</button>
      <span class="hint">Ctrl/Cmd\u00a0+\u00a0Enter to run · single SELECT/WITH only</span>
    </div>
    ${error ? html`<div class="query-error" role="alert" aria-live="assertive">${error}</div>` : ""}
    <div class="results" id="sql-results">
      ${result
        ? (result.columns.length === 0
            ? html`<div class="empty">Query ran. No columns returned.</div>`
            : (result.rows.length === 0
                ? html`<div class="empty">0 rows.</div>`
                : html`<table>
                    <thead><tr>${result.columns.map((c) => html`<th key=${c}>${c}</th>`)}</tr></thead>
                    <tbody>${result.rows.map((row, rowIndex) =>
                      html`<tr key=${rowIndex}>${row.map((v, i) => html`<td key=${i}>${v === null ? html`<span class="null">NULL</span>` : (typeof v === "object" ? JSON.stringify(v) : String(v))}</td>`)}</tr>`)}</tbody>
                  </table>`))
        : html`<div class="empty">Run a query to see results.</div>`}
    </div>`;
}
