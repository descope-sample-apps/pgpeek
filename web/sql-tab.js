// SqlTab — CodeMirror SQL editor with run, export, and saved-query CRUD.
import { html, useState, useEffect, useRef, useCallback } from "./vendor/preact-htm.js";
import { dbUrl } from "./api.js";

export function SqlTab({ active, saved, reloadSaved, dbId, setStatus }) {
  const wrapRef = useRef();
  const taRef = useRef();
  const editorRef = useRef();
  const [result, setResult] = useState(null);
  const [lastSQL, setLastSQL] = useState("");
  const [selected, setSelected] = useState("");
  const [running, setRunning] = useState(false);
  const runningRef = useRef(false);

  const getSQL = () => (editorRef.current ? editorRef.current.getValue() : taRef.current.value).trim();
  const setSQL = (v) => { if (editorRef.current) editorRef.current.setValue(v); else taRef.current.value = v; };

  const run = useCallback(async () => {
    const sql = getSQL();
    if (!sql) return;
    if (runningRef.current) return;
    runningRef.current = true; setRunning(true);
    setStatus({ text: "Running…", cls: "ok" });
    try {
      const r = await fetch(dbUrl("/api/query", dbId), {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ sql }),
      });
      const d = await r.json();
      if (!r.ok) {
        setStatus({ text: "✗ " + (d.error || r.statusText), cls: "error" });
        setResult(null);
        return;
      }
      setLastSQL(sql); setResult(d);
      const base = "✓ " + d.rowCount + " row" + (d.rowCount === 1 ? "" : "s") + " in " + d.elapsedMs + " ms";
      setStatus(d.truncated
        ? { text: base, cls: "ok", warn: "· capped (more rows available — add LIMIT or refine)" }
        : { text: base, cls: "ok" });
    } catch (e) {
      setStatus({ text: "✗ " + e.message, cls: "error" });
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

  // CodeMirror was created while hidden (zero size); refresh when shown.
  useEffect(() => { if (active && editorRef.current) editorRef.current.refresh(); }, [active]);

  const exportCSV = async () => {
    const sql = lastSQL || getSQL();
    if (!sql) return;
    const r = await fetch(dbUrl("/api/export", dbId), {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ sql }),
    });
    if (!r.ok) {
      const d = await r.json().catch(() => ({}));
      setStatus({ text: "✗ " + (d.error || "export failed"), cls: "error" });
      return;
    }
    const url = URL.createObjectURL(await r.blob());
    const a = document.createElement("a"); a.href = url; a.download = "pgpeek-export.csv"; a.click();
    setTimeout(() => URL.revokeObjectURL(url), 0);
  };

  const onPick = (e) => {
    const id = e.target.value; setSelected(id);
    const q = saved.find((x) => String(x.id) === id);
    if (q) { setSQL(q.sql); setStatus({ text: "Loaded \u201c" + q.name + "\u201d. Press Run.", cls: "ok" }); }
  };
  const selectedQ = saved.find((x) => String(x.id) === selected);

  const onSave = async () => {
    const sql = getSQL();
    if (!sql) return;
    const name = prompt("Name for this saved query:");
    if (!name) return;
    const description = prompt("Description (optional):") || "";
    const r = await fetch("/api/queries", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ name, description, sql }),
    });
    const d = await r.json();
    if (!r.ok) { setStatus({ text: "✗ " + (d.error || "save failed"), cls: "error" }); return; }
    await reloadSaved(); setSelected(String(d.id));
    setStatus({ text: "\u2713 Saved \u201c" + d.name + "\u201d.", cls: "ok" });
  };

  const onDelete = async () => {
    if (!selectedQ) return;
    if (!confirm("Delete saved query \u201c" + selectedQ.name + "\u201d?")) return;
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
