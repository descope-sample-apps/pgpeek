// @vitest-environment jsdom
import { describe, it, expect, afterEach, vi } from "vitest";
import { readUrlState, buildUrlParams, dbUrl } from "./test-helpers.js";
import { appendDataParams, getJSON } from "./api.js";

afterEach(() => {
  vi.restoreAllMocks();
  window.history.replaceState({}, "", "/");
});

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
    expect(s.filters).toContainEqual({ column: "id", op: "eq", value: "5" });
    expect(s.filters).toContainEqual({ column: "name", op: "is_null", value: "" });
  });

  it("readUrlState falls back to 'data' for an invalid tab value", async () => {
    window.history.replaceState({}, "", "/?tab=invalid");
    expect(readUrlState().tab).toBe("data");
  });

  it("buildUrlParams omits tab when data, omits falsy fields", async () => {
    const p = buildUrlParams({ db: "x", tab: "data", schema: null, table: null, offset: 0, search: "", sort: null, filters: [] });
    expect(p.has("tab")).toBe(false);
    expect(p.has("schema")).toBe(false);
    expect(p.get("db")).toBe("x");
  });

  it("buildUrlParams accepts states without filters", async () => {
    const p = buildUrlParams({ db: "x", tab: "sql", schema: null, table: null, offset: 0, search: "", sort: null });
    expect(p.get("tab")).toBe("sql");
    expect(p.has("f")).toBe(false);
  });

  it("buildUrlParams encodes is_null filter without value segment", async () => {
    const p = buildUrlParams({ db: null, tab: "data", schema: null, table: null, offset: 0, search: "", sort: null,
      filters: [{ column: "col", op: "is_null", value: "" }] });
    expect(p.get("f")).toBe("col:is_null");
  });
});

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

  it("appendDataParams skips filter entries without a column", async () => {
    const params = new URLSearchParams();
    appendDataParams(params, "", null, [{ op: "eq", value: "x" }]);
    expect(params.has("f")).toBe(false);
  });

  it("getJSON reports status text when error response is not JSON", async () => {
    globalThis.fetch = vi.fn(() => Promise.resolve({ ok: false, statusText: "Bad Gateway", json: async () => { throw new Error("html"); } }));
    await expect(getJSON("/api/tables", "pg1")).rejects.toThrow("Bad Gateway");
  });

  it("getJSON surfaces parse errors on successful non-JSON responses", async () => {
    globalThis.fetch = vi.fn(() => Promise.resolve({ ok: true, statusText: "OK", json: async () => { throw new Error("bad json"); } }));
    await expect(getJSON("/api/tables", "pg1")).rejects.toThrow("bad json");
  });
});

describe("url-state edge cases", () => {
  it("readUrlState skips malformed filter entries that lack a colon", async () => {
    // Covers url-state.js: 'if (first < 0) continue' branch.
    window.history.replaceState({}, "", "/?f=nocoion&f=col:eq:5");
    const s = readUrlState();
    expect(s.filters).toContainEqual({ column: "col", op: "eq", value: "5" });
    expect(s.filters.map((f) => f.column)).not.toContain("nocoion");
  });

  it("readUrlState keeps user filter columns out of object keys", async () => {
    window.history.replaceState({}, "", "/?f=__proto__:eq:polluted&f=constructor:eq:polluted");
    const s = readUrlState();
    expect(s.filters).toContainEqual({ column: "__proto__", op: "eq", value: "polluted" });
    expect(s.filters).toContainEqual({ column: "constructor", op: "eq", value: "polluted" });
    expect({}.polluted).toBeUndefined();
  });

  it("readUrlState defaults sort direction to 'asc' when dir param is absent", async () => {
    // Covers url-state.js: p.get('dir') || 'asc' false branch.
    window.history.replaceState({}, "", "/?sort=id");
    const s = readUrlState();
    expect(s.sort).toEqual({ col: "id", dir: "asc" });
  });

  it("readUrlState defaults sort direction to 'asc' when dir param is invalid", async () => {
    window.history.replaceState({}, "", "/?sort=id&dir=sideways");
    expect(readUrlState().sort).toEqual({ col: "id", dir: "asc" });
  });

  it("buildUrlParams skips filter entries with no op (null or falsy)", async () => {
    // Covers url-state.js: '!f || !f.op' continue branch.
    const p = buildUrlParams({
      db: null, tab: "data", schema: null, table: null,
      offset: 0, search: "", sort: null,
      filters: [null, { column: "emptyop", op: "", value: "x" }],
    });
    expect(p.has("f")).toBe(false);
  });
});
