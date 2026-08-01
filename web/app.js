// pgpeek UI — Preact + htm, no build step. This file is the entrypoint;
// feature modules live in ./theme.js, ./sidebar.js, ./data-tab.js, etc.
import {
  html, render, useState, useEffect, useRef, useCallback,
} from "./vendor/preact-htm.js";
import { ThemeSelect } from "./theme.js";
import { Sidebar, Tabs } from "./sidebar.js";
import { DataTab } from "./data-tab.js";
import { StructureTab } from "./structure-tab.js";
import { SqlTab } from "./sql-tab.js";
import { getJSON, tableKey } from "./api.js";
import { readUrlState, pushUrlState, replaceUrlState } from "./url-state.js";

const PAGE_SIZE = 100;

// ---- Database selector (hidden when ≤1 database) ----
function DatabaseSelect({ databases, currentDb, onSwitch }) {
  if (databases.length <= 1) return null;
  return html`
    <label class="db-select" title="Switch database/cluster">Database
      <select id="database-select" value=${currentDb}
        onChange=${(e) => onSwitch(e.target.value)}>
        ${databases.map(({ id, name }) => html`<option key=${id} value=${id}>${name}</option>`)}
      </select>
    </label>`;
}

function TableContext({ table }) {
  if (!table) {
    return html`<div class="context-bar empty-context" id="table-context" aria-live="polite">
      <span class="context-kicker">No table selected</span>
      <strong>Choose a table from the left to browse rows, structure, or SQL.</strong>
    </div>`;
  }
  return html`<div class="context-bar" id="table-context" aria-live="polite">
    <span class="context-kicker">Current ${table.type === "view" ? "view" : "table"}</span>
    <strong>${table.schema}<span>.</span>${table.name}</strong>
    ${table.estRows >= 0
      ? html`<span class="context-meta">~${table.estRows} rows</span>`
      : html`<span class="context-meta">row count unavailable</span>`}
  </div>`;
}

function UserBadge({ user }) {
  if (!user || user.provider !== "cloudflare-access") return null;
  return html`<span class="badge" id="current-user" title="Cloudflare Access user">${user.email || "Cloudflare Access"}</span>`;
}

function formatUptime(seconds) {
  if (!seconds) return "unknown";
  const days = Math.floor(seconds / 86400);
  const hours = Math.floor((seconds % 86400) / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  return [days && `${days}d`, hours && `${hours}h`, minutes && `${minutes}m`].filter(Boolean).join(" ") || "< 1m";
}

const ignoreRefreshError = () => {};

function About({ open, onClose, databases, build }) {
  const dialog = useRef(null);
  useEffect(() => {
    if (open && !dialog.current.open) dialog.current.showModal();
  }, [open]);
  if (!open) return null;
  const closeFromBackdrop = (e) => {
    const box = e.currentTarget.getBoundingClientRect();
    if (e.clientX < box.left || e.clientX > box.right || e.clientY < box.top || e.clientY > box.bottom) e.currentTarget.close();
  };
  return html`<dialog class="about" ref=${dialog} aria-labelledby="about-title" onClose=${onClose} onClick=${closeFromBackdrop}>
    <section class="about-card">
      <div class="about-head"><h2 id="about-title">About pgpeek</h2><button class="ghost" autofocus onClick=${() => dialog.current.close()}>Close</button></div>
      <p>A minimal, read-only, team-shared PostgreSQL browser for browsing tables, inspecting schemas, and running guarded queries.</p>
      <dl>
        <dt>Version</dt><dd>${build.version || "unknown"}</dd>
        <dt>Git commit</dt><dd>${build.commit || "unknown"}</dd>
        <dt>Built</dt><dd>${build.buildDate || "unknown"}</dd>
      </dl>
      <h3>Connected databases</h3>
      ${databases.length
        ? html`<div class="database-info">${databases.map((database, index) => html`<details class="database-card" key=${database.id} open=${databases.length === 1 || index === 0}>
            <summary>${database.name} <span>${database.id}</span></summary>
            ${database.error
              ? html`<p>${database.error}</p>`
              : html`<dl>
                  <dt>PostgreSQL</dt><dd>${database.version || "unknown"}</dd>
                  <dt>Uptime</dt><dd>${formatUptime(database.uptimeSeconds)}</dd>
                  <dt>Database size</dt><dd>${database.databaseSize || "unknown"}</dd>
                  <dt>Connections</dt><dd>${database.activeConnections || 0} / ${database.maxConnections || "unknown"}</dd>
                  <dt>pgpeek pool</dt><dd>${database.poolMaxConnections || "unknown"} max</dd>
                  <dt>Transactions</dt><dd>${database.commits || 0} committed / ${database.rollbacks || 0} rolled back</dd>
                  <dt>Cache hit</dt><dd>${database.cacheHitPercent || 0}%</dd>
                  <dt>Temp spill</dt><dd>${database.tempFiles || 0} files / ${database.tempBytes || "0 bytes"}</dd>
                  <dt>Deadlocks</dt><dd>${database.deadlocks || 0}</dd>
                  <dt>Sessions</dt><dd>${database.sessions || 0}</dd>
                  <dt>Extensions</dt><dd>${database.extensions || "none"}</dd>
                </dl>`}
          </details>`)}</div>`
        : html`<p>No databases reported.</p>`}
    </section>
  </dialog>`;
}

// ---- App ----
function App() {
  const [databases, setDatabases]     = useState([]);
  const [dbsLoaded, setDbsLoaded]     = useState(false);
  const [currentDb, setCurrentDb]     = useState(null);
  const [tables, setTables]           = useState([]);
  const [tablesLoaded, setTablesLoaded] = useState(false);
  const [rowCap, setRowCap]           = useState(PAGE_SIZE);
  const [saved, setSaved]             = useState([]);
  const [tab, setTabState]            = useState("data");
  const [sql, setSql]                 = useState(null);
  const [current, setCurrent]         = useState(null);
  const [navKey, setNavKey]           = useState(0);
  const [pendingFilters, setPendingFilters] = useState(null);
  const [urlInit, setUrlInit]         = useState(null);
  const [user, setUser]               = useState(null);
  const [aboutOpen, setAboutOpen]     = useState(false);
  const [build, setBuild]             = useState({});
  const [status, setStatus]           = useState({ text: "Ready.", cls: "ok" });
  // Refs so popstate handler always sees the latest values.
  const urlStateRef = useRef({});
  const dbRef       = useRef(null);
  const tablesRef   = useRef([]);

  const setTab = useCallback((newTab) => {
    setTabState(newTab);
    const s = { ...urlStateRef.current, tab: newTab === "data" ? null : newTab };
    pushUrlState(s);
    urlStateRef.current = s;
  }, []);

  const reloadSaved = useCallback(async () => {
    try { setSaved(await getJSON("/api/queries")); }
    catch (e) { setStatus({ text: "✗ failed to load saved queries: " + e.message, cls: "error" }); }
  }, []);

  useEffect(() => {
    let live = true;
    getJSON("/api/user")
      .then((u) => { if (live) setUser(u); })
      .catch(() => {});
    return () => { live = false; };
  }, []);

  useEffect(() => {
    if (!aboutOpen) return;
    getJSON("/healthz").then(setBuild, ignoreRefreshError);
    getJSON("/api/databases").then((result) => setDatabases(Array.isArray(result.databases) ? result.databases : []), ignoreRefreshError);
  }, [aboutOpen]);

  // Phase 1: fetch /api/databases, resolve active db, restore URL state,
  //          install popstate listener.
  useEffect(() => {
    let urlState = readUrlState();
    urlStateRef.current = urlState;

    (async () => {
      let dbId = null;
      try {
        const r = await fetch("/api/databases");

        if (!r.ok) throw new Error(r.statusText || "failed");
        const result = await r.json();
        urlState = readUrlState(); urlStateRef.current = urlState;
        const dbs = Array.isArray(result.databases) ? result.databases : [];
        setDatabases(dbs);
        const urlDb  = urlState.db;
        const valid  = dbs.find((d) => d.id === urlDb);
        dbId = valid ? urlDb : (result.defaultId || (dbs[0] && dbs[0].id) || null);
        if (urlDb && !valid && dbs.length > 0) {
          setStatus({ text: "✗ unknown database in URL, using default", cls: "error" });
        }
      } catch (e) {
        urlState = readUrlState(); urlStateRef.current = urlState;
        setStatus({ text: "✗ failed to load databases: " + e.message, cls: "error" });
      }


      // Restore tab from URL; build pending table-restore if schema+table present.
      setTabState(urlState.tab); setSql(urlState.sql);
      const finalState = { ...urlState, db: dbId };
      replaceUrlState(finalState);
      urlStateRef.current = finalState;
      dbRef.current = dbId;

      if (urlState.schema && urlState.table) {
        setUrlInit({
          schema: urlState.schema, table: urlState.table,
          offset: urlState.offset, search: urlState.search,
          sort: urlState.sort, filters: urlState.filters,
        });
      }
      setCurrentDb(dbId); setDbsLoaded(true);
    })();

    const onPopstate = () => {
      const s = readUrlState();
      const sameDb = s.db === dbRef.current;
      urlStateRef.current = s;
      dbRef.current = s.db;
      setTabState(s.tab);
      setSql(s.sql);
      setCurrentDb(s.db);
      if (!sameDb) {
        setCurrent(null);
        // Tables effect will reload; queue table restore if URL has a table.
        if (s.schema && s.table) {
          setUrlInit({ schema: s.schema, table: s.table,
            offset: s.offset, search: s.search, sort: s.sort, filters: s.filters });
        } else setUrlInit(null);
      } else if (s.schema && s.table) {
        const found = tablesRef.current.find((x) => x.schema === s.schema && x.name === s.table);
        if (found) { setCurrent(found); setNavKey((k) => k + 1); setUrlInit(s); }
        else setCurrent(null);
      } else {
        setCurrent(null);
      }
    };
    window.addEventListener("popstate", onPopstate);
    return () => { window.removeEventListener("popstate", onPopstate); };
  }, []);

  // Phase 2: reload tables + meta whenever the resolved db changes.
  useEffect(() => {
    if (!dbsLoaded) return;
    let live = true;
    setTablesLoaded(false);
    tablesRef.current = [];
    setTables([]);
    const db = currentDb;
    (async () => {
      try {
        const t = await getJSON("/api/tables", db);
        if (!live) return;
        tablesRef.current = t;
        setTables(t);
        // Consume pending URL table-restore (set during initial load or popstate).
        setUrlInit((prev) => {
          if (!prev) return null;
          const found = t.find((x) => x.schema === prev.schema && x.name === prev.table);
          if (found) { setCurrent(found); setNavKey((k) => k + 1); return prev; }
          return null;
        });
      } catch (e) {
        if (live) setStatus({ text: "✗ failed to load tables: " + e.message, cls: "error" });
      } finally { if (live) setTablesLoaded(true); }
    })();
    (async () => {
      try {
        const m = await getJSON("/api/meta", db);
        if (live && m && m.rowCap > 0) setRowCap(m.rowCap);
      } catch { /* keep default */ }
    })();
    reloadSaved();
    return () => { live = false; };
  }, [currentDb, dbsLoaded]);

  const open = (t, initFilters) => {
    const s = {
      ...urlStateRef.current, tab: null,
      schema: t.schema, table: t.name,
      offset: 0, search: "", sort: null, filters: initFilters || [],
    };
    setCurrent(t); setPendingFilters(initFilters || null); setUrlInit(null);
    setNavKey((n) => n + 1); setTabState("data");
    pushUrlState(s); urlStateRef.current = s;
  };

  const onNavigate = (ref, value) => {
    const target = tables.find((x) => x.schema === ref.schema && x.name === ref.table);
    if (!target) {
      setStatus({ text: "✗ referenced table " + ref.schema + "." + ref.table + " is not browsable", cls: "error" });
      return;
    }
    open(target, [{ column: ref.column, op: "eq", value: String(value) }]);
  };

  const switchDb = (newDb) => {
    if (newDb === currentDb) return;
    const s = { db: newDb, tab: null, schema: null, table: null, offset: 0, search: "", sort: null, filters: [], sql: urlStateRef.current.sql };
    setCurrent(null); setPendingFilters(null); setUrlInit(null);
    setTabState("data");
    dbRef.current = newDb; setCurrentDb(newDb);
    pushUrlState(s); urlStateRef.current = s;
  };

  const onDataStateChange = (dataState) => {
    const s = { ...urlStateRef.current, ...dataState };
    urlStateRef.current = s; replaceUrlState(s);
  };

  const onSqlStateChange = (value) => {
    setSql(value); const s = { ...urlStateRef.current, sql: value };
    urlStateRef.current = s; replaceUrlState(s);
  };

  const pageSize  = Math.min(PAGE_SIZE, rowCap);
  const title     = current ? tableKey(current) : "Pick a table";
  const dtFilters = urlInit ? urlInit.filters : pendingFilters;

  return html`
    <a class="skip-link" href="#main">Skip to data browser</a>
    <header>
      <h1>pgpeek</h1><span class="badge">read-only</span>
      <${UserBadge} user=${user} />
      <${DatabaseSelect} databases=${databases} currentDb=${currentDb} onSwitch=${switchDb} />
      <button class="about-button" onClick=${() => setAboutOpen(true)}>About</button>
      <${ThemeSelect} />
    </header>
    <div class="body">
      <${Sidebar} tables=${tables} loaded=${tablesLoaded}
        currentKey=${current && tableKey(current)} onSelect=${(t) => open(t)} />
      <main id="main">
        <${Tabs} tab=${tab} setTab=${setTab} title=${title} />
        <${TableContext} table=${current} />
        <div class=${"status " + status.cls} id="status" role="status">
          ${status.text}${status.warn ? html`<span class="warn"> ${status.warn}</span>` : ""}
        </div>
        <section class="panel" id="panel-data" role="tabpanel" aria-labelledby="tab-data" hidden=${tab !== "data"}>
          ${current
            ? html`<${DataTab} key=${navKey} table=${current} pageSize=${pageSize}
                dbId=${currentDb}
                initialFilters=${dtFilters || null}
                initialOffset=${urlInit ? (urlInit.offset || 0) : 0}
                initialSearch=${urlInit ? (urlInit.search || "") : ""}
                initialSort=${urlInit ? (urlInit.sort || null) : null}
                onNavigate=${onNavigate} setStatus=${setStatus}
                onStateChange=${onDataStateChange} />`
            : html`<div class="results"><div class="empty">Select a table to browse its rows.</div></div>`}
        </section>
        <section class="panel" id="panel-structure" role="tabpanel" aria-labelledby="tab-structure" hidden=${tab !== "structure"}>
          ${current && tab === "structure"
            ? html`<${StructureTab} key=${"s" + navKey} table=${current}
                dbId=${currentDb} setStatus=${setStatus} />`
            : html`<div class="results"><div class="empty">Select a table to see its structure.</div></div>`}
        </section>
        <section class="panel" id="panel-sql" role="tabpanel" aria-labelledby="tab-sql" hidden=${tab !== "sql"}>
          <${SqlTab} active=${tab === "sql"} saved=${saved} reloadSaved=${reloadSaved}
            dbId=${currentDb} setStatus=${setStatus} tables=${tables}
            initialSQL=${sql} onStateChange=${onSqlStateChange} />
        </section>
      </main>
    </div>
    <${About} open=${aboutOpen} onClose=${() => setAboutOpen(false)} databases=${databases} build=${build} />`;
}

render(html`<${App} />`, document.getElementById("app"));
