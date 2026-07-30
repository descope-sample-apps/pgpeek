// @vitest-environment jsdom
import { describe, it, expect, afterEach, vi } from "vitest";
import { readUrlState, buildUrlParams, dbUrl } from "./test-helpers.js";
import { appendDataParams, getJSON } from "./api.js";
import { compressToEncodedURIComponent } from "./vendor/lz-string.js";

function oversizedSQL() {
  let seed = 1;
  let sql = "SELECT '";
  for (let i = 0; i < 20_000; i += 1) {
    seed = (seed * 48_271) % 2_147_483_647;
    sql += String.fromCharCode(32 + (seed % 95));
  }
  return sql + "'";
}

afterEach(() => {
  vi.restoreAllMocks();
  window.history.replaceState({}, "", "/");
});

describe("url-state helpers", () => {
  it("readUrlState parses all supported params", async () => {
    window.history.replaceState({}, "", "/?db=x&tab=sql&schema=s&table=t&offset=50&search=foo&sort=id&dir=desc&f=id:eq:5&f=name:is_null&sql=SELECT+*+FROM+users");
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
    expect(s.sql).toBe("SELECT * FROM users");
  });

  it("readUrlState falls back to 'data' for an invalid tab value", async () => {
    window.history.replaceState({}, "", "/?tab=invalid");
    expect(readUrlState().tab).toBe("data");
  });

  it("buildUrlParams omits tab when data, omits falsy fields", async () => {
    const p = buildUrlParams({ db: "x", tab: "data", schema: null, table: null, offset: 0, search: "", sort: null, filters: [] });
    window.history.replaceState({}, "", "/?" + p.toString());
    const state = readUrlState();
    expect([...p.keys()]).toEqual(["s"]);
    expect(state.db).toBe("x");
    expect(state.tab).toBe("data");
    expect(state.schema).toBeNull();
  });

  it("buildUrlParams accepts states without filters", async () => {
    const p = buildUrlParams({ db: "x", tab: "sql", schema: null, table: null, offset: 0, search: "", sort: null, sql: "SELECT now();" });
    expect(p.has("s")).toBe(true);
    expect([...p.keys()]).toEqual(["s"]);
    expect(p.has("sql")).toBe(false);
    window.history.replaceState({}, "", "/?" + p.toString());
    expect(readUrlState().tab).toBe("sql");
    expect(readUrlState().sql).toBe("SELECT now();");
    expect(readUrlState().filters).toEqual([]);
  });

  it("buildUrlParams encodes is_null filter without value segment", async () => {
    const p = buildUrlParams({ db: null, tab: "data", schema: null, table: null, offset: 0, search: "", sort: null,
      filters: [{ column: "col", op: "is_null", value: "" }] });
    window.history.replaceState({}, "", "/?" + p.toString());
    expect(readUrlState().filters).toContainEqual({ column: "col", op: "is_null", value: "" });
  });

  it("round-trips Unicode SQL through the compressed URL payload", async () => {
    const sql = "SELECT '東京', '🙂' FROM users WHERE note = 'a & b';";
    const p = buildUrlParams({ tab: "sql", sql });
    window.history.replaceState({}, "", "/?" + p.toString());
    expect(readUrlState().sql).toBe(sql);
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
  it("falls back to legacy params when compressed state is invalid", async () => {
    for (const packed of ["y", "D9"]) {
      window.history.replaceState({}, "", "/?s=" + packed + "&db=legacy");
      expect(readUrlState().db).toBe("legacy");
    }
  });

  it("prefers a valid compressed empty state over legacy params", async () => {
    const p = buildUrlParams({});
    window.history.replaceState({}, "", "/?" + p.toString() + "&db=legacy");
    expect(readUrlState().db).toBeNull();
  });

  it("rejects an oversized valid packed state before restoring it", async () => {
    const packed = compressToEncodedURIComponent(new URLSearchParams({ sql: oversizedSQL() }).toString());
    expect(packed.length).toBeGreaterThan(8_192);
    window.history.replaceState({}, "", "/?" + new URLSearchParams({ s: packed, db: "legacy" }).toString());
    const state = readUrlState();
    expect(state.db).toBe("legacy");
    expect(state.sql).toBeNull();
  });

  it("omits SQL when generated packed state exceeds the sharing limit", async () => {
    const p = buildUrlParams({ db: "x", tab: "sql", sql: oversizedSQL() });
    expect(p.get("s").length).toBeLessThanOrEqual(8_192);
    window.history.replaceState({}, "", "/?" + p.toString());
    const state = readUrlState();
    expect(state.db).toBe("x");
    expect(state.sql).toBeNull();
  });

  it("falls back to empty state when non-SQL state exceeds the sharing limit", async () => {
    const p = buildUrlParams({ db: "x", search: oversizedSQL() });
    expect(p.get("s").length).toBeLessThanOrEqual(8_192);
    window.history.replaceState({}, "", "/?" + p.toString());
    const state = readUrlState();
    expect(state.db).toBeNull();
    expect(state.search).toBe("");
  });

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
