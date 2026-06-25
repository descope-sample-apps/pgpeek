// @vitest-environment jsdom
// Tests: URL db param, URL tab/table state, popstate restore,
// url-state and api module helpers, and coverage-completion edge cases.
import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import {
  flush, makeResp, TWO_DBS, NO_DBS, SAMPLE_TABLES,
  makeInstallFetch, $, click, changeSelect, loadApp, urlOf,
  readUrlState, buildUrlParams, dbUrl,
} from "./test-helpers.js";

let routes;
function setRoute(key, resp) { routes[key] = resp; }
const installFetch = makeInstallFetch(() => routes);

function defaultRoutes() {
  return {
    "GET /api/databases": makeResp({ json: TWO_DBS }),
    "GET /api/meta":      makeResp({ json: { rowCap: 100 } }),
    "GET /api/tables":    makeResp({ json: [] }),
    "GET /api/tables/*/columns": makeResp({ json: [] }),
    "GET /api/tables/*/fks":     makeResp({ json: [] }),
    "GET /api/queries":   makeResp({ json: [] }),
  };
}

beforeEach(() => {
  document.body.innerHTML = '<div id="app"></div>';
  window.history.replaceState({}, "", "/");
  routes = defaultRoutes();
  installFetch();
  globalThis.prompt = vi.fn();
  globalThis.confirm = vi.fn();
  globalThis.URL.createObjectURL = vi.fn(() => "blob:fake");
  globalThis.URL.revokeObjectURL = vi.fn();
  HTMLAnchorElement.prototype.click = vi.fn();
  Element.prototype.scrollIntoView = vi.fn();
  globalThis.requestAnimationFrame = (cb) => setTimeout(cb, 0);
  globalThis.cancelAnimationFrame  = (id) => clearTimeout(id);
  window.requestAnimationFrame = globalThis.requestAnimationFrame;
  window.cancelAnimationFrame  = globalThis.cancelAnimationFrame;
  delete window.CodeMirror;
  delete globalThis.CodeMirror;
});

afterEach(() => {
  vi.restoreAllMocks();
  window.history.replaceState({}, "", "/");
  delete window.CodeMirror;
  delete globalThis.CodeMirror;
});

// ── URL db param ──────────────────────────────────────────────────────────────

describe("URL db param", () => {
  it("writes defaultId into URL on load when no ?db= in URL", async () => {
    await loadApp();
    expect(new URLSearchParams(window.location.search).get("db")).toBe("pg1");
  });

  it("uses ?db= from URL when it matches a known database", async () => {
    window.history.replaceState({}, "", "/?db=pg2");
    await loadApp();
    expect($("database-select").value).toBe("pg2");
    expect(new URLSearchParams(window.location.search).get("db")).toBe("pg2");
  });

  it("falls back to defaultId and shows error when ?db= is unknown", async () => {
    window.history.replaceState({}, "", "/?db=nonexistent");
    await loadApp();
    expect($("status").textContent).toContain("unknown database");
    expect($("database-select").value).toBe("pg1");
  });

  it("falls back to first database when defaultId is null and URL has no db", async () => {
    setRoute("GET /api/databases", makeResp({
      json: { defaultId: null, databases: [{ id: "first", name: "First" }, { id: "second", name: "Second" }] },
    }));
    await loadApp();
    expect(new URLSearchParams(window.location.search).get("db")).toBe("first");
  });

  it("pushes new db into URL when database is switched", async () => {
    await loadApp();
    await changeSelect($("database-select"), "pg2");
    expect(new URLSearchParams(window.location.search).get("db")).toBe("pg2");
  });

  it("db switch clears schema/table/offset/search from URL", async () => {
    setRoute("GET /api/tables", makeResp({ json: SAMPLE_TABLES }));
    setRoute("GET /api/tables/*/data", makeResp({ json: { columns: ["id"], rows: [[1]], rowCount: 1, elapsedMs: 1 } }));
    await loadApp();
    await click($("tables").querySelectorAll(".tbl")[0]);
    expect(new URLSearchParams(window.location.search).get("table")).toBe("users");
    await changeSelect($("database-select"), "pg2");
    const p = new URLSearchParams(window.location.search);
    expect(p.has("schema")).toBe(false);
    expect(p.has("table")).toBe(false);
    expect(p.has("offset")).toBe(false);
  });
});

// ── URL state — tab and table ─────────────────────────────────────────────────

describe("URL state — tab and table", () => {
  it("restores tab=sql from URL on initial load", async () => {
    window.history.replaceState({}, "", "/?db=pg1&tab=sql");
    await loadApp();
    expect($("panel-sql").hidden).toBe(false);
    expect($("panel-data").hidden).toBe(true);
  });

  it("data tab is default; URL has no tab param when on data tab after load", async () => {
    await loadApp();
    expect(new URLSearchParams(window.location.search).has("tab")).toBe(false);
  });

  it("pushes tab into URL when tab changes", async () => {
    await loadApp();
    await click("tab-sql");
    expect(new URLSearchParams(window.location.search).get("tab")).toBe("sql");
  });

  it("tab=data is omitted from URL params (canonical form)", async () => {
    await loadApp();
    await click("tab-sql");
    await click("tab-data");
    expect(new URLSearchParams(window.location.search).has("tab")).toBe(false);
  });

  it("pushes schema and table into URL when table is selected", async () => {
    setRoute("GET /api/tables", makeResp({ json: SAMPLE_TABLES }));
    setRoute("GET /api/tables/*/data", makeResp({ json: { columns: ["id"], rows: [[1]], rowCount: 1, elapsedMs: 1 } }));
    await loadApp();
    await click($("tables").querySelectorAll(".tbl")[0]);
    const p = new URLSearchParams(window.location.search);
    expect(p.get("schema")).toBe("public");
    expect(p.get("table")).toBe("users");
  });

  it("restores table from URL on initial load", async () => {
    window.history.replaceState({}, "", "/?db=pg1&schema=public&table=users");
    setRoute("GET /api/tables", makeResp({ json: SAMPLE_TABLES }));
    setRoute("GET /api/tables/*/data", makeResp({ json: { columns: ["id"], rows: [[1]], rowCount: 1, elapsedMs: 1 } }));
    await loadApp();
    expect($("tab-title").textContent).toBe("public.users");
  });

  it("gracefully ignores unknown table in URL on initial load", async () => {
    window.history.replaceState({}, "", "/?db=pg1&schema=public&table=nope");
    setRoute("GET /api/tables", makeResp({ json: SAMPLE_TABLES }));
    await loadApp();
    expect($("tab-title").textContent).toBe("Pick a table");
  });
});

// ── URL state — popstate ──────────────────────────────────────────────────────

describe("URL state — popstate", () => {
  it("restores tab on popstate (sql → data)", async () => {
    await loadApp();
    await click("tab-sql");
    expect($("panel-sql").hidden).toBe(false);
    window.history.replaceState({}, "", "/?db=pg1");
    window.dispatchEvent(new PopStateEvent("popstate"));
    await flush();
    expect($("panel-data").hidden).toBe(false);
    expect($("panel-sql").hidden).toBe(true);
  });

  it("restores db on popstate", async () => {
    await loadApp();
    await changeSelect($("database-select"), "pg2");
    expect($("database-select").value).toBe("pg2");
    window.history.replaceState({}, "", "/?db=pg1");
    window.dispatchEvent(new PopStateEvent("popstate"));
    await flush();
    expect($("database-select").value).toBe("pg1");
  });
});

// ── popstate with table in URL (branch coverage) ──────────────────────────────

describe("popstate with table in URL", () => {
  it("queues table restore when db changes via popstate and URL has schema+table", async () => {
    // Covers app.js: !sameDb branch when URL has table.
    setRoute("GET /api/tables", makeResp({ json: SAMPLE_TABLES }));
    setRoute("GET /api/tables/*/data", makeResp({ json: { columns: ["id"], rows: [[1]], rowCount: 1, elapsedMs: 1 } }));
    await loadApp();
    await changeSelect($("database-select"), "pg2");
    window.history.replaceState({}, "", "/?db=pg1&schema=public&table=users");
    window.dispatchEvent(new PopStateEvent("popstate"));
    await flush();
    expect($("database-select").value).toBe("pg1");
  });

  it("restores table from already-loaded list on same-db popstate", async () => {
    // Covers app.js: sameDb branch when found.
    setRoute("GET /api/tables", makeResp({ json: SAMPLE_TABLES }));
    setRoute("GET /api/tables/*/data", makeResp({ json: { columns: ["id"], rows: [[1]], rowCount: 1, elapsedMs: 1 } }));
    await loadApp();
    await click($("tables").querySelectorAll(".tbl")[1]); // public.posts
    expect($("tab-title").textContent).toBe("public.posts");
    window.history.replaceState({}, "", "/?db=pg1&schema=public&table=users");
    window.dispatchEvent(new PopStateEvent("popstate"));
    await flush();
    expect($("tab-title").textContent).toBe("public.users");
  });

  it("clears current table on same-db popstate when table not found in list", async () => {
    // Covers app.js: sameDb branch when NOT found.
    setRoute("GET /api/tables", makeResp({ json: SAMPLE_TABLES }));
    setRoute("GET /api/tables/*/data", makeResp({ json: { columns: ["id"], rows: [[1]], rowCount: 1, elapsedMs: 1 } }));
    await loadApp();
    await click($("tables").querySelectorAll(".tbl")[0]);
    expect($("tab-title").textContent).toBe("public.users");
    window.history.replaceState({}, "", "/?db=pg1&schema=public&table=missing");
    window.dispatchEvent(new PopStateEvent("popstate"));
    await flush();
    expect($("tab-title").textContent).toBe("Pick a table");
  });
});

// ── url-state module unit tests ───────────────────────────────────────────────

describe("url-state helpers", () => {
  it("readUrlState parses all supported params", async () => {
    window.history.replaceState({}, "", "/?db=x&tab=sql&schema=s&table=t&offset=50&search=foo&sort=id&dir=desc&f=id:eq:5&f=name:is_null");
    const s = readUrlState();
    expect(s.db).toBe("x");
    expect(s.tab).toBe("sql");
    expect(s.schema).toBe("s");
    expect(s.table).toBe("t");
    expect(s.offset).toBe(50);
    expect(s.search).toBe("foo");
    expect(s.sort).toEqual({ col: "id", dir: "desc" });
    expect(s.filters["id"]).toEqual({ op: "eq", value: "5" });
    expect(s.filters["name"]).toEqual({ op: "is_null", value: "" });
  });

  it("readUrlState falls back to 'data' for an invalid tab value", async () => {
    window.history.replaceState({}, "", "/?tab=invalid");
    expect(readUrlState().tab).toBe("data");
  });

  it("buildUrlParams omits tab when data, omits falsy fields", async () => {
    const p = buildUrlParams({ db: "x", tab: "data", schema: null, table: null, offset: 0, search: "", sort: null, filters: {} });
    expect(p.has("tab")).toBe(false);
    expect(p.has("schema")).toBe(false);
    expect(p.get("db")).toBe("x");
  });

  it("buildUrlParams encodes is_null filter without value segment", async () => {
    const p = buildUrlParams({ db: null, tab: "data", schema: null, table: null, offset: 0, search: "", sort: null,
      filters: { col: { op: "is_null", value: "" } } });
    expect(p.get("f")).toBe("col:is_null");
  });
});

// ── api module unit tests ─────────────────────────────────────────────────────

describe("api helpers", () => {
  it("dbUrl appends ?db= to paths without query params", async () => {
    expect(dbUrl("/api/tables", "pg1")).toBe("/api/tables?db=pg1");
  });

  it("dbUrl appends &db= to paths that already have query params", async () => {
    expect(dbUrl("/api/tables?limit=100", "pg1")).toBe("/api/tables?limit=100&db=pg1");
  });

  it("dbUrl returns path unchanged when dbId is falsy", async () => {
    expect(dbUrl("/api/tables", null)).toBe("/api/tables");
    expect(dbUrl("/api/tables", "")).toBe("/api/tables");
  });
});

// ── url-state edge cases (branch coverage) ────────────────────────────────────

describe("url-state edge cases", () => {
  it("readUrlState skips malformed filter entries that lack a colon", async () => {
    // Covers url-state.js: 'if (first < 0) continue' branch.
    window.history.replaceState({}, "", "/?f=nocoion&f=col:eq:5");
    const s = readUrlState();
    expect(s.filters["col"]).toEqual({ op: "eq", value: "5" });
    expect(Object.keys(s.filters)).not.toContain("nocoion");
  });

  it("readUrlState defaults sort direction to 'asc' when dir param is absent", async () => {
    // Covers url-state.js: p.get('dir') || 'asc' false branch.
    window.history.replaceState({}, "", "/?sort=id");
    const s = readUrlState();
    expect(s.sort).toEqual({ col: "id", dir: "asc" });
  });

  it("buildUrlParams skips filter entries with no op (null or falsy)", async () => {
    // Covers url-state.js: '!f || !f.op' continue branch.
    const p = buildUrlParams({
      db: null, tab: "data", schema: null, table: null,
      offset: 0, search: "", sort: null,
      filters: { nullcol: null, emptyop: { op: "", value: "x" } },
    });
    expect(p.has("f")).toBe(false);
  });
});
