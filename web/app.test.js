// @vitest-environment jsdom
import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";

// Mirrors the IDs in index.html that app.js binds to.
const FIXTURE = `
  <input id="tbl-filter" />
  <div id="tables"></div>
  <button id="tab-data"></button>
  <button id="tab-structure"></button>
  <button id="tab-sql"></button>
  <span id="tab-title"></span>
  <section id="panel-data"></section>
  <section id="panel-structure" hidden></section>
  <section id="panel-sql" hidden></section>
  <input id="data-search" />
  <button id="data-clear"></button>
  <button id="prev-btn"></button>
  <button id="next-btn"></button>
  <span id="page-info"></span>
  <button id="data-export-btn"></button>
  <div id="data-results"></div>
  <div id="structure-results"></div>
  <textarea id="sql"></textarea>
  <button id="run-btn"></button>
  <button id="sql-export-btn"></button>
  <select id="presets"></select>
  <button id="save-btn"></button>
  <button id="delete-btn"></button>
  <div id="sql-results"></div>
  <div id="status"></div>
`;

const flush = () => new Promise((r) => setTimeout(r, 0));

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

// Normalize a request to a stable route key.
function routeKey(method, path) {
  if (path.endsWith("/data")) return `${method} /api/tables/*/data`;
  if (path.endsWith("/columns")) return `${method} /api/tables/*/columns`;
  if (path.startsWith("/api/queries/")) return `${method} /api/queries/:id`;
  return `${method} ${path}`;
}

function installFetch() {
  globalThis.fetch = vi.fn((url, opts) => {
    const method = (opts && opts.method) || "GET";
    const path = String(url).split("?")[0];
    const r = routes[routeKey(method, path)];
    if (typeof r === "function") return r(url, opts); // controllable (race tests)
    if (r === undefined) return Promise.reject(new Error("no route for " + method + " " + path));
    if (r instanceof Error) return Promise.reject(r);
    return Promise.resolve(r);
  });
}

async function loadApp() {
  vi.resetModules();
  await import("./app.js");
  await flush(); // let loadTables() + loadSaved() settle
}

function rowsResp(n, cols = ["n"]) {
  const rows = Array.from({ length: n }, (_, i) => [i + 1]);
  return makeResp({ json: { columns: cols, rows, rowCount: n, truncated: false, elapsedMs: 1 } });
}

beforeEach(() => {
  document.body.innerHTML = FIXTURE;
  routes = {
    "GET /api/meta": makeResp({ json: { rowCap: 1000 } }),
    "GET /api/tables": makeResp({ json: [] }),
    "GET /api/queries": makeResp({ json: [] }),
  };
  installFetch();
  globalThis.prompt = vi.fn();
  globalThis.confirm = vi.fn();
  globalThis.URL.createObjectURL = vi.fn(() => "blob:fake");
  globalThis.URL.revokeObjectURL = vi.fn();
  HTMLAnchorElement.prototype.click = vi.fn();
  delete globalThis.CodeMirror;
});

afterEach(() => vi.restoreAllMocks());

const $ = (id) => document.getElementById(id);
const click = (id) => $(id).dispatchEvent(new Event("click"));

const SAMPLE_TABLES = [
  { schema: "public", name: "users", type: "table", estRows: 5 },
  { schema: "public", name: "v_active", type: "view", estRows: -1 },
  { schema: "auth", name: "sessions", type: "table", estRows: 12 },
];

async function selectFirstTable(dataResp = rowsResp(2)) {
  routes["GET /api/tables"] = makeResp({ json: SAMPLE_TABLES });
  routes["GET /api/tables/*/data"] = dataResp;
  await loadApp();
  $("tables").querySelector(".tbl").dispatchEvent(new Event("click"));
  await flush();
}

function lastDataParams() {
  const call = [...fetch.mock.calls].reverse().find(([u]) => String(u).includes("/data"));
  return new URL("http://x" + call[0]).searchParams;
}

// ===================== sidebar / tabs =====================

describe("sidebar", () => {
  it("renders tables grouped by schema with view styling", async () => {
    routes["GET /api/tables"] = makeResp({ json: SAMPLE_TABLES });
    await loadApp();
    const schemas = $("tables").querySelectorAll(".schema");
    expect([...schemas].map((s) => s.textContent)).toEqual(["public", "auth"]);
    const btns = $("tables").querySelectorAll(".tbl");
    expect(btns.length).toBe(3);
    expect(btns[1].classList.contains("view")).toBe(true);
    expect(btns[0].title).toContain("~5 rows"); // estRows >= 0
    expect(btns[1].title).toBe("public.v_active"); // estRows < 0, no count
  });

  it("filters the table list", async () => {
    routes["GET /api/tables"] = makeResp({ json: SAMPLE_TABLES });
    await loadApp();
    $("tbl-filter").value = "session";
    $("tbl-filter").dispatchEvent(new Event("input"));
    const btns = $("tables").querySelectorAll(".tbl");
    expect(btns.length).toBe(1);
    expect(btns[0].textContent).toBe("sessions");
  });

  it("shows an empty message when nothing matches the filter", async () => {
    routes["GET /api/tables"] = makeResp({ json: SAMPLE_TABLES });
    await loadApp();
    $("tbl-filter").value = "zzz";
    $("tbl-filter").dispatchEvent(new Event("input"));
    expect($("tables").textContent).toContain("No tables match");
  });

  it("reports an error when tables fail to load", async () => {
    routes["GET /api/tables"] = new Error("offline");
    await loadApp();
    expect($("status").textContent).toContain("failed to load tables");
  });

  it("switches tabs", async () => {
    await loadApp();
    click("tab-sql");
    expect($("panel-sql").hidden).toBe(false);
    expect($("panel-data").hidden).toBe(true);
    expect($("tab-sql").classList.contains("active")).toBe(true);
    click("tab-data");
    expect($("panel-data").hidden).toBe(false);
  });
});

// ===================== data browsing =====================

describe("data browsing", () => {
  async function selectUsers(dataResp) {
    routes["GET /api/tables"] = makeResp({ json: SAMPLE_TABLES });
    routes["GET /api/tables/*/data"] = dataResp;
    await loadApp();
    $("tables").querySelector(".tbl").dispatchEvent(new Event("click")); // users
    await flush();
  }

  it("loads a table's first page and updates pager", async () => {
    await selectUsers(rowsResp(2));
    expect($("tab-title").textContent).toBe("public.users");
    expect($("panel-data").hidden).toBe(false);
    expect($("data-results").querySelectorAll("tbody td").length).toBe(2);
    expect($("prev-btn").disabled).toBe(true); // offset 0
    expect($("next-btn").disabled).toBe(true); // rowCount < PAGE_SIZE
    expect($("page-info").textContent).toBe("1–2");
    expect($("data-export-btn").disabled).toBe(false);
    expect($("tables").querySelector(".tbl.active")).not.toBeNull();
  });

  it("enables Next on a full page and paginates", async () => {
    await selectUsers(rowsResp(100));
    expect($("next-btn").disabled).toBe(false);

    // Next -> offset 100; serve a partial page.
    routes["GET /api/tables/*/data"] = rowsResp(3);
    click("next-btn");
    await flush();
    expect($("page-info").textContent).toBe("101–103");
    expect($("prev-btn").disabled).toBe(false);

    // Prev -> back to offset 0.
    routes["GET /api/tables/*/data"] = rowsResp(100);
    click("prev-btn");
    await flush();
    expect($("page-info").textContent).toBe("1–100");
  });

  it("Prev at offset 0 is a no-op", async () => {
    await selectUsers(rowsResp(2));
    fetch.mockClear();
    click("prev-btn"); // offset already 0
    await flush();
    expect(fetch).not.toHaveBeenCalled();
  });

  it("shows zero-row page info", async () => {
    await selectUsers(rowsResp(0));
    expect($("page-info").textContent).toBe("0–0");
    expect($("data-export-btn").disabled).toBe(true);
  });

  it("clears the previous active highlight when switching tables", async () => {
    routes["GET /api/tables"] = makeResp({ json: SAMPLE_TABLES });
    routes["GET /api/tables/*/data"] = rowsResp(1);
    await loadApp();
    const btns = $("tables").querySelectorAll(".tbl");
    btns[0].dispatchEvent(new Event("click")); await flush(); // users
    btns[2].dispatchEvent(new Event("click")); await flush(); // sessions
    const active = $("tables").querySelectorAll(".tbl.active");
    expect(active.length).toBe(1);
    expect(active[0].textContent).toBe("sessions");
  });

  it("shows a server error for table data", async () => {
    await selectUsers(makeResp({ ok: false, status: 400, json: { error: "no such table" } }));
    expect($("status").textContent).toContain("no such table");
  });

  it("uses statusText when the data error has no body", async () => {
    await selectUsers(makeResp({ ok: false, status: 500, statusText: "Server Error", json: {} }));
    expect($("status").textContent).toContain("Server Error");
  });

  it("handles a network error while loading data", async () => {
    await selectUsers(new Error("boom"));
    expect($("status").textContent).toContain("boom");
  });

  it("Next with no table selected is a guarded no-op", async () => {
    await loadApp();
    fetch.mockClear();
    click("next-btn"); // current is null
    await flush();
    expect(fetch).not.toHaveBeenCalled();
  });

  it("exports the current table to CSV", async () => {
    let href = "";
    HTMLAnchorElement.prototype.click = vi.fn(function () { href = this.href; });
    await selectUsers(rowsResp(2));
    click("data-export-btn");
    expect(href).toContain("/api/tables/public/users/data");
    expect(href).toContain("format=csv");
  });

  it("export with no table selected is a no-op", async () => {
    await loadApp();
    const spy = vi.spyOn(document, "createElement");
    click("data-export-btn");
    expect(spy).not.toHaveBeenCalled();
  });
});

// ===================== data filtering / sorting / search =====================

describe("data toolbar", () => {
  it("sorts on header click and toggles asc/desc", async () => {
    await selectFirstTable(rowsResp(2));
    routes["GET /api/tables/*/data"] = rowsResp(2);
    $("data-results").querySelector("th.sortable").dispatchEvent(new Event("click"));
    await flush();
    let p = lastDataParams();
    expect(p.get("sort")).toBe("n");
    expect(p.get("dir")).toBe("asc");
    expect($("data-results").querySelector("th").textContent).toContain("▲");

    $("data-results").querySelector("th.sortable").dispatchEvent(new Event("click"));
    await flush();
    p = lastDataParams();
    expect(p.get("dir")).toBe("desc");
    expect($("data-results").querySelector("th").textContent).toContain("▼");
  });

  it("switches sort column and cycles asc→desc→asc", async () => {
    const two = () => makeResp({ json: { columns: ["a", "b"], rows: [[1, 2]], rowCount: 1, truncated: false, elapsedMs: 1 } });
    await selectFirstTable(two());
    routes["GET /api/tables/*/data"] = two();
    const ths = () => $("data-results").querySelectorAll("th.sortable");
    ths()[0].dispatchEvent(new Event("click")); await flush(); // sort a asc
    expect(lastDataParams().get("sort")).toBe("a");
    ths()[1].dispatchEvent(new Event("click")); await flush(); // switch to b (else branch)
    expect(lastDataParams().get("sort")).toBe("b");
    expect(lastDataParams().get("dir")).toBe("asc");
    ths()[1].dispatchEvent(new Event("click")); await flush(); // b desc
    expect(lastDataParams().get("dir")).toBe("desc");
    ths()[1].dispatchEvent(new Event("click")); await flush(); // b back to asc
    expect(lastDataParams().get("dir")).toBe("asc");
  });

  it("skips columns with no operator and emits empty value for blank value-ops", async () => {
    const two = () => makeResp({ json: { columns: ["a", "b"], rows: [[1, 2]], rowCount: 1, truncated: false, elapsedMs: 1 } });
    await selectFirstTable(two());
    routes["GET /api/tables/*/data"] = two();
    const sels = $("data-results").querySelectorAll(".f-op");
    sels[0].value = "eq"; // column a: value-op, value box left blank
    // column b: no operator -> must be skipped
    sels[0].dispatchEvent(new Event("change"));
    await flush();
    expect(lastDataParams().getAll("f")).toEqual(["a:eq:"]);
  });

  it("applies a per-column filter (op + value) and repopulates it", async () => {
    await selectFirstTable(rowsResp(2));
    routes["GET /api/tables/*/data"] = rowsResp(2);
    const sel = $("data-results").querySelector(".f-op");
    sel.value = "gt";
    sel.parentElement.querySelector(".f-val").value = "100";
    sel.dispatchEvent(new Event("change"));
    await flush();
    expect(lastDataParams().getAll("f")).toContain("n:gt:100");
    // The rebuilt grid repopulates the filter from state.
    expect($("data-results").querySelector(".f-op").value).toBe("gt");
    expect($("data-results").querySelector(".f-val").value).toBe("100");
  });

  it("applies a value-less filter (is_null)", async () => {
    await selectFirstTable(rowsResp(2));
    routes["GET /api/tables/*/data"] = rowsResp(2);
    const sel = $("data-results").querySelector(".f-op");
    sel.value = "is_null";
    sel.dispatchEvent(new Event("change"));
    await flush();
    expect(lastDataParams().getAll("f")).toContain("n:is_null");
  });

  it("applies a filter when Enter is pressed in the value box", async () => {
    await selectFirstTable(rowsResp(2));
    routes["GET /api/tables/*/data"] = rowsResp(2);
    const sel = $("data-results").querySelector(".f-op");
    sel.value = "ilike";
    const inp = sel.parentElement.querySelector(".f-val");
    inp.value = "%x%";
    inp.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", bubbles: true }));
    await flush();
    expect(lastDataParams().getAll("f")).toContain("n:ilike:%x%");
  });

  it("searches all columns on Enter, ignores other keys", async () => {
    await selectFirstTable(rowsResp(2));
    routes["GET /api/tables/*/data"] = rowsResp(2);
    $("data-search").value = "acme";
    $("data-search").dispatchEvent(new KeyboardEvent("keydown", { key: "a", bubbles: true }));
    await flush();
    fetch.mockClear();
    $("data-search").dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", bubbles: true }));
    await flush();
    expect(lastDataParams().get("search")).toBe("acme");
  });

  it("export includes active search/filter params", async () => {
    let href = "";
    HTMLAnchorElement.prototype.click = vi.fn(function () { href = this.href; });
    await selectFirstTable(rowsResp(2));
    routes["GET /api/tables/*/data"] = rowsResp(2);
    const sel = $("data-results").querySelector(".f-op");
    sel.value = "eq";
    sel.parentElement.querySelector(".f-val").value = "5";
    sel.dispatchEvent(new Event("change"));
    await flush();
    click("data-export-btn");
    expect(href).toContain("format=csv");
    expect(decodeURIComponent(href)).toContain("f=n:eq:5");
  });

  it("Clear resets search, filters and sort", async () => {
    await selectFirstTable(rowsResp(2));
    routes["GET /api/tables/*/data"] = rowsResp(2);
    $("data-search").value = "x";
    $("data-search").dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", bubbles: true }));
    await flush();
    expect(lastDataParams().get("search")).toBe("x");

    click("data-clear");
    await flush();
    expect(lastDataParams().get("search")).toBeNull();
    expect($("data-search").value).toBe("");
  });

  it("Clear with no table selected does nothing", async () => {
    await loadApp();
    fetch.mockClear();
    click("data-clear");
    await flush();
    expect(fetch).not.toHaveBeenCalled();
  });

  it("shows 'No columns' for a column-less data result", async () => {
    await selectFirstTable(makeResp({ json: { columns: [], rows: [], rowCount: 0, truncated: false, elapsedMs: 0 } }));
    expect($("data-results").textContent).toContain("No columns");
  });

  it("resets filters when switching tables", async () => {
    await selectFirstTable(rowsResp(2));
    routes["GET /api/tables/*/data"] = rowsResp(2);
    const sel = $("data-results").querySelector(".f-op");
    sel.value = "eq";
    sel.parentElement.querySelector(".f-val").value = "1";
    sel.dispatchEvent(new Event("change"));
    await flush();
    // Switch to another table -> filters cleared.
    $("tables").querySelectorAll(".tbl")[2].dispatchEvent(new Event("click"));
    await flush();
    expect(lastDataParams().getAll("f")).toEqual([]);
  });
});

// ===================== page size (/api/meta) =====================

describe("page size from /api/meta", () => {
  async function selectUsersAndCaptureLimit() {
    routes["GET /api/tables"] = makeResp({ json: SAMPLE_TABLES });
    routes["GET /api/tables/*/data"] = rowsResp(1);
    await loadApp();
    $("tables").querySelector(".tbl").dispatchEvent(new Event("click"));
    await flush();
    const dataCall = fetch.mock.calls.find(([u]) => String(u).includes("/data"));
    return new URL("http://x" + dataCall[0]).searchParams.get("limit");
  }

  it("narrows the page size to a small row cap", async () => {
    routes["GET /api/meta"] = makeResp({ json: { rowCap: 50 } });
    expect(await selectUsersAndCaptureLimit()).toBe("50");
  });

  it("keeps page size at 100 when the cap is larger", async () => {
    routes["GET /api/meta"] = makeResp({ json: { rowCap: 5000 } });
    expect(await selectUsersAndCaptureLimit()).toBe("100");
  });

  it("ignores a non-positive / missing rowCap", async () => {
    routes["GET /api/meta"] = makeResp({ json: { rowCap: 0 } });
    expect(await selectUsersAndCaptureLimit()).toBe("100");
  });

  it("falls back to default page size if meta fails", async () => {
    routes["GET /api/meta"] = new Error("nope");
    expect(await selectUsersAndCaptureLimit()).toBe("100");
  });
});

// ===================== out-of-order request guards =====================

describe("request sequencing", () => {
  it("ignores a stale data response when the table changes", async () => {
    routes["GET /api/tables"] = makeResp({ json: SAMPLE_TABLES });
    let releaseFirst;
    let n = 0;
    routes["GET /api/tables/*/data"] = () => {
      n += 1;
      if (n === 1) {
        return new Promise((res) => {
          releaseFirst = () => res(makeResp({ json: { columns: ["A"], rows: [[1]], rowCount: 1, truncated: false, elapsedMs: 1 } }));
        });
      }
      return Promise.resolve(makeResp({ json: { columns: ["B"], rows: [[2]], rowCount: 1, truncated: false, elapsedMs: 1 } }));
    };
    await loadApp();
    const btns = $("tables").querySelectorAll(".tbl");
    btns[0].dispatchEvent(new Event("click")); // users -> request 1 (pending)
    await flush();
    btns[2].dispatchEvent(new Event("click")); // sessions -> request 2 (resolves now)
    await flush();
    expect([...$("data-results").querySelectorAll("th")].map((t) => t.textContent)).toEqual(["B"]);

    releaseFirst(); // stale users response arrives last
    await flush();
    // Still showing sessions ("B"), not the stale users ("A").
    expect([...$("data-results").querySelectorAll("th")].map((t) => t.textContent)).toEqual(["B"]);
    expect($("tab-title").textContent).toBe("auth.sessions");
  });

  it("ignores a stale structure response when the table changes", async () => {
    routes["GET /api/tables"] = makeResp({ json: SAMPLE_TABLES });
    routes["GET /api/tables/*/data"] = rowsResp(1);
    let releaseFirst;
    let n = 0;
    routes["GET /api/tables/*/columns"] = () => {
      n += 1;
      if (n === 1) {
        return new Promise((res) => {
          releaseFirst = () => res(makeResp({ json: [{ name: "stale", type: "text", nullable: true, default: null }] }));
        });
      }
      return Promise.resolve(makeResp({ json: [{ name: "fresh", type: "text", nullable: true, default: null }] }));
    };
    await loadApp();
    const btns = $("tables").querySelectorAll(".tbl");
    btns[0].dispatchEvent(new Event("click")); await flush(); // users -> data
    click("tab-structure"); await flush(); // structure request 1 (pending)
    btns[2].dispatchEvent(new Event("click")); await flush(); // sessions
    click("tab-structure"); await flush(); // structure request 2 (fresh)
    expect($("structure-results").textContent).toContain("fresh");

    releaseFirst(); // stale columns arrive last
    await flush();
    expect($("structure-results").textContent).toContain("fresh");
    expect($("structure-results").textContent).not.toContain("stale");
  });
});

// ===================== structure =====================

describe("structure tab", () => {
  async function selectAndOpenStructure(colsResp) {
    routes["GET /api/tables"] = makeResp({ json: SAMPLE_TABLES });
    routes["GET /api/tables/*/data"] = rowsResp(1);
    routes["GET /api/tables/*/columns"] = colsResp;
    await loadApp();
    $("tables").querySelector(".tbl").dispatchEvent(new Event("click"));
    await flush();
    click("tab-structure");
    await flush();
  }

  it("renders columns with nullable/default", async () => {
    await selectAndOpenStructure(
      makeResp({
        json: [
          { name: "id", type: "integer", nullable: false, default: "nextval('s')" },
          { name: "email", type: "text", nullable: true, default: null },
        ],
      })
    );
    const rows = $("structure-results").querySelectorAll("tr");
    expect(rows.length).toBe(3); // header + 2
    const firstData = rows[1].querySelectorAll("td");
    expect(firstData[0].textContent).toBe("id");
    expect(firstData[2].textContent).toBe("NO");
    expect(firstData[3].textContent).toContain("nextval");
    const secondData = rows[2].querySelectorAll("td");
    expect(secondData[2].textContent).toBe("YES");
    expect(secondData[3].textContent).toBe(""); // null default
  });

  it("renders an empty-structure message", async () => {
    await selectAndOpenStructure(makeResp({ json: [] }));
    expect($("structure-results").textContent).toContain("No columns");
  });

  it("opening structure with no table selected shows a hint", async () => {
    await loadApp();
    click("tab-structure");
    await flush();
    expect($("structure-results").textContent).toContain("Select a table");
  });

  it("shows a server error for structure", async () => {
    await selectAndOpenStructure(makeResp({ ok: false, status: 500, json: { error: "denied" } }));
    expect($("status").textContent).toContain("denied");
  });

  it("uses statusText when the structure error has no body", async () => {
    await selectAndOpenStructure(makeResp({ ok: false, status: 500, statusText: "Boom", json: {} }));
    expect($("status").textContent).toContain("Boom");
  });

  it("handles a network error for structure", async () => {
    await selectAndOpenStructure(new Error("netfail"));
    expect($("status").textContent).toContain("netfail");
  });
});

// ===================== SQL tab =====================

describe("SQL tab (textarea mode)", () => {
  it("runs a query and renders results", async () => {
    await loadApp();
    $("sql").value = "SELECT * FROM t";
    setRoute("POST /api/query", makeResp({
      json: { columns: ["a", "b"], rows: [[1, "x"], [null, { k: 1 }]], rowCount: 2, truncated: false, elapsedMs: 3 },
    }));
    click("run-btn");
    await flush();
    expect($("sql-results").querySelectorAll("th").length).toBe(2);
    const cells = $("sql-results").querySelectorAll("td");
    expect(cells[2].classList.contains("null")).toBe(true);
    expect(cells[3].textContent).toBe('{"k":1}');
    expect($("status").textContent).toContain("2 rows");
    expect($("sql-export-btn").disabled).toBe(false);
  });

  it("shows the capped warning and singular row text", async () => {
    await loadApp();
    $("sql").value = "SELECT 1";
    setRoute("POST /api/query", makeResp({ json: { columns: ["n"], rows: [[1]], rowCount: 1, truncated: true, elapsedMs: 1 } }));
    click("run-btn");
    await flush();
    expect($("status").innerHTML).toContain("capped");
    expect($("status").textContent).toContain("1 row ");
  });

  it("renders empty-columns and zero-row results", async () => {
    await loadApp();
    $("sql").value = "SELECT 1";
    setRoute("POST /api/query", makeResp({ json: { columns: [], rows: [], rowCount: 0, truncated: false, elapsedMs: 0 } }));
    click("run-btn");
    await flush();
    expect($("sql-results").textContent).toContain("No columns");

    setRoute("POST /api/query", makeResp({ json: { columns: ["n"], rows: [], rowCount: 0, truncated: false, elapsedMs: 0 } }));
    click("run-btn");
    await flush();
    expect($("sql-results").textContent).toContain("0 rows");
    expect($("sql-export-btn").disabled).toBe(true);
  });

  it("does nothing on empty SQL", async () => {
    await loadApp();
    $("sql").value = "   ";
    fetch.mockClear();
    click("run-btn");
    await flush();
    expect(fetch).not.toHaveBeenCalled();
  });

  it("shows server errors (with statusText fallback)", async () => {
    await loadApp();
    $("sql").value = "DELETE FROM t";
    setRoute("POST /api/query", makeResp({ ok: false, status: 400, json: { error: "read-only" } }));
    click("run-btn");
    await flush();
    expect($("status").textContent).toContain("read-only");
    expect($("sql-results").children.length).toBe(0);

    setRoute("POST /api/query", makeResp({ ok: false, status: 500, statusText: "Server Error", json: {} }));
    $("sql").value = "SELECT 1";
    click("run-btn");
    await flush();
    expect($("status").textContent).toContain("Server Error");
  });

  it("handles fetch rejection and re-enables Run", async () => {
    await loadApp();
    $("sql").value = "SELECT 1";
    setRoute("POST /api/query", new Error("offline"));
    click("run-btn");
    await flush();
    expect($("status").textContent).toContain("offline");
    expect($("run-btn").disabled).toBe(false);
  });

  it("runs via Ctrl+Enter, Cmd+Enter, and ignores other keys", async () => {
    await loadApp();
    $("sql").value = "SELECT 1";
    setRoute("POST /api/query", makeResp({ json: { columns: ["n"], rows: [[1]], rowCount: 1, truncated: false, elapsedMs: 1 } }));
    $("sql").dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", ctrlKey: true, bubbles: true }));
    await flush();
    expect($("status").textContent).toContain("1 row");

    $("sql").dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", metaKey: true, bubbles: true }));
    await flush();
    expect($("status").textContent).toContain("1 row");

    fetch.mockClear();
    $("sql").dispatchEvent(new KeyboardEvent("keydown", { key: "a", ctrlKey: true, bubbles: true }));
    $("sql").dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", bubbles: true }));
    await flush();
    expect(fetch).not.toHaveBeenCalled();
  });
});

describe("SQL CSV export", () => {
  it("downloads the CSV blob", async () => {
    await loadApp();
    $("sql").value = "SELECT 1";
    setRoute("POST /api/export", makeResp({ blob: new Blob(["a,b\n1,2\n"]) }));
    click("sql-export-btn");
    await flush();
    expect(URL.createObjectURL).toHaveBeenCalled();
    expect(URL.revokeObjectURL).toHaveBeenCalled();
  });

  it("does nothing with no SQL", async () => {
    await loadApp();
    $("sql").value = "";
    fetch.mockClear();
    click("sql-export-btn");
    await flush();
    expect(fetch).not.toHaveBeenCalled();
  });

  it("shows an export error", async () => {
    await loadApp();
    $("sql").value = "SELECT 1";
    setRoute("POST /api/export", makeResp({ ok: false, status: 400, json: { error: "bad export" } }));
    click("sql-export-btn");
    await flush();
    expect($("status").textContent).toContain("bad export");
  });

  it("falls back when the export error body is not JSON", async () => {
    await loadApp();
    $("sql").value = "SELECT 1";
    setRoute("POST /api/export", makeResp({ ok: false, status: 500, json: () => { throw new Error("not json"); } }));
    click("sql-export-btn");
    await flush();
    expect($("status").textContent).toContain("export failed");
  });
});

describe("saved queries", () => {
  const sample = [
    { id: 1, name: "Preset A", sql: "SELECT 1", isPreset: true },
    { id: 2, name: "Mine", sql: "SELECT 2", isPreset: false },
  ];

  it("loads presets and saved into grouped options", async () => {
    routes["GET /api/queries"] = makeResp({ json: sample });
    await loadApp();
    const groups = $("presets").querySelectorAll("optgroup");
    expect(groups.length).toBe(2);
    expect(groups[0].label).toBe("Presets");
    expect(groups[1].label).toBe("Saved");
  });

  it("loading a preset disables delete; loading a saved enables it; placeholder clears", async () => {
    routes["GET /api/queries"] = makeResp({ json: sample });
    await loadApp();

    $("presets").value = "1";
    $("presets").dispatchEvent(new Event("change"));
    expect($("delete-btn").disabled).toBe(true);
    expect($("status").textContent).toContain("Preset A");

    $("presets").value = "2";
    $("presets").dispatchEvent(new Event("change"));
    expect($("delete-btn").disabled).toBe(false);

    $("presets").value = "";
    $("presets").dispatchEvent(new Event("change"));
    expect($("delete-btn").disabled).toBe(true);
  });

  it("saves a new query (with and without description)", async () => {
    await loadApp();
    $("sql").value = "SELECT 99";
    prompt.mockReturnValueOnce("My Query").mockReturnValueOnce("a description");
    setRoute("POST /api/queries", makeResp({ status: 201, json: { id: 5, name: "My Query", isPreset: false } }));
    setRoute("GET /api/queries", makeResp({ json: [{ id: 5, name: "My Query", sql: "SELECT 99", isPreset: false }] }));
    click("save-btn");
    await flush();
    expect($("status").textContent).toContain("Saved");
    expect($("delete-btn").disabled).toBe(false);

    prompt.mockReturnValueOnce("Name").mockReturnValueOnce(null); // desc cancelled -> ""
    setRoute("POST /api/queries", makeResp({ status: 201, json: { id: 6, name: "Name", isPreset: false } }));
    click("save-btn");
    await flush();
    const posts = fetch.mock.calls.filter(([u, o]) => String(u) === "/api/queries" && o && o.method === "POST");
    expect(JSON.parse(posts.at(-1)[1].body).description).toBe("");
  });

  it("does not save when name cancelled or SQL empty", async () => {
    await loadApp();
    $("sql").value = "SELECT 1";
    prompt.mockReturnValueOnce(null);
    fetch.mockClear();
    click("save-btn");
    await flush();
    expect(fetch).not.toHaveBeenCalled();

    $("sql").value = "";
    click("save-btn");
    await flush();
    expect(fetch).not.toHaveBeenCalled();
  });

  it("shows save errors (with and without body)", async () => {
    await loadApp();
    $("sql").value = "SELECT 1";
    prompt.mockReturnValueOnce("N").mockReturnValueOnce("");
    setRoute("POST /api/queries", makeResp({ ok: false, status: 400, json: { error: "nope" } }));
    click("save-btn");
    await flush();
    expect($("status").textContent).toContain("nope");

    prompt.mockReturnValueOnce("N").mockReturnValueOnce("");
    setRoute("POST /api/queries", makeResp({ ok: false, status: 400, json: {} }));
    click("save-btn");
    await flush();
    expect($("status").textContent).toContain("save failed");
  });

  it("deletes after confirmation; aborts otherwise", async () => {
    routes["GET /api/queries"] = makeResp({ json: sample });
    await loadApp();

    // no selection -> no-op
    $("presets").value = "";
    fetch.mockClear();
    click("delete-btn");
    await flush();
    expect(fetch).not.toHaveBeenCalled();

    // selected but not confirmed
    $("presets").value = "2";
    $("presets").dispatchEvent(new Event("change"));
    confirm.mockReturnValueOnce(false);
    fetch.mockClear();
    click("delete-btn");
    await flush();
    expect(fetch).not.toHaveBeenCalled();

    // confirmed
    confirm.mockReturnValue(true);
    setRoute("DELETE /api/queries/:id", makeResp({ status: 204 }));
    setRoute("GET /api/queries", makeResp({ json: [] }));
    click("delete-btn");
    await flush();
    expect($("status").textContent).toContain("Deleted");
  });

  it("aborts delete for an unknown id and reports delete failure", async () => {
    routes["GET /api/queries"] = makeResp({ json: sample });
    await loadApp();

    // unknown id -> q not found
    $("presets").appendChild(new Option("ghost", "999"));
    $("presets").value = "999";
    confirm.mockReturnValue(true);
    fetch.mockClear();
    click("delete-btn");
    await flush();
    expect(fetch).not.toHaveBeenCalled();

    // failure path
    $("presets").value = "2";
    $("presets").dispatchEvent(new Event("change"));
    setRoute("DELETE /api/queries/:id", makeResp({ ok: false, status: 500 }));
    click("delete-btn");
    await flush();
    expect($("status").textContent).toContain("delete failed");
  });

  it("reports an error when saved queries fail to load", async () => {
    routes["GET /api/queries"] = new Error("boom");
    await loadApp();
    expect($("status").textContent).toContain("failed to load saved queries");
  });
});

// ===================== CodeMirror mode =====================

describe("CodeMirror mode", () => {
  function installCM() {
    const cm = { _v: "", getValue() { return this._v; }, setValue(v) { this._v = v; }, setOption: vi.fn() };
    const CM = function () {};
    CM.fromTextArea = (ta) => { cm._v = ta.value; return cm; };
    globalThis.CodeMirror = CM;
    return cm;
  }

  it("uses the editor for get/set and runs", async () => {
    const cm = installCM();
    await loadApp();
    cm.setValue("SELECT 42");
    setRoute("POST /api/query", makeResp({ json: { columns: ["n"], rows: [[42]], rowCount: 1, truncated: false, elapsedMs: 1 } }));
    click("run-btn");
    await flush();
    expect($("status").textContent).toContain("1 row");
  });

  it("setSQL writes into the editor when a preset is selected", async () => {
    routes["GET /api/queries"] = makeResp({ json: [{ id: 1, name: "P", sql: "SELECT 7", isPreset: true }] });
    const cm = installCM();
    await loadApp();
    $("presets").value = "1";
    $("presets").dispatchEvent(new Event("change"));
    expect(cm.getValue()).toBe("SELECT 7");
  });
});
