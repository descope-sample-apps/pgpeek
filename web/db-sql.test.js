// @vitest-environment jsdom
import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import {
  flush, makeResp, TWO_DBS, NO_DBS,
  makeInstallFetch, $, click, changeSelect, loadApp, urlOf,
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
  delete window.cm6;
  delete globalThis.cm6;
});

afterEach(() => {
  vi.restoreAllMocks();
  window.history.replaceState({}, "", "/");
  delete window.cm6;
  delete globalThis.cm6;
});

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

  it("runs CodeMirror shortcut against the latest selected database", async () => {
    let value = "select cm";
    const editor = { getValue: vi.fn(() => value), setValue: vi.fn((v) => { value = v; }), refresh: vi.fn() };
    const mount = vi.fn(() => editor);
    window.cm6 = { mount };
    globalThis.cm6 = window.cm6;
    setRoute("POST /api/query", makeResp({ json: { columns: ["n"], rows: [[1]], rowCount: 1, elapsedMs: 1 } }));
    await loadApp();
    await click("tab-sql");
    await changeSelect($("database-select"), "pg2");

    mount.mock.calls[0][2]();
    await flush();

    const call = postCall("/api/query");
    expect(urlOf(call[0]).searchParams.get("db")).toBe("pg2");
    expect(JSON.parse(call[1].body)).toEqual({ sql: "select cm" });
  });

  it("shows query errors beside SQL results", async () => {
    setRoute("POST /api/query", makeResp({
      ok: false,
      status: 400,
      json: { error: `syntax error at or near "IS" (SQLSTATE 42601)` },
    }));
    await loadApp();
    await click("tab-sql");
    $("sql").value = "SELECT * FROM access_keys tenants IS NULL";
    await click("run-btn");

    const alert = document.querySelector(".query-error");
    expect(alert.textContent).toContain(`syntax error at or near "IS"`);
    expect($("status").textContent).toContain("SQLSTATE 42601");
    expect($("sql-results").textContent).toContain("Run a query to see results.");
  });

  it("falls back to response status text when query error body is not JSON", async () => {
    setRoute("POST /api/query", makeResp({
      ok: false,
      status: 502,
      statusText: "Bad Gateway",
      json: async () => { throw new Error("html"); },
    }));
    await loadApp();
    await click("tab-sql");
    $("sql").value = "select 1";
    await click("run-btn");

    expect(document.querySelector(".query-error").textContent).toBe("Bad Gateway");
  });

  it("shows query network errors beside SQL results", async () => {
    setRoute("POST /api/query", new Error("offline"));
    await loadApp();
    await click("tab-sql");
    $("sql").value = "select 1";
    await click("run-btn");

    expect(document.querySelector(".query-error").textContent).toBe("offline");
  });

  it("clears query error after a successful run", async () => {
    setRoute("POST /api/query", makeResp({ ok: false, status: 400, json: { error: "query failed" } }));
    await loadApp();
    await click("tab-sql");
    $("sql").value = "select 1";
    await click("run-btn");
    expect(document.querySelector(".query-error")).toBeTruthy();

    setRoute("POST /api/query", makeResp({ json: { columns: ["n"], rows: [[1]], rowCount: 1, elapsedMs: 1 } }));
    await click("run-btn");

    expect(document.querySelector(".query-error")).toBeFalsy();
    expect($("sql-results").textContent).toContain("1");
  });

  it("clears query error after picking a saved query", async () => {
    setRoute("GET /api/queries", makeResp({ json: [{ id: 7, name: "Recent keys", sql: "select 1", isPreset: false }] }));
    setRoute("POST /api/query", makeResp({ ok: false, status: 400, json: { error: "query failed" } }));
    await loadApp();
    await click("tab-sql");
    $("sql").value = "bad sql";
    await click("run-btn");
    expect(document.querySelector(".query-error")).toBeTruthy();

    await changeSelect($("presets"), "7");

    expect(document.querySelector(".query-error")).toBeFalsy();
    expect($("status").textContent).toContain("Loaded “Recent keys”. Press Run.");
  });

  it("shows export network errors beside SQL results", async () => {
    setRoute("POST /api/query", makeResp({ json: { columns: ["n"], rows: [[1]], rowCount: 1, elapsedMs: 1 } }));
    setRoute("POST /api/export", new Error("network down"));
    await loadApp();
    await click("tab-sql");
    $("sql").value = "select 1";
    await click("run-btn");
    await click("sql-export-btn");

    expect(document.querySelector(".query-error").textContent).toBe("network down");
  });

  it("shows export fallback errors beside SQL results", async () => {
    setRoute("POST /api/query", makeResp({ json: { columns: ["n"], rows: [[1]], rowCount: 1, elapsedMs: 1 } }));
    setRoute("POST /api/export", makeResp({ ok: false, status: 400, json: {} }));
    await loadApp();
    await click("tab-sql");
    $("sql").value = "select 1";
    await click("run-btn");
    await click("sql-export-btn");

    expect(document.querySelector(".query-error").textContent).toBe("export failed");
  });

  it("ignores stale export errors after a newer successful query", async () => {
    let rejectExport;
    setRoute("POST /api/query", makeResp({ json: { columns: ["n"], rows: [[1]], rowCount: 1, elapsedMs: 1 } }));
    setRoute("POST /api/export", () => new Promise((_, reject) => { rejectExport = reject; }));
    await loadApp();
    await click("tab-sql");
    $("sql").value = "select 1";
    await click("run-btn");
    await click("sql-export-btn");

    await click("run-btn");
    rejectExport(new Error("late export failure"));
    await flush();

    expect(document.querySelector(".query-error")).toBeFalsy();
    expect($("status").textContent).toContain("✓ 1 row in 1 ms");
  });

  it("ignores stale export response errors after a newer successful query", async () => {
    let resolveExport;
    setRoute("POST /api/query", makeResp({ json: { columns: ["n"], rows: [[1]], rowCount: 1, elapsedMs: 1 } }));
    setRoute("POST /api/export", () => new Promise((resolve) => { resolveExport = resolve; }));
    await loadApp();
    await click("tab-sql");
    $("sql").value = "select 1";
    await click("run-btn");
    await click("sql-export-btn");

    await click("run-btn");
    resolveExport(makeResp({ ok: false, status: 400, json: { error: "late export failed" } }));
    await flush();

    expect(document.querySelector(".query-error")).toBeFalsy();
    expect($("status").textContent).toContain("✓ 1 row in 1 ms");
  });

  it("ignores stale successful exports after a newer successful query", async () => {
    let resolveExport;
    setRoute("POST /api/query", makeResp({ json: { columns: ["n"], rows: [[1]], rowCount: 1, elapsedMs: 1 } }));
    setRoute("POST /api/export", () => new Promise((resolve) => { resolveExport = resolve; }));
    await loadApp();
    await click("tab-sql");
    $("sql").value = "select 1";
    await click("run-btn");
    await click("sql-export-btn");

    await click("run-btn");
    resolveExport(makeResp({ blob: new Blob(["n\n1"]) }));
    await flush();

    expect(HTMLAnchorElement.prototype.click).not.toHaveBeenCalled();
    expect($("status").textContent).toContain("✓ 1 row in 1 ms");
  });

  it("ignores stale query response errors after picking a saved query", async () => {
    let resolveQuery;
    setRoute("GET /api/queries", makeResp({ json: [{ id: 7, name: "Recent keys", sql: "select 1", isPreset: false }] }));
    setRoute("POST /api/query", () => new Promise((resolve) => { resolveQuery = resolve; }));
    await loadApp();
    await click("tab-sql");
    $("sql").value = "select pg_sleep(1)";
    await click("run-btn");

    await changeSelect($("presets"), "7");
    resolveQuery(makeResp({ ok: false, status: 400, json: { error: "late query failed" } }));
    await flush();

    expect(document.querySelector(".query-error")).toBeFalsy();
    expect($("status").textContent).toContain("Loaded “Recent keys”. Press Run.");
  });

  it("ignores stale query network errors after picking a saved query", async () => {
    let rejectQuery;
    setRoute("GET /api/queries", makeResp({ json: [{ id: 7, name: "Recent keys", sql: "select 1", isPreset: false }] }));
    setRoute("POST /api/query", () => new Promise((_, reject) => { rejectQuery = reject; }));
    await loadApp();
    await click("tab-sql");
    $("sql").value = "select pg_sleep(1)";
    await click("run-btn");

    await changeSelect($("presets"), "7");
    rejectQuery(new Error("late query network"));
    await flush();

    expect(document.querySelector(".query-error")).toBeFalsy();
    expect($("status").textContent).toContain("Loaded “Recent keys”. Press Run.");
  });

  it("ignores stale successful queries after picking a saved query", async () => {
    let resolveQuery;
    setRoute("GET /api/queries", makeResp({ json: [{ id: 7, name: "Recent keys", sql: "select 1", isPreset: false }] }));
    setRoute("POST /api/query", () => new Promise((resolve) => { resolveQuery = resolve; }));
    await loadApp();
    await click("tab-sql");
    $("sql").value = "select pg_sleep(1)";
    await click("run-btn");

    await changeSelect($("presets"), "7");
    resolveQuery(makeResp({ json: { columns: ["n"], rows: [[1]], rowCount: 1, elapsedMs: 1 } }));
    await flush();

    expect($("sql-results").textContent).toContain("Run a query to see results.");
    expect($("status").textContent).toContain("Loaded “Recent keys”. Press Run.");
  });

  it("ignores stale export errors after picking a saved query", async () => {
    let rejectExport;
    setRoute("GET /api/queries", makeResp({ json: [{ id: 7, name: "Recent keys", sql: "select 1", isPreset: false }] }));
    setRoute("POST /api/query", makeResp({ json: { columns: ["n"], rows: [[1]], rowCount: 1, elapsedMs: 1 } }));
    setRoute("POST /api/export", () => new Promise((_, reject) => { rejectExport = reject; }));
    await loadApp();
    await click("tab-sql");
    $("sql").value = "select 1";
    await click("run-btn");
    await click("sql-export-btn");

    await changeSelect($("presets"), "7");
    rejectExport(new Error("late export failure"));
    await flush();

    expect(document.querySelector(".query-error")).toBeFalsy();
    expect($("status").textContent).toContain("Loaded “Recent keys”. Press Run.");
  });
});

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
