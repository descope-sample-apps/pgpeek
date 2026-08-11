// allow: SIZE_OK — one editor action-generation state machine prevents stale async results.
import { html, useState, useEffect, useRef, useCallback } from "./vendor/preact-htm.js";
import { dbUrl, getJSON, tablePath } from "./api.js";
import { interceptRun, runEasterEgg, EXPLAIN_JOKE } from "./easter-eggs.js";
import { countQuery, exportQuery } from "./sql-actions.js";
import { DEFAULT_SQL, queryStatusText, responseError, SqlResults } from "./sql-results.js";

const RUNNING_TIME_DELAY_SECONDS = 10;

export function SqlTab({ active, saved, reloadSaved, dbId, setStatus, tables, initialSQL, onStateChange }) {
  const wrapRef = useRef();
  const taRef = useRef();
  const editorRef = useRef();
  const [result, setResult] = useState(null);
  const [resultKey, setResultKey] = useState(0);
  const [lastSQL, setLastSQL] = useState("");
  const [selected, setSelected] = useState("");
  const [running, setRunning] = useState(false);
  const [error, setError] = useState("");
  const runningRef = useRef(false);
  const exportingRef = useRef(false);
  const requestRef = useRef(null);
  const runningLabelRef = useRef("Previewing");
  const actionRef = useRef(0);
  const initialSQLRef = useRef(initialSQL);
  const dbRef = useRef(dbId);

  const invalidate = () => {
    requestRef.current?.abort(); requestRef.current = null;
    actionRef.current += 1;
    if (!exportingRef.current) { runningRef.current = false; setRunning(false); }
    setResult(null); setLastSQL(""); setError("");
  };
  const onEdit = (value) => { invalidate(); onStateChange(value); };
  if (dbRef.current !== dbId) {
    dbRef.current = dbId; actionRef.current += 1;
    if (!exportingRef.current) runningRef.current = false;
  }

  const getSQL = () => (editorRef.current ? editorRef.current.getValue() : taRef.current.value).trim();
  const setSQL = (v) => {
    if (editorRef.current) editorRef.current.setValue(v);
    else { taRef.current.value = v; onEdit(v); }
  };

  const run = useCallback(async () => {
    const sql = getSQL();
    if (!sql) return;
    if (runningRef.current) return;
    // Easter eggs: magic queries, DROP rewrite, VACUUM/ANALYZE whisper.
    const eg = interceptRun(sql);
    if (eg) {
      runEasterEgg(eg, { sql, setLastSQL, setResult, setError, setSQL, setStatus, wrapRef });
      return;
    }
    const action = actionRef.current + 1;
    actionRef.current = action;
    const controller = new AbortController(); requestRef.current = controller;
    runningLabelRef.current = "Previewing"; runningRef.current = true; setRunning(true);
    setError("");
    setStatus({ text: "Previewing…", cls: "ok" });
    try {
      const r = await fetch(dbUrl("/api/query", dbId), {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ sql }),
        signal: controller.signal,
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
      setLastSQL(sql); setResult(d); setResultKey(action);
      const base = queryStatusText(d.rowCount, d.elapsedMs);
      const warnings = [];
      if (d.truncated) warnings.push("· capped (more rows available; add LIMIT or refine)");
      if (d.cellsTruncated) warnings.push("· large cells shortened; expand to load full value");
      setStatus(warnings.length ? { text: base, cls: "ok", warn: warnings.join(" ") } : { text: base, cls: "ok" });
    } catch (e) {
      if (e.name === "AbortError") return;
      if (action === actionRef.current) {
        setError(e.message);
        setStatus({ text: "✗ " + e.message, cls: "error" });
      }
    } finally {
      if (requestRef.current === controller) requestRef.current = null;
      if (action === actionRef.current) { runningRef.current = false; setRunning(false); }
    }
  }, [dbId]);

  const runRef = useRef(run);
  useEffect(() => { runRef.current = run; }, [run]);
  useEffect(() => { invalidate(); return () => requestRef.current?.abort(); }, [dbId]);

  useEffect(() => {
    if (!running) return;
    const startedAt = Date.now();
    let timer;
    const update = () => {
      const seconds = Math.floor((Date.now() - startedAt) / 1000);
      if (runningRef.current) {
        setStatus({ text: `${runningLabelRef.current}… (${seconds}s)`, cls: "ok" });
      }
    };
    const delay = setTimeout(() => {
      update();
      timer = setInterval(update, 1000);
    }, RUNNING_TIME_DELAY_SECONDS * 1000);
    return () => { clearTimeout(delay); clearInterval(timer); };
  }, [running, setStatus]);

  // Init CodeMirror once into a Preact-stable wrapper it fully owns.
  useEffect(() => {
    if (window.cm6) {
      editorRef.current = window.cm6.mount(wrapRef.current, initialSQL ?? DEFAULT_SQL, () => runRef.current(), onEdit);
      return;
    }
    const ta = document.createElement("textarea");
    ta.id = "sql";
    ta.setAttribute("aria-label", "SQL query");
    ta.value = initialSQL ?? DEFAULT_SQL;
    wrapRef.current.appendChild(ta);
    taRef.current = ta;
    ta.addEventListener("input", () => onEdit(ta.value));
    ta.addEventListener("keydown", (e) => {
      if ((e.metaKey || e.ctrlKey) && e.key === "Enter") { e.preventDefault(); runRef.current(); }
    });
  }, []);

  useEffect(() => {
    if (initialSQL === initialSQLRef.current) return;
    initialSQLRef.current = initialSQL;
    const value = initialSQL ?? DEFAULT_SQL;
    const current = editorRef.current ? editorRef.current.getValue() : taRef.current.value;
    if (current === value) return;
    invalidate();
    if (editorRef.current && editorRef.current.getValue() !== value) editorRef.current.setValue(value, false);
    if (taRef.current && taRef.current.value !== value) taRef.current.value = value;
  }, [initialSQL]);

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
    let live = true; const controller = new AbortController(), queue = tables.values();
    // Build schema → table-names entries up front (sync).
    const schema = {}, tableNameCounts = {};
    for (const tbl of tables) {
      if (!schema[tbl.schema]) schema[tbl.schema] = [];
      schema[tbl.schema].push(tbl.name);
      tableNameCounts[tbl.name] = (tableNameCounts[tbl.name] || 0) + 1;
    }
    const columnsByRelation = {}, baseConfig = { schema, columnsByRelation, defaultSchema: schema.public ? "public" : tables[0].schema };
    editorRef.current.setSQLConfig(baseConfig);
    // Fetch columns async; populate qualified-table → column-names entries.
    (async () => {
      await Promise.all(Array.from({ length: Math.min(6, tables.length) }, async () => {
        for (const tbl of queue) {
          if (!live) return;
          try {
            const cols = await getJSON(tablePath(tbl) + "/columns", dbId, { signal: controller.signal });
            if (!live) return;
            const names = cols.map((c) => c.name), qualified = tbl.schema + "." + tbl.name;
            schema[qualified] = names;
            columnsByRelation[qualified] = names;
            if (tableNameCounts[tbl.name] === 1) columnsByRelation[tbl.name] = names;
          } catch { /* partial autocomplete ok */ }
        }
      }));
      if (!live) return;
      editorRef.current?.setSQLConfig(baseConfig);
    })();
    return () => { live = false; controller.abort(); };
  }, [active, tables, dbId]);

  // CodeMirror was created while hidden (zero size); refresh when shown.
  useEffect(() => { if (active && editorRef.current) editorRef.current.refresh(); }, [active]);

  const countRows = async () => {
    const sql = getSQL();
    if (!sql || runningRef.current) return;
    const action = actionRef.current + 1;
    actionRef.current = action;
    const controller = new AbortController(); requestRef.current = controller;
    runningLabelRef.current = "Counting"; runningRef.current = true; setRunning(true); setError("");
    setStatus({ text: "Counting…", cls: "ok" });
    try {
      const data = await countQuery(sql, dbId, controller.signal);
      if (action !== actionRef.current) return;
      const rowCount = BigInt(data.rowCount);
      const rows = rowCount.toLocaleString();
      setStatus({ text: `✓ ${rows} row${rowCount === 1n ? "" : "s"} in ${data.elapsedMs} ms`, cls: "ok" });
    } catch (e) {
      if (e.name === "AbortError") return;
      if (action === actionRef.current) {
        setError(e.message);
        setStatus({ text: "✗ " + e.message, cls: "error" });
      }
    } finally {
      if (requestRef.current === controller) requestRef.current = null;
      if (action === actionRef.current) { runningRef.current = false; setRunning(false); }
    }
  };

  const exportCSV = () => {
    const sql = getSQL();
    if (!sql || runningRef.current) return;
    exportingRef.current = true;
    runningLabelRef.current = "Exporting"; runningRef.current = true; setRunning(true);
    setError("");
    setStatus({ text: "Preparing export…", cls: "ok" });
    exportQuery(sql, dbId, (message) => {
      exportingRef.current = false;
      runningRef.current = false; setRunning(false);
      if (message) {
        setError(message);
        setStatus({ text: "✗ " + message, cls: "error" });
      } else {
        setStatus({ text: "✓ Download started.", cls: "ok" });
      }
    });
  };

  const onPick = (e) => {
    const id = e.target.value; setSelected(id);
    const q = saved.find((x) => String(x.id) === id);
    if (q) {
      setSQL(q.sql); setError(""); setStatus({ text: "Loaded \u201c" + q.name + "\u201d. Press Preview.", cls: "ok" });
    }
  };
  const selectedQ = saved.find((x) => String(x.id) === selected);

  const onSave = async () => {
    if (runningRef.current) return;
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
    if (runningRef.current) return;
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
      <button class="primary" id="run-btn" disabled=${running} onClick=${run}>Preview</button>
      <button class="ghost" id="count-btn" disabled=${running} onClick=${countRows}>Count</button>
      <button class="action secondary" id="sql-export-btn"
        disabled=${running}
        onClick=${exportCSV}>Export .csv.gz</button>
      <select id="presets" title="Saved & preset queries" value=${selected} onChange=${onPick}>
        <option value="">Saved queries…</option>
        ${presets.length ? html`<optgroup label="Presets">${presets.map((q) =>
          html`<option key=${q.id} value=${q.id}>${q.name}</option>`)}</optgroup>` : ""}
        ${mine.length ? html`<optgroup label="Saved">${mine.map((q) =>
          html`<option key=${q.id} value=${q.id}>${q.name}</option>`)}</optgroup>` : ""}
      </select>
      <button class="ghost" id="save-btn" disabled=${running} onClick=${onSave}>Save</button>
      <button class="ghost" id="delete-btn"
        disabled=${running || !(selectedQ && !selectedQ.isPreset)} onClick=${onDelete}>Delete</button>
      <span class="hint">Ctrl/Cmd\u00a0+\u00a0Enter to preview · single SELECT/WITH only · ${EXPLAIN_JOKE}</span>
    </div>
    ${error ? html`<div class="query-error" role="alert" aria-live="assertive">${error}</div>` : ""}
    <div class="results" id="sql-results">
      <${SqlResults} result=${result} resultKey=${resultKey} sql=${lastSQL} dbId=${dbId} />
    </div>`;
}
