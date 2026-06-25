// @vitest-environment jsdom
// Tests: database selector rendering, API db params (GET and POST),
// SQL null/object cell rendering, and selector error-path coverage.
import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import {
  flush, makeResp, TWO_DBS, ONE_DB, NO_DBS, SAMPLE_TABLES,
  makeInstallFetch, $, click, changeSelect, loadApp, callsTo, urlOf,
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

// ── database selector ────────────────────────────────────────────────────────

describe("database selector", () => {
  it("renders selector with friendly names when multiple databases exist", async () => {
    await loadApp();
    const sel = $("database-select");
    expect(sel).toBeTruthy();
    const opts = [...sel.options].map((o) => ({ value: o.value, text: o.textContent }));
    expect(opts).toContainEqual({ value: "pg1", text: "Cluster A" });
    expect(opts).toContainEqual({ value: "pg2", text: "Cluster B" });
    expect(sel.value).toBe("pg1");
  });

  it("does not render selector when only one database", async () => {
    setRoute("GET /api/databases", makeResp({ json: ONE_DB }));
    await loadApp();
    expect($("database-select")).toBeNull();
  });

  it("does not render selector when databases list is empty", async () => {
    setRoute("GET /api/databases", makeResp({ json: NO_DBS }));
    await loadApp();
    expect($("database-select")).toBeNull();
  });

  it("shows error status on /api/databases failure but app remains functional", async () => {
    setRoute("GET /api/databases", new Error("db list down"));
    await loadApp();
    expect($("status").textContent).toContain("failed to load databases");
    expect($("status").className).toContain("error");
    expect($("database-select")).toBeNull();
    expect(callsTo("/api/tables").length).toBeGreaterThan(0);
  });

  it("shows error for non-ok /api/databases response but remains functional", async () => {
    setRoute("GET /api/databases", makeResp({ ok: false, status: 500, json: { error: "forbidden" } }));
    await loadApp();
    expect($("status").textContent).toContain("failed to load databases");
  });

  it("switches to the selected database on change", async () => {
    setRoute("GET /api/tables", makeResp({ json: SAMPLE_TABLES }));
    await loadApp();
    fetch.mockClear();
    await changeSelect($("database-select"), "pg2");
    const tablesCall = callsTo("/api/tables").find(([u]) =>
      !String(u).includes("/data") && !String(u).includes("/columns") && !String(u).includes("/fks"));
    expect(tablesCall).toBeTruthy();
    expect(urlOf(tablesCall[0]).searchParams.get("db")).toBe("pg2");
    expect($("database-select").value).toBe("pg2");
  });

  it("clears selected table when switching database", async () => {
    setRoute("GET /api/tables", makeResp({ json: SAMPLE_TABLES }));
    setRoute("GET /api/tables/*/data", makeResp({ json: { columns: ["id"], rows: [[1]], rowCount: 1, elapsedMs: 1 } }));
    await loadApp();
    await click($("tables").querySelectorAll(".tbl")[0]);
    expect($("tab-title").textContent).toBe("public.users");
    await changeSelect($("database-select"), "pg2");
    expect($("tab-title").textContent).toBe("Pick a table");
  });
});

// ── API db params — GET requests ──────────────────────────────────────────────

describe("API db params — GET requests", () => {
  it("sends ?db on GET /api/meta", async () => {
    await loadApp();
    const call = callsTo("/api/meta")[0];
    expect(urlOf(call[0]).searchParams.get("db")).toBe("pg1");
  });

  it("sends ?db on GET /api/tables", async () => {
    await loadApp();
    const call = callsTo("/api/tables").find(([u]) =>
      !String(u).includes("/data") && !String(u).includes("/columns") && !String(u).includes("/fks"));
    expect(urlOf(call[0]).searchParams.get("db")).toBe("pg1");
  });

  it("sends ?db on table data fetch", async () => {
    setRoute("GET /api/tables", makeResp({ json: SAMPLE_TABLES }));
    setRoute("GET /api/tables/*/data", makeResp({ json: { columns: ["id"], rows: [[1]], rowCount: 1, elapsedMs: 1 } }));
    await loadApp();
    await click($("tables").querySelectorAll(".tbl")[0]);
    const dataCall = fetch.mock.calls.find(([u]) => /\/tables\/.*\/data/.test(String(u)));
    expect(urlOf(dataCall[0]).searchParams.get("db")).toBe("pg1");
  });

  it("sends ?db on table columns fetch", async () => {
    setRoute("GET /api/tables", makeResp({ json: SAMPLE_TABLES }));
    setRoute("GET /api/tables/*/data", makeResp({ json: { columns: ["id"], rows: [[1]], rowCount: 1, elapsedMs: 1 } }));
    setRoute("GET /api/tables/*/columns", makeResp({ json: [{ name: "id", type: "int", nullable: false, default: null }] }));
    await loadApp();
    await click($("tables").querySelectorAll(".tbl")[0]);
    await click("tab-structure");
    expect(urlOf(callsTo("/columns")[0][0]).searchParams.get("db")).toBe("pg1");
  });

  it("sends ?db on table fks fetch", async () => {
    setRoute("GET /api/tables", makeResp({ json: SAMPLE_TABLES }));
    setRoute("GET /api/tables/*/data", makeResp({ json: { columns: ["id"], rows: [[1]], rowCount: 1, elapsedMs: 1 } }));
    await loadApp();
    await click($("tables").querySelectorAll(".tbl")[0]);
    expect(urlOf(callsTo("/fks")[0][0]).searchParams.get("db")).toBe("pg1");
  });

  it("includes ?db in data export CSV link href", async () => {
    setRoute("GET /api/tables", makeResp({ json: SAMPLE_TABLES }));
    setRoute("GET /api/tables/*/data", makeResp({ json: { columns: ["id"], rows: [[1]], rowCount: 1, elapsedMs: 1 } }));
    await loadApp();
    await click($("tables").querySelectorAll(".tbl")[0]);
    const href = new URL($("data-export-btn").href);
    expect(href.searchParams.get("db")).toBe("pg1");
    expect(href.searchParams.get("format")).toBe("csv");
  });

  it("does not send ?db on /api/queries (saved query CRUD is DB-independent)", async () => {
    await loadApp();
    const call = callsTo("/api/queries")[0];
    expect(urlOf(call[0]).searchParams.has("db")).toBe(false);
  });

  it("omits ?db on all requests when no databases configured", async () => {
    setRoute("GET /api/databases", makeResp({ json: NO_DBS }));
    setRoute("GET /api/tables", makeResp({ json: SAMPLE_TABLES }));
    setRoute("GET /api/tables/*/data", makeResp({ json: { columns: ["id"], rows: [[1]], rowCount: 1, elapsedMs: 1 } }));
    await loadApp();
    expect(urlOf(callsTo("/api/meta")[0][0]).searchParams.has("db")).toBe(false);
    const tc = callsTo("/api/tables").find(([u]) =>
      !String(u).includes("/data") && !String(u).includes("/columns") && !String(u).includes("/fks"));
    expect(urlOf(tc[0]).searchParams.has("db")).toBe(false);
    await click($("tables").querySelectorAll(".tbl")[0]);
    expect(urlOf(callsTo("/data")[0][0]).searchParams.has("db")).toBe(false);
  });
});

// ── API db params — POST requests ─────────────────────────────────────────────

describe("API db params — POST requests", () => {
  function postCall(path) {
    return [...fetch.mock.calls].reverse()
      .find(([u, opts]) => String(u).split("?")[0].includes(path) && opts?.body);
  }

  it("sends db as URL param on POST /api/query and keeps body {sql} only", async () => {
    setRoute("POST /api/query", makeResp({ json: { columns: ["n"], rows: [[1]], rowCount: 1, elapsedMs: 1 } }));
    await loadApp();
    await click("tab-sql");
    $("sql").value = "select 1";
    await click("run-btn");
    const call = postCall("/api/query");
    expect(urlOf(call[0]).searchParams.get("db")).toBe("pg1");
    expect(JSON.parse(call[1].body)).toEqual({ sql: "select 1" });
  });

  it("sends db as URL param on POST /api/export and keeps body {sql} only", async () => {
    setRoute("POST /api/query", makeResp({ json: { columns: ["n"], rows: [[1]], rowCount: 1, elapsedMs: 1 } }));
    setRoute("POST /api/export", makeResp({ blob: new Blob(["n\n1"]) }));
    await loadApp();
    await click("tab-sql");
    $("sql").value = "select 1";
    await click("run-btn");
    await click("sql-export-btn");
    const call = postCall("/api/export");
    expect(urlOf(call[0]).searchParams.get("db")).toBe("pg1");
    expect(JSON.parse(call[1].body)).toEqual({ sql: "select 1" });
  });

  it("omits db URL param on POST requests when no databases configured", async () => {
    setRoute("GET /api/databases", makeResp({ json: NO_DBS }));
    setRoute("POST /api/query", makeResp({ json: { columns: ["n"], rows: [[1]], rowCount: 1, elapsedMs: 1 } }));
    await loadApp();
    await click("tab-sql");
    $("sql").value = "select 1";
    await click("run-btn");
    const call = postCall("/api/query");
    expect(urlOf(call[0]).searchParams.has("db")).toBe(false);
    expect(JSON.parse(call[1].body)).toEqual({ sql: "select 1" });
  });
});

// ── SQL rendering coverage ────────────────────────────────────────────────────

describe("SQL tab null and object cell rendering", () => {
  it("renders NULL span and JSON-serialises objects in SQL results", async () => {
    // Covers sql-tab.js: v === null AND typeof v === 'object' branches.
    setRoute("POST /api/query", makeResp({
      json: { columns: ["a", "b", "c"], rows: [[null, { x: 1 }, "str"]], rowCount: 1, elapsedMs: 1 },
    }));
    await loadApp();
    await click("tab-sql");
    $("sql").value = "select null, jsonb, text";
    await click("run-btn");
    expect($("sql-results").querySelector(".null")).toBeTruthy();
    expect($("sql-results").textContent).toContain("NULL");
    expect($("sql-results").textContent).toContain('{"x":1}');
  });
});

// ── selector error-path and branch coverage ───────────────────────────────────

describe("database selector — statusText error path", () => {
  it("uses statusText in error when response is non-ok and statusText is set", async () => {
    // Covers app.js: r.statusText || 'failed' — truthy statusText path.
    setRoute("GET /api/databases", makeResp({ ok: false, status: 502, statusText: "Bad Gateway", json: {} }));
    await loadApp();
    expect($("status").textContent).toContain("Bad Gateway");
  });

  it("no-op when switchDb is called with the already-active database", async () => {
    // Covers app.js: if (newDb === currentDb) return.
    setRoute("GET /api/tables", makeResp({ json: SAMPLE_TABLES }));
    await loadApp();
    fetch.mockClear();
    await changeSelect($("database-select"), "pg1");
    const extraTables = fetch.mock.calls.filter(([u]) =>
      !String(u).includes("/data") && !String(u).includes("/columns") &&
      !String(u).includes("/fks") && String(u).includes("/api/tables"));
    expect(extraTables).toHaveLength(0);
  });
});

describe("database selector — malformed response", () => {
  it("treats missing databases key as empty list (Array.isArray false branch)", async () => {
    // Covers app.js: Array.isArray(result.databases) ? ... : [] false path.
    setRoute("GET /api/databases", makeResp({ json: { defaultId: null } }));
    await loadApp();
    expect($("database-select")).toBeNull();
    const metaCall = fetch.mock.calls.find(([u]) => String(u).includes("/api/meta"));
    expect(metaCall).toBeTruthy();
    expect(new URL("http://x" + metaCall[0]).searchParams.has("db")).toBe(false);
  });
});
