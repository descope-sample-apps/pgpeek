// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { makeInstallFetch, makeResp, ONE_DB, click, flush, loadApp } from "./test-helpers.js";

let routes;
const installFetch = makeInstallFetch(() => routes);
const TABLES = Array.from({ length: 20 }, (_, i) => ({
  schema: "public",
  name: `table_${i}`,
  type: "table",
}));

beforeEach(() => {
  document.body.innerHTML = '<div id="app"></div>';
  window.history.replaceState({}, "", "/");
  routes = {
    "GET /api/databases": makeResp({ json: ONE_DB }),
    "GET /api/user": makeResp({ json: { provider: "anonymous", email: "" } }),
    "GET /healthz": makeResp({ json: { status: "ok", version: "test" } }),
    "GET /api/meta": makeResp({ json: { rowCap: 100 } }),
    "GET /api/tables": makeResp({ json: TABLES }),
    "GET /api/queries": makeResp({ json: [] }),
  };
  installFetch();
  const editor = {
    getValue: vi.fn(() => "SELECT 1"),
    setValue: vi.fn(),
    refresh: vi.fn(),
    setSQLConfig: vi.fn(),
  };
  window.cm6 = { mount: vi.fn(() => editor) };
  globalThis.cm6 = window.cm6;
});

afterEach(() => {
  vi.restoreAllMocks();
  delete window.cm6;
  delete globalThis.cm6;
});

describe("SQL autocomplete", () => {
  it("bounds concurrent column metadata requests for large schemas", async () => {
    let active = 0;
    let completed = 0;
    let peak = 0;
    const pending = [];
    routes["GET /api/tables/*/columns"] = () => new Promise((resolve) => {
      active += 1;
      peak = Math.max(peak, active);
      pending.push(() => {
        active -= 1;
        completed += 1;
        resolve(makeResp({ json: [{ name: "id" }] }));
      });
    });

    await loadApp();
    await vi.waitFor(() => expect(document.querySelectorAll(".tbl")).toHaveLength(TABLES.length));
    await click("tab-sql");
    while (completed < TABLES.length) {
      const batch = pending.splice(0);
      batch.forEach((release) => release());
      await flush();
    }

    expect(peak).toBeLessThanOrEqual(6);
  });

  it("refills available slots without waiting for the slowest request", async () => {
    let completed = 0;
    const releases = new Map();
    const started = [];
    routes["GET /api/tables/*/columns"] = (url) => new Promise((resolve) => {
      const key = String(url);
      started.push(key);
      releases.set(key, () => {
        releases.delete(key);
        completed += 1;
        resolve(makeResp({ json: [{ name: "id" }] }));
      });
    });

    await loadApp();
    await vi.waitFor(() => expect(document.querySelectorAll(".tbl")).toHaveLength(TABLES.length));
    await click("tab-sql");
    await vi.waitFor(() => expect(started).toHaveLength(6));

    const blocked = started[0];
    releases.get(started[1])();
    await vi.waitFor(() => expect(started).toHaveLength(7));
    expect(releases.has(blocked)).toBe(true);

    while (completed < TABLES.length) {
      [...releases.values()].forEach((release) => release());
      await flush();
    }
    expect(completed).toBe(TABLES.length);
  });

  it("aborts retired column loads before reactivation", async () => {
    let active = 0;
    let aborted = 0;
    let completed = 0;
    let peak = 0;
    let started = 0;
    const pending = [];
    routes["GET /api/tables/*/columns"] = (_url, options) => new Promise((resolve, reject) => {
      active += 1;
      started += 1;
      peak = Math.max(peak, active);
      let settled = false;
      const onAbort = () => {
        if (settled) return;
        settled = true;
        active -= 1;
        aborted += 1;
        reject(new DOMException("Aborted", "AbortError"));
      };
      options?.signal?.addEventListener("abort", onAbort, { once: true });
      pending.push(() => {
        if (settled) return;
        settled = true;
        options?.signal?.removeEventListener("abort", onAbort);
        active -= 1;
        completed += 1;
        resolve(makeResp({ json: [{ name: "id" }] }));
      });
    });

    await loadApp();
    await vi.waitFor(() => expect(document.querySelectorAll(".tbl")).toHaveLength(TABLES.length));
    await click("tab-sql");
    await vi.waitFor(() => expect(started).toBe(6));

    await click("tab-data");
    await vi.waitFor(() => expect({ active, aborted, started }).toEqual({ active: 0, aborted: 6, started: 6 }));

    await click("tab-sql");
    await vi.waitFor(() => expect(started).toBe(12));
    while (completed < TABLES.length) {
      const batch = pending.splice(0);
      batch.forEach((release) => release());
      await flush();
    }

    expect({ active, completed, peak, started }).toEqual({ active: 0, completed: 20, peak: 6, started: 26 });
  });
});
