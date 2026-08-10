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

beforeEach(() => {
  document.body.innerHTML = '<div id="app"></div>';
  window.history.replaceState({}, "", "/");
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

async function becomeVisible() {
  document.dispatchEvent(new Event("visibilitychange"));
  await flush();
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
});
