// @vitest-environment jsdom
import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { readFileSync } from "node:fs";
import {
  makeResp, ONE_DB, makeInstallFetch, $, changeSelect, click, flush, loadApp,
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
    expect(html).toMatch(/\.results\s*\{[^}]*overflow:\s*auto/s);
    expect(html).toMatch(/table\s*\{[^}]*min-width:\s*max-content/s);
    expect(html).toMatch(/\.cell-detail\s*>\s*summary/s);
    expect(html).toMatch(/\.cell-preview mark\s*\{[^}]*background:\s*var\(--match-bg\)/s);
    expect(html).toMatch(/@supports\s*\(background:\s*color-mix/s);
    expect(html).toMatch(/\.body\s*\{[^}]*min-height:\s*0/s);
    expect(html).toMatch(/main\s*\{[^}]*min-height:\s*0/s);
    expect(html).toMatch(/main\s*\{[^}]*min-width:\s*0/s);
    expect(html).toMatch(/\.panel\s*\{[^}]*min-height:\s*0/s);
    expect(html).toMatch(/#app\s*\{[^}]*min-height:\s*0/s);
  });
});
