// Shared test infrastructure for the split database feature test files.
// Not a test file itself — excluded from vitest coverage tracking by design.
import { vi } from "vitest";

export async function flush() {
  for (let i = 0; i < 10; i += 1) {
    await Promise.resolve();
    await new Promise((r) => setTimeout(r, 0));
  }
}

export function makeResp({ ok = true, status = 200, json, blob, statusText = "" } = {}) {
  return {
    ok, status, statusText,
    json: async () => (typeof json === "function" ? json() : json),
    blob: async () => blob ?? new Blob(["data"]),
  };
}

export const TWO_DBS = {
  defaultId: "pg1",
  databases: [{ id: "pg1", name: "Cluster A" }, { id: "pg2", name: "Cluster B" }],
};
export const ONE_DB  = { defaultId: "only", databases: [{ id: "only", name: "Solo" }] };
export const NO_DBS  = { defaultId: null, databases: [] };

export const SAMPLE_TABLES = [
  { schema: "public", name: "users", type: "table", estRows: 5 },
  { schema: "public", name: "posts", type: "table", estRows: 10 },
];

export function routeKey(method, path) {
  if (path.endsWith("/data")) return `${method} /api/tables/*/data`;
  if (path.endsWith("/columns")) return `${method} /api/tables/*/columns`;
  if (path.endsWith("/fks")) return `${method} /api/tables/*/fks`;
  if (path.startsWith("/api/queries/")) return `${method} /api/queries/:id`;
  return `${method} ${path}`;
}

/** Returns an installFetch() bound to the caller's routes object via a getter. */
export function makeInstallFetch(getRoutes) {
  return () => {
    globalThis.fetch = vi.fn((url, opts) => {
      const method = (opts && opts.method) || "GET";
      const path = String(url).split("?")[0];
      const r = getRoutes()[routeKey(method, path)];
      if (typeof r === "function") return r(url, opts);
      if (r === undefined) return Promise.reject(new Error("no route for " + method + " " + path));
      if (r instanceof Error) return Promise.reject(r);
      return Promise.resolve(r);
    });
    window.fetch = globalThis.fetch;
  };
}

export const $ = (id) => document.getElementById(id);

export async function click(target) {
  const el = typeof target === "string" ? $(target) : target;
  el.click();
  await flush();
}

export async function changeSelect(el, value) {
  el.value = value;
  el.dispatchEvent(new Event("change", { bubbles: true }));
  await flush();
}

export async function loadApp() {
  vi.resetModules();
  await import("./app.js");
  await flush();
}

export function callsTo(path) {
  return fetch.mock.calls.filter(([u]) => String(u).includes(path));
}

export function urlOf(rawUrl) {
  return new URL("http://x" + String(rawUrl));
}

// Re-export pure url/api helpers so split test files can import statically.
export { readUrlState, buildUrlParams } from "./url-state.js";
export { dbUrl } from "./api.js";
