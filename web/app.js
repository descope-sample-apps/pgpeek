// pgpeek UI. Kept in a separate file so the Content-Security-Policy can forbid
// inline scripts (script-src 'self' + the CodeMirror CDN, no 'unsafe-inline').
(function () {
  "use strict";
  const $ = (id) => document.getElementById(id);
  const statusEl = $("status");
  const resultsEl = $("results");
  const presetsEl = $("presets");
  let editor = null;
  let savedQueries = [];
  let lastSQL = "";

  // Progressive enhancement: CodeMirror if it loaded, else the raw textarea.
  if (window.CodeMirror) {
    editor = CodeMirror.fromTextArea($("sql"), {
      mode: "text/x-pgsql", lineNumbers: true, theme: "default",
      lineWrapping: true, autofocus: true,
    });
    editor.setOption("extraKeys", { "Cmd-Enter": run, "Ctrl-Enter": run });
  } else {
    $("sql").addEventListener("keydown", (e) => {
      if ((e.metaKey || e.ctrlKey) && e.key === "Enter") { e.preventDefault(); run(); }
    });
  }
  const getSQL = () => (editor ? editor.getValue() : $("sql").value).trim();
  const setSQL = (v) => editor ? editor.setValue(v) : ($("sql").value = v);

  function setStatus(msg, cls) { statusEl.className = "status " + cls; statusEl.textContent = msg; }
  function setStatusHTML(html, cls) { statusEl.className = "status " + cls; statusEl.innerHTML = html; }

  function renderTable(res) {
    resultsEl.replaceChildren();
    if (!res.columns.length) { resultsEl.append(empty("Query ran. No columns returned.")); return; }
    if (!res.rows.length) { resultsEl.append(empty("0 rows.")); return; }
    const table = document.createElement("table");
    const thead = document.createElement("thead");
    const htr = document.createElement("tr");
    for (const c of res.columns) { const th = document.createElement("th"); th.textContent = c; htr.append(th); }
    thead.append(htr); table.append(thead);
    const tbody = document.createElement("tbody");
    for (const row of res.rows) {
      const tr = document.createElement("tr");
      for (const cell of row) {
        const td = document.createElement("td");
        if (cell === null || cell === undefined) { td.className = "null"; td.textContent = "NULL"; }
        else td.textContent = typeof cell === "object" ? JSON.stringify(cell) : String(cell);
        tr.append(td);
      }
      tbody.append(tr);
    }
    table.append(tbody);
    resultsEl.append(table);
  }

  function empty(text) { const d = document.createElement("div"); d.className = "empty"; d.textContent = text; return d; }

  async function run() {
    const sql = getSQL();
    if (!sql) return;
    setStatus("Running…", "ok");
    $("run-btn").disabled = true; $("export-btn").disabled = true;
    try {
      const r = await fetch("/api/query", {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ sql }),
      });
      const data = await r.json();
      if (!r.ok) { setStatus("✗ " + (data.error || r.statusText), "error"); resultsEl.replaceChildren(); return; }
      lastSQL = sql;
      renderTable(data);
      let msg = "✓ " + data.rowCount + " row" + (data.rowCount === 1 ? "" : "s") + " in " + data.elapsedMs + " ms";
      if (data.truncated) {
        setStatusHTML(msg + ' <span class="warn">· capped (more rows available — add LIMIT or refine)</span>', "ok");
      } else {
        setStatus(msg, "ok");
      }
      $("export-btn").disabled = data.rowCount === 0;
    } catch (e) {
      setStatus("✗ " + e.message, "error");
    } finally {
      $("run-btn").disabled = false;
    }
  }

  async function exportCSV() {
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
    presetsEl.replaceChildren(new Option("Saved queries…", ""));
    const addGroup = (label, items) => {
      if (!items.length) return;
      const g = document.createElement("optgroup"); g.label = label;
      for (const q of items) g.append(new Option(q.name, q.id));
      presetsEl.append(g);
    };
    addGroup("Presets", savedQueries.filter((q) => q.isPreset));
    addGroup("Saved", savedQueries.filter((q) => !q.isPreset));
  }

  presetsEl.addEventListener("change", () => {
    const id = presetsEl.value;
    const q = savedQueries.find((x) => String(x.id) === id);
    $("delete-btn").disabled = !(q && !q.isPreset);
    if (q) { setSQL(q.sql); setStatus("Loaded “" + q.name + "”. Press Run.", "ok"); }
  });

  $("run-btn").addEventListener("click", run);
  $("export-btn").addEventListener("click", exportCSV);

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
    await loadSaved(); presetsEl.value = d.id; $("delete-btn").disabled = false;
    setStatus("✓ Saved “" + d.name + "”.", "ok");
  });

  $("delete-btn").addEventListener("click", async () => {
    const id = presetsEl.value;
    if (!id) return;
    const q = savedQueries.find((x) => String(x.id) === id);
    if (!q || !confirm("Delete saved query “" + q.name + "”?")) return;
    const r = await fetch("/api/queries/" + id, { method: "DELETE" });
    if (!r.ok && r.status !== 204) { setStatus("✗ delete failed", "error"); return; }
    await loadSaved(); $("delete-btn").disabled = true;
    setStatus("✓ Deleted.", "ok");
  });

  loadSaved().catch((e) => setStatus("✗ failed to load saved queries: " + e.message, "error"));
})();
