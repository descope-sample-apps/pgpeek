// @vitest-environment jsdom
import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";

// allow: SIZE_OK — characterization suite pins one frozen component tree end-to-end.

async function flush() {
  for (let i = 0; i < 10; i += 1) {
    await Promise.resolve();
    await new Promise((r) => setTimeout(r, 0));
  }
}

function makeResp({ ok = true, status = 200, json, blob, statusText = "" } = {}) {
  return {
    ok,
    status,
    statusText,
    json: async () => (typeof json === "function" ? json() : json),
    blob: async () => blob ?? new Blob(["data"]),
  };
}

let routes;
function setRoute(key, resp) { routes[key] = resp; }

function routeKey(method, path) {
  if (path.endsWith("/data")) return `${method} /api/tables/*/data`;
  if (path.endsWith("/columns")) return `${method} /api/tables/*/columns`;
  if (path.endsWith("/fks")) return `${method} /api/tables/*/fks`;
  if (path.startsWith("/api/queries/")) return `${method} /api/queries/:id`;
  return `${method} ${path}`;
}

function installFetch() {
  globalThis.fetch = vi.fn((url, opts) => {
    const method = (opts && opts.method) || "GET";
    const path = String(url).split("?")[0];
    const r = routes[routeKey(method, path)];
    if (typeof r === "function") return r(url, opts);
    if (r === undefined) return Promise.reject(new Error("no route for " + method + " " + path));
    if (r instanceof Error) return Promise.reject(r);
    return Promise.resolve(r);
  });
  window.fetch = globalThis.fetch;
}

async function loadApp() {
  vi.resetModules();
  await import("./app.js");
  await flush();
}

function dataResp({ columns = ["n"], rows, rowCount, truncated = false, elapsedMs = 1 } = {}) {
  const finalRows = rows ?? Array.from({ length: rowCount ?? 0 }, (_, i) => columns.map((_, j) => (j === 0 ? i + 1 : `${columns[j]}-${i + 1}`)));
  return makeResp({ json: { columns, rows: finalRows, rowCount: rowCount ?? finalRows.length, truncated, elapsedMs } });
}

function rowsResp(n, cols = ["n"]) {
  return dataResp({ columns: cols, rowCount: n });
}

const SAMPLE_TABLES = [
  { schema: "public", name: "users", type: "table", estRows: 5 },
  { schema: "public", name: "v_active", type: "view", estRows: -1 },
  { schema: "auth", name: "sessions", type: "table", estRows: 12 },
  { schema: "public", name: "companies", type: "table", estRows: 3 },
];

beforeEach(() => {
  document.body.innerHTML = '<div id="app"></div>';
  routes = {
    "GET /api/meta": makeResp({ json: { rowCap: 1000 } }),
    "GET /api/tables": makeResp({ json: [] }),
    "GET /api/tables/*/columns": makeResp({ json: [] }),
    "GET /api/tables/*/fks": makeResp({ json: [] }),
    "GET /api/queries": makeResp({ json: [] }),
  };
  installFetch();
  globalThis.prompt = vi.fn();
  globalThis.confirm = vi.fn();
  globalThis.URL.createObjectURL = vi.fn(() => "blob:fake");
  globalThis.URL.revokeObjectURL = vi.fn();
  HTMLAnchorElement.prototype.click = vi.fn();
  globalThis.requestAnimationFrame = (cb) => setTimeout(cb, 0);
  globalThis.cancelAnimationFrame = (id) => clearTimeout(id);
  window.requestAnimationFrame = globalThis.requestAnimationFrame;
  window.cancelAnimationFrame = globalThis.cancelAnimationFrame;
  delete window.cm6;
  delete globalThis.cm6;
});

afterEach(() => {
  vi.restoreAllMocks();
  delete window.cm6;
  delete globalThis.cm6;
});

const $ = (id) => document.getElementById(id);

async function click(target) {
  const el = typeof target === "string" ? $(target) : target;
  el.click();
  await flush();
}

async function dispatchClick(target) {
  const el = typeof target === "string" ? $(target) : target;
  el.dispatchEvent(new MouseEvent("click", { bubbles: true }));
  await flush();
}

async function input(id, value) {
  const el = $(id);
  el.value = value;
  el.dispatchEvent(new Event("input", { bubbles: true }));
  await flush();
}

async function keydown(target, key, init = {}) {
  const el = typeof target === "string" ? $(target) : target;
  el.dispatchEvent(new KeyboardEvent("keydown", { key, bubbles: true, ...init }));
  await flush();
}

async function changeSelect(el, value) {
  el.value = value;
  el.dispatchEvent(new Event("change", { bubbles: true }));
  await flush();
}

function lastDataParams() {
  const call = [...fetch.mock.calls].reverse().find(([u]) => String(u).includes("/data"));
  return new URL("http://x" + call[0]).searchParams;
}

function postBody(path) {
  const call = [...fetch.mock.calls].reverse().find(([u, opts]) => String(u).includes(path) && opts?.body);
  return JSON.parse(call[1].body);
}

function callsTo(path) {
  return fetch.mock.calls.filter(([u]) => String(u).includes(path));
}

async function selectTable(index = 0, data = rowsResp(2)) {
  setRoute("GET /api/tables", makeResp({ json: SAMPLE_TABLES }));
  setRoute("GET /api/tables/*/data", data);
  await loadApp();
  await click($("tables").querySelectorAll(".tbl")[index]);
}

function deferred() {
  let resolve;
  const promise = new Promise((r) => { resolve = r; });
  return { promise, resolve };
}

describe("sidebar and tabs", () => {
  it("renders initial shell, empty-table copy, and no-table panel hints", async () => {
    await loadApp();

    expect($("tab-title").textContent).toBe("Pick a table");
    expect($("tables").textContent).toContain("No tables.");
    expect($("panel-data").textContent).toContain("Select a table to browse its rows.");
    await click("tab-structure");
    expect($("panel-structure").hidden).toBe(false);
    expect($("panel-structure").textContent).toContain("Select a table to see its structure.");
    await click("tab-sql");
    expect($("panel-sql").hidden).toBe(false);
    expect($("panel-data").hidden).toBe(true);
    expect($("tab-sql").classList.contains("active")).toBe(true);
  });

  it("shows loading copy while tables have not resolved", async () => {
    setRoute("GET /api/tables", () => new Promise(() => {}));
    await loadApp();

    expect($("tables").textContent).toContain("Loading tables…");
  });

  it("groups schemas, styles views, filters matches and no-matches", async () => {
    setRoute("GET /api/tables", makeResp({ json: SAMPLE_TABLES }));
    await loadApp();

    expect([...$("tables").querySelectorAll(".schema")].map((s) => s.textContent)).toEqual(["public", "auth", "public"]);
    const buttons = $("tables").querySelectorAll(".tbl");
    expect(buttons).toHaveLength(4);
    expect(buttons[1].classList.contains("view")).toBe(true);
    expect(buttons[0].title).toContain("~5 rows");
    expect(buttons[1].title).toBe("public.v_active");

    await input("tbl-filter", "session");
    expect([...$("tables").querySelectorAll(".tbl")].map((b) => b.textContent)).toEqual(["sessions"]);
    await input("tbl-filter", "zzz");
    expect($("tables").textContent).toContain("No tables match.");
  });

  it("marks one active table and clears it when another table opens", async () => {
    setRoute("GET /api/tables", makeResp({ json: SAMPLE_TABLES }));
    setRoute("GET /api/tables/*/data", rowsResp(1));
    await loadApp();

    const buttons = $("tables").querySelectorAll(".tbl");
    await click(buttons[0]);
    expect($("tab-title").textContent).toBe("public.users");
    expect($("table-context").textContent).toContain("Current table");
    expect($("table-context").textContent).toContain("public.users");
    expect($("tables").querySelector(".tbl.active").textContent).toBe("users");
    expect($("tables").querySelector(".tbl.active").getAttribute("aria-current")).toBe("true");
    await click(buttons[2]);
    const active = $("tables").querySelectorAll(".tbl.active");
    expect(active).toHaveLength(1);
    expect(active[0].textContent).toBe("sessions");
    expect($("table-context").textContent).toContain("auth.sessions");
  });

  it("reports table load errors", async () => {
    setRoute("GET /api/tables", makeResp({ ok: false, status: 500, json: { error: "catalog down" } }));
    await loadApp();
    expect($("status").classList.contains("error")).toBe(true);
    expect($("status").textContent).toContain("failed to load tables: catalog down");
    expect($("tables").textContent).toContain("No tables.");
  });
});

describe("data tab", () => {
  it("loads rows, renders values, and updates status/pager/export", async () => {
    await selectTable(0, dataResp({ columns: ["id", "meta", "none"], rows: [[1, { a: 1 }, null]], rowCount: 1, elapsedMs: 7 }));

    expect($("status").className).toContain("ok");
    expect($("status").textContent).toContain("✓ 1 row in 7 ms");
    expect($("prev-btn").disabled).toBe(true);
    expect($("next-btn").disabled).toBe(true);
    expect($("page-info").textContent).toBe("1–1");
    expect($("data-results").textContent).toContain('{"a":1}');
    expect($("data-results").querySelector("td.null").textContent).toBe("NULL");
    expect($("data-export-btn").tagName).toBe("A");
    expect($("data-export-btn").className).toContain("secondary");
    expect($("data-export-btn").getAttribute("role")).toBe("button");
    expect($("data-export-btn").getAttribute("download")).toBe("users.csv");
  });

  it("shows loading, no-column, and zero-row states", async () => {
    const pending = deferred();
    setRoute("GET /api/tables", makeResp({ json: SAMPLE_TABLES }));
    setRoute("GET /api/tables/*/data", () => pending.promise);
    await loadApp();
    await click($("tables").querySelector(".tbl"));
    expect($("data-results").textContent).toContain("Loading…");
    pending.resolve(makeResp({ json: { columns: [], rows: [], rowCount: 0, elapsedMs: 2 } }));
    await flush();
    expect($("data-results").textContent).toContain("No columns.");
    expect($("page-info").textContent).toBe("0–0");

    document.body.innerHTML = '<div id="app"></div>';
    await selectTable(0, dataResp({ columns: ["id"], rows: [], rowCount: 0 }));
    expect($("data-results").textContent).toContain("0 rows.");
    expect($("page-info").textContent).toBe("0–0");
  });

  it("paginates with default and small rowCap page sizes", async () => {
    await selectTable(0, rowsResp(100));
    expect(lastDataParams().get("limit")).toBe("100");
    expect($("next-btn").disabled).toBe(false);

    setRoute("GET /api/tables/*/data", rowsResp(3));
    await click("next-btn");
    expect(lastDataParams().get("offset")).toBe("100");
    expect($("page-info").textContent).toBe("101–103");
    expect($("prev-btn").disabled).toBe(false);

    setRoute("GET /api/tables/*/data", rowsResp(100));
    await click("prev-btn");
    expect(lastDataParams().get("offset")).toBe("0");

    document.body.innerHTML = '<div id="app"></div>';
    setRoute("GET /api/meta", makeResp({ json: { rowCap: 3 } }));
    setRoute("GET /api/tables/*/data", rowsResp(3));
    await selectTable(0, rowsResp(3));
    expect(lastDataParams().get("limit")).toBe("3");
  });

  it("refetches open table when delayed meta narrows page size", async () => {
    const meta = deferred();
    setRoute("GET /api/meta", () => meta.promise);
    setRoute("GET /api/tables", makeResp({ json: SAMPLE_TABLES }));
    setRoute("GET /api/tables/*/data", rowsResp(5));
    await loadApp();
    await click($("tables").querySelector(".tbl"));

    expect(callsTo("/data")).toHaveLength(1);
    expect(lastDataParams().get("limit")).toBe("100");
    meta.resolve(makeResp({ json: { rowCap: 5 } }));
    await flush();
    expect(callsTo("/data")).toHaveLength(2);
    expect(lastDataParams().get("limit")).toBe("5");
  });

  it("keeps default page size for large, missing, non-positive, or failed meta", async () => {
    for (const meta of [makeResp({ json: { rowCap: 1000 } }), makeResp({ json: {} }), makeResp({ json: { rowCap: 0 } }), new Error("meta down")]) {
      document.body.innerHTML = '<div id="app"></div>';
      setRoute("GET /api/meta", meta);
      setRoute("GET /api/tables", makeResp({ json: SAMPLE_TABLES }));
      setRoute("GET /api/tables/*/data", rowsResp(100));
      await loadApp();
      await click($("tables").querySelector(".tbl"));
      expect(lastDataParams().get("limit")).toBe("100");
    }
  });

  it("sorts, searches, clears, and builds export URLs from active params", async () => {
    await selectTable(0, dataResp({ columns: ["id", "email"], rows: [[1, "a@x"]], rowCount: 1 }));
    const headers = $("data-results").querySelectorAll("th.sortable");

    await click(headers[0]);
    expect(lastDataParams().get("sort")).toBe("id");
    expect(lastDataParams().get("dir")).toBe("asc");
    expect(headers[0].textContent).toContain("▲");
    await click($("data-results").querySelectorAll("th.sortable")[0]);
    expect(lastDataParams().get("dir")).toBe("desc");
    expect($("data-results").querySelectorAll("th.sortable")[0].textContent).toContain("▼");
    await click($("data-results").querySelectorAll("th.sortable")[0]);
    expect(lastDataParams().get("dir")).toBe("asc");
    await click($("data-results").querySelectorAll("th.sortable")[1]);
    expect(lastDataParams().get("sort")).toBe("email");
    expect(lastDataParams().get("dir")).toBe("asc");

    await input("data-search", " ignored ");
    await keydown("data-search", "Escape");
    expect(lastDataParams().has("search")).toBe(false);
    await keydown("data-search", "Enter");
    expect(lastDataParams().get("search")).toBe("ignored");

    const val = $("data-results").querySelector('input.f-val[data-col="email"]');
    val.value = "a@x";
    val.dispatchEvent(new Event("input", { bubbles: true }));
    await flush();
    await changeSelect($("data-results").querySelector('select.f-op[data-col="email"]'), "eq");

    const href = new URL($("data-export-btn").href);
    expect(href.pathname).toBe("/api/tables/public/users/data");
    expect(href.searchParams.get("format")).toBe("csv");
    expect(href.searchParams.get("search")).toBe("ignored");
    expect(href.searchParams.get("sort")).toBe("email");
    expect(href.searchParams.get("f")).toBe("email:eq:a@x");

    await click("data-clear");
    expect(lastDataParams().has("search")).toBe(false);
    expect(lastDataParams().has("sort")).toBe(false);
    expect(lastDataParams().has("f")).toBe(false);
  });

  it("emits every filter operator and preserves blank value operands", async () => {
    await selectTable(0, dataResp({ columns: ["id"], rows: [[1]], rowCount: 1 }));
    const op = () => $("data-results").querySelector('select.f-op[data-col="id"]');
    const val = () => $("data-results").querySelector('input.f-val[data-col="id"]');

    for (const name of ["eq", "ne", "lt", "lte", "gt", "gte", "ilike", "like"]) {
      val().value = name === "eq" ? "" : "7";
      val().dispatchEvent(new Event("input", { bubbles: true }));
      await flush();
      await changeSelect(op(), name);
      expect(lastDataParams().get("f")).toBe(`id:${name}:${name === "eq" ? "" : "7"}`);
    }
    await changeSelect(op(), "is_null");
    expect(lastDataParams().get("f")).toBe("id:is_null");
    await changeSelect(op(), "is_not_null");
    expect(lastDataParams().get("f")).toBe("id:is_not_null");
    await changeSelect(op(), "");
    expect(lastDataParams().has("f")).toBe(false);

    val().value = "11";
    val().dispatchEvent(new Event("input", { bubbles: true }));
    await flush();
    await keydown(val(), "Escape");
    expect(lastDataParams().has("f")).toBe(false);
    await keydown(val(), "Enter");
    expect(lastDataParams().has("f")).toBe(false);
  });

  it("skips no-op filter entries when data params are appended", async () => {
    await selectTable(0, dataResp({ columns: ["id"], rows: [[1]], rowCount: 1 }));
    const realKeys = Object.keys;
    Object.defineProperty(Object.prototype, "__blankFilter__", { value: { op: "" }, configurable: true });
    const keysSpy = vi.spyOn(Object, "keys").mockImplementation((obj) => {
      const keys = realKeys(obj);
      return (new Error().stack || "").includes("appendDataParams") ? [...keys, "__blankFilter__"] : keys;
    });
    try {
      await click($("data-results").querySelector("th.sortable"));
      expect(lastDataParams().getAll("f")).toEqual([]);
    } finally {
      keysSpy.mockRestore();
      delete Object.prototype.__blankFilter__;
    }
  });

  it("resets filters when switching tables and has no clear control before selection", async () => {
    setRoute("GET /api/tables", makeResp({ json: SAMPLE_TABLES }));
    setRoute("GET /api/tables/*/data", dataResp({ columns: ["id"], rows: [[1]], rowCount: 1 }));
    await loadApp();
    expect($("data-clear")).toBeNull();

    const buttons = $("tables").querySelectorAll(".tbl");
    await click(buttons[0]);
    await changeSelect($("data-results").querySelector('select.f-op[data-col="id"]'), "is_null");
    expect(lastDataParams().get("f")).toBe("id:is_null");
    await click(buttons[2]);
    expect($("tab-title").textContent).toBe("auth.sessions");
    expect(lastDataParams().has("f")).toBe(false);
  });

  it("reports body, statusText, and network data load errors", async () => {
    await selectTable(0, makeResp({ ok: false, status: 400, json: { error: "bad filter" } }));
    expect($("status").textContent).toContain("bad filter");

    document.body.innerHTML = '<div id="app"></div>';
    await selectTable(0, makeResp({ ok: false, status: 500, statusText: "Server Error", json: {} }));
    expect($("status").textContent).toContain("Server Error");

    document.body.innerHTML = '<div id="app"></div>';
    await selectTable(0, new Error("network gone"));
    expect($("status").textContent).toContain("network gone");
  });

  it("ignores stale data responses after a table switch", async () => {
    const first = deferred();
    setRoute("GET /api/tables", makeResp({ json: SAMPLE_TABLES }));
    setRoute("GET /api/tables/*/data", (url) => String(url).includes("/public/users/") ? first.promise : Promise.resolve(dataResp({ columns: ["sid"], rows: [[9]], rowCount: 1 })));
    await loadApp();

    const buttons = $("tables").querySelectorAll(".tbl");
    await click(buttons[0]);
    expect($("status").textContent).toContain("Loading public.users");
    await click(buttons[2]);
    expect($("tab-title").textContent).toBe("auth.sessions");
    expect($("data-results").textContent).toContain("sid");

    first.resolve(dataResp({ columns: ["stale"], rows: [[1]], rowCount: 1 }));
    await flush();
    expect($("tab-title").textContent).toBe("auth.sessions");
    expect($("data-results").textContent).not.toContain("stale");

    document.body.innerHTML = '<div id="app"></div>';
    const staleError = deferred();
    setRoute("GET /api/tables", makeResp({ json: SAMPLE_TABLES }));
    setRoute("GET /api/tables/*/data", (url) => String(url).includes("/public/users/") ? staleError.promise : Promise.resolve(dataResp({ columns: ["sid"], rows: [[9]], rowCount: 1 })));
    await loadApp();
    const nextButtons = $("tables").querySelectorAll(".tbl");
    await click(nextButtons[0]);
    await click(nextButtons[2]);
    staleError.resolve(Promise.reject(new Error("stale data failed")));
    await flush();
    expect($("tab-title").textContent).toBe("auth.sessions");
    expect($("status").textContent).not.toContain("stale data failed");
  });
});

describe("foreign keys", () => {
  it("renders FK buttons, navigates with an eq filter, and reports non-browsable refs", async () => {
    setRoute("GET /api/tables", makeResp({ json: SAMPLE_TABLES }));
    setRoute("GET /api/tables/*/fks", makeResp({ json: [{ column: "company_id", refSchema: "public", refTable: "companies", refColumn: "id" }] }));
    setRoute("GET /api/tables/*/data", dataResp({ columns: ["id", "company_id"], rows: [[1, 42]], rowCount: 1 }));
    await loadApp();
    await click($("tables").querySelector(".tbl"));

    const fk = $("data-results").querySelector("button.fk");
    expect(fk.title).toBe("→ public.companies.id");
    await click(fk);
    expect($("tab-title").textContent).toBe("public.companies");
    expect(lastDataParams().get("f")).toBe("id:eq:42");

    setRoute("GET /api/tables/*/fks", makeResp({ json: [{ column: "company_id", refSchema: "missing", refTable: "nope", refColumn: "id" }] }));
    await click($("tables").querySelectorAll(".tbl")[0]);
    await click($("data-results").querySelector("button.fk"));
    expect($("status").textContent).toContain("referenced table missing.nope is not browsable");
  });

  it("silently omits FK links when introspection fails or goes stale", async () => {
    const firstFk = deferred();
    setRoute("GET /api/tables", makeResp({ json: SAMPLE_TABLES }));
    setRoute("GET /api/tables/*/data", dataResp({ columns: ["id"], rows: [[1]], rowCount: 1 }));
    setRoute("GET /api/tables/*/fks", (url) => String(url).includes("/public/users/") ? firstFk.promise : Promise.reject(new Error("fk down")));
    await loadApp();
    const buttons = $("tables").querySelectorAll(".tbl");
    await click(buttons[0]);
    await click(buttons[2]);
    firstFk.resolve(makeResp({ json: [{ column: "id", refSchema: "public", refTable: "companies", refColumn: "id" }] }));
    await flush();
    expect($("data-results").querySelector("button.fk")).toBeNull();
    expect($("status").className).not.toContain("error");
  });
});

describe("structure tab", () => {
  it("defers columns fetch until the structure tab is active", async () => {
    setRoute("GET /api/tables", makeResp({ json: SAMPLE_TABLES }));
    setRoute("GET /api/tables/*/data", rowsResp(1));
    setRoute("GET /api/tables/*/columns", makeResp({ json: [{ name: "id", type: "int", nullable: false, default: null }] }));
    await loadApp();

    await click($("tables").querySelector(".tbl"));
    expect(callsTo("/columns")).toHaveLength(0);
    await click("tab-structure");
    expect(callsTo("/columns")).toHaveLength(1);
    expect($("structure-results").textContent).toContain("id");
  });

  it("shows loading, renders columns, and ignores stale column responses", async () => {
    const stale = deferred();
    setRoute("GET /api/tables", makeResp({ json: SAMPLE_TABLES }));
    setRoute("GET /api/tables/*/data", rowsResp(1));
    setRoute("GET /api/tables/*/columns", (url) => String(url).includes("/public/users/")
      ? stale.promise
      : Promise.resolve(makeResp({ json: [{ name: "sid", type: "uuid", nullable: false, default: null }] })));
    await loadApp();
    const buttons = $("tables").querySelectorAll(".tbl");
    await click(buttons[0]);
    await click("tab-structure");
    expect($("structure-results").textContent).toContain("Loading…");

    await click(buttons[2]);
    await click("tab-structure");
    stale.resolve(makeResp({ json: [{ name: "stale", type: "text", nullable: true, default: "x" }] }));
    await flush();
    expect($("structure-results").textContent).toContain("sid");
    expect($("structure-results").textContent).toContain("NO");
    expect($("structure-results").textContent).not.toContain("stale");

    document.body.innerHTML = '<div id="app"></div>';
    const staleError = deferred();
    setRoute("GET /api/tables", makeResp({ json: SAMPLE_TABLES }));
    setRoute("GET /api/tables/*/data", rowsResp(1));
    setRoute("GET /api/tables/*/columns", (url) => String(url).includes("/public/users/")
      ? staleError.promise
      : Promise.resolve(makeResp({ json: [{ name: "sid", type: "uuid", nullable: false, default: null }] })));
    await loadApp();
    const nextButtons = $("tables").querySelectorAll(".tbl");
    await click(nextButtons[0]);
    await click("tab-structure");
    await click(nextButtons[2]);
    staleError.resolve(Promise.reject(new Error("stale columns failed")));
    await flush();
    expect($("status").textContent).not.toContain("stale columns failed");
  });

  it("renders nullable/default variants, empty structures, and errors", async () => {
    setRoute("GET /api/tables/*/columns", makeResp({ json: [
      { name: "id", type: "int", nullable: false, default: "nextval()" },
      { name: "nickname", type: "text", nullable: true, default: null },
    ] }));
    await selectTable(0, rowsResp(1));
    await click("tab-structure");
    expect($("structure-results").textContent).toContain("id");
    expect($("structure-results").textContent).toContain("NO");
    expect($("structure-results").textContent).toContain("YES");
    expect($("structure-results").textContent).toContain("nextval()");

    setRoute("GET /api/tables/*/columns", makeResp({ json: [] }));
    await click($("tables").querySelector(".tbl"));
    await click("tab-structure");
    expect($("structure-results").textContent).toContain("No columns.");

    for (const [err, text] of [
      [makeResp({ ok: false, status: 500, json: { error: "bad columns" } }), "bad columns"],
      [makeResp({ ok: false, status: 500, statusText: "Column Error", json: {} }), "Column Error"],
      [new Error("columns offline"), "columns offline"],
    ]) {
      document.body.innerHTML = '<div id="app"></div>';
      setRoute("GET /api/tables", makeResp({ json: SAMPLE_TABLES }));
      setRoute("GET /api/tables/*/data", () => new Promise(() => {}));
      setRoute("GET /api/tables/*/columns", err);
      await loadApp();
      await click($("tables").querySelector(".tbl"));
      await click("tab-structure");
      expect($("status").className).toContain("error");
      expect($("status").textContent).toContain(text);
    }
  });
});

describe("SQL tab textarea mode", () => {
  async function openSql() {
    await loadApp();
    await click("tab-sql");
  }

  it("runs queries, renders rows, and shows capped warnings", async () => {
    setRoute("POST /api/query", makeResp({ json: { columns: ["n"], rows: [[1]], rowCount: 1, elapsedMs: 4, truncated: true } }));
    await openSql();
    $("sql").value = " select 1 ";
    await click("run-btn");

    expect(postBody("/api/query")).toEqual({ sql: "select 1" });
    expect($("sql-results").textContent).toContain("1");
    expect($("status").textContent).toContain("✓ 1 row in 4 ms");
    expect($("status").querySelector(".warn").textContent).toContain("capped");
    expect($("sql-export-btn").disabled).toBe(false);
  });

  it("disables run and export during in-flight runs and prevents overlap", async () => {
    const pending = deferred();
    let queryCount = 0;
    setRoute("POST /api/query", () => {
      queryCount += 1;
      return queryCount === 1
        ? Promise.resolve(makeResp({ json: { columns: ["n"], rows: [[1]], rowCount: 1, elapsedMs: 1 } }))
        : pending.promise;
    });
    await openSql();
    $("sql").value = "select 1";
    await click("run-btn");
    expect($("sql-export-btn").disabled).toBe(false);

    $("sql").value = "select 2";
    $("run-btn").click();
    $("run-btn").click();
    await flush();
    expect(callsTo("/api/query")).toHaveLength(2);
    expect($("run-btn").disabled).toBe(true);
    expect($("sql-export-btn").disabled).toBe(true);

    pending.resolve(makeResp({ json: { columns: ["n"], rows: [[2]], rowCount: 1, elapsedMs: 2 } }));
    await flush();
    expect($("run-btn").disabled).toBe(false);
    expect($("sql-export-btn").disabled).toBe(false);
  });

  it("clears running after failed in-flight runs", async () => {
    for (const failure of [makeResp({ ok: false, status: 500, json: { error: "nope" } }), new Error("query offline")]) {
      document.body.innerHTML = '<div id="app"></div>';
      const pending = deferred();
      setRoute("POST /api/query", () => pending.promise.then((value) => {
        if (value instanceof Error) throw value;
        return value;
      }));
      await openSql();
      $("sql").value = "select fail";
      $("run-btn").click();
      await flush();
      expect($("run-btn").disabled).toBe(true);
      expect($("sql-export-btn").disabled).toBe(true);

      pending.resolve(failure);
      await flush();
      expect($("run-btn").disabled).toBe(false);
      expect($("sql-export-btn").disabled).toBe(true);
      expect($("status").className).toContain("error");
    }
  });

  it("guards empty SQL and renders empty result variants", async () => {
    await openSql();
    $("sql").value = "   ";
    fetch.mockClear();
    await click("run-btn");
    expect(fetch).not.toHaveBeenCalled();

    setRoute("POST /api/query", makeResp({ json: { columns: [], rows: [], rowCount: 0, elapsedMs: 1 } }));
    $("sql").value = "select nothing";
    await click("run-btn");
    expect($("sql-results").textContent).toContain("Query ran. No columns returned.");
    expect($("sql-export-btn").disabled).toBe(true);

    setRoute("POST /api/query", makeResp({ json: { columns: ["n"], rows: [], rowCount: 0, elapsedMs: 2 } }));
    $("sql").value = "select n from empty";
    await click("run-btn");
    expect($("sql-results").textContent).toContain("0 rows.");
  });

  it("reports query server and network errors", async () => {
    await openSql();
    for (const err of [makeResp({ ok: false, status: 400, json: { error: "read only" } }), makeResp({ ok: false, status: 500, statusText: "SQL Error", json: {} }), new Error("query offline")]) {
      setRoute("POST /api/query", err);
      $("sql").value = "select bad";
      await click("run-btn");
      expect($("status").className).toContain("error");
    }
  });

  it("runs from Ctrl/Cmd Enter and ignores other keys", async () => {
    setRoute("POST /api/query", makeResp({ json: { columns: ["n"], rows: [[2]], rowCount: 1, elapsedMs: 1 } }));
    await openSql();
    $("sql").value = "select 2";
    await keydown("sql", "Escape", { ctrlKey: true });
    expect(fetch.mock.calls.filter(([u]) => String(u) === "/api/query")).toHaveLength(0);
    await keydown("sql", "Enter", { ctrlKey: true });
    await keydown("sql", "Enter", { metaKey: true });
    expect(fetch.mock.calls.filter(([u]) => String(u) === "/api/query")).toHaveLength(2);
  });
});

describe("SQL CSV export", () => {
  async function openSqlWithText(sql = "select 1") {
    await loadApp();
    await click("tab-sql");
    $("sql").value = sql;
  }

  it("downloads the CSV from the last run SQL or current SQL", async () => {
    setRoute("POST /api/query", makeResp({ json: { columns: ["n"], rows: [[1]], rowCount: 1, elapsedMs: 1 } }));
    setRoute("POST /api/export", makeResp({ blob: new Blob(["n\n1"]) }));
    await openSqlWithText("select 1");
    await click("run-btn");
    $("sql").value = "select changed";
    await click("sql-export-btn");
    expect(postBody("/api/export")).toEqual({ sql: "select 1" });
    expect(URL.createObjectURL).toHaveBeenCalled();
    expect(HTMLAnchorElement.prototype.click).toHaveBeenCalled();
    expect(URL.revokeObjectURL).toHaveBeenCalledWith("blob:fake");

    document.body.innerHTML = '<div id="app"></div>';
    setRoute("POST /api/export", makeResp());
    await openSqlWithText("select fresh");
    await dispatchClick("sql-export-btn");
    expect(postBody("/api/export")).toEqual({ sql: "select fresh" });
  });

  it("guards empty exports and reports export errors", async () => {
    await openSqlWithText("   ");
    fetch.mockClear();
    await dispatchClick("sql-export-btn");
    expect(fetch).not.toHaveBeenCalled();

    for (const err of [makeResp({ ok: false, status: 500, json: { error: "export denied" } }), makeResp({ ok: false, status: 500, json: () => { throw new Error("bad json"); } })]) {
      setRoute("POST /api/export", err);
      $("sql").value = "select export";
      await dispatchClick("sql-export-btn");
      expect($("status").className).toContain("error");
    }
    expect($("status").textContent).toContain("export failed");
  });
});

describe("saved queries", () => {
  const SAVED = [
    { id: 1, name: "Preset one", description: "", sql: "select preset", isPreset: true },
    { id: 2, name: "Mine", description: "", sql: "select mine", isPreset: false },
  ];

  async function openWithSaved(saved = SAVED) {
    setRoute("GET /api/queries", makeResp({ json: saved }));
    await loadApp();
    await click("tab-sql");
  }

  it("groups presets/saved, picks queries, and toggles delete", async () => {
    await openWithSaved();
    expect([...$("presets").querySelectorAll("optgroup")].map((g) => g.label)).toEqual(["Presets", "Saved"]);
    expect($("delete-btn").disabled).toBe(true);

    await changeSelect($("presets"), "1");
    expect($("sql").value).toBe("select preset");
    expect($("status").textContent).toContain("Loaded “Preset one”. Press Run.");
    expect($("delete-btn").disabled).toBe(true);

    await changeSelect($("presets"), "2");
    expect($("sql").value).toBe("select mine");
    expect($("delete-btn").disabled).toBe(false);
    await changeSelect($("presets"), "");
    expect($("delete-btn").disabled).toBe(true);
    const confirmCalls = confirm.mock.calls.length;
    await dispatchClick("delete-btn");
    expect(confirm.mock.calls).toHaveLength(confirmCalls);
  });

  it("reports reload errors", async () => {
    setRoute("GET /api/queries", makeResp({ ok: false, status: 500, json: { error: "store down" } }));
    await loadApp();
    expect($("status").textContent).toContain("failed to load saved queries: store down");
  });

  it("saves new queries, handles prompt cancellation, blank SQL, and save errors", async () => {
    await openWithSaved([]);
    $("sql").value = "   ";
    fetch.mockClear();
    await click("save-btn");
    expect(fetch).not.toHaveBeenCalled();

    $("sql").value = "select 1";
    prompt.mockReturnValueOnce("");
    await click("save-btn");
    expect(fetch.mock.calls.filter(([u]) => String(u) === "/api/queries" && fetch.mock.calls[0]?.[1]?.method === "POST")).toHaveLength(0);

    prompt.mockReturnValueOnce("My query").mockReturnValueOnce(null);
    setRoute("POST /api/queries", makeResp({ ok: false, status: 500, json: { error: "save denied" } }));
    await click("save-btn");
    expect($("status").textContent).toContain("save denied");

    prompt.mockReturnValueOnce("My query").mockReturnValueOnce("desc");
    setRoute("POST /api/queries", makeResp({ json: { id: 7, name: "My query" } }));
    setRoute("GET /api/queries", makeResp({ json: [{ id: 7, name: "My query", sql: "select 1", isPreset: false }] }));
    await click("save-btn");
    expect(postBody("/api/queries")).toEqual({ name: "My query", description: "desc", sql: "select 1" });
    expect($("presets").value).toBe("7");
    expect($("status").textContent).toContain("✓ Saved “My query”.");

    prompt.mockReturnValueOnce("Again").mockReturnValueOnce("");
    setRoute("POST /api/queries", makeResp({ ok: false, status: 500, json: {} }));
    await click("save-btn");
    expect($("status").textContent).toContain("save failed");
  });

  it("deletes after confirmation, aborts otherwise, and handles failures", async () => {
    await openWithSaved();
    await changeSelect($("presets"), "2");
    confirm.mockReturnValueOnce(false);
    await click("delete-btn");
    expect(fetch.mock.calls.some(([u, opts]) => String(u).includes("/api/queries/2") && opts?.method === "DELETE")).toBe(false);

    confirm.mockReturnValueOnce(true);
    setRoute("DELETE /api/queries/:id", makeResp({ status: 204 }));
    setRoute("GET /api/queries", makeResp({ json: [SAVED[0]] }));
    await click("delete-btn");
    expect($("status").textContent).toContain("✓ Deleted.");
    expect($("presets").value).toBe("");

    await changeSelect($("presets"), "1");
    await dispatchClick("delete-btn");
    expect(confirm).toHaveBeenCalled();

    document.body.innerHTML = '<div id="app"></div>';
    await openWithSaved();
    await changeSelect($("presets"), "2");
    confirm.mockReturnValueOnce(true);
    setRoute("DELETE /api/queries/:id", makeResp({ ok: false, status: 204 }));
    setRoute("GET /api/queries", makeResp({ json: [] }));
    await dispatchClick("delete-btn");
    expect($("status").textContent).toContain("✓ Deleted.");

    document.body.innerHTML = '<div id="app"></div>';
    await openWithSaved();
    await changeSelect($("presets"), "2");
    confirm.mockReturnValueOnce(true);
    setRoute("DELETE /api/queries/:id", makeResp({ ok: false, status: 500 }));
    await click("delete-btn");
    expect($("status").textContent).toContain("delete failed");
  });
});

describe("CodeMirror 6 mode", () => {
  function installCM6(sql = "select cm") {
    let value = sql;
    const editor = {
      getValue: vi.fn(() => value),
      setValue: vi.fn((v) => { value = v; }),
      refresh: vi.fn(),
    };
    const mount = vi.fn(() => editor);
    window.cm6 = { mount };
    globalThis.cm6 = window.cm6;
    return { editor, mount };
  }

  it("mounts cm6, runs via the editor keymap callback, and refreshes when activated", async () => {
    const { editor, mount } = installCM6();
    setRoute("POST /api/query", makeResp({ json: { columns: ["n"], rows: [[3]], rowCount: 1, elapsedMs: 1 } }));
    setRoute("GET /api/queries", makeResp({ json: [{ id: 9, name: "CM", sql: "select picked", isPreset: false }] }));
    await loadApp();

    expect(mount).toHaveBeenCalledWith(expect.any(HTMLElement), "SELECT now();", expect.any(Function));
    expect(editor.refresh).not.toHaveBeenCalled();
    await click("tab-sql");
    expect(editor.refresh).toHaveBeenCalled();

    // The keymap "run" callback handed to mount triggers a query from the editor value.
    const onRun = mount.mock.calls[0][2];
    onRun();
    await flush();
    expect(editor.getValue).toHaveBeenCalled();
    expect(postBody("/api/query")).toEqual({ sql: "select cm" });

    await changeSelect($("presets"), "9");
    expect(editor.setValue).toHaveBeenCalledWith("select picked");
  });
});

describe("theme switcher", () => {
  function memStorage() {
    const store = {};
    return {
      getItem: (k) => (k in store ? store[k] : null),
      setItem: (k, v) => { store[k] = String(v); },
      removeItem: (k) => { delete store[k]; },
      clear: () => { for (const k of Object.keys(store)) delete store[k]; },
    };
  }
  beforeEach(() => {
    vi.stubGlobal("localStorage", memStorage());
    document.documentElement.removeAttribute("data-theme");
  });
  afterEach(() => {
    vi.unstubAllGlobals();
    document.documentElement.removeAttribute("data-theme");
  });

  it("defaults to no data-theme and lists the available themes", async () => {
    await loadApp();
    const sel = $("theme-select");
    expect(sel).toBeTruthy();
    expect(sel.value).toBe("");
    expect(document.documentElement.hasAttribute("data-theme")).toBe(false);
    const labels = [...sel.options].map((o) => o.textContent);
    expect(labels).toContain("Default");
    expect(labels).toContain("Dark+");
    expect(labels).toContain("Dracula");
  });

  it("applies and persists a chosen theme", async () => {
    await loadApp();
    await changeSelect($("theme-select"), "dracula");
    expect(document.documentElement.getAttribute("data-theme")).toBe("dracula");
    expect(localStorage.getItem("pgpeek-theme")).toBe("dracula");
  });

  it("restores the saved theme on load and clears back to default", async () => {
    localStorage.setItem("pgpeek-theme", "monokai");
    await loadApp();
    expect($("theme-select").value).toBe("monokai");
    expect(document.documentElement.getAttribute("data-theme")).toBe("monokai");
    await changeSelect($("theme-select"), "");
    expect(document.documentElement.hasAttribute("data-theme")).toBe(false);
    expect(localStorage.getItem("pgpeek-theme")).toBe("");
  });

  it("stays usable when localStorage read and write both throw", async () => {
    vi.stubGlobal("localStorage", {
      getItem: () => { throw new Error("blocked"); },
      setItem: () => { throw new Error("blocked"); },
      removeItem: () => {},
      clear: () => {},
    });
    await loadApp();
    const sel = $("theme-select");
    expect(sel.value).toBe("");
    await changeSelect(sel, "nord");
    expect(document.documentElement.getAttribute("data-theme")).toBe("nord");
  });
});
