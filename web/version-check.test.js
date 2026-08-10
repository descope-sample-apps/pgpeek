// @vitest-environment jsdom
// Tests: the open tab notices a released build and offers a reload.
import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import {
  flush, makeResp, NO_DBS, makeInstallFetch, $, click, loadApp,
} from "./test-helpers.js";

let routes;
const installFetch = makeInstallFetch(() => routes);

function healthz(version) {
  return makeResp({ json: { status: "ok", version, commit: "abc1234", buildDate: "2026-08-01T00:00:00Z" } });
}

// stampBuild mimics the server writing the running build into the document.
function stampBuild(version) {
  document.head.innerHTML = `<meta name="pgpeek-build" content="${version}" />`;
}

beforeEach(() => {
  document.head.innerHTML = "";
  document.body.innerHTML = '<div id="app"></div>';
  window.history.replaceState({}, "", "/");
  setVisibility("visible");
  routes = {
    "GET /api/databases": makeResp({ json: NO_DBS }),
    "GET /api/user":      makeResp({ json: { provider: "anonymous", email: "" } }),
    "GET /healthz":       healthz("1.2.3"),
    "GET /api/meta":      makeResp({ json: { rowCap: 100 } }),
    "GET /api/tables":    makeResp({ json: [] }),
    "GET /api/tables/*/columns": makeResp({ json: [] }),
    "GET /api/tables/*/fks":     makeResp({ json: [] }),
    "GET /api/queries":   makeResp({ json: [] }),
  };
  installFetch();
  globalThis.requestAnimationFrame = (cb) => setTimeout(cb, 0);
  globalThis.cancelAnimationFrame  = (id) => clearTimeout(id);
  window.requestAnimationFrame = globalThis.requestAnimationFrame;
  window.cancelAnimationFrame  = globalThis.cancelAnimationFrame;
  Element.prototype.scrollIntoView = vi.fn();
});

afterEach(() => { vi.restoreAllMocks(); });

function setVisibility(state) {
  Object.defineProperty(document, "visibilityState", { configurable: true, value: state });
}

async function becomeVisible() {
  setVisibility("visible");
  document.dispatchEvent(new Event("visibilitychange"));
  await flush();
}

function deferred() {
  let resolve;
  const promise = new Promise((r) => { resolve = r; });
  return { promise, resolve };
}

describe("release awareness", () => {
  it("stays quiet while the served build matches the loaded one", async () => {
    await loadApp();
    await becomeVisible();
    expect($("update-badge")).toBeNull();
  });

  it("offers a reload once a new version is served", async () => {
    await loadApp();
    routes["GET /healthz"] = healthz("1.3.0");
    await becomeVisible();
    expect($("update-badge")).not.toBeNull();
  });

  it("reloads the page so the browser revalidates the assets", async () => {
    await loadApp();
    routes["GET /healthz"] = healthz("1.3.0");
    await becomeVisible();
    const reload = vi.fn();
    Object.defineProperty(window, "location", {
      configurable: true,
      value: { ...window.location, reload },
    });
    await click("update-badge");
    expect(reload).toHaveBeenCalled();
  });

  it("ignores a healthz blip rather than nagging about a phantom release", async () => {
    await loadApp();
    routes["GET /healthz"] = makeResp({ ok: false, status: 503, json: {}, statusText: "unavailable" });
    await becomeVisible();
    expect($("update-badge")).toBeNull();
  });

  it("survives a healthz payload with no version rather than throwing", async () => {
    routes["GET /healthz"] = makeResp({ json: { status: "ok" } });
    await loadApp();
    await becomeVisible();

    // Nothing was pinned, so the first real version is a baseline, not a release.
    routes["GET /healthz"] = healthz("1.2.3");
    await becomeVisible();
    expect($("update-badge")).toBeNull();
  });

  it("does not poll while the tab is hidden", async () => {
    await loadApp();
    const before = fetch.mock.calls.filter(([u]) => String(u) === "/healthz").length;

    setVisibility("hidden");
    document.dispatchEvent(new Event("visibilitychange"));
    await flush();

    const after = fetch.mock.calls.filter(([u]) => String(u) === "/healthz").length;
    expect(after).toBe(before);
  });

  it("compares against the build stamped into the document", async () => {
    // The tab loaded 1.2.3's assets; a release landed before the first check,
    // so healthz already answers 1.3.0. Without the stamp this would silently
    // become the baseline and the tab would keep running old code.
    stampBuild("1.2.3");
    routes["GET /healthz"] = healthz("1.3.0");
    await loadApp();

    expect($("update-badge")).not.toBeNull();
  });

  it("stays quiet when the stamped build matches what is served", async () => {
    stampBuild("1.2.3");
    await loadApp();
    await becomeVisible();

    expect($("update-badge")).toBeNull();
  });

  it("falls back to the first healthz when the document is unstamped", async () => {
    stampBuild("");
    routes["GET /healthz"] = healthz("1.3.0");
    await loadApp();

    expect($("update-badge")).toBeNull();
  });

  it("drops a healthz response that lands after the tab tore down", async () => {
    const pending = deferred();
    routes["GET /healthz"] = () => pending.promise;
    await loadApp();

    const { render } = await import("./vendor/preact-htm.js");
    render(null, $("app"));
    pending.resolve(healthz("9.9.9"));
    await flush();

    expect($("update-badge")).toBeNull();
  });
});
