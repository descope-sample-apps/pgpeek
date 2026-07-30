// URL state helpers — read, push, and replace browser history for pgpeek.
// Stable param names match API names where they exist.
import { compressToEncodedURIComponent, decompressFromEncodedURIComponent } from "./vendor/lz-string.js";

export function readUrlState() {
  let p = new URLSearchParams(window.location.search);
  if (p.has("s")) {
    const packed = p.get("s");
    let unpacked = null;
    try { unpacked = decompressFromEncodedURIComponent(packed); }
    catch { unpacked = null; }
    if (unpacked !== null && compressToEncodedURIComponent(unpacked) === packed) p = new URLSearchParams(unpacked);
  }
  const filters = [];
  for (const f of p.getAll("f")) {
    const first = f.indexOf(":");
    if (first < 0) continue;
    const col = f.slice(0, first);
    const rest = f.slice(first + 1);
    const second = rest.indexOf(":");
    const op = second >= 0 ? rest.slice(0, second) : rest;
    const value = second >= 0 ? rest.slice(second + 1) : "";
    if (col && op) filters.push({ column: col, op, value });
  }
  const sort = p.get("sort")
    ? { col: p.get("sort"), dir: p.get("dir") === "desc" ? "desc" : "asc" }
    : null;
  return {
    db: p.get("db") || null,
    tab: ["data", "structure", "sql"].includes(p.get("tab")) ? p.get("tab") : "data",
    schema: p.get("schema") || null,
    table: p.get("table") || null,
    offset: Math.max(0, parseInt(p.get("offset"), 10) || 0),
    search: p.get("search") || "",
    sort,
    filters,
    sql: p.has("sql") ? p.get("sql") : null,
  };
}

export function buildUrlParams(state) {
  const stateParams = new URLSearchParams();
  if (state.db) stateParams.set("db", state.db);
  if (state.tab && state.tab !== "data") stateParams.set("tab", state.tab);
  if (state.schema) stateParams.set("schema", state.schema);
  if (state.table) stateParams.set("table", state.table);
  if (state.offset) stateParams.set("offset", String(state.offset));
  if (state.search) stateParams.set("search", state.search);
  if (state.sort) { stateParams.set("sort", state.sort.col); stateParams.set("dir", state.sort.dir); }
  if (state.sql !== null && state.sql !== undefined) stateParams.set("sql", state.sql);
  if (state.filters) {
    for (const f of state.filters) {
      if (!f || !f.column || !f.op) continue;
      const noVal = f.op === "is_null" || f.op === "is_not_null";
      stateParams.append("f", noVal ? `${f.column}:${f.op}` : `${f.column}:${f.op}:${f.value || ""}`);
    }
  }
  const p = new URLSearchParams();
  p.set("s", compressToEncodedURIComponent(stateParams.toString()));
  return p;
}

const qs = (p) => "?" + p.toString();

export const pushUrlState = (state) =>
  window.history.pushState(null, "", window.location.pathname + qs(buildUrlParams(state)));

export const replaceUrlState = (state) =>
  window.history.replaceState(null, "", window.location.pathname + qs(buildUrlParams(state)));
