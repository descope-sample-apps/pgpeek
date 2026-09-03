// allow: SIZE_OK — one editor action-generation state machine prevents stale async results.
import { html, useState, useEffect, useRef, useCallback } from "./vendor/preact-htm.js";
import { dbUrl, getJSON } from "./api.js";
import { interceptRun, runEasterEgg } from "./easter-eggs.js";
import { countQuery, exportQuery } from "./sql-actions.js";
import { DEFAULT_SQL, queryStatusText, SqlResults, ResultMeta, ResultViews } from "./sql-results.js";

const RUNNING_TIME_DELAY_SECONDS = 10;
const SPLIT_MIN_EDITOR = 120;
const SPLIT_STORAGE_KEY = "pgpeek.sql.split";

/** One-based `position` from the server maps back into the editor via the runnable offset. */
function editorOffset(runnable, position) {
  const relative = Array.from(runnable.sql).slice(0, position - 1).join("").length;
  return Math.min(runnable.to, runnable.from + relative);
}

function readSplitPreference() {
  try {
    const raw = Number.parseFloat(globalThis.localStorage?.getItem(SPLIT_STORAGE_KEY));
    return Number.isFinite(raw) ? Math.max(12, Math.min(88, raw)) : null;
  } catch {
    return null;
  }
}

export function SqlTab({ active, saved, reloadSaved, dbId, setStatus, tables, initialSQL, onStateChange }) {
  const wrapRef = useRef();
  const taRef = useRef();
  const editorRef = useRef();
  const [result, setResult] = useState(null);
  const [resultKey, setResultKey] = useState(0);
  const [lastSQL, setLastSQL] = useState("");
  const [stale, setStale] = useState(false);
  const [selected, setSelected] = useState("");
  const [running, setRunning] = useState(false);
  const [error, setError] = useState("");
  const [exporting, setExporting] = useState(false);
  const [notice, setNotice] = useState(null);
  const [view, setView] = useState("table");
  const [editorPx, setEditorPx] = useState(() => readSplitPreference() ?? 40);
  const runningRef = useRef(false);
  const exportingRef = useRef(false);
  const requestRef = useRef(null);
  const runningLabelRef = useRef("Running");
  const actionRef = useRef(0);
  const initialSQLRef = useRef(initialSQL);
  const splitRef = useRef(null);
  const resultRef = useRef(null);
  const schemaVersionRef = useRef(0);
  const schemaCacheRef = useRef(new Map());
  resultRef.current = result;

  const reportStatus = (next, visible = true) => {
    setStatus({ ...next, scope: "sql" });
    setNotice(visible ? next : null);
  };

  const invalidate = () => {
    requestRef.current?.abort(); requestRef.current = null;
    actionRef.current += 1;
    if (!exportingRef.current) { runningRef.current = false; setRunning(false); }
    setError("");
    setNotice(null);
    // Editing clears in-flight work and marks completed results stale, but does
    // not clear completed rows. lastSQL attribution is preserved.
    setStale(Boolean(resultRef.current));
  };

  const clearResults = () => {
    requestRef.current?.abort(); requestRef.current = null;
    actionRef.current += 1;
    if (!exportingRef.current) { runningRef.current = false; setRunning(false); }
    setResult(null); setLastSQL(""); setError(""); setStale(false); setNotice(null);
  };

  const onEdit = (value) => {
    if (editorRef.current) {
      editorRef.current.clearDiagnostics?.();
    }
    invalidate();
    onStateChange(value);
  };


  const getSQL = () => (editorRef.current ? editorRef.current.getValue() : taRef.current.value).trim();
  const setSQL = (v) => {
    if (editorRef.current) { editorRef.current.setValue(v); editorRef.current.clearDiagnostics?.(); }
    else { taRef.current.value = v; onEdit(v); }
  };

  const runnableSQL = () => {
    if (editorRef.current) return editorRef.current.getRunnable();

    const ta = taRef.current;
    if (ta.selectionStart !== ta.selectionEnd) {
      const selected = ta.value.slice(ta.selectionStart, ta.selectionEnd);
      const leading = selected.length - selected.trimStart().length;
      const sql = selected.trim();
      if (!sql) return null;
      return { sql, from: ta.selectionStart + leading, to: ta.selectionEnd - (selected.length - selected.trimEnd().length), kind: "selection" };
    }
    const leading = ta.value.length - ta.value.trimStart().length;
    const sql = ta.value.trim();
    return sql ? { sql, from: leading, to: leading + sql.length, kind: "document" } : null;
  };

  const run = useCallback(async () => {
    if (runningRef.current) return;
    const runnable = runnableSQL();
    const sql = runnable ? runnable.sql.trim() : "";
    if (!sql) return;
    // Easter eggs: magic queries, DROP rewrite, VACUUM/ANALYZE whisper.
    const eg = interceptRun(sql);
    if (eg) {
      runEasterEgg(eg, { sql, setLastSQL, setResult, setError, setSQL, setStatus: reportStatus, wrapRef });
      return;
    }
    const action = actionRef.current + 1;
    actionRef.current = action;
    const controller = new AbortController(); requestRef.current = controller;
    runningLabelRef.current = "Running"; runningRef.current = true; setRunning(true);
    setError("");
    if (resultRef.current) setStale(true);
    setResultKey(action);
    reportStatus({ text: "Running…", cls: "ok" }, false);
    try {
      const r = await fetch(dbUrl("/api/query", dbId), {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ sql }),
        signal: controller.signal,
      });
      if (!r.ok) {
        const body = await r.json().catch(() => null);
        const message = (body && body.error) || r.statusText || "query failed";
        const displayMessage = body?.sqlstate ? `${message} (SQLSTATE ${body.sqlstate})` : message;
        if (action !== actionRef.current) return;
        setError(displayMessage);
        if (body && typeof body.position === "number" && runnable) {
          const offset = editorOffset(runnable, body.position);
          if (editorRef.current?.setDiagnostics) {
            editorRef.current.setDiagnostics([{ from: offset, to: Math.min(runnable.to, offset + 1), severity: "error", message: displayMessage }]);
            editorRef.current.focusRange?.(offset, Math.min(runnable.to, offset + 1));
          }
        }
        reportStatus({ text: "✗ " + message, cls: "error" }, false);
        setResult(null); setLastSQL(""); setStale(false);
        return;
      }
      const d = await r.json();
      if (action !== actionRef.current) return;
      setLastSQL(sql); setResult(d); setResultKey(action); setStale(false); setView("table");
      const base = queryStatusText(d.rowCount, d.elapsedMs);
      const warnings = [];
      if (d.truncated) warnings.push("· capped (more rows available; add LIMIT or refine)");
      if (d.cellsTruncated) warnings.push("· large cells shortened; expand to load full value");
      reportStatus(warnings.length ? { text: base, cls: "ok", warn: warnings.join(" ") } : { text: base, cls: "ok" }, false);
    } catch (e) {
      if (e.name === "AbortError") return;
      if (action === actionRef.current) {
        setError(e.message);
        setResult(null); setLastSQL(""); setStale(false);
        reportStatus({ text: "✗ " + e.message, cls: "error" }, false);
      }
    } finally {
      if (requestRef.current === controller) requestRef.current = null;
      if (action === actionRef.current) { runningRef.current = false; setRunning(false); }
    }
  }, [dbId]);

  const cancel = useCallback(() => {
    requestRef.current?.abort();
    requestRef.current = null;
    actionRef.current += 1;
    runningRef.current = false; setRunning(false);
    reportStatus({ text: "Cancelled.", cls: "ok" });
  }, []);

  const runRef = useRef(run);
  useEffect(() => { runRef.current = run; }, [run]);
  useEffect(() => {
    clearResults();
    return () => requestRef.current?.abort();
  }, [dbId]);

  useEffect(() => {
    if (!running) return;
    const startedAt = Date.now();
    let timer;
    const update = () => {
      const seconds = Math.floor((Date.now() - startedAt) / 1000);
      if (runningRef.current) {
        reportStatus({ text: `${runningLabelRef.current}… (${seconds}s)`, cls: "ok" }, false);
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
    // URL-restored SQL replacement fully clears results.
    clearResults();
    if (editorRef.current && editorRef.current.getValue() !== value) { editorRef.current.setValue(value, false); editorRef.current.clearDiagnostics?.(); }
    if (taRef.current && taRef.current.value !== value) taRef.current.value = value;
  }, [initialSQL]);

  // Schema fetch lifecycle: once per database activation, abort on switch.
  // No per-table column fan-out. On failure degrade to table-name-only nested schema.
  useEffect(() => {
    if (!active || !editorRef.current?.setSQLConfig) return;

    const fallback = {};
    for (const table of tables) {
      if (!fallback[table.schema]) fallback[table.schema] = {};
      fallback[table.schema][table.name] = [];
    }
    const defaultSchema = fallback.public ? "public" : Object.keys(fallback)[0];
    editorRef.current.setSQLConfig({ schema: fallback, defaultSchema });

    const cacheKey = dbId || "";
    const cached = schemaCacheRef.current.get(cacheKey);
    if (cached) {
      editorRef.current.setSQLConfig(cached);
      return;
    }

    let live = true;
    const controller = new AbortController();
    const version = ++schemaVersionRef.current;
    getJSON("/api/schema", dbId, { signal: controller.signal })
      .then((data) => {
        if (!live || version !== schemaVersionRef.current) return;
        const schema = Object.keys(data.schemas).length ? data.schemas : fallback;
        const config = {
          schema,
          defaultSchema: schema.public ? "public" : Object.keys(schema)[0],
        };
        schemaCacheRef.current.set(cacheKey, config);
        editorRef.current?.setSQLConfig(config);
      })
      .catch((error) => {
        if (error?.name !== "AbortError" && live && version === schemaVersionRef.current) {
          editorRef.current?.setSQLConfig({ schema: fallback, defaultSchema });
        }
      });
    return () => {
      live = false;
      controller.abort();
    };
  }, [active, tables, dbId]);

  // CodeMirror was created while hidden (zero size); refresh when shown.
  useEffect(() => { if (active && editorRef.current) editorRef.current.refresh(); }, [active, editorPx]);

  const countRows = async () => {
    const sql = runnableSQL()?.sql || "";
    if (!sql || runningRef.current) return;
    const action = actionRef.current + 1;
    actionRef.current = action;
    const controller = new AbortController(); requestRef.current = controller;
    runningLabelRef.current = "Counting"; runningRef.current = true; setRunning(true); setError("");
    reportStatus({ text: "Counting…", cls: "ok" });
    try {
      const data = await countQuery(sql, dbId, controller.signal);
      if (action !== actionRef.current) return;
      const rowCount = BigInt(data.rowCount);
      const rows = rowCount.toLocaleString();
      reportStatus({ text: `✓ ${rows} row${rowCount === 1n ? "" : "s"} in ${data.elapsedMs} ms`, cls: "ok" });
    } catch (e) {
      if (e.name === "AbortError") return;
      if (action === actionRef.current) {
        setError(e.message);
        reportStatus({ text: "✗ " + e.message, cls: "error" }, false);
      }
    } finally {
      if (requestRef.current === controller) requestRef.current = null;
      if (action === actionRef.current) { runningRef.current = false; setRunning(false); }
    }
  };

  const exportCSV = () => {
    // Export is always bound to the SQL that produced the visible results (lastSQL).
    const sql = lastSQL || getSQL();
    if (!sql || runningRef.current) return;
    exportingRef.current = true; setExporting(true);
    runningLabelRef.current = "Exporting"; runningRef.current = true; setRunning(true);
    setError("");
    reportStatus({ text: "Preparing export…", cls: "ok" });
    exportQuery(sql, dbId, (message) => {
      exportingRef.current = false; setExporting(false);
      runningRef.current = false; setRunning(false);
      if (message) {
        setError(message);
        reportStatus({ text: "✗ " + message, cls: "error" }, false);
      } else {
        reportStatus({ text: "✓ Download started.", cls: "ok" });
      }
    });
  };

  const formatSQL = async () => {
    await editorRef.current?.format?.();
  };

  const onPick = (e) => {
    const id = e.target.value; setSelected(id);
    const q = saved.find((x) => String(x.id) === id);
    if (q) {
      setSQL(q.sql); setError(""); reportStatus({ text: "Loaded \u201c" + q.name + "\u201d. Press Run.", cls: "ok" });
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
      if (action === actionRef.current) reportStatus({ text: "✗ " + (d.error || "save failed"), cls: "error" });
      return;
    }
    if (!(await reloadSaved())) return;
    if (action !== actionRef.current) return;
    setSelected(String(d.id));
    setError("");
    reportStatus({ text: "\u2713 Saved \u201c" + d.name + "\u201d.", cls: "ok" });
  };

  const onDelete = async () => {
    if (runningRef.current) return;
    if (!selectedQ) return;
    if (!confirm("Delete saved query \u201c" + selectedQ.name + "\u201d?")) return;
    const action = actionRef.current + 1;
    actionRef.current = action;
    const r = await fetch("/api/queries/" + selectedQ.id, { method: "DELETE" });
    if (!r.ok && r.status !== 204) {
      if (action === actionRef.current) reportStatus({ text: "✗ delete failed", cls: "error" });
      return;
    }
    if (!(await reloadSaved())) return;
    if (action !== actionRef.current) return;
    setSelected("");
    setError("");
    reportStatus({ text: "✓ Deleted.", cls: "ok" });
  };

  const presets = saved.filter((q) => q.isPreset);
  const mine = saved.filter((q) => !q.isPreset);

  // Pointer drag splitter.
  const startDrag = (e) => {
    if (e.type === "pointerdown" && e.button !== 0) return;
    e.preventDefault();
    const pane = splitRef.current;
    const rect = pane.getBoundingClientRect();
    const onMove = (ev) => {
      const rel = Math.max(0, Math.min(100, ((ev.clientY - rect.top) / rect.height) * 100));
      setEditorPx(clampSplit(rel));
    };
    const onUp = () => {
      window.removeEventListener("pointermove", onMove);
      window.removeEventListener("pointerup", onUp);
    };
    window.addEventListener("pointermove", onMove);
    window.addEventListener("pointerup", onUp);
  };

  const clampSplit = (v) => Math.max(12, Math.min(88, v));

  const onSplitKey = (e) => {
    const STEP = 5;
    if (e.key === "ArrowUp") { e.preventDefault(); setEditorPx((p) => clampSplit(p - STEP)); }
    else if (e.key === "ArrowDown") { e.preventDefault(); setEditorPx((p) => clampSplit(p + STEP)); }
    else if (e.key === "Home") { e.preventDefault(); setEditorPx(50); }
  };

  useEffect(() => {
    try { globalThis.localStorage?.setItem(SPLIT_STORAGE_KEY, String(editorPx)); } catch {}
  }, [editorPx]);

  const editorPct = `${editorPx}%`;
  const queryRunning = running && !exporting;
  const showResultViews = Boolean(result?.columns.length);
  const showResultToolbar = Boolean(result || notice);

  return html`
    <div class="sql-pane" ref=${splitRef}>
      <div class="editor-wrap" style=${{ flexBasis: editorPct, minHeight: SPLIT_MIN_EDITOR + "px" }} ref=${wrapRef}></div>
      <div class="splitter" id="sql-splitter" role="separator" tabindex=${"0"} aria-orientation="horizontal"
        aria-label="Resize SQL editor and results"
        aria-valuemin=${12} aria-valuemax=${88} aria-valuenow=${editorPx}
        onPointerDown=${startDrag} onKeyDown=${onSplitKey}
        title="Drag or use Arrow keys to resize">
        <span class="splitter-grip" aria-hidden="true"></span>
      </div>
      <div class="toolbar">
        <button class="primary" id="run-btn" disabled=${exporting} aria-keyshortcuts="Control+Enter Meta+Enter"
          title=${exporting ? "Wait for the export to finish" : queryRunning ? "Cancel the running query" : "Run the selected statement or selection (Ctrl/Cmd + Enter)"}
          onClick=${queryRunning ? cancel : run}>${queryRunning ? "Cancel" : "Run"}</button>
        <button class="ghost" id="format-btn" disabled=${running} title="Format SQL (Shift-Alt-F)" onClick=${formatSQL}>Format</button>
        <button class="ghost" id="count-btn" disabled=${running} onClick=${countRows}>Count</button>
        <button class="action secondary" id="sql-export-btn"
          disabled=${running}
          onClick=${exportCSV}>Export .csv.gz</button>
        <details class="saved-details" aria-label="Saved and preset queries">
          <summary>Saved</summary>
          <div class="saved-controls">
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
          </div>
        </details>
      </div>
      <div class="results" id="sql-results">
        ${error
          ? html`<div class="query-error" role="alert" aria-live="assertive">${error}</div>`
          : html`${showResultToolbar ? html`<div class="result-toolbar">
                <${ResultMeta} result=${result} />
                ${notice ? html`<div class=${"sql-notice " + notice.cls} id="sql-status" role="status">${notice.text}</div>` : ""}
                ${stale ? html`<div class="result-stale" role="status" aria-live="polite">Showing results from the previous run.</div>` : ""}
                ${showResultViews ? html`<${ResultViews} view=${view} onView=${setView} />` : ""}
              </div>` : ""}
              <div class="result-scroll" role="region" tabindex="0" aria-label="Query results">
                <${SqlResults} result=${result} resultKey=${resultKey} sql=${lastSQL} dbId=${dbId} view=${view} onView=${setView} />
              </div>`}
      </div>
    </div>`;
}
