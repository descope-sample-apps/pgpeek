// @vitest-environment jsdom
import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";

// ---- DOM fixture --------------------------------------------------------
const FIXTURE = `
  <select id="presets"></select>
  <button id="save-btn"></button>
  <button id="delete-btn"></button>
  <textarea id="sql"></textarea>
  <button id="run-btn"></button>
  <button id="export-btn"></button>
  <div id="status"></div>
  <div id="results"></div>
`;

const flush = () => new Promise((r) => setTimeout(r, 0));

function makeResp({ ok = true, status = 200, json, blob, statusText = "" } = {}) {
  return {
    ok,
    status,
    statusText,
    json: async () => {
      if (typeof json === "function") return json();
      return json;
    },
    blob: async () => blob ?? new Blob(["data"]),
  };
}

// route(method, path) -> response; tests register handlers.
let routes;
function setRoute(key, resp) {
  routes[key] = resp;
}

function installFetch() {
  globalThis.fetch = vi.fn((url, opts) => {
    const method = (opts && opts.method) || "GET";
    const path = String(url).split("?")[0];
    let key = `${method} ${path}`;
    if (!routes[key] && path.startsWith("/api/queries/")) {
      key = `${method} /api/queries/:id`;
    }
    const r = routes[key];
    if (r === undefined) return Promise.reject(new Error("no route for " + key));
    if (r instanceof Error) return Promise.reject(r);
    return Promise.resolve(r);
  });
}

async function loadApp() {
  vi.resetModules();
  await import("./app.js");
  await flush(); // let the initial loadSaved() settle
}

beforeEach(() => {
  document.body.innerHTML = FIXTURE;
  routes = { "GET /api/queries": makeResp({ json: [] }) };
  installFetch();
  globalThis.prompt = vi.fn();
  globalThis.confirm = vi.fn();
  globalThis.URL.createObjectURL = vi.fn(() => "blob:fake");
  globalThis.URL.revokeObjectURL = vi.fn();
  HTMLAnchorElement.prototype.click = vi.fn(); // avoid jsdom navigation noise
  delete globalThis.CodeMirror;
});

afterEach(() => {
  vi.restoreAllMocks();
});

const $ = (id) => document.getElementById(id);
const click = (id) => $(id).dispatchEvent(new Event("click"));

// ---- textarea (no CodeMirror) mode --------------------------------------

describe("query execution (textarea mode)", () => {
  it("runs a query and renders a multi-row table", async () => {
    await loadApp();
    $("sql").value = "SELECT * FROM t";
    setRoute(
      "POST /api/query",
      makeResp({
        json: { columns: ["a", "b"], rows: [[1, "x"], [null, { k: 1 }]], rowCount: 2, truncated: false, elapsedMs: 3 },
      })
    );
    click("run-btn");
    await flush();

    expect($("results").querySelectorAll("th").length).toBe(2);
    const cells = $("results").querySelectorAll("td");
    expect(cells.length).toBe(4);
    expect(cells[2].classList.contains("null")).toBe(true); // null cell
    expect(cells[3].textContent).toBe('{"k":1}'); // object cell stringified
    expect($("status").textContent).toContain("2 rows");
    expect($("export-btn").disabled).toBe(false);
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
    expect($("results").textContent).toContain("No columns");

    setRoute("POST /api/query", makeResp({ json: { columns: ["n"], rows: [], rowCount: 0, truncated: false, elapsedMs: 0 } }));
    click("run-btn");
    await flush();
    expect($("results").textContent).toContain("0 rows");
    expect($("export-btn").disabled).toBe(true); // rowCount 0 keeps export disabled
  });

  it("does nothing on empty SQL", async () => {
    await loadApp();
    $("sql").value = "   ";
    fetch.mockClear();
    click("run-btn");
    await flush();
    expect(fetch).not.toHaveBeenCalled();
  });

  it("shows server error responses", async () => {
    await loadApp();
    $("sql").value = "DELETE FROM t";
    setRoute("POST /api/query", makeResp({ ok: false, status: 400, json: { error: "read-only" } }));
    click("run-btn");
    await flush();
    expect($("status").textContent).toContain("read-only");
    expect($("results").children.length).toBe(0);
  });

  it("shows server error with statusText fallback", async () => {
    await loadApp();
    $("sql").value = "SELECT 1";
    setRoute("POST /api/query", makeResp({ ok: false, status: 500, statusText: "Server Error", json: {} }));
    click("run-btn");
    await flush();
    expect($("status").textContent).toContain("Server Error");
  });

  it("handles fetch rejection (network error)", async () => {
    await loadApp();
    $("sql").value = "SELECT 1";
    setRoute("POST /api/query", new Error("offline"));
    click("run-btn");
    await flush();
    expect($("status").textContent).toContain("offline");
    expect($("run-btn").disabled).toBe(false); // re-enabled in finally
  });

  it("triggers run via Ctrl+Enter in the textarea", async () => {
    await loadApp();
    $("sql").value = "SELECT 1";
    setRoute("POST /api/query", makeResp({ json: { columns: ["n"], rows: [[1]], rowCount: 1, truncated: false, elapsedMs: 1 } }));
    $("sql").dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", ctrlKey: true, bubbles: true }));
    await flush();
    expect($("status").textContent).toContain("1 row");
  });

  it("triggers run via Cmd+Enter (metaKey)", async () => {
    await loadApp();
    $("sql").value = "SELECT 1";
    setRoute("POST /api/query", makeResp({ json: { columns: ["n"], rows: [[1]], rowCount: 1, truncated: false, elapsedMs: 1 } }));
    $("sql").dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", metaKey: true, bubbles: true }));
    await flush();
    expect($("status").textContent).toContain("1 row");
  });

  it("ignores other keystrokes in the textarea", async () => {
    await loadApp();
    $("sql").value = "SELECT 1";
    fetch.mockClear();
    $("sql").dispatchEvent(new KeyboardEvent("keydown", { key: "a", ctrlKey: true, bubbles: true }));
    $("sql").dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", bubbles: true })); // no modifier
    await flush();
    expect(fetch).not.toHaveBeenCalled();
  });
});

describe("CSV export", () => {
  it("downloads the CSV blob", async () => {
    await loadApp();
    $("sql").value = "SELECT 1";
    setRoute("POST /api/export", makeResp({ blob: new Blob(["a,b\n1,2\n"]) }));
    click("export-btn");
    await flush();
    expect(URL.createObjectURL).toHaveBeenCalled();
    expect(URL.revokeObjectURL).toHaveBeenCalled();
  });

  it("does nothing with no SQL", async () => {
    await loadApp();
    $("sql").value = "";
    fetch.mockClear();
    click("export-btn");
    await flush();
    expect(fetch).not.toHaveBeenCalled();
  });

  it("shows an error from the export endpoint", async () => {
    await loadApp();
    $("sql").value = "SELECT 1";
    setRoute("POST /api/export", makeResp({ ok: false, status: 400, json: { error: "bad export" } }));
    click("export-btn");
    await flush();
    expect($("status").textContent).toContain("bad export");
  });

  it("falls back when the error body is not JSON", async () => {
    await loadApp();
    $("sql").value = "SELECT 1";
    setRoute(
      "POST /api/export",
      makeResp({ ok: false, status: 500, json: () => { throw new Error("not json"); } })
    );
    click("export-btn");
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

  it("loading a preset disables delete; loading a saved enables it", async () => {
    routes["GET /api/queries"] = makeResp({ json: sample });
    await loadApp();

    $("presets").value = "1"; // preset
    $("presets").dispatchEvent(new Event("change"));
    expect($("delete-btn").disabled).toBe(true);
    expect($("status").textContent).toContain("Preset A");

    $("presets").value = "2"; // saved
    $("presets").dispatchEvent(new Event("change"));
    expect($("delete-btn").disabled).toBe(false);
  });

  it("selecting the placeholder option clears selection", async () => {
    routes["GET /api/queries"] = makeResp({ json: sample });
    await loadApp();
    $("presets").value = ""; // placeholder, no match
    $("presets").dispatchEvent(new Event("change"));
    expect($("delete-btn").disabled).toBe(true);
  });

  it("saves a new query with name and description", async () => {
    await loadApp();
    $("sql").value = "SELECT 99";
    prompt.mockReturnValueOnce("My Query").mockReturnValueOnce("a description");
    setRoute("POST /api/queries", makeResp({ status: 201, json: { id: 5, name: "My Query", isPreset: false } }));
    setRoute("GET /api/queries", makeResp({ json: [{ id: 5, name: "My Query", sql: "SELECT 99", isPreset: false }] }));
    click("save-btn");
    await flush();
    expect($("status").textContent).toContain("Saved");
    expect($("delete-btn").disabled).toBe(false);
  });

  it("defaults description to empty when prompt is cancelled", async () => {
    await loadApp();
    $("sql").value = "SELECT 1";
    prompt.mockReturnValueOnce("Name").mockReturnValueOnce(null); // desc cancelled
    setRoute("POST /api/queries", makeResp({ status: 201, json: { id: 6, name: "Name", isPreset: false } }));
    click("save-btn");
    await flush();
    const postCall = fetch.mock.calls.find(([u, o]) => String(u) === "/api/queries" && o && o.method === "POST");
    expect(JSON.parse(postCall[1].body).description).toBe("");
  });

  it("does not save when the name prompt is cancelled", async () => {
    await loadApp();
    $("sql").value = "SELECT 1";
    prompt.mockReturnValueOnce(null);
    fetch.mockClear();
    click("save-btn");
    await flush();
    expect(fetch).not.toHaveBeenCalled();
  });

  it("does not save with empty SQL", async () => {
    await loadApp();
    $("sql").value = "";
    fetch.mockClear();
    click("save-btn");
    await flush();
    expect(fetch).not.toHaveBeenCalled();
  });

  it("shows an error when saving fails", async () => {
    await loadApp();
    $("sql").value = "SELECT 1";
    prompt.mockReturnValueOnce("Name").mockReturnValueOnce("");
    setRoute("POST /api/queries", makeResp({ ok: false, status: 400, json: { error: "nope" } }));
    click("save-btn");
    await flush();
    expect($("status").textContent).toContain("nope");
  });

  it("falls back to a generic message when save error has no body", async () => {
    await loadApp();
    $("sql").value = "SELECT 1";
    prompt.mockReturnValueOnce("Name").mockReturnValueOnce("");
    setRoute("POST /api/queries", makeResp({ ok: false, status: 400, json: {} }));
    click("save-btn");
    await flush();
    expect($("status").textContent).toContain("save failed");
  });

  it("deletes a saved query after confirmation", async () => {
    routes["GET /api/queries"] = makeResp({ json: sample });
    await loadApp();
    $("presets").value = "2";
    $("presets").dispatchEvent(new Event("change"));
    confirm.mockReturnValue(true);
    setRoute("DELETE /api/queries/:id", makeResp({ status: 204 }));
    setRoute("GET /api/queries", makeResp({ json: [] }));
    click("delete-btn");
    await flush();
    expect($("status").textContent).toContain("Deleted");
    expect($("delete-btn").disabled).toBe(true);
  });

  it("does nothing when delete has no selection", async () => {
    await loadApp();
    $("presets").value = "";
    fetch.mockClear();
    click("delete-btn");
    await flush();
    expect(fetch).not.toHaveBeenCalled();
  });

  it("aborts delete when not confirmed", async () => {
    routes["GET /api/queries"] = makeResp({ json: sample });
    await loadApp();
    $("presets").value = "2";
    $("presets").dispatchEvent(new Event("change"));
    confirm.mockReturnValue(false);
    fetch.mockClear();
    click("delete-btn");
    await flush();
    expect(fetch).not.toHaveBeenCalled();
  });

  it("aborts delete when the id is unknown", async () => {
    routes["GET /api/queries"] = makeResp({ json: sample });
    await loadApp();
    // Inject an option whose id isn't in savedQueries.
    const opt = new Option("ghost", "999");
    $("presets").appendChild(opt);
    $("presets").value = "999";
    confirm.mockReturnValue(true);
    fetch.mockClear();
    click("delete-btn");
    await flush();
    expect(fetch).not.toHaveBeenCalled();
  });

  it("shows an error when delete fails", async () => {
    routes["GET /api/queries"] = makeResp({ json: sample });
    await loadApp();
    $("presets").value = "2";
    $("presets").dispatchEvent(new Event("change"));
    confirm.mockReturnValue(true);
    setRoute("DELETE /api/queries/:id", makeResp({ ok: false, status: 500 }));
    click("delete-btn");
    await flush();
    expect($("status").textContent).toContain("delete failed");
  });

  it("reports an error when the initial load fails", async () => {
    routes["GET /api/queries"] = new Error("boom");
    await loadApp();
    expect($("status").textContent).toContain("failed to load");
  });
});

// ---- CodeMirror mode ----------------------------------------------------

describe("CodeMirror mode", () => {
  function installCodeMirror() {
    const cm = {
      _v: "",
      getValue() { return this._v; },
      setValue(v) { this._v = v; },
      setOption: vi.fn(),
    };
    const CM = function () {};
    CM.fromTextArea = (ta) => {
      cm._v = ta.value;
      return cm;
    };
    globalThis.CodeMirror = CM;
    return cm;
  }

  it("uses the editor for get/set and runs", async () => {
    const cm = installCodeMirror();
    await loadApp();
    cm.setValue("SELECT 42");
    setRoute("POST /api/query", makeResp({ json: { columns: ["n"], rows: [[42]], rowCount: 1, truncated: false, elapsedMs: 1 } }));
    click("run-btn");
    await flush();
    expect($("status").textContent).toContain("1 row");
  });

  it("setSQL writes into the editor when a preset is selected", async () => {
    routes["GET /api/queries"] = makeResp({ json: [{ id: 1, name: "P", sql: "SELECT 7", isPreset: true }] });
    const cm = installCodeMirror();
    await loadApp();
    $("presets").value = "1";
    $("presets").dispatchEvent(new Event("change"));
    expect(cm.getValue()).toBe("SELECT 7");
  });
});
