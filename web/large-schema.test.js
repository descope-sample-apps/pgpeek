// @vitest-environment jsdom
import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { readFileSync } from "node:fs";
import {
  makeResp, ONE_DB, makeInstallFetch, $, click, loadApp,
} from "./test-helpers.js";

let routes;
const installFetch = makeInstallFetch(() => routes);

const TABLES = Array.from({ length: 50 }, (_, i) => ({
  schema: i < 25 ? "public" : "analytics",
  name: `table_${String(i + 1).padStart(2, "0")}`,
  type: "table",
  estRows: (i + 1) * 100,
}));

const COLUMNS = Array.from({ length: 20 }, (_, i) => `field_${String(i + 1).padStart(2, "0")}`);

function dataResp() {
  return makeResp({
    json: {
      columns: COLUMNS,
      rows: [COLUMNS.map((col) => `${col}_value`)],
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
  it("keeps 50 tables and 20 fields inside scrollable UI regions", async () => {
    await loadApp();

    expect($("tables").querySelectorAll(".tbl")).toHaveLength(50);
    expect(document.querySelector(".side-head span:last-child").textContent).toBe("50");
    expect($("tables").textContent).toContain("table_50");

    await click($("tables").querySelector(".tbl"));

    expect($("data-results").classList.contains("results")).toBe(true);
    expect($("data-results").querySelectorAll("th.sortable")).toHaveLength(20);
    expect($("data-results").querySelectorAll("tr.filter-row td")).toHaveLength(20);
    expect($("data-results").querySelectorAll("tbody td")).toHaveLength(20);

    await click("tab-structure");

    expect($("structure-results").classList.contains("results")).toBe(true);
    expect($("structure-results").querySelectorAll("tbody tr")).toHaveLength(20);
    expect($("structure-results").textContent).toContain("field_20");
  });

  it("defines overflow at the sidebar and result boundaries", () => {
    const html = readFileSync("web/index.html", "utf8");

    expect(html).toMatch(/#tables\s*\{[^}]*overflow:\s*auto/s);
    expect(html).toMatch(/\.results\s*\{[^}]*overflow:\s*auto/s);
    expect(html).toMatch(/\.body\s*\{[^}]*min-height:\s*0/s);
    expect(html).toMatch(/main\s*\{[^}]*min-height:\s*0/s);
    expect(html).toMatch(/main\s*\{[^}]*min-width:\s*0/s);
    expect(html).toMatch(/\.panel\s*\{[^}]*min-height:\s*0/s);
    expect(html).toMatch(/#app\s*\{[^}]*min-height:\s*0/s);
  });
});
