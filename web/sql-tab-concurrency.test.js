// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { makeInstallFetch, makeResp, ONE_DB, changeSelect, click, flush, loadApp } from "./test-helpers.js";

let routes;
const installFetch = makeInstallFetch(() => routes);
const TABLES = [
  { schema: "public", name: "users", type: "table" },
  { schema: "auth", name: "sessions", type: "table" },
];

function installEditor() {
  const editor = {
    getValue: vi.fn(() => "SELECT 1"),
    getRunnable: vi.fn(() => ({ sql: "SELECT 1", from: 0, to: 8, kind: "statement" })),
    setValue: vi.fn(),
    refresh: vi.fn(),
    setSQLConfig: vi.fn(),
    clearDiagnostics: vi.fn(),
    format: vi.fn(),
  };
  window.cm6 = { mount: vi.fn(() => editor) };
  globalThis.cm6 = window.cm6;
  return editor;
}

const schemaCalls = () => fetch.mock.calls.filter(([url]) => String(url).includes("/api/schema")).length;

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
});

afterEach(() => {
  vi.restoreAllMocks();
  delete window.cm6;
  delete globalThis.cm6;
});

describe("SQL autocomplete catalog", () => {
  it("loads one catalog per database and reuses it after tab reactivation", async () => {
    const editor = installEditor();
    routes["GET /api/schema"] = makeResp({ json: { schemas: {
      public: { users: ["id", "email"] },
      auth: { sessions: ["sid"] },
    } } });

    await loadApp();
    await vi.waitFor(() => expect(document.querySelectorAll(".tbl")).toHaveLength(TABLES.length));
    expect(schemaCalls()).toBe(0);

    await click("tab-sql");
    await vi.waitFor(() => expect(schemaCalls()).toBe(1));
    expect(editor.setSQLConfig.mock.calls.at(-1)[0].schema.public.users).toEqual(["id", "email"]);

    await click("tab-data");
    await click("tab-sql");
    await flush();
    expect(schemaCalls()).toBe(1);
  });

  it("aborts a retired catalog request when the database changes", async () => {
    const editor = installEditor();
    routes["GET /api/databases"] = makeResp({ json: {
      defaultId: "db1",
      databases: [{ id: "db1", name: "Primary" }, { id: "db2", name: "Analytics" }],
    } });
    let firstSignal;
    routes["GET /api/schema"] = (url, options) => {
      if (!String(url).includes("db=db1")) return makeResp({ json: { schemas: {} } });
      firstSignal = options.signal;
      return new Promise((resolve, reject) => {
        firstSignal.addEventListener("abort", () => reject(new DOMException("Aborted", "AbortError")), { once: true });
      });
    };

    await loadApp();
    await click("tab-sql");
    await vi.waitFor(() => expect(firstSignal).toBeTruthy());
    editor.setSQLConfig.mockClear();

    routes["GET /api/tables"] = makeResp({ json: [] });
    await changeSelect(document.getElementById("database-select"), "db2");
    await vi.waitFor(() => expect(firstSignal.aborted).toBe(true));
    expect(editor.setSQLConfig.mock.calls.some(([config]) => config.schema?.public?.users?.length)).toBe(false);
  });

  it("keeps table-name completion when the catalog request fails", async () => {
    const editor = installEditor();
    routes["GET /api/schema"] = new Error("catalog unavailable");

    await loadApp();
    await click("tab-sql");
    await flush();

    const config = editor.setSQLConfig.mock.calls.at(-1)[0];
    expect(config.schema).toEqual({ public: { users: [] }, auth: { sessions: [] } });
    expect(config.defaultSchema).toBe("public");
  });

  it("keeps table-name completion when the catalog is empty", async () => {
    const editor = installEditor();
    routes["GET /api/schema"] = makeResp({ json: { schemas: {} } });

    await loadApp();
    await vi.waitFor(() => expect(document.querySelectorAll(".tbl")).toHaveLength(TABLES.length));
    await click("tab-sql");
    await flush();

    const config = editor.setSQLConfig.mock.calls.at(-1)[0];
    expect(config.schema).toEqual({ public: { users: [] }, auth: { sessions: [] } });
  });
});
