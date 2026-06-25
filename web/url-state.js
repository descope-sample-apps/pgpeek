// URL state helpers — read, push, and replace browser history for pgpeek.
// Stable param names match API names where they exist.

export function readUrlState() {
  const p = new URLSearchParams(window.location.search);
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
  };
}

export function buildUrlParams(state) {
  const p = new URLSearchParams();
  if (state.db) p.set("db", state.db);
  if (state.tab && state.tab !== "data") p.set("tab", state.tab);
  if (state.schema) p.set("schema", state.schema);
  if (state.table) p.set("table", state.table);
  if (state.offset) p.set("offset", String(state.offset));
  if (state.search) p.set("search", state.search);
  if (state.sort) { p.set("sort", state.sort.col); p.set("dir", state.sort.dir); }
  if (state.filters) {
    for (const f of state.filters) {
      if (!f || !f.column || !f.op) continue;
      const noVal = f.op === "is_null" || f.op === "is_not_null";
      p.append("f", noVal ? `${f.column}:${f.op}` : `${f.column}:${f.op}:${f.value || ""}`);
    }
  }
  return p;
}

function qs(p) { const s = p.toString(); return s ? "?" + s : ""; }

export const pushUrlState = (state) =>
  window.history.pushState(null, "", window.location.pathname + qs(buildUrlParams(state)));

export const replaceUrlState = (state) =>
  window.history.replaceState(null, "", window.location.pathname + qs(buildUrlParams(state)));
