// @vitest-environment jsdom
import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { readFileSync } from "node:fs";
import {
  makeResp, ONE_DB, TWO_DBS, makeInstallFetch, $, changeSelect, click, flush, loadApp,
} from "./test-helpers.js";

let routes;
const installFetch = makeInstallFetch(() => routes);

const TABLES = Array.from({ length: 50 }, (_, i) => ({
  schema: i < 25 ? "public" : "analytics",
  name: `table_${String(i + 1).padStart(2, "0")}`,
  type: "table",
  estRows: (i + 1) * 100,
}));

const COLUMNS = Array.from({ length: 50 }, (_, i) => `field_${String(i + 1).padStart(2, "0")}`);
const LONG_MATCH = `${"diagnostic context ".repeat(24)}renewal-blocked-needle ${"after-match evidence ".repeat(12)}`;

function dataResp() {
  return makeResp({
    json: {
      columns: COLUMNS,
      rows: [COLUMNS.map((col, i) => {
        if (i === 1) return { ticketId: "SUP-0042", reason: "renewal-blocked-needle", events: [{ kind: "webhook", ok: false }] };
        if (i === 49) return LONG_MATCH;
        return `${col}_value`;
      })],
      rowCount: 1,
      elapsedMs: 2,
    },
  });
}

function columnResp() {
  return makeResp({
    json: COLUMNS.map((name, i) => ({
      name,
      type: i % 3 === 0 ? "uuid" : "text",
      nullable: i % 2 === 0,
      default: i === 0 ? "gen_random_uuid()" : null,
    })),
  });
}

function defaultRoutes() {
  return {
    "GET /api/databases": makeResp({ json: ONE_DB }),
    "GET /api/meta": makeResp({ json: { rowCap: 100 } }),
    "GET /api/tables": makeResp({ json: TABLES }),
    "GET /api/tables/*/data": dataResp(),
    "GET /api/tables/*/columns": columnResp(),
    "GET /api/tables/*/fks": makeResp({ json: [] }),
    "GET /api/queries": makeResp({ json: [] }),
  };
}

beforeEach(() => {
  document.body.innerHTML = '<div id="app"></div>';
  window.history.replaceState({}, "", "/");
  routes = defaultRoutes();
  installFetch();
  Element.prototype.scrollIntoView = vi.fn();
  globalThis.requestAnimationFrame = (cb) => setTimeout(cb, 0);
  globalThis.cancelAnimationFrame = (id) => clearTimeout(id);
  window.requestAnimationFrame = globalThis.requestAnimationFrame;
  window.cancelAnimationFrame = globalThis.cancelAnimationFrame;
  delete window.cm6;
  delete globalThis.cm6;
});

afterEach(() => {
  vi.restoreAllMocks();
  window.history.replaceState({}, "", "/");
  delete window.cm6;
  delete globalThis.cm6;
});

describe("large schema rendering", () => {
  it("keeps 50 tables and 50 fields inside scrollable UI regions", async () => {
    await loadApp();

    expect($("tables").querySelectorAll(".tbl")).toHaveLength(50);
    expect(document.querySelector(".side-head span:last-child").textContent).toBe("50");
    expect($("tables").textContent).toContain("table_50");

    await click($("tables").querySelector(".tbl"));

    expect($("data-results").classList.contains("results")).toBe(true);
    expect($("data-results").querySelectorAll("th.sortable")).toHaveLength(50);
    expect($("data-results").querySelectorAll("tr.filter-row td")).toHaveLength(50);
    expect($("data-results").querySelectorAll("tbody td")).toHaveLength(50);

    await click("tab-structure");

    expect($("structure-results").classList.contains("results")).toBe(true);
    expect($("structure-results").querySelectorAll("tbody tr")).toHaveLength(50);
    expect($("structure-results").textContent).toContain("field_50");
  });

  it("shows a highlighted match snippet when the filtered value is deep in long text", async () => {
    await loadApp();
    await click($("tables").querySelector(".tbl"));

    const filterCell = $("data-results").querySelectorAll("tr.filter-row td")[49];
    await changeSelect(filterCell.querySelector("select"), "ilike");
    const input = filterCell.querySelector("input");
    input.value = "renewal-blocked-needle";
    input.dispatchEvent(new Event("input", { bubbles: true }));
    input.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", bubbles: true }));
    await flush();

    const cell = $("data-results").querySelectorAll("tbody td")[49];
    expect(cell.querySelector("details")).not.toBeNull();
    expect(cell.querySelector("mark").textContent).toBe("renewal-blocked-needle");
    expect(cell.textContent).toContain("renewal-blocked-needle");
  });

  it("keeps highlighted matches readable at long-cell boundaries", async () => {
    const term = "renewal-blocked-needle";
    routes["GET /api/tables/*/data"] = makeResp({
      json: {
        columns: ["field_01"],
        rows: [[`${term} ${"after-match evidence ".repeat(18)}`], [`${"before-match evidence ".repeat(18)} ${term}`]],
        rowCount: 2,
        elapsedMs: 1,
      },
    });

    await loadApp();
    await click($("tables").querySelector(".tbl"));
    const filterCell = $("data-results").querySelector("tr.filter-row td");
    await changeSelect(filterCell.querySelector("select"), "ilike");
    const input = filterCell.querySelector("input");
    input.value = term;
    input.dispatchEvent(new Event("input", { bubbles: true }));
    input.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", bubbles: true }));
    await flush();

    const cells = $("data-results").querySelectorAll("tbody td");
    expect(cells[0].querySelector(".cell-preview").textContent.startsWith(term)).toBe(true);
    expect(cells[1].querySelector(".cell-preview").textContent.endsWith(term)).toBe(true);
    expect($("data-results").querySelectorAll("mark")).toHaveLength(2);
  });

  it("loads server-truncated table cells instead of rendering the marker JSON", async () => {
    routes["GET /api/tables/*/data"] = makeResp({
      json: {
        columns: ["id", "payload"],
        rows: [[1, "large row preview…"], [2, "normal row"]],
        rowCount: 2,
        cellsTruncated: true,
        truncatedCells: [{ row: 0, column: 1, hash: "cell-hash" }],
        elapsedMs: 1,
      },
    });
    routes["GET /api/tables/*/data/cell"] = makeResp({ json: { value: "full large row" } });
    await loadApp();
    await click($("tables").querySelector(".tbl"));

    const detail = $("data-results").querySelector("details");
    expect(detail.textContent).toContain("large row preview");
    expect(detail.textContent).not.toContain('"truncated"');
    detail.open = true;
    detail.dispatchEvent(new Event("toggle"));
    await flush();

    expect(detail.textContent).toContain("full large row");
  });

  it("shows reload guidance when a truncated table cell changed", async () => {
    routes["GET /api/tables/*/data"] = makeResp({
      json: { columns: ["payload"], rows: [["preview…"]], rowCount: 1, truncatedCells: [{ row: 0, column: 0 }], elapsedMs: 1 },
    });
    routes["GET /api/tables/*/data/cell"] = makeResp({
      ok: false,
      status: 409,
      json: { error: "table result changed; reload the page" },
    });
    await loadApp();
    await click($("tables").querySelector(".tbl"));

    const detail = $("data-results").querySelector("details");
    detail.open = true;
    detail.dispatchEvent(new Event("toggle"));
    await flush();

    expect(detail.querySelector('[role="alert"]').textContent).toBe("table result changed; reload the page");
  });

  it("resets expanded table cells when sorting reloads the result", async () => {
    let load = 0;
    routes["GET /api/tables/*/data"] = () => {
      load += 1;
      return Promise.resolve(makeResp({
        json: {
          columns: ["payload"],
          rows: [[`preview ${load}…`]],
          rowCount: 1,
          truncatedCells: [{ row: 0, column: 0 }],
          elapsedMs: 1,
        },
      }));
    };
    routes["GET /api/tables/*/data/cell"] = (url) => makeResp({ json: { value: String(url).includes("sort=") ? "full 2" : "full 1" } });
    await loadApp();
    await click($("tables").querySelector(".tbl"));

    let detail = $("data-results").querySelector("details");
    detail.open = true;
    detail.dispatchEvent(new Event("toggle"));
    await flush();
    expect(detail.textContent).toContain("full 1");

    await click($("data-results").querySelector("th.sortable"));
    detail = $("data-results").querySelector("details");
    expect(detail.open).toBe(false);
    detail.open = true;
    detail.dispatchEvent(new Event("toggle"));
    await flush();
    expect(detail.textContent).toContain("full 2");
  });

  it("navigates truncated foreign keys after loading the full value", async () => {
    routes["GET /api/tables"] = makeResp({ json: [
      { schema: "public", name: "source", type: "table" },
      { schema: "public", name: "target", type: "table" },
    ] });
    routes["GET /api/tables/*/data"] = makeResp({
      json: {
        columns: ["target_id"],
        rows: [["target-preview…"]],
        rowCount: 1,
        truncatedCells: [{ row: 0, column: 0 }],
        elapsedMs: 1,
      },
    });
    routes["GET /api/tables/*/data/cell"] = makeResp({ json: { value: "target-full-id" } });
    routes["GET /api/tables/*/fks"] = makeResp({
      json: [{ column: "target_id", refSchema: "public", refTable: "target", refColumn: "id" }],
    });
    await loadApp();
    await click($("tables").querySelector(".tbl"));

    const detail = $("data-results").querySelector("details");
    detail.open = true;
    detail.dispatchEvent(new Event("toggle"));
    await flush();
    await click(detail.querySelector(".cell-full-action"));

    expect(document.body.textContent).toContain("public.target");
  });

  it("sorts wide columns from the keyboard", async () => {
    await loadApp();
    await click($("tables").querySelector(".tbl"));

    const header = $("data-results").querySelector("th.sortable");
    header.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", bubbles: true }));
    await flush();
    expect(header.getAttribute("aria-sort")).toBe("ascending");

    header.dispatchEvent(new KeyboardEvent("keydown", { key: " ", bubbles: true }));
    await flush();
    expect(header.getAttribute("aria-sort")).toBe("descending");

    header.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
    await flush();
    expect(header.getAttribute("aria-sort")).toBe("descending");
  });

  it("defines overflow at the sidebar and result boundaries", () => {
    const html = readFileSync("web/index.html", "utf8");

    expect(html).toMatch(/#tables\s*\{[^}]*overflow:\s*auto/s);
    expect(html).toMatch(/\.partition-toggle:hover:not\(:disabled\)/s);
    expect(html).toMatch(/\.tbl\.partition:not\(\.active\)\s*\{[^}]*color:\s*var\(--muted\)/s);
    expect(html).toMatch(/\.results\s*\{[^}]*overflow:\s*auto/s);
    expect(html).toMatch(/#sql-results\s*\{[^}]*display:\s*flex[^}]*overflow:\s*hidden/s);
    expect(html).toMatch(/\.result-toolbar\s*\{[^}]*flex:\s*none/s);
    expect(html).toMatch(/\.result-scroll\s*\{[^}]*overflow:\s*auto/s);
    expect(html).toMatch(/table\s*\{[^}]*min-width:\s*max-content/s);
    expect(html).toMatch(/\.sql-results-view table\s*\{[^}]*border-collapse:\s*separate/s);
    expect(html).toMatch(/\.sql-results-view thead th\s*\{[^}]*top:\s*-1px[^}]*box-shadow:/s);
    expect(html).toMatch(/\.cell-detail\s*>\s*summary/s);
    expect(html).toMatch(/\.cell-preview mark\s*\{[^}]*background:\s*var\(--match-bg\)/s);
    expect(html).toMatch(/@supports\s*\(background:\s*color-mix/s);
    expect(html).toMatch(/\.body\s*\{[^}]*min-height:\s*0/s);
    expect(html).toMatch(/main\s*\{[^}]*min-height:\s*0/s);
    expect(html).toMatch(/main\s*\{[^}]*min-width:\s*0/s);
    expect(html).toMatch(/\.panel\s*\{[^}]*min-height:\s*0/s);
    expect(html).toMatch(/#app\s*\{[^}]*min-height:\s*0/s);
  });

  it("collapses partitions under their parent while keeping search useful", async () => {
    const parent = { schema: "public", name: "events", type: "table", estRows: 100 };
    const partitions = Array.from({ length: 22 }, (_, i) => ({
      schema: "public",
      name: `events_${String(i + 1).padStart(2, "0")}`,
      type: "table",
      estRows: 10,
      isPartition: true,
      parentSchema: "public",
      parentName: "events",
    }));
    const subpartition = {
      schema: "public", name: "events_2026", type: "table", estRows: 10, isPartition: true,
      parentSchema: "public", parentName: "events",
    };
    const nestedPartition = {
      schema: "public", name: "events_2026_01", type: "table", estRows: 10, isPartition: true,
      parentSchema: "public", parentName: "events_2026",
    };
    const inherited = {
      schema: "public", name: "legacy_events", type: "table", estRows: 10,
      parentSchema: "public", parentName: "events",
    };
    routes["GET /api/tables"] = makeResp({ json: [parent, ...partitions, subpartition, nestedPartition, inherited, TABLES[0]] });

    await loadApp();

    expect($("tables").querySelectorAll(".tbl")).toHaveLength(3);
    expect($("tables").textContent).toContain("24 partitions");
    expect($("tables").textContent).toContain("table_01");
    expect($("tables").textContent).toContain("legacy_events");
    expect($("tables").textContent).not.toContain("events_2026_01");

    await click($("tables").querySelector(".partition-toggle"));
    expect($("tables").querySelectorAll(".tbl")).toHaveLength(27);
    expect($("tables").querySelector(".partition-list").querySelectorAll(".tbl")).toHaveLength(24);
    expect($("tables").textContent).toContain("events_2026_01");

    await click([...$("tables").querySelectorAll(".tbl")].find((el) => el.textContent === "events_2026_01"));
    await click($("tables").querySelector(".partition-toggle"));
    expect($("tables").querySelectorAll(".tbl")).toHaveLength(3);

    await click($("tables").querySelector(".partition-toggle"));

    const filter = $("tbl-filter");
    filter.value = "events_2026_01";
    filter.dispatchEvent(new Event("input", { bubbles: true }));
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect($("tables").querySelectorAll(".tbl")).toHaveLength(2);
    expect($("tables").textContent).toContain("events_2026_01");
    expect($("tables").querySelector(".partition-toggle").disabled).toBe(true);
  });

  it("groups partitions with dotted identifiers without key collisions", async () => {
    routes["GET /api/tables"] = makeResp({ json: [
      { schema: "a.b", name: "events", type: "table", estRows: 10 },
      {
        schema: "a.b", name: "events_01", type: "table", estRows: 5, isPartition: true,
        parentSchema: "a.b", parentName: "events",
      },
      { schema: "a", name: "b.events", type: "table", estRows: 10 },
      {
        schema: "a", name: "b.events_01", type: "table", estRows: 5, isPartition: true,
        parentSchema: "a", parentName: "b.events",
      },
      TABLES[0],
    ] });

    await loadApp();

    const groups = $("tables").querySelectorAll(".table-group");
    expect(groups).toHaveLength(2);
    expect($("tables").querySelectorAll(".tbl")).toHaveLength(3);
    await click(groups[0].querySelector(".tbl"));
    expect($("tables").querySelectorAll(".tbl.active")).toHaveLength(1);
    await click(groups[0].querySelector(".partition-toggle"));
    expect(groups[0].querySelector(".partition-list .tbl").textContent).toBe("events_01");
    expect(groups[1].querySelector(".partition-list")).toBeNull();
  });

  it("resets expanded partition groups when switching databases", async () => {
    const tables = [
      { schema: "public", name: "events", type: "table", estRows: 10 },
      {
        schema: "public", name: "events_01", type: "table", estRows: 5, isPartition: true,
        parentSchema: "public", parentName: "events",
      },
    ];
    routes["GET /api/databases"] = makeResp({ json: TWO_DBS });
    routes["GET /api/tables"] = makeResp({ json: tables });

    await loadApp();
    await click($("tables").querySelector(".partition-toggle"));
    expect($("tables").querySelector(".partition-list")).not.toBeNull();

    await changeSelect($("database-select"), "pg2");
    expect($("tables").querySelector(".partition-list")).toBeNull();
  });
});
